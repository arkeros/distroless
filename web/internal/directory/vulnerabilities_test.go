package directory_test

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/arkeros/distroless/web/internal/directory"
)

// A scan with something in it, dated so the page has dates to show.
func scanned(findings ...directory.Finding) *fakeSource {
	return &fakeSource{digest: testDigest, scan: &directory.Scan{
		Scanner:  "grype 0.118.0",
		Database: time.Date(2026, 9, 3, 0, 34, 4, 0, time.UTC),
		Finished: time.Date(2026, 9, 3, 21, 12, 53, 0, time.UTC),
		Findings: findings,
	}}
}

// The whole table is in the initial response, worst first, with the parts a
// reader acts on: the identifier, what it is matched to, and the fix.
func TestVulnerabilitiesPageRendersEveryFindingBySeverity(t *testing.T) {
	source := scanned(
		directory.Finding{ID: "CVE-2026-0002", Severity: "Low", Package: "zlib1g", Version: "1.3-1", FixedIn: []string{"1.3-2"}},
		directory.Finding{ID: "CVE-2026-0001", Severity: "High", Package: "busybox", Version: "1:1.38.0-3", FixState: "not-fixed"},
	)

	response := get(t, source, "/directory/image/nginx/latest/vulnerabilities", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d:\n%s", response.Code, http.StatusOK, response.Body.String())
	}
	body := response.Body.String()
	high, low := strings.Index(body, "CVE-2026-0001"), strings.Index(body, "CVE-2026-0002")
	if high < 0 || low < 0 {
		t.Fatalf("missing finding rows: high=%d low=%d\n%s", high, low, body)
	}
	if high > low {
		t.Errorf("rows out of severity order: high=%d low=%d", high, low)
	}
	for _, want := range []string{"busybox", "1:1.38.0-3", "zlib1g", "1.3-2", "High", "Low"} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not show %q", want)
		}
	}
	if !strings.Contains(body, "distroless.io/nginx:latest") {
		t.Error("page does not name the image as the reader would pull it")
	}
}

// Severity is sorted on its rank, the way versions are on theirs, so the
// shared script needs no branch for this page either.
func TestVulnerabilitiesPageEmitsSeverityRank(t *testing.T) {
	source := scanned(
		directory.Finding{ID: "a", Severity: "Critical"},
		directory.Finding{ID: "b", Severity: "Unknown"},
	)

	body := get(t, source, "/directory/image/nginx/latest/vulnerabilities", nil).Body.String()

	if !strings.Contains(body, `data-sort-key="0"`) || !strings.Contains(body, `data-sort-key="5"`) {
		t.Errorf("severity ranks not emitted as sort keys:\n%s", body)
	}
}

func TestVulnerabilitiesPageEscapesFindingFields(t *testing.T) {
	source := scanned(directory.Finding{ID: `<script>alert(1)</script>`, Package: "x"})

	body := get(t, source, "/directory/image/nginx/latest/vulnerabilities", nil).Body.String()

	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Errorf("finding rendered unescaped:\n%s", body)
	}
}

// The dates are what make the result mean anything: a scan is only as current
// as the database it ran against.
func TestVulnerabilitiesPageDatesTheScan(t *testing.T) {
	body := get(t, scanned(), "/directory/image/nginx/latest/vulnerabilities", nil).Body.String()

	for _, want := range []string{"2026-09-03", "grype 0.118.0"} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not say %q:\n%s", want, body)
		}
	}
}

// Nothing found is a result, not an absence of one — but only said in those
// words, next to the dates, so it cannot be mistaken for "not scanned".
func TestVulnerabilitiesPageSaysWhenNothingWasFound(t *testing.T) {
	body := get(t, scanned(), "/directory/image/nginx/latest/vulnerabilities", nil).Body.String()

	if !strings.Contains(body, "No findings") {
		t.Errorf("empty scan is not said in words:\n%s", body)
	}
}

