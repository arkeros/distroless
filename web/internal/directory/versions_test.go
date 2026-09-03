package directory_test

import (
	"strings"
	"testing"
	"time"

	"github.com/arkeros/distroless/web/internal/directory"
)

func digestOf(n string) string { return "sha256:" + strings.Repeat(n, 64) }

// names is the ordering under test; where each tag leads is the handler's job.
func names(tags []directory.Tag) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		out = append(out, tag.Name)
	}
	return out
}

// Tags are what a reader types; a Release is what they get. Several tags
// naming one build is the normal case — `mirror_push` applies a whole
// tag_list to a single digest — and saying so is the reason this page exists.
func TestVersionsGroupsTagsThatNameTheSameBuild(t *testing.T) {
	versions := directory.NewVersions("distroless.io/nginx", []directory.Version{
		{Tag: "1.27.3", Digest: digestOf("a")},
		{Tag: "latest", Digest: digestOf("a")},
		{Tag: "1.27", Digest: digestOf("a")},
		{Tag: "1.26.2", Digest: digestOf("b")},
	})

	if len(versions.Releases) != 2 {
		t.Fatalf("grouped into %d releases, want 2: %+v", len(versions.Releases), versions.Releases)
	}
	first := versions.Releases[0]
	if first.Digest != digestOf("a") || len(first.Tags) != 3 {
		t.Errorf("first release = %+v, want the three tags of %s", first, digestOf("a"))
	}
}

// A plain string sort puts 1.27.10 below 1.27.3. CompareVersion, already here
// for the SBOM table, is what gets this right.
func TestVersionsOrdersReleasesNewestFirst(t *testing.T) {
	versions := directory.NewVersions("distroless.io/nginx", []directory.Version{
		{Tag: "1.27.3", Digest: digestOf("a")},
		{Tag: "1.27.10", Digest: digestOf("b")},
		{Tag: "1.26.2", Digest: digestOf("c")},
	})

	var order []string
	for _, release := range versions.Releases {
		order = append(order, release.Tags[0].Name)
	}
	if want := []string{"1.27.10", "1.27.3", "1.26.2"}; !equal(order, want) {
		t.Errorf("release order = %v, want %v", order, want)
	}
}

// nginx as actually published: channel tags (`stable`, `mainline`) alongside
// `latest` and `debug`, each channel in a plain and a debug build. `latest` is
// what a reader came for, so its build leads however many other non-version
// tags there are.
func nginxAsPublished() *directory.Versions {
	return directory.NewVersions("distroless.io/nginx", []directory.Version{
		{Tag: "stable-debug", Digest: digestOf("a")},
		{Tag: "1.30-debug", Digest: digestOf("a")},
		{Tag: "stable", Digest: digestOf("b")},
		{Tag: "1.30", Digest: digestOf("b")},
		{Tag: "mainline-debug", Digest: digestOf("c")},
		{Tag: "debug", Digest: digestOf("c")},
		{Tag: "1.31-debug", Digest: digestOf("c")},
		{Tag: "mainline", Digest: digestOf("d")},
		{Tag: "latest", Digest: digestOf("d")},
		{Tag: "1.31", Digest: digestOf("d")},
	})
}

func TestVersionsLeadsWithTheBuildLatestNames(t *testing.T) {
	if got := nginxAsPublished().Releases[0].Tags[0].Name; got != "latest" {
		t.Errorf("first release leads with %q, want %q", got, "latest")
	}
}

// Newest upstream first, and within one upstream version the plain build
// before its debug variant — otherwise `1.31-debug` outranks `1.31`, because
// a revision sorts above no revision at all.
func TestVersionsOrdersReleasesByVersionThenVariant(t *testing.T) {
	var leading []string
	for _, release := range nginxAsPublished().Releases {
		leading = append(leading, release.Tags[len(release.Tags)-1].Name)
	}

	if want := []string{"1.31", "1.31-debug", "1.30", "1.30-debug"}; !equal(leading, want) {
		t.Errorf("release order by version = %v, want %v", leading, want)
	}
}

// Within one build: `latest` first, then the other names it answers to, then
// the versions it pins.
func TestVersionsOrdersTagsWithinARelease(t *testing.T) {
	releases := nginxAsPublished().Releases

	if want := []string{"latest", "mainline", "1.31"}; !equal(names(releases[0].Tags), want) {
		t.Errorf("tag order = %v, want %v", names(releases[0].Tags), want)
	}
	if want := []string{"debug", "mainline-debug", "1.31-debug"}; !equal(names(releases[1].Tags), want) {
		t.Errorf("tag order = %v, want %v", names(releases[1].Tags), want)
	}
}

