package directory_test

import (
	"bytes"
	"context"
	"errors"
	"mime"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/arkeros/distroless/web/internal/directory"
)

type fakeSource struct {
	digest     string
	components []directory.Component
	document   []byte
	err        error
}

func (f fakeSource) SBOM(_ context.Context, _ string) (string, []directory.Component, error) {
	return f.digest, f.components, f.err
}

func (f fakeSource) Document(_ context.Context, _ string) (string, []byte, error) {
	return f.digest, f.document, f.err
}

func get(t *testing.T, source directory.Source, target string, headers http.Header) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	for name, values := range headers {
		request.Header[name] = values
	}
	recorder := httptest.NewRecorder()
	directory.NewHandler(source, "distroless.io").ServeHTTP(recorder, request)
	return recorder
}

// The whole table is in the initial response: no JS has run, and none is
// needed to read the page or to index it.
func TestPageRendersEveryComponentInNameOrder(t *testing.T) {
	source := fakeSource{digest: "sha256:abc", components: []directory.Component{
		{Name: "zlib1g", Version: "1.3", License: "Zlib", Type: "deb"},
		{Name: "libc6", Version: "2.36", License: "LGPL-2.1", Type: "deb"},
		{Name: "openssl", Version: "3.0.11", License: "Apache-2.0", Type: "deb"},
	}}

	body := get(t, source, "/directory/image/java/sbom", nil).Body.String()

	libc, openssl, zlib := strings.Index(body, "libc6"), strings.Index(body, "openssl"), strings.Index(body, "zlib1g")
	if libc < 0 || openssl < 0 || zlib < 0 {
		t.Fatalf("missing component rows: libc6=%d openssl=%d zlib1g=%d", libc, openssl, zlib)
	}
	if !(libc < openssl && openssl < zlib) {
		t.Errorf("rows out of name order: libc6=%d openssl=%d zlib1g=%d", libc, openssl, zlib)
	}
	if !strings.Contains(body, "LGPL-2.1") {
		t.Error("license column not rendered")
	}
}

// The browser sorts the version column on this attribute.
func TestPageEmitsVersionRank(t *testing.T) {
	source := fakeSource{digest: "sha256:abc", components: []directory.Component{
		{Name: "a", Version: "1.10"},
		{Name: "b", Version: "1.9"},
	}}

	body := get(t, source, "/directory/image/java/sbom", nil).Body.String()

	if !strings.Contains(body, `data-version-rank="0"`) || !strings.Contains(body, `data-version-rank="1"`) {
		t.Errorf("version ranks not emitted as data attributes:\n%s", body)
	}
}

func TestPageEscapesComponentFields(t *testing.T) {
	source := fakeSource{digest: "sha256:abc", components: []directory.Component{
		{Name: `<script>alert(1)</script>`, Version: "1.0"},
	}}

	body := get(t, source, "/directory/image/java/sbom", nil).Body.String()

	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Errorf("component name rendered unescaped:\n%s", body)
	}
}

// An SBOM is immutable for a Digest, so a revalidation should cost no body.
func TestPageRevalidatesOnDigest(t *testing.T) {
	source := fakeSource{digest: "sha256:abc", components: []directory.Component{{Name: "libc6", Version: "2.36"}}}

	first := get(t, source, "/directory/image/java/sbom", nil)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the first response")
	}

	second := get(t, source, "/directory/image/java/sbom", http.Header{"If-None-Match": {etag}})
	if second.Code != http.StatusNotModified {
		t.Errorf("status = %d, want %d", second.Code, http.StatusNotModified)
	}
	if second.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", second.Body.String())
	}
}

func TestPageReportsUnknownImage(t *testing.T) {
	source := fakeSource{err: errors.New("manifest unknown")}

	if code := get(t, source, "/directory/image/nope/sbom", nil).Code; code == http.StatusOK {
		t.Errorf("status = %d, want an error status", code)
	}
}

