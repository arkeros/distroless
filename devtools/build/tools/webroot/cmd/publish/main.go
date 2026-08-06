// Command publish writes a built webroot into a GCS bucket, stamping each
// object with the Cache-Control and Content-Type its path earns.
//
// This is the bucket origin's counterpart to `registry_push`: the artifact
// exists in Bazel's output tree, and this makes it real. Unlike an image
// push it runs *after* the Terraform root it belongs to, because the bucket
// has to exist before anything can be written into it.
//
// `bucket_push` generates a wrapper that resolves the built webroot and the
// rules JSON out of runfiles and passes them as flags, so the target is
// runnable with no arguments. The flags are also the whole interface for
// operating by hand — publishing a locally built webroot into a scratch
// bucket, say — without a second Bazel target to express it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/storage"
	"github.com/arkeros/senku/devtools/build/tools/webroot"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

// uploadConcurrency bounds in-flight writes. A webroot is a few hundred
// small objects, so the wall time is round-trips rather than bytes; going
// wider stops helping well before it starts costing.
const uploadConcurrency = 16

func main() {
	log.SetFlags(0)
	log.SetPrefix("publish: ")

	bucket := flag.String("bucket", "", "GCS bucket to publish into")
	webrootDir := flag.String("webroot", "", "directory to publish")
	rulesPath := flag.String("rules", "", "cache rules JSON")
	dryRun := flag.Bool("dry-run", false, "print the plan without writing anything")
	flag.Parse()

	if err := run(context.Background(), *bucket, *webrootDir, *rulesPath, *dryRun); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, bucket, webrootDir, rulesPath string, dryRun bool) error {
	switch {
	case bucket == "":
		return errors.New("-bucket is required")
	case webrootDir == "":
		return errors.New("-webroot is required")
	case rulesPath == "":
		return errors.New("-rules is required")
	}

	rules, err := readRules(rulesPath)
	if err != nil {
		return err
	}

	local, err := walk(webrootDir)
	if err != nil {
		return err
	}
	if len(local) == 0 {
		// An empty upload would otherwise plan a delete of every object in
		// the bucket — a build that produced nothing must not read as an
		// instruction to take the site down.
		return fmt.Errorf("webroot %s is empty", webrootDir)
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("GCS client: %w", err)
	}
	defer client.Close()
	bkt := client.Bucket(bucket)

	remote, state, err := list(ctx, bkt)
	if err != nil {
		return err
	}

	changes := webroot.Plan(local, remote)
	date, remove := webroot.Retire(orphans(changes.Orphaned, state), rules)

	first, last := changes.UploadOrder(rules)

	if dryRun {
		// The plan prints as the waves the publish applies rather than as
		// one flat list. The ordering is the part of a plan worth reading —
		// a reviewer has to be able to see index.html follow the chunks it
		// names, which a sorted list of every object hides.
		printUploads(bucket, "content-addressed, written first", first, rules)
		printUploads(bucket, "mutable, written once the first wave has landed", last, rules)
		fmt.Println("# content-addressed orphans, dated now and collected by the bucket's lifecycle rule")
		for _, name := range date {
			fmt.Printf("retire gs://%s/%s\n", bucket, name)
		}
		fmt.Println("# stable-named orphans, deleted now — nothing holds a URL their removal did not mean to break")
		for _, name := range remove {
			fmt.Printf("delete gs://%s/%s\n", bucket, name)
		}
		return nil
	}

	// Uploads go in two waves, the second beginning only once the first has
	// landed, so that a client reading a new index.html never holds a URL
	// for an object that is not there yet. Orphans are handled last, and
	// how depends on the name — see webroot.Retire.
	if err := uploadAll(ctx, bkt, webrootDir, first, rules); err != nil {
		return err
	}
	if err := uploadAll(ctx, bkt, webrootDir, last, rules); err != nil {
		return err
	}
	retireSkipped, err := retireAll(ctx, bkt, date, state, time.Now())
	if err != nil {
		return err
	}
	deleteSkipped, err := deleteAll(ctx, bkt, remove, state)
	if err != nil {
		return err
	}

	log.Printf("published %d objects to gs://%s (%d retired, %d deleted, %d awaiting collection)",
		len(changes.Upload), bucket, len(date)-retireSkipped, len(remove)-deleteSkipped,
		len(changes.Orphaned)-len(date)-len(remove))
	// Silence here would read as a clean publish that quietly did less than
	// it planned, which is the same trap the plan output exists to avoid.
	if skipped := retireSkipped + deleteSkipped; skipped > 0 {
		log.Printf("left %d object(s) alone: rewritten by another publish after this one listed the bucket", skipped)
	}
	return nil
}

// printUploads renders one wave of a dry run. why labels it, because a plan
// whose interesting property is its ordering has to say what the ordering
// buys — a bare list of object names does not.
func printUploads(bucket, why string, names []string, rules webroot.Rules) {
	fmt.Printf("# %s\n", why)
	for _, name := range names {
		fmt.Printf("upload gs://%s/%s  [%s] [%s]\n",
			bucket, name, webroot.ContentType(name), rules.CacheControl(name))
	}
}

func readRules(path string) (webroot.Rules, error) {
	f, err := os.Open(path)
	if err != nil {
		return webroot.Rules{}, fmt.Errorf("opening cache rules: %w", err)
	}
	defer f.Close()
	return webroot.ParseRules(f)
}

