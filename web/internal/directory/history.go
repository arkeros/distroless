package directory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Move is one event in a family's tag ledger: a tag pointed at a build, from
// when. The `release` job appends one whenever a tag it applies names a
// different build from the one the ledger last recorded (ADR 0017).
//
// Like a Version, a Move is registry metadata that no attestation covers.
// What it gives a reader is the way in: which digest's scan records are the
// tag's between this event and the next one for the same tag.
type Move struct {
	At     time.Time
	Tag    string
	Digest string
	// Run is the URL of the CI run that applied the tag: where a reader goes
	// to see the binding being made.
	Run string
}

// Era is one build's tenure under a tag: the build, when the tag came to
// name it, and every scan of it. Scans made after the next Era began are not
// this tag's — the build may still be another tag's, and still be scanned —
// and NewHistory leaves them out.
type Era struct {
	Digest string
	// From is zero when unknown: a build reached by its digest, or the
	// current build of a family whose ledger has nothing yet. Such an Era is
	// the last.
	From  time.Time
	Scans []*Scan
}

// The chart stacks findings in one band per severity, worst first, unknown
// last: it is not a low severity but the absence of one.
const bands = 6

var bandNames = [bands]string{"Critical", "High", "Medium", "Low", "Negligible", "Unknown"}

// bandClasses is the class each band's marks carry, in the same order: the
// stylesheet gives each its own colour, red through yellow for the ones a
// reader acts on, blues for low and negligible, grey for unknown.
var bandClasses = [bands]string{"critical", "high", "medium", "low", "negligible", "unknown"}

func band(severity Severity) int {
	switch severity {
	case Critical:
		return 0
	case High:
		return 1
	case Medium:
		return 2
	case Low:
		return 3
	case Negligible:
		return 4
	default:
		return 5
	}
}

// Point is one scan reduced to what a history shows: when, of which build,
// and how many findings stood at each band.
type Point struct {
	Digest   string
	Finished time.Time
	Database time.Time
	// Counts is the open findings per band, worst first; Open is their sum.
	// Counted as the vulnerabilities page counts, for one architecture with
	// the per-package copies of a finding folded, so the two agree.
	Counts     [bands]int
	Open       int
	Suppressed int
	// Short is the digest as a table cell shows it.
	Short string
	// URL is the vulnerabilities page of this scan's build. Set by the
	// handler, which knows the reader's own path.
	URL string
}

// Band is one legend entry.
type Band struct {
	Name  string
	Class string
}

// Segment is one band's share of a bar, in viewBox units.
type Segment struct {
	Class  string
	Y      float64
	Height float64
}

// Bar is one Point drawn: its column, bottom segment first.
type Bar struct {
	X        float64
	Width    float64
	Right    float64
	Segments []Segment
	// Title is what a hover says about the bar, since the marks carry no
	// numbers of their own.
	Title string
}

// Tick is one gridline of the value axis.
type Tick struct {
	Value int
	Y     float64
	Label string
}

// DateTick is one label of the time axis.
type DateTick struct {
	X     float64
	Label string
}

// Chart is the history as an SVG's worth of geometry, computed here rather
// than in the browser: the page has no script it depends on, and a chart
// that renders from the first byte is one that prints, indexes and reads
// without one.
type Chart struct {
	Width, Height float64
	// Left and Baseline are the axes: the value axis runs up from Baseline
	// at Left, the time axis runs right along it. Top is where the plot
	// begins and PlotHeight how far it is to the Baseline.
	Left, Right, Top, Baseline, PlotHeight float64
	Bars                                   []Bar
	Ticks                                  []Tick
	Dates                                  []DateTick
	Bands                                  []Band
}

// The viewBox and the margins that hold the axis labels.
const (
	chartWidth  = 720.0
	chartHeight = 240.0
	chartLeft   = 36.0
	chartRight  = 8.0
	chartTop    = 12.0
	chartBottom = 24.0
	// A bar is at most this wide, however few there are: a column that
	// fills its slot reads as an area.
	maxBarWidth = 24.0
	minBarWidth = 3.0
	// surfaceGap separates stacked segments, in the surface colour.
	surfaceGap = 2.0
	// maxDateLabels bounds the time axis so labels do not collide.
	maxDateLabels = 8
)

