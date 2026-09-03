package directory

import (
	"cmp"
	"slices"
)

// Component is one entry of an SBOM as the page shows it: a package, its
// version, and enough about it to tell whether a scanner could route a finding
// to it.
type Component struct {
	Name    string
	Version string
	License string
	// Type is the purl ecosystem — deb, apk, golang, generic.
	Type string
	PURL string
	// Arch is the architecture the Component was built for, read from the
	// purl's `arch` qualifier. Empty or "all" means architecture-independent.
	Arch string
	// CPE is set when the Component carries one. Together with Type it is
	// what decides whether a Component is Routable: a scanner can look it up
	// iff the purl ecosystem is one it matches on, or a cpe is present. A
	// Component that is neither is invisible to scanning, which is the
	// silent-zero hazard //oci:supply_chain.bzl gates on.
	CPE string
}

// Row is a Component prepared for rendering.
type Row struct {
	Component
	// VersionRank is the Component's position under CompareVersion ordering.
	// The browser sorts the version column on this integer rather than
	// reimplementing dpkg's algorithm in JavaScript.
	VersionRank int
}

// Table is one SBOM ready to render, for one architecture.
type Table struct {
	// Image is what the reader asked for, e.g. "java:latest".
	Image string
	// Digest is what they actually got. Every page is rendered for a digest,
	// so it can be cached forever.
	Digest string
	// Arch is the architecture these Rows describe. An Index carries every
	// architecture at once, and showing them together reads as a table where
	// every package is listed twice — so the page shows one and links to the
	// rest.
	Arch string
	// Architectures is every architecture the Index carries, sorted.
	Architectures []string
	// Download is where the signed document behind this page can be had.
	// Set by the handler, which knows the reader's own path and reference;
	// the Table is built from an image name that has already been rewritten
	// for display.
	Download string
	// Permalink is this same page addressed by Digest rather than by the tag
	// the reader arrived on — the URL that will still show this exact build
	// after the tag has moved. Empty when the reader is already at it.
	Permalink string
	Rows      []Row
}

// defaultArch is what a reader gets when they express no preference.
const defaultArch = "amd64"

// NewTable selects the Components belonging to arch, orders them by name —
// the order the page is served in, and the tiebreak the browser falls back to
// when sorting on another column — and ranks them by version.
//
// arch may be empty or name something the Index does not carry; either way a
// real table comes back, defaulting to amd64 and otherwise to whatever is
// there. A reader who guesses a wrong architecture should see the image, not
// an empty page.
func NewTable(image, digest, arch string, components []Component) *Table {
	available := architectures(components)
	arch = resolveArch(arch, available)

	rows := make([]Row, 0, len(components))
	for _, component := range components {
		if belongsTo(component, arch) {
			rows = append(rows, Row{Component: component})
		}
	}
	slices.SortStableFunc(rows, func(a, b Row) int {
		return cmp.Or(
			cmp.Compare(a.Name, b.Name),
			CompareVersion(a.Version, b.Version),
		)
	})
	rankVersions(rows)

	return &Table{
		Image:         image,
		Digest:        digest,
		Arch:          arch,
		Architectures: available,
		Rows:          rows,
	}
}

// belongsTo reports whether a Component is shown for arch. An absent or "all"
// architecture marks a Component as architecture-independent, which means it
// belongs to every one of them rather than to none.
func belongsTo(component Component, arch string) bool {
	return component.Arch == arch || component.Arch == "" || component.Arch == "all"
}

// architectures lists the concrete architectures an SBOM covers.
func architectures(components []Component) []string {
	found := make([]string, 0, len(components))
	for _, component := range components {
		if component.Arch != "" && component.Arch != "all" {
			found = append(found, component.Arch)
		}
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// resolveArch picks the architecture to render.
func resolveArch(requested string, available []string) string {
	switch {
	case slices.Contains(available, requested):
		return requested
	case slices.Contains(available, defaultArch):
		return defaultArch
	case len(available) > 0:
		return available[0]
	default:
		// Nothing is architecture-specific, so there is nothing to choose.
		return ""
	}
}

// rankVersions numbers rows by version order, leaving equal versions on the
// same rank so a stable sort in the browser keeps them in name order.
func rankVersions(rows []Row) {
	order := make([]int, len(rows))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return CompareVersion(rows[a].Version, rows[b].Version)
	})

	rank := 0
	for position, row := range order {
		if position > 0 && CompareVersion(rows[order[position-1]].Version, rows[row].Version) != 0 {
			rank++
		}
		rows[row].VersionRank = rank
	}
}
