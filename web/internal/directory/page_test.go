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
	"time"

	"github.com/arkeros/distroless/web/internal/directory"
)

// A digest as the registry spells it. The tests need a well-formed one because
// the handler now validates the reference before it reaches the Source, and
// because it is what the permalink is built from.
var testDigest = "sha256:0da1844626f2a1628c878b60fdf8b491e" + strings.Repeat("0", 31)

// A second, unmistakably different build.
var otherDigest = "sha256:" + strings.Repeat("b", 64)

type fakeSource struct {
	digest     string
	components []directory.Component
	document   []byte
	versions   []directory.Version
	tags       []string
	tagsErr    error
	err        error

	// family and ref record what the handler asked for, so a test can check
	// the handler passes the path through rather than re-deriving it.
	family, ref string
}

func (f *fakeSource) SBOM(_ context.Context, family, ref string) (string, []directory.Component, error) {
	f.family, f.ref = family, ref
	return f.digest, f.components, f.err
}

func (f *fakeSource) Document(_ context.Context, family, ref string) (string, []byte, error) {
	f.family, f.ref = family, ref
	return f.digest, f.document, f.err
}

func (f *fakeSource) Versions(_ context.Context, family string) ([]directory.Version, error) {
	f.family = family
	return f.versions, f.err
}

func (f *fakeSource) Tags(_ context.Context, family string) ([]string, error) {
	f.family = family
	return f.tags, f.tagsErr
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
	source := &fakeSource{digest: testDigest, components: []directory.Component{
		{Name: "zlib1g", Version: "1.3", License: "Zlib", Type: "deb"},
		{Name: "libc6", Version: "2.36", License: "LGPL-2.1", Type: "deb"},
		{Name: "openssl", Version: "3.0.11", License: "Apache-2.0", Type: "deb"},
	}}

	body := get(t, source, "/directory/image/java/latest/sbom", nil).Body.String()

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

// dpkg ordering is the server's to compute; the browser sorts on the result.
// It rides the same per-cell mechanism the versions page's size column uses,
// so the shared script needs no branch for either.
func TestPageEmitsVersionRank(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{
		{Name: "a", Version: "1.10"},
		{Name: "b", Version: "1.9"},
	}}

	body := get(t, source, "/directory/image/java/latest/sbom", nil).Body.String()

	if !strings.Contains(body, `data-sort-key="0"`) || !strings.Contains(body, `data-sort-key="1"`) {
		t.Errorf("version ranks not emitted as sort keys:\n%s", body)
	}
	if strings.Contains(body, "data-version-rank") {
		t.Errorf("version rank still rides its own attribute:\n%s", body)
	}
}

func TestPageEscapesComponentFields(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{
		{Name: `<script>alert(1)</script>`, Version: "1.0"},
	}}

	body := get(t, source, "/directory/image/java/latest/sbom", nil).Body.String()

	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Errorf("component name rendered unescaped:\n%s", body)
	}
}

// An SBOM is immutable for a Digest, so a revalidation should cost no body.
func TestPageRevalidatesOnDigest(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6", Version: "2.36"}}}

	first := get(t, source, "/directory/image/java/latest/sbom", nil)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the first response")
	}

	second := get(t, source, "/directory/image/java/latest/sbom", http.Header{"If-None-Match": {etag}})
	if second.Code != http.StatusNotModified {
		t.Errorf("status = %d, want %d", second.Code, http.StatusNotModified)
	}
	if second.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", second.Body.String())
	}
}

func TestPageReportsUnknownImage(t *testing.T) {
	source := &fakeSource{err: errors.New("manifest unknown")}

	if code := get(t, source, "/directory/image/nope/latest/sbom", nil).Code; code == http.StatusOK {
		t.Errorf("status = %d, want an error status", code)
	}
}

func TestPageServesTheRequestedArchitecture(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{
		{Name: "libc6", Version: "2.43-4", Arch: "amd64"},
		{Name: "libc6", Version: "2.43-4", Arch: "arm64"},
	}}

	body := get(t, source, "/directory/image/nginx/latest/sbom?arch=arm64", nil).Body.String()

	if strings.Count(body, "libc6") != 1 {
		t.Errorf("libc6 appears %d times, want once", strings.Count(body, "libc6"))
	}
}

// Two architectures are two documents. Sharing an ETag would let a cache serve
// one for the other.
func TestPageVariesETagByArchitecture(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{
		{Name: "libc6", Arch: "amd64"},
		{Name: "libc6", Arch: "arm64"},
	}}

	amd64 := get(t, source, "/directory/image/nginx/latest/sbom?arch=amd64", nil).Header().Get("ETag")
	arm64 := get(t, source, "/directory/image/nginx/latest/sbom?arch=arm64", nil).Header().Get("ETag")

	if amd64 == arm64 {
		t.Errorf("both architectures share ETag %s", amd64)
	}
}

// The reader should see the name they would pull by, not the family alone and
// not the GHCR path the bytes actually came from.
func TestPageTitlesTheImageByItsMirrorName(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6"}}}

	body := get(t, source, "/directory/image/nginx/latest/sbom", nil).Body.String()

	if !strings.Contains(body, "distroless.io/nginx:latest") {
		t.Errorf("page does not name the image as distroless.io/nginx:latest")
	}
}

