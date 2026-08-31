package directory_test

import (
	"slices"
	"testing"

	"github.com/arkeros/distroless/web/internal/directory"
)

func TestNewTableOrdersRowsByName(t *testing.T) {
	table := directory.NewTable("java:latest", "sha256:abc", "", []directory.Component{
		{Name: "zlib1g", Version: "1.3"},
		{Name: "libc6", Version: "2.36"},
		{Name: "openssl", Version: "3.0.11"},
	})

	got := make([]string, len(table.Rows))
	for i, row := range table.Rows {
		got[i] = row.Name
	}
	want := []string{"libc6", "openssl", "zlib1g"}
	if !slices.Equal(got, want) {
		t.Errorf("row order = %v, want %v", got, want)
	}
}

// The browser sorts the version column on VersionRank, so the ranks — not the
// version strings — are what has to carry dpkg ordering.
func TestVersionRankFollowsVersionOrder(t *testing.T) {
	table := directory.NewTable("java:latest", "sha256:abc", "", []directory.Component{
		{Name: "a", Version: "1.10"},
		{Name: "b", Version: "1.9"},
		{Name: "c", Version: "1.0~rc1"},
	})

	rank := map[string]int{}
	for _, row := range table.Rows {
		rank[row.Name] = row.VersionRank
	}
	if !(rank["c"] < rank["b"] && rank["b"] < rank["a"]) {
		t.Errorf("ranks = %v, want c < b < a (1.0~rc1 < 1.9 < 1.10)", rank)
	}
}

// Sharing a rank lets the browser's stable sort fall back to the name order
// already on the page, instead of shuffling equal versions arbitrarily.
func TestEqualVersionsShareARank(t *testing.T) {
	table := directory.NewTable("java:latest", "sha256:abc", "", []directory.Component{
		{Name: "a", Version: "1.007"},
		{Name: "b", Version: "1.7"},
		{Name: "c", Version: "2.0"},
	})

	rank := map[string]int{}
	for _, row := range table.Rows {
		rank[row.Name] = row.VersionRank
	}
	if rank["a"] != rank["b"] {
		t.Errorf("rank(1.007) = %d, rank(1.7) = %d, want equal", rank["a"], rank["b"])
	}
	if rank["c"] == rank["a"] {
		t.Errorf("rank(2.0) = %d, want distinct from rank(1.7)", rank["c"])
	}
}

// An Index SBOM covers every architecture at once, so without filtering every
// package shows up once per arch and the table reads as if it were duplicated.
func TestNewTableShowsOneArchitectureAtATime(t *testing.T) {
	table := directory.NewTable("nginx:latest", "sha256:abc", "arm64", []directory.Component{
		{Name: "libc6", Version: "2.43-4", Arch: "amd64"},
		{Name: "libc6", Version: "2.43-4", Arch: "arm64"},
		{Name: "zlib1g", Version: "1.3", Arch: "amd64"},
		{Name: "zlib1g", Version: "1.3", Arch: "arm64"},
	})

	if len(table.Rows) != 2 {
		t.Fatalf("got %d rows, want 2 — one per package, not one per package per arch", len(table.Rows))
	}
	for _, row := range table.Rows {
		if row.Arch != "arm64" {
			t.Errorf("row %s has arch %q, want arm64", row.Name, row.Arch)
		}
	}
}

func TestNewTableDefaultsToAmd64(t *testing.T) {
	table := directory.NewTable("nginx:latest", "sha256:abc", "", []directory.Component{
		{Name: "libc6", Arch: "arm64"},
		{Name: "libc6", Arch: "amd64"},
	})

	if table.Arch != "amd64" {
		t.Errorf("arch = %q, want amd64 by default", table.Arch)
	}
	if len(table.Rows) != 1 || table.Rows[0].Arch != "amd64" {
		t.Errorf("rows = %+v, want the single amd64 row", table.Rows)
	}
}

// Asking for something the Index does not carry should show a real table, not
// an empty one.
func TestNewTableFallsBackWhenArchIsAbsent(t *testing.T) {
	table := directory.NewTable("nginx:latest", "sha256:abc", "riscv64", []directory.Component{
		{Name: "libc6", Arch: "arm64"},
	})

	if table.Arch != "arm64" {
		t.Errorf("arch = %q, want arm64 — the only one present", table.Arch)
	}
	if len(table.Rows) != 1 {
		t.Errorf("got %d rows, want the arm64 row", len(table.Rows))
	}
}

// Architecture-independent packages belong to every architecture.
func TestNewTableKeepsArchIndependentComponents(t *testing.T) {
	table := directory.NewTable("nginx:latest", "sha256:abc", "arm64", []directory.Component{
		{Name: "libc6", Arch: "arm64"},
		{Name: "tzdata", Arch: "all"},
		{Name: "upstream-tarball", Arch: ""},
	})

	if len(table.Rows) != 3 {
		t.Fatalf("got %d rows, want all 3: the arm64 one plus both arch-independent ones", len(table.Rows))
	}
}

func TestNewTableListsAvailableArchitectures(t *testing.T) {
	table := directory.NewTable("nginx:latest", "sha256:abc", "", []directory.Component{
		{Name: "a", Arch: "arm64"},
		{Name: "b", Arch: "amd64"},
		{Name: "c", Arch: "all"},
		{Name: "d", Arch: ""},
	})

	if !slices.Equal(table.Architectures, []string{"amd64", "arm64"}) {
		t.Errorf("architectures = %v, want [amd64 arm64] — sorted, without \"all\" or the empty one", table.Architectures)
	}
}
