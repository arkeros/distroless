package webroot_test

import (
	"slices"
	"testing"
	"time"

	"github.com/arkeros/senku/devtools/build/tools/webroot"
)

// A publish must not delete a content-addressed object it orphans. A client
// that loaded the previous index.html holds URLs for chunks this build no
// longer produces, and react-router fetches a lazy route's chunk when the
// user navigates — which can be long after the page loaded. Dating the
// object hands the deletion to the bucket's lifecycle rule, a window later.
func TestRetireDatesAContentAddressedOrphan(t *testing.T) {
	date, remove := webroot.Retire([]webroot.Orphan{
		{Name: "dino_bundle/Play-OLD.js"},
		{Name: "dino_bundle/chunk-OLD.js"},
	}, appRules())

	want := []string{"dino_bundle/Play-OLD.js", "dino_bundle/chunk-OLD.js"}
	if !slices.Equal(date, want) {
		t.Errorf("date = %v, want %v", date, want)
	}
	if len(remove) != 0 {
		t.Errorf("remove = %v, want empty", remove)
	}
}

// Every publish sees the same orphan until the bucket collects it. Re-dating
// one would push its deletion out by a full window each time, so an object
// orphaned once would outlive every retention policy it was given.
func TestRetireLeavesAnAlreadyDatedOrphanAlone(t *testing.T) {
	date, remove := webroot.Retire([]webroot.Orphan{{
		Name:       "dino_bundle/Play-OLD.js",
		OrphanedAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
	}}, appRules())

	if len(date) != 0 || len(remove) != 0 {
		t.Errorf("date = %v, remove = %v, want both empty — the date stands", date, remove)
	}
}

// Retention exists for one reason: an old entry document hands out hashed
// URLs. A stable-named orphan is something else — a route the app dropped,
// say — and nothing holds a URL to it that the removal did not mean to
// break. Retaining it would serve a page the router no longer has, under a
// 200, for a month.
func TestRetireDeletesAStableNamedOrphanAtOnce(t *testing.T) {
	date, remove := webroot.Retire([]webroot.Orphan{
		{Name: "how-to-play/index.html"},
		{Name: "old-icon.png"},
	}, appRules())

	want := []string{"how-to-play/index.html", "old-icon.png"}
	if !slices.Equal(remove, want) {
		t.Errorf("remove = %v, want %v", remove, want)
	}
	if len(date) != 0 {
		t.Errorf("date = %v, want empty", date)
	}
}

func TestPlanUploadsEverythingIntoAnEmptyBucket(t *testing.T) {
	got := webroot.Plan(
		[]string{"index.html", "dino_bundle/dino_main.js"},
		nil,
	)

	want := []string{"dino_bundle/dino_main.js", "index.html"}
	if !slices.Equal(got.Upload, want) {
		t.Errorf("Upload = %v, want %v", got.Upload, want)
	}
	if len(got.Orphaned) != 0 {
		t.Errorf("Orphaned = %v, want empty", got.Orphaned)
	}
}

// Every object is re-uploaded rather than compared. The immutable ones are
// content-addressed, so a same-named object is byte-identical and the write
// changes nothing; the mutable ones are exactly the ones that must be
// overwritten. Skipping on a hash comparison would buy a little bandwidth
// and cost a correctness argument.
func TestPlanReuploadsExistingObjects(t *testing.T) {
	got := webroot.Plan([]string{"index.html"}, []string{"index.html"})

	want := []string{"index.html"}
	if !slices.Equal(got.Upload, want) {
		t.Errorf("Upload = %v, want %v", got.Upload, want)
	}
}

// index.html and the unhashed entry bundle are how a client discovers every
// hashed URL, so they cannot be written before the objects they name. A
// client that fetches a freshly written index.html mid-publish would
// otherwise resolve a chunk it references to nothing.
func TestUploadOrderWritesContentAddressedObjectsFirst(t *testing.T) {
	changes := webroot.Plan([]string{
		"index.html",
		"dino_bundle/dino_main-9F8E7D6C.js",
		"dino_bundle/chunk-A1B2C3D4.js",
		"assets/dino_styles.11cc90d013b2.css",
		"assets/sprite-0F0F0F0F.png",
		"manifest.webmanifest",
	}, nil)

	first, last := changes.UploadOrder(appRules())

	// Everything on the render path — the entry, its chunks, the
	// stylesheets — is content-addressed, so the whole of it lands before
	// anything hands those names out.
	wantFirst := []string{
		"assets/dino_styles.11cc90d013b2.css",
		"assets/sprite-0F0F0F0F.png",
		"dino_bundle/chunk-A1B2C3D4.js",
		"dino_bundle/dino_main-9F8E7D6C.js",
	}
	if !slices.Equal(first, wantFirst) {
		t.Errorf("first wave = %v, want %v", first, wantFirst)
	}
	// index.html is the entry document. manifest.webmanifest matches no rule
	// at all and the default revalidates, so it lands here too — an
	// unclassified file is assumed to name others rather than assumed not to.
	wantLast := []string{"index.html", "manifest.webmanifest"}
	if !slices.Equal(last, wantLast) {
		t.Errorf("last wave = %v, want %v", last, wantLast)
	}
}

// Stale content-addressed chunks accumulate forever otherwise: every deploy
// adds a new hash and nothing ever removes the old one.
func TestPlanOrphansObjectsTheBuildNoLongerProduces(t *testing.T) {
	got := webroot.Plan(
		[]string{"index.html", "dino_bundle/chunk-NEW.js"},
		[]string{"index.html", "dino_bundle/chunk-OLD.js"},
	)

	want := []string{"dino_bundle/chunk-OLD.js"}
	if !slices.Equal(got.Orphaned, want) {
		t.Errorf("Orphaned = %v, want %v", got.Orphaned, want)
	}
}
