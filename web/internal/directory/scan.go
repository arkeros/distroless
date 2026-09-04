package directory

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"
)

// Scan is one vulnerability scan of an Index, read off a verified
// vulnerability attestation.
//
// A scan is a statement about a moment: which findings a given scanner, with a
// given database, matched against the SBOM. Both dates are carried because an
// empty result is only evidence if the reader can see how fresh the database
// was — a scan from before a CVE was published says nothing about it.
type Scan struct {
	// Scanner names the tool and version that produced the scan, e.g.
	// "grype 0.118.0".
	Scanner string
	// Database is when the vulnerability database the scan consulted was
	// built. The freshness that matters: a scanner only knows the CVEs its
	// database does.
	Database time.Time
	// Finished is when the scan ran.
	Finished time.Time
	Findings []Finding
}

// Fingerprint summarises everything on a page that can change while the
// Digest stays the same: the scan, and what the VEX document says about each
// finding. Two attestations are re-issued under one digest — a scan when the
// database moves, a VEX document when a statement does — so the digest cannot
// be a page's cache validator on its own, and neither can the scan time,
// which a new VEX document leaves untouched. Hashing what is rendered rather
// than naming each source is what keeps a third source from reopening the
// hole.
func (s *Scan) Fingerprint() string {
	sum := sha256.New()
	sum.Write([]byte(s.Scanner + "\n" + s.Database.UTC().Format(time.RFC3339Nano) + "\n" + s.Finished.UTC().Format(time.RFC3339Nano) + "\n"))
	for _, finding := range s.Findings {
		sum.Write([]byte(finding.ID + "\x1f" + finding.PURL + "\x1f" + finding.Severity.String() + "\x1f" + finding.FixState + "\x1f" + strings.Join(finding.FixedIn, ",")))
		if finding.Suppressed != nil {
			sum.Write([]byte("\x1f" + finding.Suppressed.Status + "\x1f" + finding.Suppressed.Justification + "\x1f" + finding.Suppressed.Impact))
		}
		sum.Write([]byte("\n"))
	}
	return hex.EncodeToString(sum.Sum(nil))[:16]
}

// Finding is one vulnerability matched to one Component.
type Finding struct {
	// ID is the vulnerability's identifier — a CVE, or a GHSA where the
	// ecosystem has no CVE for it yet.
	ID       string
	Severity Severity
	// Package and Version identify the Component the finding was matched to.
	Package string
	Version string
	// Type is the purl ecosystem of that Component — deb, rpm, generic.
	Type string
	// Arch is the architecture the Component was built for. An Index scan
	// covers every architecture, so the same finding recurs once per
	// architecture and this is what tells the copies apart.
	Arch string
	PURL string
	// FixedIn lists the versions that carry a fix, when the scanner knows of
	// any. Empty with FixState saying why.
	FixedIn []string
	// FixState is the scanner's word on a fix: fixed, not-fixed, wont-fix or
	// unknown.
	FixState string
	// URL is where the scanner read the vulnerability from — the distro's
	// tracker entry, most often — which is where a reader goes to learn more.
	URL         string
	Description string
	// Suppressed is set when the scanner set this finding aside on the
	// strength of a VEX statement; nil for a finding that stands. The finding
	// is still listed, because a VEX statement silences on the record and the
	// record is what this page shows.
	Suppressed *Suppression
}

// Suppression is what a VEX statement said about a finding.
type Suppression struct {
	// Status is the OpenVEX status the statement carries: not_affected, or
	// fixed for a version the scanner's database has not caught up with.
	Status string
	// Justification is why, for a not_affected statement, in OpenVEX's
	// vocabulary: vulnerable_code_not_in_execute_path and the like.
	Justification string
	// Impact is the statement's own explanation, in prose, when it gave one.
	Impact string
}

// Words says a Suppression the way a reader would: "not affected: vulnerable
// code not in execute path". OpenVEX's vocabulary is snake_case identifiers,
// which are for machines.
func (s Suppression) Words() string {
	words := strings.ReplaceAll(s.Status, "_", " ")
	if s.Justification != "" {
		words += ": " + strings.ReplaceAll(s.Justification, "_", " ")
	}
	return words
}

// Severity is the scanner's scale, worst first. Parsed once where a scanner's
// output enters the model, so that everything downstream — the order rows are
// served in, the summary, the class a cell is styled with — works on the scale
// and never on a string the scanner sent.
type Severity int