// A reference is either a tag or a digest, and the two are not joined to the
// family the same way. Rendering `nginx:sha256:...` would name an image nobody
// can pull.
func TestPageNamesADigestReferenceWithAnAt(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6"}}}

	body := get(t, source, "/directory/image/nginx/"+testDigest+"/sbom", nil).Body.String()

	if !strings.Contains(body, "distroless.io/nginx@"+testDigest) {
		t.Errorf("digest reference not named as distroless.io/nginx@%s:\n%s", testDigest, body)
	}
}

// The reference is the Source's to interpret — the handler must hand over the
// family and the reference it was asked for, not a string it pre-joined.
func TestPagePassesTheReferenceToTheSource(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6"}}}

	get(t, source, "/directory/image/nginx/1.27/sbom", nil)

	if source.family != "nginx" || source.ref != "1.27" {
		t.Errorf("source asked for (%q, %q), want (%q, %q)", source.family, source.ref, "nginx", "1.27")
	}
}

// The short form stays typeable, but there is one canonical URL per page.
func TestRefLessURLRedirectsToLatest(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6"}}}

	for from, want := range map[string]string{
		"/directory/image/nginx/sbom":            "/directory/image/nginx/latest/sbom",
		"/directory/image/nginx/sbom.json":       "/directory/image/nginx/latest/sbom.json",
		"/directory/image/nginx/sbom?arch=arm64": "/directory/image/nginx/latest/sbom?arch=arm64",
	} {
		response := get(t, source, from, nil)

		if response.Code != http.StatusMovedPermanently {
			t.Errorf("GET %s = %d, want %d", from, response.Code, http.StatusMovedPermanently)
		}
		if got := response.Header().Get("Location"); got != want {
			t.Errorf("GET %s redirected to %q, want %q", from, got, want)
		}
		// A bare 301 is cached by browsers heuristically and forever. The
		// mapping is permanent, but it must stay retractable.
		if got := response.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=300") {
			t.Errorf("GET %s Cache-Control = %q, want a bounded max-age", from, got)
		}
	}
}

// The reference is reader-supplied and reaches both a registry reference and a
// response header. Syntactic nonsense should not cost a registry round trip.
func TestRejectsAMalformedReference(t *testing.T) {
	for _, ref := range []string{
		"bad!!",                             // outside the OCI tag charset
		".leading-dot",                      // a tag may not start with a period
		"sha256:xyz",                        // digest-shaped, not hex
		"sha256:" + strings.Repeat("a", 63), // one hex digit short
		"sha512:" + strings.Repeat("a", 64), // not the algorithm we publish
		strings.Repeat("a", 129),            // longer than a tag may be
	} {
		source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6"}}}

		for _, resource := range []string{"sbom", "sbom.json"} {
			target := "/directory/image/nginx/" + ref + "/" + resource
			if code := get(t, source, target, nil).Code; code != http.StatusBadRequest {
				t.Errorf("GET %s = %d, want %d", target, code, http.StatusBadRequest)
			}
		}
		if source.family != "" {
			t.Errorf("malformed reference %q reached the source", ref)
		}
	}
}

// The reference lives in the path now. A leftover ?tag= is somebody's stale
// bookmark and must not quietly override it.
func TestTagQueryParameterIsIgnored(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6"}}}

	body := get(t, source, "/directory/image/nginx/1.27/sbom?tag=9.9", nil).Body.String()

	if source.ref != "1.27" {
		t.Errorf("source asked for ref %q, want %q", source.ref, "1.27")
	}
	if !strings.Contains(body, "distroless.io/nginx:1.27") {
		t.Errorf("page does not name the image as distroless.io/nginx:1.27")
	}
}

// A page at a digest is immutable data, but its rendering is not: the template
// changes on deploy, and a digest URL has no cache-busting lever. So it is
// long-lived, not immutable. The document behind it has no presentation and
// can be cached forever.
func TestCacheControlLengthensAtADigest(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6"}}, document: []byte("{}")}

	for target, want := range map[string]string{
		"/directory/image/nginx/latest/sbom":                  "public, max-age=300",
		"/directory/image/nginx/latest/sbom.json":             "public, max-age=300",
		"/directory/image/nginx/" + testDigest + "/sbom":      "public, max-age=86400",
		"/directory/image/nginx/" + testDigest + "/sbom.json": "public, max-age=31536000, immutable",
	} {
		if got := get(t, source, target, nil).Header().Get("Cache-Control"); got != want {
			t.Errorf("GET %s Cache-Control = %q, want %q", target, got, want)
		}
	}
}

// The permalink is the whole point of putting the reference in the path made
// visible: one click turns "nginx, whatever that is today" into this exact
// build.
func TestPageLinksTheDigestAsAPermalink(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6"}}}

	body := get(t, source, "/directory/image/nginx/latest/sbom", nil).Body.String()

	if !strings.Contains(body, `href="/directory/image/nginx/`+testDigest+`/sbom"`) {
		t.Errorf("page does not link the digest as a permalink:\n%s", body)
	}
}

// A permalink that silently changes what you are looking at is a worse
// permalink.
func TestPermalinkKeepsTheArchitecture(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{
		{Name: "libc6", Arch: "amd64"},
		{Name: "libc6", Arch: "arm64"},
	}}

	body := get(t, source, "/directory/image/nginx/latest/sbom?arch=arm64", nil).Body.String()

	if !strings.Contains(body, `href="/directory/image/nginx/`+testDigest+`/sbom?arch=arm64"`) {
		t.Errorf("permalink drops the architecture:\n%s", body)
	}
}

