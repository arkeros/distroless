package directory_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arkeros/distroless/web/internal/directory"
)

type fakeSource struct {
	digest     string
	components []directory.Component
	err        error
}

func (f fakeSource) SBOM(_ context.Context, _ string) (string, []directory.Component, error) {
	return f.digest, f.components, f.err
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