func TestPageServesTheRequestedArchitecture(t *testing.T) {
	source := fakeSource{digest: "sha256:abc", components: []directory.Component{
		{Name: "libc6", Version: "2.43-4", Arch: "amd64"},
		{Name: "libc6", Version: "2.43-4", Arch: "arm64"},
	}}

	body := get(t, source, "/directory/image/nginx/sbom?arch=arm64", nil).Body.String()

	if strings.Count(body, "libc6") != 1 {
		t.Errorf("libc6 appears %d times, want once", strings.Count(body, "libc6"))
	}
}

// Two architectures are two documents. Sharing an ETag would let a cache serve
// one for the other.
func TestPageVariesETagByArchitecture(t *testing.T) {
	source := fakeSource{digest: "sha256:abc", components: []directory.Component{
		{Name: "libc6", Arch: "amd64"},
		{Name: "libc6", Arch: "arm64"},
	}}

	amd64 := get(t, source, "/directory/image/nginx/sbom?arch=amd64", nil).Header().Get("ETag")
	arm64 := get(t, source, "/directory/image/nginx/sbom?arch=arm64", nil).Header().Get("ETag")

	if amd64 == arm64 {
		t.Errorf("both architectures share ETag %s", amd64)
	}
}

// The reader should see the name they would pull by, not the family alone and
// not the GHCR path the bytes actually came from.
func TestPageTitlesTheImageByItsMirrorName(t *testing.T) {
	source := fakeSource{digest: "sha256:abc", components: []directory.Component{{Name: "libc6"}}}

	body := get(t, source, "/directory/image/nginx/sbom", nil).Body.String()

	if !strings.Contains(body, "distroless.io/nginx:latest") {
		t.Errorf("page does not name the image as distroless.io/nginx:latest")
	}
}

// A font the stylesheet asks for but the binary does not embed is a 404 the
// reader sees as the fallback face, so every url() must resolve.
func TestStaticAssetsResolveEveryStylesheetReference(t *testing.T) {
	source := fakeSource{digest: "sha256:abc"}

	css := get(t, source, "/directory/static/sbom.css", nil)
	if css.Code != http.StatusOK {
		t.Fatalf("stylesheet status = %d, want %d", css.Code, http.StatusOK)
	}

	references := regexp.MustCompile(`url\(([^)]*)\)`).FindAllStringSubmatch(css.Body.String(), -1)
	if len(references) == 0 {
		t.Fatal("stylesheet references no assets")
	}
	for _, reference := range references {
		asset := strings.Trim(reference[1], `'"`)
		if code := get(t, source, asset, nil).Code; code != http.StatusOK {
			t.Errorf("GET %s = %d, want %d", asset, code, http.StatusOK)
		}
	}
}