// History is a tag's, or a build's, scans over time, ready to render.
type History struct {
	// Image is what the reader asked for, e.g. "nginx:latest".
	Image string
	// Digest is the build the reference names today.
	Digest string
	// Tag is what the history is of; empty when the page was reached by
	// digest, in which case it is that one build's.
	Tag string
	// Arch is the architecture the counts are for; Architectures is every
	// one any scan covers, sorted. As on the vulnerabilities page.
	Arch          string
	Architectures []string
	// Logo is the family's mark, or empty. Set by the handler.
	Logo template.HTML
	// Topbar is the strip every page shares. Set by the handler.
	Topbar Topbar
	Links
	// Points is every scan, oldest first; From and To are the first and
	// the last of them. Builds is how many distinct builds they are of.
	Points   []Point
	From, To time.Time
	Builds   int
	// Chart is nil when there is nothing to draw.
	Chart *Chart
	// Ledger is false when the tag's history could not be read and only
	// the current build is shown, so the page can say so.
	Ledger bool
}

// NewHistory turns the eras of a tag into points and a chart.
//
// Eras are ordered by when they began, unknown last, and each one's scans
// are cut where the next begins. The points that remain are ordered by
// scan time regardless of era: a tag that moved back to an earlier build
// has that build's later scans after the interlude, not before it.
func NewHistory(image, digest, arch string, eras []Era) *History {
	eras = slices.Clone(eras)
	slices.SortStableFunc(eras, func(a, b Era) int {
		switch {
		case a.From.IsZero() && b.From.IsZero():
			return 0
		case a.From.IsZero():
			return 1
		case b.From.IsZero():
			return -1
		default:
			return a.From.Compare(b.From)
		}
	})

	var scans []*Scan
	for _, era := range eras {
		for _, scan := range era.Scans {
			scans = append(scans, scan)
		}
	}
	available := architectures(findingsOf(scans), func(f Finding) string { return f.Arch })
	arch = resolveArch(arch, available)

	history := &History{Image: image, Digest: digest, Arch: arch, Architectures: available}
	builds := map[string]bool{}
	for i, era := range eras {
		var until time.Time
		if i+1 < len(eras) {
			until = eras[i+1].From
		}
		for _, scan := range era.Scans {
			if !until.IsZero() && !scan.Finished.Before(until) {
				continue
			}
			report := NewReport(image, era.Digest, arch, scan)
			point := Point{Digest: era.Digest, Short: shortDigest(era.Digest), Finished: scan.Finished, Database: scan.Database, Open: report.Open, Suppressed: report.Suppressed}
			for _, count := range report.Summary {
				point.Counts[band(count.Severity)] += count.Count
			}
			history.Points = append(history.Points, point)
			builds[era.Digest] = true
		}
	}
	slices.SortStableFunc(history.Points, func(a, b Point) int { return a.Finished.Compare(b.Finished) })
	if len(history.Points) > 0 {
		history.From = history.Points[0].Finished
		history.To = history.Points[len(history.Points)-1].Finished
	}
	history.Builds = len(builds)
	history.Chart = draw(history.Points)
	return history
}

// findingsOf flattens the scans' findings, for the architecture list.
func findingsOf(scans []*Scan) []Finding {
	var findings []Finding
	for _, scan := range scans {
		findings = append(findings, scan.Findings...)
	}
	return findings
}

// Fingerprint summarises everything the page shows that can change while
// the reference stays the same: which scans, of which builds, with which
// counts. For the cache validator.
func (h *History) Fingerprint() string {
	sum := sha256.New()
	for _, point := range h.Points {
		fmt.Fprintf(sum, "%s\x1f%s\x1f%v\x1f%d\n", point.Digest, point.Finished.UTC().Format(time.RFC3339Nano), point.Counts, point.Suppressed)
	}
	return hex.EncodeToString(sum.Sum(nil))[:16]
}