// Already at the permalink, so there is nowhere for it to go.
func TestDigestPageDoesNotLinkToItself(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6"}}}

	body := get(t, source, "/directory/image/nginx/"+testDigest+"/sbom", nil).Body.String()

	if strings.Contains(body, `href="/directory/image/nginx/`+testDigest+`/sbom"`) {
		t.Errorf("digest page links to itself:\n%s", body)
	}
	if !strings.Contains(body, testDigest) {
		t.Errorf("digest page does not show its digest:\n%s", body)
	}
}

// A font the stylesheet asks for but the binary does not embed is a 404 the
// reader sees as the fallback face, so every url() must resolve.
func TestStaticAssetsResolveEveryStylesheetReference(t *testing.T) {
	source := &fakeSource{digest: testDigest}

	css := get(t, source, "/directory/static/directory.css", nil)
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

	response := get(t, &fakeSource{}, "/directory/static/fonts/fira-sans-400-latin.woff2", nil)

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
	source := &fakeSource{digest: testDigest, document: document}

	response := get(t, source, "/directory/image/nginx/latest/sbom.json", nil)

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
	source := &fakeSource{digest: testDigest, document: []byte("{}")}

	disposition := get(t, source, "/directory/image/nginx/1.27/sbom.json", nil).Header().Get("Content-Disposition")

	for _, want := range []string{"attachment", "nginx", "1.27", "0da1844626f2", ".cdx.json"} {
		if !strings.Contains(disposition, want) {
			t.Errorf("Content-Disposition = %q, want it to contain %q", disposition, want)
		}
	}
}

// At a digest the reference already is the digest, and repeating it — once
// mangled by the filename sanitiser, once shortened — names nothing extra.
func TestDownloadDoesNotRepeatTheDigestInTheFilename(t *testing.T) {
	source := &fakeSource{digest: testDigest, document: []byte("{}")}

	disposition := get(t, source, "/directory/image/nginx/"+testDigest+"/sbom.json", nil).Header().Get("Content-Disposition")

	if want := `attachment; filename="nginx-0da1844626f2.cdx.json"`; disposition != want {
		t.Errorf("Content-Disposition = %q, want %q", disposition, want)
	}
}

// The family is reader-supplied and lands in a response header, so it cannot
// be allowed to carry quotes or newlines into one.
func TestDownloadSanitisesTheFilename(t *testing.T) {
	source := &fakeSource{digest: testDigest, document: []byte("{}")}

	disposition := get(t, source, `/directory/image/a%22b%0d%0aX-Evil:+1/latest/sbom.json`, nil).Header().Get("Content-Disposition")

	const prefix = `attachment; filename="`
	if !strings.HasPrefix(disposition, prefix) || !strings.HasSuffix(disposition, `"`) {
		t.Fatalf("Content-Disposition = %q, want a quoted attachment filename", disposition)
	}
	// The quotes delimiting the filename are the header's own; what must not
	// survive is any of that syntax arriving from the path.
	filename := strings.TrimSuffix(strings.TrimPrefix(disposition, prefix), `"`)
	if strings.ContainsAny(filename, "\"\r\n;") {
		t.Errorf("filename %q carries header syntax out of the path", filename)
	}
}

// The document is immutable for a Digest, so revalidation should cost no body.
func TestDownloadRevalidatesOnDigest(t *testing.T) {
	source := &fakeSource{digest: testDigest, document: []byte("{}")}

	first := get(t, source, "/directory/image/nginx/latest/sbom.json", nil)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the first response")
	}

	second := get(t, source, "/directory/image/nginx/latest/sbom.json", http.Header{"If-None-Match": {etag}})
	if second.Code != http.StatusNotModified {
		t.Errorf("status = %d, want %d", second.Code, http.StatusNotModified)
	}
}

func TestDownloadReportsUnknownImage(t *testing.T) {
	source := &fakeSource{err: errors.New("manifest unknown")}

	if code := get(t, source, "/directory/image/nope/latest/sbom.json", nil).Code; code == http.StatusOK {
		t.Errorf("status = %d, want an error status", code)
	}
}

// The page has to offer the download for it to exist as far as a reader is
// concerned, and it must offer the one for the reference being shown.
func TestPageOffersTheDownload(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6"}}}

	body := get(t, source, "/directory/image/nginx/1.27/sbom", nil).Body.String()

	if !strings.Contains(body, "/directory/image/nginx/1.27/sbom.json") {
		t.Errorf("page does not link to the download:\n%s", body)
	}
}

// A family and a reference name one build. That is where a page describing
// the build itself belongs — its entrypoint, user, labels — so it stays
// reserved rather than answering with the versions list, which is about every
// build at once.
func TestImageWithAReferenceButNoResourceIsNotFound(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6"}}}

	target := "/directory/image/nginx/latest"
	if code := get(t, source, target, nil).Code; code != http.StatusNotFound {
		t.Errorf("GET %s = %d, want %d", target, code, http.StatusNotFound)
	}
}