// java publishes no floating tag at all, so version order is the whole story.
func TestVersionsOrdersAFamilyWithNoFloatingTag(t *testing.T) {
	versions := directory.NewVersions("distroless.io/java", []directory.Version{
		{Tag: "17", Digest: digestOf("a")},
		{Tag: "25-debug", Digest: digestOf("b")},
		{Tag: "21", Digest: digestOf("c")},
		{Tag: "25", Digest: digestOf("d")},
	})

	var order []string
	for _, release := range versions.Releases {
		order = append(order, release.Tags[0].Name)
	}
	if want := []string{"25", "25-debug", "21", "17"}; !equal(order, want) {
		t.Errorf("release order = %v, want %v", order, want)
	}
}

// The image config's `created` is the upstream-snapshot anchor, so it is a
// property of the build and every tag naming it reports the same one.
func TestVersionsKeepsTheBuildHorizonOfEachRelease(t *testing.T) {
	snapshot := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	versions := directory.NewVersions("distroless.io/nginx", []directory.Version{
		{Tag: "1.27.3", Digest: digestOf("a"), Created: snapshot},
		{Tag: "latest", Digest: digestOf("a"), Created: snapshot},
	})

	if got := versions.Releases[0].Created; !got.Equal(snapshot) {
		t.Errorf("created = %s, want %s", got, snapshot)
	}
}

func TestVersionsNamesTheImage(t *testing.T) {
	versions := directory.NewVersions("distroless.io/nginx", nil)

	if versions.Image != "distroless.io/nginx" {
		t.Errorf("image = %q, want %q", versions.Image, "distroless.io/nginx")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// What a pull transfers, rendered the way a registry reports it.
func TestSizeReadsAsADownload(t *testing.T) {
	for size, want := range map[directory.Size]string{
		0:             "unknown",
		512:           "512 B",
		2_400:         "2 kB",
		23867899:      "23.9 MB",
		1_500_000_000: "1500.0 MB",
	} {
		if got := size.String(); got != want {
			t.Errorf("Size(%d) = %q, want %q", int64(size), got, want)
		}
	}
}

// The column has to sort as a number. "9 MB" above "23.9 MB" is what a text
// sort gives, and it is wrong in the direction that matters most here.
func TestSizeExposesItsBytesForSorting(t *testing.T) {
	if got := directory.Size(23867899).Bytes(); got != 23867899 {
		t.Errorf("Bytes() = %d, want %d", got, 23867899)
	}
}

// Size belongs to the build, so every tag naming it reports the same one.
func TestVersionsKeepsTheSizeOfEachRelease(t *testing.T) {
	sizes := map[string]directory.Size{"amd64": 23867899}
	versions := directory.NewVersions("distroless.io/nginx", []directory.Version{
		{Tag: "1.27.3", Digest: digestOf("a"), Sizes: sizes},
		{Tag: "latest", Digest: digestOf("a"), Sizes: sizes},
	})

	if got := versions.Releases[0].Sizes; len(got) != 1 || got[0] != 23867899 {
		t.Errorf("sizes = %v, want [23867899]", got)
	}
}

// A build is not one size: every architecture it publishes is different bytes.
// The page gives each its own column, so the union of what the family
// publishes decides the table's shape.
func TestVersionsColumnsEveryPublishedArchitecture(t *testing.T) {
	versions := directory.NewVersions("distroless.io/nginx", []directory.Version{
		{Tag: "latest", Digest: digestOf("a"), Sizes: map[string]directory.Size{"amd64": 23867899, "arm64": 23345603}},
		{Tag: "1.30", Digest: digestOf("b"), Sizes: map[string]directory.Size{"amd64": 23800000, "arm64": 23300000}},
	})

	if want := []string{"amd64", "arm64"}; !equal(versions.Architectures, want) {
		t.Fatalf("architectures = %v, want %v", versions.Architectures, want)
	}
	if got := versions.Releases[0].Sizes; len(got) != 2 || got[0] != 23867899 || got[1] != 23345603 {
		t.Errorf("sizes = %v, want them aligned with the architecture columns", got)
	}
}

// A build that does not publish one of the family's architectures still has to
// keep the row's shape, or every cell after it lands under the wrong header.
func TestVersionsAlignsABuildMissingAnArchitecture(t *testing.T) {
	versions := directory.NewVersions("distroless.io/nginx", []directory.Version{
		{Tag: "latest", Digest: digestOf("a"), Sizes: map[string]directory.Size{"amd64": 100, "arm64": 200}},
		{Tag: "0.9", Digest: digestOf("b"), Sizes: map[string]directory.Size{"amd64": 50}},
	})

	older := versions.Releases[1]
	if len(older.Sizes) != len(versions.Architectures) {
		t.Fatalf("row has %d cells for %d columns", len(older.Sizes), len(versions.Architectures))
	}
	if older.Sizes[0] != 50 || older.Sizes[1] != 0 {
		t.Errorf("sizes = %v, want amd64 filled and arm64 blank", older.Sizes)
	}
}