// The summary is the sentence a reader takes away: how many, how bad.
func TestVulnerabilitiesPageSummarisesBySeverity(t *testing.T) {
	source := scanned(
		directory.Finding{ID: "a", Severity: "High"},
		directory.Finding{ID: "b", Severity: "High"},
		directory.Finding{ID: "c", Severity: "Negligible"},
	)

	body := get(t, source, "/directory/image/nginx/latest/vulnerabilities", nil).Body.String()

	for _, want := range []string{"2 high", "1 negligible"} {
		if !strings.Contains(body, want) {
			t.Errorf("summary does not say %q:\n%s", want, body)
		}
	}
}

// A suppressed finding stays on the page, marked, with the reason the VEX
// statement gave: silencing on the record is the whole point of one.
func TestVulnerabilitiesPageMarksSuppressedFindings(t *testing.T) {
	source := scanned(directory.Finding{
		ID: "CVE-2013-0337", Severity: "Medium", Package: "nginx",
		Suppressed: &directory.Suppression{Status: "not_affected", Justification: "vulnerable_code_not_in_execute_path"},
	})

	body := get(t, source, "/directory/image/nginx/latest/vulnerabilities", nil).Body.String()

	if !strings.Contains(body, "CVE-2013-0337") {
		t.Fatal("suppressed finding dropped from the page")
	}
	if !strings.Contains(body, `class="suppressed"`) {
		t.Errorf("suppressed finding not marked:\n%s", body)
	}
	if !strings.Contains(body, "vulnerable code not in execute path") {
		t.Errorf("justification not shown in words:\n%s", body)
	}
}

// The identifier is a way in: it links where the scanner read it.
func TestVulnerabilitiesPageLinksEachFindingToItsSource(t *testing.T) {
	source := scanned(directory.Finding{ID: "CVE-2026-0001", URL: "https://security-tracker.debian.org/tracker/CVE-2026-0001"})

	body := get(t, source, "/directory/image/nginx/latest/vulnerabilities", nil).Body.String()

	if !strings.Contains(body, `href="https://security-tracker.debian.org/tracker/CVE-2026-0001"`) {
		t.Errorf("finding not linked to its source:\n%s", body)
	}
}

// An unscanned image is the normal state of a build published before scans
// were attached, and the page has to say so rather than show an empty table —
// an empty table would read as a clean result.
func TestVulnerabilitiesPageReportsAnUnscannedImage(t *testing.T) {
	source := &fakeSource{digest: testDigest, scanErr: errors.New("no verified vulnerability attestation")}

	response := get(t, source, "/directory/image/nginx/latest/vulnerabilities", nil)

	if response.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if !strings.Contains(response.Body.String(), "no vulnerability scan published") {
		t.Errorf("body = %q, want it to say no scan is published", response.Body.String())
	}
}

func TestVulnerabilitiesPageServesTheRequestedArchitecture(t *testing.T) {
	source := scanned(
		directory.Finding{ID: "CVE-2026-0001", Package: "busybox", Arch: "amd64"},
		directory.Finding{ID: "CVE-2026-0001", Package: "busybox", Arch: "arm64"},
	)

	body := get(t, source, "/directory/image/nginx/latest/vulnerabilities?arch=arm64", nil).Body.String()

	if got := strings.Count(body, "CVE-2026-0001"); got != 1 {
		t.Errorf("CVE-2026-0001 appears %d times, want once", got)
	}
}

// Immutable for a Digest and an architecture, like the SBOM page.
func TestVulnerabilitiesPageRevalidatesOnDigestAndArchitecture(t *testing.T) {
	source := scanned(
		directory.Finding{ID: "a", Arch: "amd64"},
		directory.Finding{ID: "a", Arch: "arm64"},
	)

	first := get(t, source, "/directory/image/nginx/latest/vulnerabilities?arch=arm64", nil)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the first response")
	}
	if other := get(t, source, "/directory/image/nginx/latest/vulnerabilities?arch=amd64", nil).Header().Get("ETag"); other == etag {
		t.Errorf("both architectures share ETag %s", etag)
	}

	second := get(t, source, "/directory/image/nginx/latest/vulnerabilities?arch=arm64", http.Header{"If-None-Match": {etag}})
	if second.Code != http.StatusNotModified || second.Body.Len() != 0 {
		t.Errorf("status = %d with %d bytes, want %d and no body", second.Code, second.Body.Len(), http.StatusNotModified)
	}
}