// Four columns of package metadata are wider than a phone. The table scrolls
// inside its own box; without the wrapper the whole document scrolls sideways
// and the heading, filter and count leave the screen. Only the structure is
// checked here — that the box actually scrolls is a browser's judgement.
func TestPageWrapsTheTableInAScrollContainer(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6"}}}

	body := get(t, source, "/directory/image/nginx/latest/sbom", nil).Body.String()

	scroller := strings.Index(body, `class="scroller"`)
	table := strings.Index(body, `<table data-sortable id="sbom"`)
	if scroller < 0 || table < 0 || scroller > table {
		t.Errorf("table is not inside a scroll container: scroller=%d table=%d", scroller, table)
	}
}

// The versions page is the one page in the directory that is inherently
// mutable: it answers "what is published right now".
func TestVersionsPageListsEveryPublishedTag(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{
		{Tag: "latest", Digest: testDigest, Created: time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)},
		{Tag: "1.27.3", Digest: testDigest, Created: time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)},
		{Tag: "1.26.2", Digest: otherDigest, Created: time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)},
	}}

	response := get(t, source, "/directory/image/nginx/versions", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	for _, tag := range []string{"latest", "1.27.3", "1.26.2"} {
		if !strings.Contains(body, tag) {
			t.Errorf("versions page does not list %q:\n%s", tag, body)
		}
	}
	if !strings.Contains(body, "distroless.io/nginx") {
		t.Error("versions page does not name the image")
	}
}

// Telling a reader that `latest` and `1.27.3` are one build is the reason this
// page exists next to a digest-addressed SBOM page.
func TestVersionsPageGroupsTagsThatShareABuild(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{
		{Tag: "latest", Digest: testDigest},
		{Tag: "1.27.3", Digest: testDigest},
		{Tag: "1.26.2", Digest: otherDigest},
	}}

	body := get(t, source, "/directory/image/nginx/versions", nil).Body.String()

	// Two builds, so two rows: the shared build is listed once, carrying both
	// of the tags that name it.
	shared := `href="/directory/image/nginx/` + testDigest + `/sbom"`
	if got := strings.Count(body, shared); got != 1 {
		t.Errorf("build %s is listed %d times, want once — its tags are not grouped", testDigest, got)
	}
	// One row per build. Counting links would not do: every tag is a link too.
	if got := strings.Count(body, `<td class="tags">`); got != 2 {
		t.Errorf("page lists %d builds, want 2", got)
	}
	// Both tags belong to that one row, so neither may appear after the older
	// build's row has started.
	older := strings.Index(body, otherDigest)
	if latest, tagged := strings.Index(body, ">latest<"), strings.Index(body, ">1.27.3<"); latest > older || tagged > older {
		t.Errorf("tags of the shared build are not both in its row: latest=%d 1.27.3=%d older=%d", latest, tagged, older)
	}
}

// Each row is a way into the evidence: the SBOM for that exact build, at its
// permanent URL rather than through the tag that happens to name it today.
func TestVersionsPageLinksEachBuildToItsSBOM(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{{Tag: "latest", Digest: testDigest}}}

	body := get(t, source, "/directory/image/nginx/versions", nil).Body.String()

	if !strings.Contains(body, `href="/directory/image/nginx/`+testDigest+`/sbom"`) {
		t.Errorf("versions page does not link a build to its SBOM:\n%s", body)
	}
}

// Not a push date: it is the upstream-snapshot anchor an admission controller
// gates a build horizon on, so it says how fresh the packages inside are.
func TestVersionsPageShowsTheBuildHorizon(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{
		{Tag: "latest", Digest: testDigest, Created: time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)},
	}}

	body := get(t, source, "/directory/image/nginx/versions", nil).Body.String()

	if !strings.Contains(body, "2026-08-14") {
		t.Errorf("versions page does not show the build horizon:\n%s", body)
	}
}

// A build published without a readable config still has tags worth listing.
func TestVersionsPageToleratesAMissingBuildHorizon(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{{Tag: "latest", Digest: testDigest}}}

	body := get(t, source, "/directory/image/nginx/versions", nil).Body.String()

	if strings.Contains(body, "0001-01-01") {
		t.Errorf("versions page renders a zero time as a date:\n%s", body)
	}
}

// The bare family is a question about the whole family, which is what the
// versions page answers.
func TestImageRedirectsToVersions(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{{Tag: "latest", Digest: testDigest}}}

	response := get(t, source, "/directory/image/nginx", nil)

	if response.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want %d", response.Code, http.StatusMovedPermanently)
	}
	if got, want := response.Header().Get("Location"), "/directory/image/nginx/versions"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	if got := response.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=300") {
		t.Errorf("Cache-Control = %q, want a bounded max-age", got)
	}
}

// Tags move, and a fresh push showing up promptly is the whole value of this
// page — so it is cached far more briefly than the digest-addressed ones, and
// revalidation costs no body.
func TestVersionsPageIsCachedBriefly(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{{Tag: "latest", Digest: testDigest}}}

	first := get(t, source, "/directory/image/nginx/versions", nil)

	if got, want := first.Header().Get("Cache-Control"), "public, max-age=60"; got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the versions page")
	}

	second := get(t, source, "/directory/image/nginx/versions", http.Header{"If-None-Match": {etag}})
	if second.Code != http.StatusNotModified {
		t.Errorf("status = %d, want %d", second.Code, http.StatusNotModified)
	}
}

