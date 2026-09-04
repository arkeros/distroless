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
		directory.Finding{ID: "CVE-2026-0003", Severity: directory.Negligible},
		directory.Finding{ID: "CVE-2026-0001", Severity: directory.Critical},
		directory.Finding{ID: "CVE-2026-0004", Severity: directory.Unknown},
		directory.Finding{ID: "CVE-2026-0002", Severity: directory.High},
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
		directory.Finding{ID: "CVE-2019-0001", Severity: directory.High},
		directory.Finding{ID: "CVE-2026-0001", Severity: directory.High},
		directory.Finding{ID: "CVE-2024-0001", Severity: directory.High},
	)

	want := []string{"CVE-2026-0001", "CVE-2024-0001", "CVE-2019-0001"}
	if got := ids(r.Rows); !slices.Equal(got, want) {
		t.Errorf("row order = %v, want %v", got, want)
	}
}

// A scanner's label enters the model once, through ParseSeverity, and a label
// off the scale becomes Unknown there rather than surviving as a string every
// consumer has to defend against.
func TestParseSeverity(t *testing.T) {
	for label, want := range map[string]directory.Severity{
		"Critical":     directory.Critical,
		"high":         directory.High,
		"MEDIUM":       directory.Medium,
		"Low":          directory.Low,
		"Negligible":   directory.Negligible,
		"Unknown":      directory.Unknown,
		"Catastrophic": directory.Unknown,
		"":             directory.Unknown,
	} {
		if got := directory.ParseSeverity(label); got != want {
			t.Errorf("ParseSeverity(%q) = %v, want %v", label, got, want)
		}
	}
}

// The scale ranks worst first, which is what the rows are sorted on and what
// the browser sorts on — and its zero value is Unknown, so a Finding built
// without a severity does not read as Critical.
func TestSeverityScaleRanksWorstFirst(t *testing.T) {
	scale := []directory.Severity{directory.Critical, directory.High, directory.Medium, directory.Low, directory.Negligible, directory.Unknown}
	for i := 1; i < len(scale); i++ {
		if !(scale[i-1].Rank() < scale[i].Rank()) {
			t.Errorf("%v does not rank before %v", scale[i-1], scale[i])
		}
	}
	var zero directory.Severity
	if zero != directory.Unknown {
		t.Errorf("zero value is %v, want Unknown", zero)
	}
	if directory.High.String() != "High" || directory.High.Lower() != "high" {
		t.Errorf("High renders as %q / %q", directory.High.String(), directory.High.Lower())
	}
}

// A finding the scanner set aside on the strength of a VEX statement is still
// listed — silencing on the record is the point of a VEX statement — but after
// the findings that stand.
func TestNewReportListsSuppressedFindingsLast(t *testing.T) {
	r := report("",
		directory.Finding{ID: "CVE-2026-0001", Severity: directory.Critical, Suppressed: &directory.Suppression{Status: "not_affected", Justification: "vulnerable_code_not_in_execute_path"}},
		directory.Finding{ID: "CVE-2026-0002", Severity: directory.Low},
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
		directory.Finding{ID: "a", Severity: directory.High},
		directory.Finding{ID: "b", Severity: directory.High},
		directory.Finding{ID: "c", Severity: directory.Negligible},
		directory.Finding{ID: "d", Severity: directory.Critical, Suppressed: &directory.Suppression{Status: "not_affected"}},
	)

	want := []directory.SeverityCount{{Severity: directory.High, Count: 2}, {Severity: directory.Negligible, Count: 1}}
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