// A distroless image has no /etc/mime.types, and Go's built-in table has no
// woff2 entry, so a handler that leaves the type to mime.TypeByExtension
// serves the fonts as application/octet-stream there while looking correct on
// a developer machine. Standing in a wrong answer is how that host-dependence
// shows up as a failure here rather than only in production.
func TestStaticFontsAreServedAsWoff2(t *testing.T) {
	mime.AddExtensionType(".woff2", "application/x-not-a-font")

	response := get(t, fakeSource{}, "/directory/static/fonts/fira-sans-400-latin.woff2", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "font/woff2" {
		t.Errorf("Content-Type = %q, want %q", got, "font/woff2")
	}
}

// The download is the attestation's own bytes. Re-serialising it — to drop the
// architectures the page is not showing, say — would hand the reader a
// document nobody signed.
func TestDownloadServesTheDocumentUnaltered(t *testing.T) {
	document := []byte(`{"bomFormat":"CycloneDX","specVersion":"1.5"}`)
	source := fakeSource{digest: "sha256:abc", document: document}

	response := get(t, source, "/directory/image/nginx/sbom.json", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Body.Bytes(); !bytes.Equal(got, document) {
		t.Errorf("body = %s, want %s", got, document)
	}
	if got := response.Header().Get("Content-Type"); got != "application/vnd.cyclonedx+json" {
		t.Errorf("Content-Type = %q, want the CycloneDX media type", got)
	}
}

// A reader who clicks download should get a file named after what they were
// looking at, not "sbom.json" among ten other sbom.json.
func TestDownloadNamesTheFileAfterTheImage(t *testing.T) {
	source := fakeSource{digest: "sha256:0da1844626f2a1628c878b60fdf8b491e", document: []byte("{}")}

	disposition := get(t, source, "/directory/image/nginx/sbom.json?tag=1.27", nil).Header().Get("Content-Disposition")

	for _, want := range []string{"attachment", "nginx", "1.27", "0da1844626f2", ".cdx.json"} {
		if !strings.Contains(disposition, want) {
			t.Errorf("Content-Disposition = %q, want it to contain %q", disposition, want)
		}
	}
}

// A tag is reader-supplied and lands in a response header, so it cannot be
// allowed to carry quotes or newlines into one.
func TestDownloadSanitisesTheFilename(t *testing.T) {
	source := fakeSource{digest: "sha256:abc", document: []byte("{}")}

	disposition := get(t, source, `/directory/image/nginx/sbom.json?tag=a"b%0d%0aX-Evil:+1`, nil).Header().Get("Content-Disposition")

	const prefix = `attachment; filename="`
	if !strings.HasPrefix(disposition, prefix) || !strings.HasSuffix(disposition, `"`) {
		t.Fatalf("Content-Disposition = %q, want a quoted attachment filename", disposition)
	}
	// The quotes delimiting the filename are the header's own; what must not
	// survive is any of that syntax arriving from the tag.
	filename := strings.TrimSuffix(strings.TrimPrefix(disposition, prefix), `"`)
	if strings.ContainsAny(filename, "\"\r\n;") {
		t.Errorf("filename %q carries header syntax out of the tag", filename)
	}
}

// The document is immutable for a Digest, so revalidation should cost no body.
func TestDownloadRevalidatesOnDigest(t *testing.T) {
	source := fakeSource{digest: "sha256:abc", document: []byte("{}")}

	first := get(t, source, "/directory/image/nginx/sbom.json", nil)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the first response")
	}

	second := get(t, source, "/directory/image/nginx/sbom.json", http.Header{"If-None-Match": {etag}})
	if second.Code != http.StatusNotModified {
		t.Errorf("status = %d, want %d", second.Code, http.StatusNotModified)
	}
}

func TestDownloadReportsUnknownImage(t *testing.T) {
	source := fakeSource{err: errors.New("manifest unknown")}

	if code := get(t, source, "/directory/image/nope/sbom.json", nil).Code; code == http.StatusOK {
		t.Errorf("status = %d, want an error status", code)
	}
}

// The page has to offer the download for it to exist as far as a reader is
// concerned.
func TestPageOffersTheDownload(t *testing.T) {
	source := fakeSource{digest: "sha256:abc", components: []directory.Component{{Name: "libc6"}}}

	body := get(t, source, "/directory/image/nginx/sbom?tag=1.27", nil).Body.String()

	if !strings.Contains(body, "/directory/image/nginx/sbom.json?tag=1.27") {
		t.Errorf("page does not link to the download:\n%s", body)
	}
}

// Four columns of package metadata are wider than a phone. The table scrolls
// inside its own box; without the wrapper the whole document scrolls sideways
// and the heading, filter and count leave the screen. Only the structure is
// checked here — that the box actually scrolls is a browser's judgement.
func TestPageWrapsTheTableInAScrollContainer(t *testing.T) {
	source := fakeSource{digest: "sha256:abc", components: []directory.Component{{Name: "libc6"}}}

	body := get(t, source, "/directory/image/nginx/sbom", nil).Body.String()

	scroller := strings.Index(body, `class="scroller"`)
	table := strings.Index(body, `<table id="sbom"`)
	if scroller < 0 || table < 0 || scroller > table {
		t.Errorf("table is not inside a scroll container: scroller=%d table=%d", scroller, table)
	}
}