// The validator has to follow the tags, or a moved tag is served from cache.
func TestVersionsETagFollowsTheTags(t *testing.T) {
	before := &fakeSource{versions: []directory.Version{{Tag: "latest", Digest: testDigest}}}
	after := &fakeSource{versions: []directory.Version{{Tag: "latest", Digest: otherDigest}}}

	if a, b := etagOf(t, before), etagOf(t, after); a == b {
		t.Errorf("a tag that moved kept ETag %s", a)
	}
}

func etagOf(t *testing.T, source directory.Source) string {
	t.Helper()
	return get(t, source, "/directory/image/nginx/versions", nil).Header().Get("ETag")
}

func TestVersionsPageReportsUnknownFamily(t *testing.T) {
	source := &fakeSource{err: errors.New("repository unknown")}

	if code := get(t, source, "/directory/image/nope/versions", nil).Code; code == http.StatusOK {
		t.Errorf("status = %d, want an error status", code)
	}
}

// Sorting is shared by both tables; the filter is not. The script finds its
// table by the marker rather than by either page's id, so a page opts in by
// carrying it — and a header only sorts if it says which column it is.
func TestBothTablesOptIntoSharedSorting(t *testing.T) {
	source := &fakeSource{
		digest:     testDigest,
		components: []directory.Component{{Name: "libc6"}},
		versions:   []directory.Version{{Tag: "latest", Digest: testDigest}},
	}

	for _, page := range []struct {
		name, target string
		columns      []string
	}{
		{"sbom", "/directory/image/nginx/latest/sbom", []string{"name", "version", "license", "type"}},
		{"versions", "/directory/image/nginx/versions", []string{"tags", "created", "digest"}},
	} {
		body := get(t, source, page.target, nil).Body.String()

		if !strings.Contains(body, "<table data-sortable") {
			t.Errorf("%s table does not opt into sorting:\n%s", page.name, body)
		}
		if !strings.Contains(body, `<script src="/directory/static/main.mjs" type="module"></script>`) {
			t.Errorf("%s page does not load the shared entry point", page.name)
		}
		for _, column := range page.columns {
			if !strings.Contains(body, `data-column="`+column+`"`) {
				t.Errorf("%s page has no sortable %q header", page.name, column)
			}
		}
	}
}

// The filter belongs to the SBOM page alone: the versions page lists a handful
// of builds, and the shared script must not assume the controls are there.
func TestOnlyTheSBOMPageCarriesAFilter(t *testing.T) {
	source := &fakeSource{
		digest:     testDigest,
		components: []directory.Component{{Name: "libc6"}},
		versions:   []directory.Version{{Tag: "latest", Digest: testDigest}},
	}

	if body := get(t, source, "/directory/image/nginx/latest/sbom", nil).Body.String(); !strings.Contains(body, `id="filter"`) {
		t.Error("sbom page lost its filter")
	}
	if body := get(t, source, "/directory/image/nginx/versions", nil).Body.String(); strings.Contains(body, `id="filter"`) {
		t.Error("versions page grew a filter over a handful of rows")
	}
}

// A row offers two ways in, and they answer different questions: a tag URL
// follows the family and will name a different build once the tag moves; the
// digest beside it is frozen. Offering only the digest hides the URL a reader
// would actually share.
func TestVersionsPageLinksEachTagToItsOwnURL(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{
		{Tag: "latest", Digest: testDigest},
		{Tag: "1.27.3", Digest: testDigest},
		{Tag: "1.26.2", Digest: otherDigest},
	}}

	body := get(t, source, "/directory/image/nginx/versions", nil).Body.String()

	for _, tag := range []string{"latest", "1.27.3", "1.26.2"} {
		if want := `href="/directory/image/nginx/` + tag + `/sbom"`; !strings.Contains(body, want) {
			t.Errorf("tag %q is not a link to %s:\n%s", tag, want, body)
		}
	}
	// And the build's own permanent URL is still there beside them.
	if want := `href="/directory/image/nginx/` + testDigest + `/sbom"`; !strings.Contains(body, want) {
		t.Errorf("build no longer links to its permanent URL %s", want)
	}
}

// distroless exists to be small, so size is a headline number rather than a
// footnote — and it has to sort numerically, which a rendered "23.9 MB"
// cannot do on its own.
func TestVersionsPageShowsSize(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{
		{Tag: "latest", Digest: testDigest, Sizes: map[string]directory.Size{"amd64": 23867899}},
	}}

	body := get(t, source, "/directory/image/nginx/versions", nil).Body.String()

	if !strings.Contains(body, "23.9 MB") {
		t.Errorf("versions page does not show the size:\n%s", body)
	}
	if !strings.Contains(body, `data-sort-key="23867899"`) {
		t.Errorf("size column carries no numeric sort key:\n%s", body)
	}
	if !strings.Contains(body, `data-column="size-amd64"`) {
		t.Error("size column is not sortable")
	}
	// "Size" alone invites a reader to assume the size on disk. It is the
	// compressed download, and the header should say which.
	if !strings.Contains(body, "Compressed size") {
		t.Errorf("size column does not say it is the compressed size:\n%s", body)
	}
}