// draw lays the points out as stacked columns on a time axis. Nil for no
// points: an empty chart says "no findings", which "no scans" is not.
func draw(points []Point) *Chart {
	if len(points) == 0 {
		return nil
	}

	chart := &Chart{
		Width:      chartWidth,
		Height:     chartHeight,
		Left:       chartLeft,
		Right:      chartWidth - chartRight,
		Top:        chartTop,
		Baseline:   chartHeight - chartBottom,
		PlotHeight: chartHeight - chartBottom - chartTop,
	}
	for i := range bands {
		chart.Bands = append(chart.Bands, Band{Name: bandNames[i], Class: bandClasses[i]})
	}
	plotWidth := chart.Right - chart.Left
	plotHeight := chart.Baseline - chartTop

	// The value axis: round ticks up to the worst scan, five at most.
	worst := 0
	for _, point := range points {
		worst = max(worst, point.Open)
	}
	step := niceStep(worst)
	top := max(step, int(math.Ceil(float64(worst)/float64(step)))*step)
	scale := plotHeight / float64(top)
	for value := 0; value <= top; value += step {
		chart.Ticks = append(chart.Ticks, Tick{Value: value, Y: chart.Baseline - float64(value)*scale, Label: strconv.Itoa(value)})
	}

	// The time axis: bars sit where their scan falls between the first and
	// the last, so a week without a scan shows as a gap rather than being
	// closed up. One bar takes the whole width to itself.
	first, last := points[0].Finished, points[len(points)-1].Finished
	span := last.Sub(first)
	days := int(math.Ceil(span.Hours()/24)) + 1
	width := math.Min(maxBarWidth, math.Max(minBarWidth, plotWidth/float64(days)*0.6))
	x := func(t time.Time) float64 {
		if span == 0 {
			return chart.Left + plotWidth/2
		}
		return chart.Left + width/2 + float64(t.Sub(first))/float64(span)*(plotWidth-width)
	}

	for _, point := range points {
		bar := Bar{X: x(point.Finished) - width/2, Width: width, Right: x(point.Finished) + width/2, Title: title(point)}
		y := chart.Baseline
		// Stacked from the least band up, so critical is the top of the
		// bar: the edge a reader's eye lands on is the band to read first.
		for i := bands - 1; i >= 0; i-- {
			count := point.Counts[i]
			if count == 0 {
				continue
			}
			height := float64(count) * scale
			gap := 0.0
			if len(bar.Segments) > 0 && height > 2*surfaceGap {
				gap = surfaceGap
			}
			bar.Segments = append(bar.Segments, Segment{Class: bandClasses[i], Y: y - height + gap, Height: height - gap})
			y -= height
		}
		chart.Bars = append(chart.Bars, bar)
	}

	every := max(1, int(math.Ceil(float64(days)/maxDateLabels)))
	for day := 0; day < days; day += every {
		date := first.Add(time.Duration(day) * 24 * time.Hour)
		chart.Dates = append(chart.Dates, DateTick{X: x(date), Label: date.UTC().Format("2 Jan")})
	}
	return chart
}

// niceStep is the tick interval for a value axis up to worst: 1, 2 or 5
// times a power of ten, chosen so the axis has five ticks at most.
func niceStep(worst int) int {
	if worst < 5 {
		return 1
	}
	magnitude := math.Pow(10, math.Floor(math.Log10(float64(worst)/5)))
	for _, unit := range []float64{1, 2, 5, 10} {
		step := unit * magnitude
		if float64(worst)/step <= 5 {
			return int(step)
		}
	}
	return int(10 * magnitude)
}

// title is a bar's hover text: the date, the open count, and the bands.
func title(point Point) string {
	parts := make([]string, 0, bands)
	for i, count := range point.Counts {
		if count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, strings.ToLower(bandNames[i])))
		}
	}
	text := point.Finished.UTC().Format("2006-01-02") + ": "
	switch {
	case point.Open == 0:
		text += "no findings"
	case point.Open == 1:
		text += "1 finding (" + strings.Join(parts, ", ") + ")"
	default:
		text += fmt.Sprintf("%d findings (%s)", point.Open, strings.Join(parts, ", "))
	}
	if point.Suppressed > 0 {
		text += fmt.Sprintf(", %d set aside", point.Suppressed)
	}
	return text
}
