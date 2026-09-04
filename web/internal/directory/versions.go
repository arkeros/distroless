package directory

import (
	"cmp"
	"fmt"
	"html/template"
	"slices"
	"time"
)

// Size is what pulling a build transfers: the sum of its compressed layers,
// which is the number a registry reports and the one distroless exists to
// keep small. Not the size on disk, which is the uncompressed rootfs and
// cannot be known without downloading it.
type Size int64

// Bytes is the raw count, for a browser that has to sort on it. "9 MB" above
// "23.9 MB" is what sorting the rendered string gives.
func (s Size) Bytes() int64 { return int64(s) }

// String renders a Size the way a registry does. Zero means the manifest
// could not be read: no published build is empty, so there is nothing to
// confuse it with.
func (s Size) String() string {
	switch {
	case s == 0:
		return "unknown"
	case s < 1_000:
		return fmt.Sprintf("%d B", int64(s))
	case s < 1_000_000:
		return fmt.Sprintf("%.0f kB", float64(s)/1_000)
	default:
		return fmt.Sprintf("%.1f MB", float64(s)/1_000_000)
	}
}

// Version is one published tag and the build it currently names.
//
// Unlike everything else this package renders, a Version is not read off a
// verified attestation — it is registry metadata, tamper-evident by content
// addressing but signed by nobody. The evidence starts at the SBOM each
// Release links to.
type Version struct {
	Tag    string
	Digest string
	// Created is the image config's `created`, which //oci:created_timestamp.bzl
	// sets to the upstream-snapshot anchor of the distro lockfile rather than
	// to a build or push time. It is the build horizon an admission controller
	// gates on: how fresh the packages inside are. Zero when it could not be
	// read, which is a gap in the display and not an error.
	Created time.Time
	// Sizes is what pulling this build transfers, per architecture. A build
	// is not one size: every architecture it publishes is different bytes.
	// Empty when the manifest could not be read.
	Sizes map[string]Size
}

// Tag is one published name and where it leads.
//
// A tag URL follows the family: it will show a different build once the tag
// moves. That is a different question from the Release's own digest URL, and
// the reason a row offers both.
type Tag struct {
	Name string
	// URL is where this name currently leads: the same view, for the build
	// the tag names today. Set by the handler, which knows the reader's own
	// path and which view they are on.
	URL string
}

// Release is one build and every tag that names it. A tag_list in
// //images/... is applied to a single digest, so several tags naming one
// build is the normal case rather than the exception — and saying which ones
// is what a reader comes to this page for.
type Release struct {
	Digest  string
	Tags    []Tag
	Created time.Time
	// Sizes is aligned with Versions.Architectures rather than keyed, so a
	// row and the header carry the same shape: cell i is always column i.
	// A build that does not publish an architecture has a zero there.
	Sizes []Size
	// SBOM is where this build's evidence starts, at its permanent URL
	// rather than through a tag that may since have moved. The digest is the
	// link. Set by the handler, which knows the reader's own path.
	SBOM string
}

// Versions is the list of what a family currently publishes, ready to render.
type Versions struct {
	// Image is the family as a reader would pull it, without a reference:
	// e.g. "distroless.io/nginx".
	Image string
	// Architectures is every architecture any listed build publishes,
	// sorted. One size column each.
	Architectures []string
	Releases      []Release
	// Logo is the family's mark, the same one the front page draws, or
	// empty for a family that has none. Set by the handler.
	Logo template.HTML
	// SBOM and Vulnerabilities are the other two views in the navigation
	// this page shares with them: the evidence for the build a bare pull
	// would get, since a family-level page has no build of its own.
	SBOM            string
	Vulnerabilities string
}

