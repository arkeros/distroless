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
	"os"
	"path/filepath"
	"sync"

	"cloud.google.com/go/storage"
	"github.com/arkeros/senku/devtools/build/tools/webroot"
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

	remote, err := list(ctx, bkt)
	if err != nil {
		return err
	}

	changes := webroot.Plan(local, remote)

	first, last := changes.UploadOrder(rules)

	if dryRun {
		// The plan prints as the waves the publish applies rather than as
		// one flat list. The ordering is the part of a plan worth reading —
		// a reviewer has to be able to see index.html follow the chunks it
		// names, which a sorted list of every object hides.
		printUploads(bucket, "content-addressed, written first", first, rules)
		printUploads(bucket, "mutable, written once the first wave has landed", last, rules)
		fmt.Println("# stale, removed once every upload has landed")
		for _, name := range changes.Delete {
			fmt.Printf("delete gs://%s/%s\n", bucket, name)
		}
		return nil
	}

	// Three waves, each finishing before the next begins, so that at no
	// point in the publish does a live client hold a URL for an object that
	// is not there. The chunks a new index.html names have to exist before
	// index.html does; the chunks the old index.html named have to keep
	// existing until nothing can still be reading it.
	if err := uploadAll(ctx, bkt, webrootDir, first, rules); err != nil {
		return err
	}
	if err := uploadAll(ctx, bkt, webrootDir, last, rules); err != nil {
		return err
	}
	if err := deleteAll(ctx, bkt, changes.Delete); err != nil {
		return err
	}

	log.Printf("published %d objects to gs://%s (%d stale removed)",
		len(changes.Upload), bucket, len(changes.Delete))
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

func list(ctx context.Context, bkt *storage.BucketHandle) ([]string, error) {
	var names []string
	it := bkt.Objects(ctx, nil)
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return names, nil
		}
		if err != nil {
			return nil, fmt.Errorf("listing bucket: %w", err)
		}
		names = append(names, attrs.Name)
	}
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

func deleteAll(ctx context.Context, bkt *storage.BucketHandle, names []string) error {
	return eachConcurrently(names, func(name string) error {
		// Tolerate an already-absent object: a retried publish, or two
		// running at once, should converge rather than fail.
		if err := bkt.Object(name).Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
			return fmt.Errorf("deleting %s: %w", name, err)
		}
		return nil
	})
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