// A build whose manifest would not read still has tags worth listing.
func TestVersionsPageToleratesAnUnknownSize(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{{Tag: "latest", Digest: testDigest}}}

	body := get(t, source, "/directory/image/nginx/versions", nil).Body.String()

	if strings.Contains(body, "0 B") {
		t.Errorf("versions page renders a missing size as zero bytes:\n%s", body)
	}
}

// One column per architecture, each sorting on its own bytes. A single
// number would have to pick an architecture and silently drop the other.
func TestVersionsPageColumnsEachArchitecture(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{
		{Tag: "latest", Digest: testDigest, Sizes: map[string]directory.Size{"amd64": 23867899, "arm64": 23345603}},
	}}

	body := get(t, source, "/directory/image/nginx/versions", nil).Body.String()

	for arch, size := range map[string]string{"amd64": "23.9 MB", "arm64": "23.3 MB"} {
		if !strings.Contains(body, `data-column="size-`+arch+`"`) {
			t.Errorf("no sortable %s column:\n%s", arch, body)
		}
		if !strings.Contains(body, size) {
			t.Errorf("%s size %s not shown", arch, size)
		}
	}
	if !strings.Contains(body, `data-sort-key="23867899"`) || !strings.Contains(body, `data-sort-key="23345603"`) {
		t.Error("architecture columns do not each carry their own sort key")
	}
	if !strings.Contains(body, "Compressed size") {
		t.Error("the columns do not say they are compressed sizes")
	}
}

// "What do I type into docker pull" is the question a reader arrives with.
// The host answering it is not the one the bytes come from, so leaving them to
// assemble it invites the GHCR path.
func TestPagesShowThePullCommand(t *testing.T) {
	source := &fakeSource{
		digest:     testDigest,
		components: []directory.Component{{Name: "libc6"}},
		versions:   []directory.Version{{Tag: "latest", Digest: testDigest}},
	}

	for _, page := range []struct{ name, target, want string }{
		// A versions page is about the family, and every tag in the table is
		// one way to finish the command.
		{"versions", "/directory/image/nginx/versions", ">docker pull distroless.io/nginx</code>"},
		// An SBOM page knows exactly which build it is showing, so it says so
		// — by tag, or by digest when that is how the reader arrived.
		{"tag", "/directory/image/nginx/1.27/sbom", ">docker pull distroless.io/nginx:1.27</code>"},
		{"digest", "/directory/image/nginx/" + testDigest + "/sbom", ">docker pull distroless.io/nginx@" + testDigest + "</code>"},
	} {
		body := get(t, source, page.target, nil).Body.String()

		if !strings.Contains(body, page.want) {
			t.Errorf("%s page does not show %q:\n%s", page.name, page.want, body)
		}
	}
}

// The copy button is built in the browser, so the page's part of the contract
// is only to mark what is worth copying. A reader without JavaScript gets a
// plain command to select rather than a button that does nothing.
func TestPullCommandIsMarkedCopyable(t *testing.T) {
	source := &fakeSource{
		digest:     testDigest,
		components: []directory.Component{{Name: "libc6"}},
		versions:   []directory.Version{{Tag: "latest", Digest: testDigest}},
	}

	for _, target := range []string{
		"/directory/image/java/versions",
		"/directory/image/java/latest/sbom",
	} {
		body := get(t, source, target, nil).Body.String()

		if !strings.Contains(body, `<code data-copyable tabindex="0">docker pull distroless.io/java`) {
			t.Errorf("GET %s does not mark the pull command copyable:\n%s", target, body)
		}
		if strings.Contains(body, "<button") && strings.Contains(body, "Copy</button>") {
			t.Errorf("GET %s ships a copy button in the HTML; it should be built in the browser", target)
		}
	}
}

// Pulling and building on top are different jobs, and a reader doing the
// second should not have to retype the reference into a FROM.
func TestSBOMPageOffersTheDockerfileLine(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6"}}}

	for _, page := range []struct{ name, target, reference string }{
		{"tag", "/directory/image/java/21-debug/sbom", "distroless.io/java:21-debug"},
		// A digest is what belongs in a Dockerfile that has to build the same
		// way twice, so the permanent page offers the permanent reference.
		{"digest", "/directory/image/java/" + testDigest + "/sbom", "distroless.io/java@" + testDigest},
	} {
		body := get(t, source, page.target, nil).Body.String()

		if want := ">FROM " + page.reference + "</code>"; !strings.Contains(body, want) {
			t.Errorf("%s page has no Dockerfile line %q:\n%s", page.name, want, body)
		}
		if want := ">docker pull " + page.reference + "</code>"; !strings.Contains(body, want) {
			t.Errorf("%s page lost its pull command %q", page.name, want)
		}
	}
}

// The shell prompt is decoration. Inside the copyable element it would be
// copied too, and `$ docker pull ...` pasted into a shell is a broken command.
func TestPromptIsNotCopiedWithTheCommand(t *testing.T) {
	source := &fakeSource{digest: testDigest, components: []directory.Component{{Name: "libc6"}}}

	body := get(t, source, "/directory/image/java/21-debug/sbom", nil).Body.String()

	if !strings.Contains(body, `<code data-copyable tabindex="0">docker pull`) {
		t.Errorf("the copyable command does not begin with the command itself:\n%s", body)
	}
	if strings.Contains(body, `<code data-copyable tabindex="0">$`) {
		t.Error("the prompt is inside the copyable element")
	}
}