// walk collects the webroot's files as bucket-relative object names. Bazel
// writes the tree with forward slashes on every platform it supports, and
// object names use them too, so no separator translation is needed.
func walk(dir string) ([]string, error) {
	var names []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking webroot %s: %w", dir, err)
	}
	return names, nil
}

// objectState is what this publish saw when it listed the bucket: the date a
// previous publish stamped on the object, if any, and the generation it had
// at that moment.
//
// The generation is what makes the plan safe to act on later. Everything
// between the listing and the writes is decided from a snapshot, and another
// publish may have moved on since — see the preconditions below.
type objectState struct {
	orphanedAt time.Time
	generation int64
}

func list(ctx context.Context, bkt *storage.BucketHandle) ([]string, map[string]objectState, error) {
	var names []string
	state := map[string]objectState{}
	it := bkt.Objects(ctx, nil)
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return names, state, nil
		}
		if err != nil {
			return nil, nil, fmt.Errorf("listing bucket: %w", err)
		}
		names = append(names, attrs.Name)
		state[attrs.Name] = objectState{
			orphanedAt: attrs.CustomTime,
			generation: attrs.Generation,
		}
	}
}

// orphans pairs each name the build no longer produces with the date a
// previous publish stamped on it, zero if none did.
func orphans(names []string, state map[string]objectState) []webroot.Orphan {
	out := make([]webroot.Orphan, 0, len(names))
	for _, name := range names {
		out = append(out, webroot.Orphan{Name: name, OrphanedAt: state[name].orphanedAt})
	}
	return out
}

// staleGeneration reports whether err is GCS refusing a write because the
// object moved since this publish listed it.
func staleGeneration(err error) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == http.StatusPreconditionFailed
}

func uploadAll(ctx context.Context, bkt *storage.BucketHandle, dir string, names []string, rules webroot.Rules) error {
	return eachConcurrently(names, func(name string) error {
		if err := upload(ctx, bkt, dir, name, rules); err != nil {
			return fmt.Errorf("uploading %s: %w", name, err)
		}
		return nil
	})
}

func upload(ctx context.Context, bkt *storage.BucketHandle, dir, name string, rules webroot.Rules) error {
	src, err := os.Open(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		return err
	}
	defer src.Close()

	w := bkt.Object(name).NewWriter(ctx)
	// A bucket has no request-time logic, so these two are the whole of
	// what the origin will ever say about this object.
	w.ContentType = webroot.ContentType(name)
	w.CacheControl = rules.CacheControl(name)
	if _, err := io.Copy(w, src); err != nil {
		w.CloseWithError(err)
		return err
	}
	return w.Close()
}

// retireAll stamps each newly-orphaned content-addressed object with the
// time it went stale. Deleting one here is what breaks a client still on the
// previous version of the site, so the bucket's lifecycle rule does the
// deleting, a retention window later — see `staticsite` and webroot.Retire.
func retireAll(ctx context.Context, bkt *storage.BucketHandle, names []string, state map[string]objectState, now time.Time) (int, error) {
	var skipped atomic.Int64
	err := eachConcurrently(names, func(name string) error {
		obj := bkt.Object(name).If(storage.Conditions{GenerationMatch: state[name].generation})
		_, err := obj.Update(ctx, storage.ObjectAttrsToUpdate{CustomTime: now})
		switch {
		case err == nil:
			return nil
		// The object was rewritten after this publish listed it, so it is
		// live again and this plan is talking about a generation that no
		// longer exists. Dating it would hand a file somebody just uploaded
		// to the lifecycle rule.
		case staleGeneration(err):
			skipped.Add(1)
			return nil
		// Tolerate an already-absent object: a retried publish, or two
		// running at once, should converge rather than fail.
		case errors.Is(err, storage.ErrObjectNotExist):
			return nil
		}
		return fmt.Errorf("retiring %s: %w", name, err)
	})
	return int(skipped.Load()), err
}

// deleteAll removes stable-named orphans. Retention would keep answering at
// a URL somebody deliberately took down, so these go now.
//
// The generation precondition is what stops a stale plan from undoing a
// newer one. Two publishes can overlap — nothing serialises them, by the
// decision in ADR 0010 — and without it an older run could delete a route
// object a newer run had just uploaded, leaving a declared route 404ing
// until somebody published again. A mismatch means the newer writer won, so
// there is nothing to remove and the next publish replans from what it finds.
func deleteAll(ctx context.Context, bkt *storage.BucketHandle, names []string, state map[string]objectState) (int, error) {
	var skipped atomic.Int64
	err := eachConcurrently(names, func(name string) error {
		obj := bkt.Object(name).If(storage.Conditions{GenerationMatch: state[name].generation})
		err := obj.Delete(ctx)
		switch {
		case err == nil:
			return nil
		case staleGeneration(err):
			skipped.Add(1)
			return nil
		case errors.Is(err, storage.ErrObjectNotExist):
			return nil
		}
		return fmt.Errorf("deleting %s: %w", name, err)
	})
	return int(skipped.Load()), err
}

// eachConcurrently runs fn over every name with bounded parallelism and
// reports every failure, not just the first. A publish that half-worked is
// worth seeing in full — knowing only that "something failed" leaves the
// bucket in a state nobody can describe.
func eachConcurrently(names []string, fn func(string) error) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	sem := make(chan struct{}, uploadConcurrency)

	for _, name := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := fn(name); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}
