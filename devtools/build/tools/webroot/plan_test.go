package webroot_test

import (
	"slices"
	"testing"

	"github.com/arkeros/senku/devtools/build/tools/webroot"
)

func TestPlanUploadsEverythingIntoAnEmptyBucket(t *testing.T) {
	got := webroot.Plan(
		[]string{"index.html", "dino_bundle/dino_main.js"},
		nil,
	)

	want := []string{"dino_bundle/dino_main.js", "index.html"}
	if !slices.Equal(got.Upload, want) {
		t.Errorf("Upload = %v, want %v", got.Upload, want)
	}
	if len(got.Delete) != 0 {
		t.Errorf("Delete = %v, want empty", got.Delete)
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
func TestPlanDeletesObjectsTheBuildNoLongerProduces(t *testing.T) {
	got := webroot.Plan(
		[]string{"index.html", "dino_bundle/chunk-NEW.js"},
		[]string{"index.html", "dino_bundle/chunk-OLD.js"},
	)

	want := []string{"dino_bundle/chunk-OLD.js"}
	if !slices.Equal(got.Delete, want) {
		t.Errorf("Delete = %v, want %v", got.Delete, want)
	}
}
