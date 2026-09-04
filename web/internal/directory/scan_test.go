package directory_test

import (
	"slices"
	"testing"
	"time"

	"github.com/arkeros/distroless/web/internal/directory"
)

func report(arch string, findings ...directory.Finding) *directory.Report {
	return directory.NewReport("nginx:latest", "sha256:abc", arch, &directory.Scan{
		Scanner:  "grype 0.118.0",
		Database: time.Date(2026, 9, 3, 0, 34, 4, 0, time.UTC),
		Finished: time.Date(2026, 9, 3, 21, 12, 53, 0, time.UTC),
		Findings: findings,
	})
}

func ids(rows []directory.FindingRow) []string {
	got := make([]string, len(rows))
	for i, row := range rows {
		got[i] = row.ID
	}
	return got
}

// The worst news first: a reader scanning the table should meet the finding
// that matters most before the twenty negligible ones.
func TestNewReportOrdersRowsBySeverity(t *testing.T) {
	r := report("",
		directory.Finding{ID: "CVE-2026-0003", Severity: "Negligible"},
		directory.Finding{ID: "CVE-2026-0001", Severity: "Critical"},
		directory.Finding{ID: "CVE-2026-0004", Severity: "Unknown"},
		directory.Finding{ID: "CVE-2026-0002", Severity: "High"},
	)

	want := []string{"CVE-2026-0001", "CVE-2026-0002", "CVE-2026-0003", "CVE-2026-0004"}
	if got := ids(r.Rows); !slices.Equal(got, want) {
		t.Errorf("row order = %v, want %v", got, want)
	}
}

// Within one severity, the newest identifier first: a CVE numbered this year
// is the one a reader has not heard about yet.
func TestNewReportOrdersEqualSeveritiesByIDNewestFirst(t *testing.T) {
	r := report("",
		directory.Finding{ID: "CVE-2019-0001", Severity: "High"},
		directory.Finding{ID: "CVE-2026-0001", Severity: "High"},
		directory.Finding{ID: "CVE-2024-0001", Severity: "High"},
	)

	want := []string{"CVE-2026-0001", "CVE-2024-0001", "CVE-2019-0001"}
	if got := ids(r.Rows); !slices.Equal(got, want) {
		t.Errorf("row order = %v, want %v", got, want)
	}
}

// The browser sorts the severity column on the rank, so the rank — not the
// label — has to carry the order, and unknown has to land last rather than
// wherever the alphabet puts it.
func TestSeverityRankFollowsSeverityOrder(t *testing.T) {
	r := report("",
		directory.Finding{ID: "a", Severity: "Unknown"},
		directory.Finding{ID: "b", Severity: "Negligible"},
		directory.Finding{ID: "c", Severity: "Low"},
		directory.Finding{ID: "d", Severity: "Medium"},
		directory.Finding{ID: "e", Severity: "High"},
		directory.Finding{ID: "f", Severity: "Critical"},
	)

	rank := map[string]int{}
	for _, row := range r.Rows {
		rank[row.ID] = row.SeverityRank
	}
	if !(rank["f"] < rank["e"] && rank["e"] < rank["d"] && rank["d"] < rank["c"] && rank["c"] < rank["b"] && rank["b"] < rank["a"]) {
		t.Errorf("ranks = %v, want critical < high < medium < low < negligible < unknown", rank)
	}
}

// A finding the scanner set aside on the strength of a VEX statement is still
// listed — silencing on the record is the point of a VEX statement — but after
// the findings that stand.
func TestNewReportListsSuppressedFindingsLast(t *testing.T) {
	r := report("",
		directory.Finding{ID: "CVE-2026-0001", Severity: "Critical", Suppressed: &directory.Suppression{Status: "not_affected", Justification: "vulnerable_code_not_in_execute_path"}},
		directory.Finding{ID: "CVE-2026-0002", Severity: "Low"},
	)

	want := []string{"CVE-2026-0002", "CVE-2026-0001"}
	if got := ids(r.Rows); !slices.Equal(got, want) {
		t.Errorf("row order = %v, want open findings before suppressed ones", got)
	}
	if r.Open != 1 || r.Suppressed != 1 {
		t.Errorf("open = %d, suppressed = %d, want 1 and 1", r.Open, r.Suppressed)
	}
}

// An Index scan covers every architecture, so a finding against busybox shows
// up once per architecture and the table reads as if it were duplicated.
func TestNewReportShowsOneArchitectureAtATime(t *testing.T) {
	r := report("arm64",
		directory.Finding{ID: "CVE-2026-0001", Package: "busybox", Arch: "amd64"},
		directory.Finding{ID: "CVE-2026-0001", Package: "busybox", Arch: "arm64"},
		directory.Finding{ID: "CVE-2026-0002", Package: "tzdata", Arch: "all"},
	)

	if len(r.Rows) != 2 {
		t.Fatalf("got %d rows, want 2 — the arm64 finding and the architecture-independent one", len(r.Rows))
	}
	if r.Arch != "arm64" {
		t.Errorf("arch = %q, want arm64", r.Arch)
	}
	if !slices.Equal(r.Architectures, []string{"amd64", "arm64"}) {
		t.Errorf("architectures = %v, want [amd64 arm64]", r.Architectures)
	}
}

func TestNewReportDefaultsToAmd64(t *testing.T) {
	r := report("",
		directory.Finding{ID: "a", Arch: "arm64"},
		directory.Finding{ID: "a", Arch: "amd64"},
	)

	if r.Arch != "amd64" || len(r.Rows) != 1 {
		t.Errorf("arch = %q with %d rows, want amd64 and one row", r.Arch, len(r.Rows))
	}
}

// The summary is what a reader takes away without reading the table. Only the
// severities that occur, in order, and only for findings that stand: a
// suppressed critical is not a critical.
func TestNewReportSummarisesOpenFindingsBySeverity(t *testing.T) {
	r := report("",
		directory.Finding{ID: "a", Severity: "High"},
		directory.Finding{ID: "b", Severity: "High"},
		directory.Finding{ID: "c", Severity: "Negligible"},
		directory.Finding{ID: "d", Severity: "Critical", Suppressed: &directory.Suppression{Status: "not_affected"}},
	)

	want := []directory.SeverityCount{{Severity: "High", Count: 2}, {Severity: "Negligible", Count: 1}}
	if !slices.Equal(r.Summary, want) {
		t.Errorf("summary = %v, want %v", r.Summary, want)
	}
}

// Fix versions are the actionable part of a finding, and a reader wants them
// as one cell rather than as a list.
func TestFindingRowJoinsFixVersions(t *testing.T) {
	r := report("", directory.Finding{ID: "a", FixedIn: []string{"1.2.3-1", "1.3.0-1"}})

	if got := r.Rows[0].Fix; got != "1.2.3-1, 1.3.0-1" {
		t.Errorf("fix = %q, want the versions joined", got)
	}
}

// A scan with nothing in it is a real page, not an error: it says what was
// scanned, with what, and when — which is what makes an empty table mean
// something.
func TestNewReportWithNoFindings(t *testing.T) {
	r := report("")

	if len(r.Rows) != 0 || r.Open != 0 {
		t.Errorf("rows = %v, want none", r.Rows)
	}
	if r.Scanner != "grype 0.118.0" {
		t.Errorf("scanner = %q, want it carried over", r.Scanner)
	}
	if r.Database.IsZero() || r.Finished.IsZero() {
		t.Error("scan dates dropped; an empty result without them is a silent zero")
	}
}