// NewVersions groups tags by the build they name and orders both, newest
// first.
func NewVersions(image string, versions []Version) *Versions {
	byDigest := make(map[string]*Release)
	sizes := make(map[string]map[string]Size)
	order := make([]string, 0, len(versions))
	for _, version := range versions {
		release, seen := byDigest[version.Digest]
		if !seen {
			release = &Release{Digest: version.Digest, Created: version.Created}
			sizes[version.Digest] = version.Sizes
			byDigest[version.Digest] = release
			order = append(order, version.Digest)
		}
		release.Tags = append(release.Tags, Tag{Name: version.Tag})
	}

	architectures := publishedArchitectures(sizes)
	releases := make([]Release, 0, len(order))
	for _, digest := range order {
		release := byDigest[digest]
		slices.SortStableFunc(release.Tags, func(a, b Tag) int { return compareTag(a.Name, b.Name) })
		release.Sizes = make([]Size, len(architectures))
		for i, architecture := range architectures {
			release.Sizes[i] = sizes[digest][architecture]
		}
		releases = append(releases, *release)
	}
	slices.SortStableFunc(releases, compareRelease)

	return &Versions{Image: image, Architectures: architectures, Releases: releases}
}

// publishedArchitectures is every architecture any build carries, sorted —
// the union rather than any one build's, so a build that stopped publishing
// one still lines up under the headers.
func publishedArchitectures(sizes map[string]map[string]Size) []string {
	found := make([]string, 0, len(sizes))
	for _, byArchitecture := range sizes {
		for architecture := range byArchitecture {
			found = append(found, architecture)
		}
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// Tags fall into three kinds, and a reader wants them in this order: the one
// name that always means "the current build", then the other names this
// family answers to — channels like `stable` and `mainline`, and `debug` —
// then the versions each build pins.
const (
	tagLatest = iota
	tagName
	tagVersion
)

// tagRank classifies a published tag. A tag names a version iff it starts with a
// digit: `1.31`, `1.31-debug` and `21` do, `latest`, `stable` and
// `mainline-debug` do not.
func tagRank(tag string) int {
	switch {
	case tag == defaultRef:
		return tagLatest
	case tag == "" || tag[0] < '0' || tag[0] > '9':
		return tagName
	default:
		return tagVersion
	}
}

// compareTag orders the tags of one Release.
func compareTag(a, b string) int {
	if byKind := cmp.Compare(tagRank(a), tagRank(b)); byKind != 0 {
		return byKind
	}
	if tagRank(a) == tagVersion {
		return compareVersionTag(a, b)
	}
	// Two names rather than two versions: nothing orders them but the
	// alphabet, and a stable answer is worth more than a clever one.
	return cmp.Compare(a, b)
}

// compareVersionTag orders two version tags for display: newer upstream
// first, and within one upstream version the plain build before its variants.
//
// Not just CompareVersion reversed. That would put `1.31-debug` above `1.31`,
// because to dpkg a revision outranks no revision at all — true of Debian
// package revisions, wrong for a tag suffix naming a variant of the same
// build.
func compareVersionTag(a, b string) int {
	aEpoch, aUpstream, aVariant := splitVersion(a)
	bEpoch, bUpstream, bVariant := splitVersion(b)
	return cmp.Or(
		bEpoch-aEpoch,
		-compareFragment(aUpstream, bUpstream),
		compareFragment(aVariant, bVariant),
	)
}

// compareRelease orders builds as a reader scans them: whatever `latest`
// currently names, then by the version each build pins.
func compareRelease(a, b Release) int {
	return cmp.Or(
		compareNamedLatest(a, b),
		compareVersionTag(a.version(), b.version()),
		// Nothing but names to go on, e.g. a family that pins no versions.
		compareTag(a.Tags[0].Name, b.Tags[0].Name),
	)
}

func compareNamedLatest(a, b Release) int {
	switch {
	case a.namesLatest() && !b.namesLatest():
		return -1
	case !a.namesLatest() && b.namesLatest():
		return 1
	default:
		return 0
	}
}

// namesLatest reports whether this is the build a bare pull would get.
func (r Release) namesLatest() bool {
	return len(r.Tags) > 0 && r.Tags[0].Name == defaultRef
}

// version is the best version tag pinning this Release, or empty if it pins
// none. Tags are already ordered, so it is the first one that is a version.
func (r Release) version() string {
	for _, tag := range r.Tags {
		if tagRank(tag.Name) == tagVersion {
			return tag.Name
		}
	}
	return ""
}