// A family is not a reference, and `FROM distroless.io/java` pins nothing —
// the versions page lists the tags precisely so a reader can choose one.
func TestVersionsPageDoesNotOfferAnUnpinnedDockerfileLine(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{{Tag: "latest", Digest: testDigest}}}

	body := get(t, source, "/directory/image/java/versions", nil).Body.String()

	if strings.Contains(body, "FROM ") {
		t.Errorf("versions page offers an unpinned FROM:\n%s", body)
	}
}

// Heading, then what it is, then how to get it. The digest identifies the
// build the commands below refer to, so it reads before them rather than as a
// footnote after.
func TestIdentityReadsBeforeTheCommands(t *testing.T) {
	source := &fakeSource{
		digest:     testDigest,
		components: []directory.Component{{Name: "libc6"}},
		versions:   []directory.Version{{Tag: "latest", Digest: testDigest}},
	}

	for _, target := range []string{
		"/directory/image/java/latest/sbom",
		"/directory/image/java/versions",
	} {
		body := get(t, source, target, nil).Body.String()

		heading := strings.Index(body, "<h1>")
		identity := strings.Index(body, `<p class="digest">`)
		command := strings.Index(body, "docker pull")
		if identity < heading || command < identity {
			t.Errorf("GET %s orders heading=%d identity=%d command=%d, want that order",
				target, heading, identity, command)
		}
	}
}

// The same panel on both pages: "how do I get this" is one question, and it
// should not look like two.
func TestVersionsPageUsesTheInstallPanel(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{{Tag: "latest", Digest: testDigest}}}

	body := get(t, source, "/directory/image/java/versions", nil).Body.String()

	if !strings.Contains(body, `<section class="install">`) {
		t.Errorf("versions page does not use the install panel:\n%s", body)
	}
	if !strings.Contains(body, "Install from the command line") {
		t.Error("versions page does not label its command")
	}
	if !strings.Contains(body, `<p class="command">`) {
		t.Error("versions page does not box its command")
	}
}

// A trailing slash is the same question as without one, and Go's ServeMux does
// not add one for you the way it strips one.
func TestFamilyWithATrailingSlashRedirects(t *testing.T) {
	source := &fakeSource{versions: []directory.Version{{Tag: "latest", Digest: testDigest}}}

	response := get(t, source, "/directory/image/java/", nil)

	if response.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want %d", response.Code, http.StatusMovedPermanently)
	}
	if got, want := response.Header().Get("Location"), "/directory/image/java/versions"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// A command scrolls sideways rather than wrapping, so it stays one line the
// way a terminal would show it. That makes it a region the keyboard has to be
// able to reach — the same reason the components table is focusable.
func TestScrollableCommandsAreFocusable(t *testing.T) {
	source := &fakeSource{
		digest:     testDigest,
		components: []directory.Component{{Name: "libc6"}},
		versions:   []directory.Version{{Tag: "latest", Digest: testDigest}},
	}

	for _, target := range []string{
		"/directory/image/java/latest/sbom",
		"/directory/image/java/versions",
	} {
		body := get(t, source, target, nil).Body.String()

		if strings.Contains(body, "<code data-copyable>") {
			t.Errorf("GET %s has a scrollable command the keyboard cannot reach:\n%s", target, body)
		}
		if !strings.Contains(body, `<code data-copyable tabindex="0">`) {
			t.Errorf("GET %s does not make its command focusable", target)
		}
	}
}

// From a build's own page there was no way to reach a sibling tag without
// going back to the versions list.
func TestSBOMPageSwitchesBetweenTags(t *testing.T) {
	source := &fakeSource{
		digest:     testDigest,
		components: []directory.Component{{Name: "libc6"}},
		tags:       []string{"latest", "5", "5.3"},
	}

	body := get(t, source, "/directory/image/bash/5.3/sbom", nil).Body.String()

	for _, tag := range []string{"latest", "5", "5.3"} {
		if want := `href="/directory/image/bash/` + tag + `/sbom"`; !strings.Contains(body, want) {
			t.Errorf("no way to reach tag %q (%s):\n%s", tag, want, body)
		}
	}
	// Real links in a disclosure, not a <select>: a select cannot navigate
	// without JavaScript, and a control that does nothing is worse than none.
	if !strings.Contains(body, "<details") || strings.Contains(body, "<select") {
		t.Error("the switcher is not a disclosure of links")
	}
	if !strings.Contains(body, `<span class="value">5.3</span></summary>`) {
		t.Errorf("the switcher does not name the tag being shown:\n%s", body)
	}
}

// Switching tag should not silently switch architecture as well.
func TestTagSwitcherKeepsTheArchitecture(t *testing.T) {
	source := &fakeSource{
		digest: testDigest,
		components: []directory.Component{
			{Name: "libc6", Arch: "amd64"},
			{Name: "libc6", Arch: "arm64"},
		},
		tags: []string{"latest", "5.3"},
	}

	body := get(t, source, "/directory/image/bash/5.3/sbom?arch=arm64", nil).Body.String()

	if want := `href="/directory/image/bash/latest/sbom?arch=arm64"`; !strings.Contains(body, want) {
		t.Errorf("the switcher drops the architecture:\n%s", body)
	}
}