// A digest can be re-scanned, and then the same URL shows a different page. The
// digest alone would let a cache hold the old one forever.
func TestVulnerabilitiesETagChangesWithTheScan(t *testing.T) {
	source := scanned(directory.Finding{ID: "a"})
	first := get(t, source, "/directory/image/nginx/latest/vulnerabilities", nil).Header().Get("ETag")

	source.scan.Finished = source.scan.Finished.Add(24 * time.Hour)
	second := get(t, source, "/directory/image/nginx/latest/vulnerabilities", nil).Header().Get("ETag")

	if first == second {
		t.Errorf("ETag %s unchanged by a newer scan", first)
	}
}

// So can the VEX document: a statement withdrawn or added changes what the page
// says about a finding, with the digest and the scan untouched.
func TestVulnerabilitiesETagChangesWithTheSuppressions(t *testing.T) {
	source := scanned(directory.Finding{ID: "CVE-2013-0337", Severity: "Medium"})
	first := get(t, source, "/directory/image/nginx/latest/vulnerabilities", nil).Header().Get("ETag")

	source.scan.Findings[0].Suppressed = &directory.Suppression{Status: "not_affected", Justification: "vulnerable_code_not_in_execute_path"}
	second := get(t, source, "/directory/image/nginx/latest/vulnerabilities", nil).Header().Get("ETag")

	if first == second {
		t.Errorf("ETag %s unchanged by a VEX statement covering a finding", first)
	}
}

func TestVulnerabilitiesCacheControlLengthensAtADigest(t *testing.T) {
	source := scanned(directory.Finding{ID: "a"})
	source.scanDocument = []byte("{}")

	for target, want := range map[string]string{
		"/directory/image/nginx/latest/vulnerabilities":                  "public, max-age=300",
		"/directory/image/nginx/latest/vulnerabilities.json":             "public, max-age=300",
		"/directory/image/nginx/" + testDigest + "/vulnerabilities":      "public, max-age=86400",
		"/directory/image/nginx/" + testDigest + "/vulnerabilities.json": "public, max-age=31536000, immutable",
	} {
		if got := get(t, source, target, nil).Header().Get("Cache-Control"); got != want {
			t.Errorf("GET %s Cache-Control = %q, want %q", target, got, want)
		}
	}
}

// One build, two views of it. Each has to reach the other for the same
// reference, so a reader switching does not lose the build they were on.
func TestSBOMAndVulnerabilitiesPagesLinkEachOther(t *testing.T) {
	source := scanned(directory.Finding{ID: "a"})
	source.components = []directory.Component{{Name: "libc6"}}

	sbom := get(t, source, "/directory/image/nginx/1.27/sbom?arch=arm64", nil).Body.String()
	if !strings.Contains(sbom, `href="/directory/image/nginx/1.27/vulnerabilities?arch=arm64"`) {
		t.Errorf("SBOM page does not link to the vulnerabilities of the same build and architecture:\n%s", sbom)
	}

	vulnerabilities := get(t, source, "/directory/image/nginx/1.27/vulnerabilities?arch=arm64", nil).Body.String()
	if !strings.Contains(vulnerabilities, `href="/directory/image/nginx/1.27/sbom?arch=arm64"`) {
		t.Errorf("vulnerabilities page does not link to the SBOM of the same build and architecture:\n%s", vulnerabilities)
	}
}

// Every build on the versions list has two kinds of evidence, and the list
// should offer both.
func TestVersionsPageLinksEachBuildToItsVulnerabilities(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{{Tag: "latest", Digest: testDigest}}}

	body := get(t, source, "/directory/image/nginx/versions", nil).Body.String()

	if !strings.Contains(body, `href="/directory/image/nginx/`+testDigest+`/vulnerabilities"`) {
		t.Errorf("versions page does not link a build to its vulnerabilities:\n%s", body)
	}
}