const (
	// Unknown is the zero value on purpose: a Finding that never had a
	// severity set must not read as Critical. It is also what the scanner
	// says when its source gives no severity, and what a label off the scale
	// becomes here — a scanner that invents a seventh level is a scanner
	// bug, the page names the scanner, and the signed download keeps the
	// string for anyone who wants it.
	Unknown Severity = iota
	Critical
	High
	Medium
	Low
	Negligible
)

var severityNames = [...]string{"Unknown", "Critical", "High", "Medium", "Low", "Negligible"}

// scale is the severities worst first: the order rows are served in, the
// summary reads in, and Rank counts in. Unknown last, because a finding whose
// severity nobody has assessed is not a reason to push a known one down.
var scale = [...]Severity{Critical, High, Medium, Low, Negligible, Unknown}

// ParseSeverity places a scanner's label on the scale, case-insensitively.
func ParseSeverity(label string) Severity {
	for i, name := range severityNames {
		if strings.EqualFold(label, name) {
			return Severity(i)
		}
	}
	return Unknown
}

// String is the label as shown: "High".
func (s Severity) String() string {
	if s < 0 || int(s) >= len(severityNames) {
		return severityNames[Unknown]
	}
	return severityNames[s]
}

// Lower is the label in lower case: for a class name, and mid-sentence.
func (s Severity) Lower() string { return strings.ToLower(s.String()) }

// Rank is the position on the scale, worst first, for the browser to sort on:
// sorted as text, "Critical" would land between "Low" and "Unknown".
func (s Severity) Rank() int {
	for rank, known := range scale {
		if known == s {
			return rank
		}
	}
	return len(scale) - 1
}

// FindingRow is a Finding prepared for rendering.
type FindingRow struct {
	Finding
	// Fix is FixedIn as one cell.
	Fix string
}

// SeverityCount is how many open findings a Report has at one severity.
type SeverityCount struct {
	Severity Severity
	Count    int
}

// Report is one Scan ready to render, for one architecture.
type Report struct {
	// Image is what the reader asked for, e.g. "nginx:latest".
	Image string
	// Digest is what they actually got.
	Digest string
	// Arch is the architecture these Rows describe; Architectures is every
	// one the Index carries, sorted. The same arrangement as the SBOM page,
	// for the same reason.
	Arch          string
	Architectures []string
	Scanner       string
	Database      time.Time
	Finished      time.Time
	Links
	Rows []FindingRow
	// Open counts the Rows that stand; Suppressed the ones a VEX statement
	// set aside.
	Open       int
	Suppressed int
	// Summary counts the open findings by severity, worst first, listing only
	// the severities that occur.
	Summary []SeverityCount
}

// NewReport selects the Findings belonging to arch and orders them as a reader
// wants to meet them: the findings that stand before the suppressed ones, the
// worst severity first, and within a severity the newest identifier first.
//
// arch is resolved as on the SBOM page: empty or absent falls back to amd64,
// then to whatever is there.
func NewReport(image, digest, arch string, scan *Scan) *Report {
	available := architectures(scan.Findings, func(f Finding) string { return f.Arch })
	arch = resolveArch(arch, available)

	rows := make([]FindingRow, 0, len(scan.Findings))
	for _, finding := range scan.Findings {
		if belongsTo(finding.Arch, arch) {
			rows = append(rows, FindingRow{
				Finding: finding,
				Fix:     strings.Join(finding.FixedIn, ", "),
			})
		}
	}
	slices.SortStableFunc(rows, func(a, b FindingRow) int {
		return cmp.Or(
			compareSuppressed(a, b),
			cmp.Compare(a.Severity.Rank(), b.Severity.Rank()),
			cmp.Compare(b.ID, a.ID),
			cmp.Compare(a.Package, b.Package),
		)
	})

	report := &Report{
		Image:         image,
		Digest:        digest,
		Arch:          arch,
		Architectures: available,
		Scanner:       scan.Scanner,
		Database:      scan.Database,
		Finished:      scan.Finished,
		Rows:          rows,
	}
	var counts [len(scale)]int
	for _, row := range rows {
		if row.Suppressed != nil {
			report.Suppressed++
			continue
		}
		report.Open++
		counts[row.Severity.Rank()]++
	}
	for rank, count := range counts {
		if count > 0 {
			report.Summary = append(report.Summary, SeverityCount{Severity: scale[rank], Count: count})
		}
	}
	return report
}

// compareSuppressed orders open findings before suppressed ones.
func compareSuppressed(a, b FindingRow) int {
	switch {
	case a.Suppressed == nil && b.Suppressed != nil:
		return -1
	case a.Suppressed != nil && b.Suppressed == nil:
		return 1
	default:
		return 0
	}
}
