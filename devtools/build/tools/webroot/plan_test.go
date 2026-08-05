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
