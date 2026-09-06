package directory_test

import (
	"strings"
	"testing"
	"time"

	"github.com/arkeros/distroless/web/internal/directory"
)

func day(d int) time.Time { return time.Date(2026, 9, d, 12, 0, 0, 0, time.UTC) }

// scanOn is a scan finished on the given day with the given open findings,
// each spelled "severity" or "severity/arch"; a suppressed finding is
// spelled with a leading "-".
func scanOn(d int, findings ...string) *directory.Scan {
	scan := &directory.Scan{Scanner: "grype 0.118.0", Database: day(d).Add(-6 * time.Hour), Finished: day(d)}
	for i, spec := range findings {
		var suppressed *directory.Suppression
		if strings.HasPrefix(spec, "-") {
			spec = spec[1:]
			suppressed = &directory.Suppression{Status: "not_affected"}
		}
		severity, arch, _ := strings.Cut(spec, "/")
		if arch == "" {
			arch = "amd64"
		}
		scan.Findings = append(scan.Findings, directory.Finding{
			ID:         "CVE-2026-" + strings.Repeat("0", 3) + string(rune('1'+i)),
			Severity:   directory.ParseSeverity(severity),
			Package:    "pkg" + string(rune('a'+i)),
			Version:    "1.0",
			Arch:       arch,
			Suppressed: suppressed,
		})
	}
	return scan
}

func TestHistoryOrdersPointsOldestFirstAcrossBuilds(t *testing.T) {
	history := directory.NewHistory("nginx:latest", testDigest, "", []directory.Era{
		{Digest: otherDigest, From: day(5), Scans: []*directory.Scan{scanOn(6), scanOn(5)}},
		{Digest: testDigest, From: day(1), Scans: []*directory.Scan{scanOn(2), scanOn(1)}},
	})

	if len(history.Points) != 4 {
		t.Fatalf("got %d points, want 4: %+v", len(history.Points), history.Points)
	}
	for i := 1; i < len(history.Points); i++ {
		if !history.Points[i-1].Finished.Before(history.Points[i].Finished) {
			t.Errorf("point %d (%v) is not before point %d (%v)", i-1, history.Points[i-1].Finished, i, history.Points[i].Finished)
		}
	}
	if history.Points[0].Digest != testDigest || history.Points[3].Digest != otherDigest {
		t.Errorf("points name %s then %s, want the earlier build first", history.Points[0].Digest, history.Points[3].Digest)
	}
}

// A build keeps being scanned after a tag has left it — it may still be
// another tag's — and those scans are not this tag's history.
func TestHistoryEndsABuildsScansWhereTheNextBuildBegins(t *testing.T) {
	history := directory.NewHistory("nginx:latest", otherDigest, "", []directory.Era{
		{Digest: testDigest, From: day(1), Scans: []*directory.Scan{scanOn(2), scanOn(9)}},
		{Digest: otherDigest, From: day(5), Scans: []*directory.Scan{scanOn(5)}},
	})

	if len(history.Points) != 2 {
		t.Fatalf("got %d points, want 2: %+v", len(history.Points), history.Points)
	}
	for _, point := range history.Points {
		if point.Digest == testDigest && !point.Finished.Before(day(5)) {
			t.Errorf("point at %v is on %s after the tag moved away", point.Finished, testDigest)
		}
	}
}

// Counted the way the vulnerabilities page counts: one architecture, the
// copies of a finding per binary package folded, suppressed set aside.
func TestHistoryCountsOpenFindingsPerBandLikeTheReport(t *testing.T) {
	scan := scanOn(1, "Critical", "High", "High/arm64", "Medium", "Low", "Negligible", "Unknown", "-High")
	history := directory.NewHistory("nginx:latest", testDigest, "", []directory.Era{
		{Digest: testDigest, Scans: []*directory.Scan{scan}},
	})

	if len(history.Points) != 1 {
		t.Fatalf("got %d points, want 1", len(history.Points))
	}
	point := history.Points[0]
	want := [4]int{1, 1, 1, 3}
	if point.Counts != want {
		t.Errorf("counts = %v, want %v (critical, high, medium, low and below; the arm64 copy left out)", point.Counts, want)
	}
	if point.Open != 6 || point.Suppressed != 1 {
		t.Errorf("open = %d, suppressed = %d, want 6 and 1", point.Open, point.Suppressed)
	}
	if history.Arch != "amd64" {
		t.Errorf("arch = %q, want amd64 by default", history.Arch)
	}
}

func TestHistoryDrawsOneBarPerPointScaledToTheWorstScan(t *testing.T) {
	history := directory.NewHistory("nginx:latest", testDigest, "", []directory.Era{
		{Digest: testDigest, Scans: []*directory.Scan{
			scanOn(1, "High", "High"),
			scanOn(2, "Critical", "High", "Medium", "Low", "Low"),
			scanOn(3),
		}},
	})

	chart := history.Chart
	if chart == nil {
		t.Fatal("no chart drawn for three points")
	}
	if len(chart.Bars) != 3 {
		t.Fatalf("got %d bars, want 3", len(chart.Bars))
	}
	if len(chart.Bars[2].Segments) != 0 {
		t.Errorf("a clean scan drew %d segments, want none", len(chart.Bars[2].Segments))
	}
	if len(chart.Bars[1].Segments) != 4 {
		t.Errorf("the worst scan drew %d segments, want one per band that occurs", len(chart.Bars[1].Segments))
	}
	if top := chart.Ticks[len(chart.Ticks)-1]; top.Value < 5 {
		t.Errorf("top tick = %v, want at least the worst scan's 5 findings", top.Value)
	}
	if !(chart.Bars[0].X < chart.Bars[1].X && chart.Bars[1].X < chart.Bars[2].X) {
		t.Errorf("bars are not left to right in time: %v", chart.Bars)
	}
}

func TestHistoryWithNoScansHasNoChart(t *testing.T) {
	history := directory.NewHistory("nginx:latest", testDigest, "", []directory.Era{{Digest: testDigest}})
	if history.Chart != nil || len(history.Points) != 0 {
		t.Errorf("history = %+v, want no points and no chart", history)
	}
}