// The SBOM is what the page is for. A registry that will not list tags costs
// the switcher, not the page.
func TestSBOMPageSurvivesATagListingFailure(t *testing.T) {
	source := &fakeSource{
		digest:     testDigest,
		components: []directory.Component{{Name: "libc6"}},
		tagsErr:    errors.New("tags/list refused"),
	}

	response := get(t, source, "/directory/image/bash/5.3/sbom", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "libc6") {
		t.Error("the table is missing")
	}
	if strings.Contains(response.Body.String(), "<details") {
		t.Error("an empty switcher was rendered anyway")
	}
}

// One tag is not a choice.
func TestSBOMPageOmitsASwitcherForASingleTag(t *testing.T) {
	source := &fakeSource{
		digest:     testDigest,
		components: []directory.Component{{Name: "libc6"}},
		tags:       []string{"latest"},
	}

	body := get(t, source, "/directory/image/bash/latest/sbom", nil).Body.String()

	if strings.Contains(body, "<details") {
		t.Errorf("rendered a switcher offering nothing to switch to:\n%s", body)
	}
}

// A digest names a build, not a name — the switcher still offers the family's
// tags, but none of them is what the reader is looking at.
func TestDigestPageSwitcherNamesTheDigest(t *testing.T) {
	source := &fakeSource{
		digest:     testDigest,
		components: []directory.Component{{Name: "libc6"}},
		tags:       []string{"latest", "5.3"},
	}

	body := get(t, source, "/directory/image/bash/"+testDigest+"/sbom", nil).Body.String()

	if !strings.Contains(body, `<span class="value">0da1844626f2</span></summary>`) {
		t.Errorf("the switcher does not name the digest being shown:\n%s", body)
	}
}

// Tag and architecture answer the same shape of question — "show me a
// different one of these" — so they look alike and sit together with the
// other controls over the table.
func TestSwitchersSitTogetherInTheControls(t *testing.T) {
	source := &fakeSource{
		digest: testDigest,
		components: []directory.Component{
			{Name: "libc6", Arch: "amd64"},
			{Name: "libc6", Arch: "arm64"},
		},
		tags: []string{"latest", "5.3"},
	}

	body := get(t, source, "/directory/image/bash/5.3/sbom?arch=arm64", nil).Body.String()

	controls := strings.Index(body, `<div class="controls">`)
	table := strings.Index(body, "<table")
	if controls < 0 {
		t.Fatal("no controls")
	}
	// Both switchers live inside the controls, which come before the table.
	block := body[controls:table]
	if got := strings.Count(block, `<details class="switcher">`); got != 2 {
		t.Errorf("controls hold %d switchers, want 2 (tag and architecture):\n%s", got, block)
	}
	if strings.Contains(body, `class="arches"`) {
		t.Error("the architecture nav survived alongside its replacement")
	}

	// Each has to say which question it answers; alone, "arm64" and "5.3" are
	// two unlabelled boxes.
	for _, want := range []string{
		`<span class="key">Tag:</span><span class="value">5.3</span>`,
		`<span class="key">Arch:</span><span class="value">arm64</span>`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("no switcher labelled %s:\n%s", want, block)
		}
	}
	if !strings.Contains(block, `href="?arch=amd64"`) {
		t.Error("the architecture switcher does not link the other architecture")
	}
}

// A registry lists tags alphabetically, which puts 17 above 25 and a -debug
// variant above the release it varies. The switcher orders them the way the
// versions page does, by the same rules.
func TestTagSwitcherOrdersTagsLikeTheVersionsPage(t *testing.T) {
	source := &fakeSource{
		digest:     testDigest,
		components: []directory.Component{{Name: "libc6"}},
		// As the registry hands them over.
		tags: []string{"17", "17-debug", "21", "21-debug", "25", "25-debug", "latest"},
	}

	body := get(t, source, "/directory/image/java/21/sbom", nil).Body.String()

	block := regexp.MustCompile(`(?s)<details class="switcher">.*?</details>`).FindString(body)
	var order []string
	for _, m := range regexp.MustCompile(`<a href="[^"]*">([^<]+)</a>`).FindAllStringSubmatch(block, -1) {
		order = append(order, m[1])
	}

	want := []string{"latest", "25", "25-debug", "21", "21-debug", "17", "17-debug"}
	if !equal(order, want) {
		t.Errorf("tag order = %v, want %v", order, want)
	}
}

// A browser refuses to execute a module served as anything but a JavaScript
// type — no fallback, no sniffing. Go's built-in table has no .mjs entry, and
// the distroless image we ship in has no /etc/mime.types to fall back on, so
// mime.TypeByExtension answers differently here than in production unless the
// handler states the type itself. Standing in a wrong answer is how that
// host-dependence shows up as a failure here rather than only in a reader's
// browser.
func TestModulesAreServedAsJavaScript(t *testing.T) {
	mime.AddExtensionType(".mjs", "application/x-not-javascript")

	for _, asset := range []string{
		"/directory/static/main.mjs",
		"/directory/static/directory.mjs",
	} {
		response := get(t, &fakeSource{}, asset, nil)

		if response.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want %d", asset, response.Code, http.StatusOK)
		}
		if got := response.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
			t.Errorf("GET %s Content-Type = %q, want a JavaScript type", asset, got)
		}
	}
}