func TestVulnerabilitiesPageLinksTheDigestAsAPermalink(t *testing.T) {
	source := scanned(directory.Finding{ID: "a"})

	body := get(t, source, "/directory/image/nginx/latest/vulnerabilities", nil).Body.String()

	if !strings.Contains(body, `href="/directory/image/nginx/`+testDigest+`/vulnerabilities"`) {
		t.Errorf("page does not link the digest as a permalink:\n%s", body)
	}
}

func TestRefLessVulnerabilitiesURLRedirectsToLatest(t *testing.T) {
	source := scanned(directory.Finding{ID: "a"})

	for from, want := range map[string]string{
		"/directory/image/nginx/vulnerabilities":      "/directory/image/nginx/latest/vulnerabilities",
		"/directory/image/nginx/vulnerabilities.json": "/directory/image/nginx/latest/vulnerabilities.json",
	} {
		response := get(t, source, from, nil)
		if response.Code != http.StatusMovedPermanently || response.Header().Get("Location") != want {
			t.Errorf("GET %s = %d to %q, want %d to %q", from, response.Code, response.Header().Get("Location"), http.StatusMovedPermanently, want)
		}
	}
}

func TestVulnerabilitiesRejectsAMalformedReference(t *testing.T) {
	source := scanned(directory.Finding{ID: "a"})

	for _, resource := range []string{"vulnerabilities", "vulnerabilities.json"} {
		target := "/directory/image/nginx/bad!!/" + resource
		if code := get(t, source, target, nil).Code; code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want %d", target, code, http.StatusBadRequest)
		}
	}
	if source.family != "" {
		t.Error("malformed reference reached the source")
	}
}

// The download is the signed predicate itself — the scanner's report inside
// the envelope our workflow signed — not the projection on the page.
func TestScanDownloadServesTheDocumentUnaltered(t *testing.T) {
	document := []byte(`{"scanner":{"uri":"pkg:github/anchore/grype@v0.118.0"},"metadata":{}}`)
	source := &fakeSource{digest: testDigest, scanDocument: document}

	response := get(t, source, "/directory/image/nginx/1.27/vulnerabilities.json", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !bytes.Equal(response.Body.Bytes(), document) {
		t.Errorf("body = %s, want %s", response.Body.Bytes(), document)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got, want := response.Header().Get("Content-Disposition"), `attachment; filename="nginx-1.27-0da1844626f2.vuln.json"`; got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}
}

// The record behind the download can be replaced by a re-scan without the
// digest changing, so the digest alone cannot be its validator.
func TestScanDownloadETagChangesWithTheDocument(t *testing.T) {
	source := &fakeSource{digest: testDigest, scanDocument: []byte(`{"metadata":{"scanFinishedOn":"2026-09-03T00:00:00Z"}}`)}
	first := get(t, source, "/directory/image/nginx/latest/vulnerabilities.json", nil).Header().Get("ETag")

	source.scanDocument = []byte(`{"metadata":{"scanFinishedOn":"2026-09-10T00:00:00Z"}}`)
	second := get(t, source, "/directory/image/nginx/latest/vulnerabilities.json", nil).Header().Get("ETag")

	if first == second {
		t.Errorf("ETag %s unchanged by a newer record", first)
	}
}

func TestScanDownloadReportsAnUnscannedImage(t *testing.T) {
	source := &fakeSource{digest: testDigest, scanErr: errors.New("no verified vulnerability attestation")}

	if code := get(t, source, "/directory/image/nginx/latest/vulnerabilities.json", nil).Code; code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", code, http.StatusNotFound)
	}
}

// The page has to offer the download for it to exist as far as a reader is
// concerned.
func TestVulnerabilitiesPageOffersTheDownload(t *testing.T) {
	source := scanned(directory.Finding{ID: "a"})

	body := get(t, source, "/directory/image/nginx/1.27/vulnerabilities", nil).Body.String()

	if !strings.Contains(body, `href="/directory/image/nginx/1.27/vulnerabilities.json"`) {
		t.Errorf("page does not link to the download:\n%s", body)
	}
}
