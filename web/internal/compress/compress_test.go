package compress_test

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/arkeros/distroless/web/internal/compress"
)

// page is a body worth compressing: long, and repetitive the way markup is.
var page = strings.Repeat("<tr><td>libc6</td><td>2.36-9+deb12u7</td><td>LGPL-2.1</td></tr>\n", 100)

// serve runs handler behind the compression wrapper, with the request
// carrying headers, and returns what came out.
func serve(t *testing.T, handler http.Handler, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	compress.Handler(handler).ServeHTTP(recorder, request)
	return recorder
}

// text is a handler serving body under contentType, the way every page and
// stylesheet handler does: type first, then the bytes.
func text(contentType, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		io.WriteString(w, body)
	})
}

// gunzip is what a browser does with a gzip body, and it fails the test if
// the bytes are not one.
func gunzip(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatalf("body is not gzip: %v", err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading gzip body: %v", err)
	}
	return string(body)
}

// A browser that says it takes gzip gets it, and gets the same bytes back
// out — the point of the whole exercise being that nothing else changes.
func TestCompressesTextForABrowserThatAcceptsIt(t *testing.T) {
	response := serve(t, text("text/html; charset=utf-8", page), map[string]string{"Accept-Encoding": "gzip, deflate"})

	if got := response.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, changed by compressing", got)
	}
	if got := gunzip(t, response); got != page {
		t.Errorf("decompressed body differs from what the handler wrote")
	}
	if response.Body.Len() >= len(page) {
		t.Errorf("compressed body is %d bytes, original %d", response.Body.Len(), len(page))
	}
}

// Vary is what tells a cache that the two answers to one URL are both right,
// and it has to be on the plain answer too, or a cache that saw the plain one
// first would hand it to every browser after.
func TestVariesOnAcceptEncodingEitherWay(t *testing.T) {
	for name, headers := range map[string]map[string]string{
		"accepting": {"Accept-Encoding": "gzip"},
		"plain":     {},
	} {
		response := serve(t, text("text/html; charset=utf-8", page), headers)
		if got := response.Header().Get("Vary"); got != "Accept-Encoding" {
			t.Errorf("%s: Vary = %q, want Accept-Encoding", name, got)
		}
	}
}

// A browser that did not ask — or asked for gzip not to be used — gets the
// bytes as written.
func TestLeavesTheBodyAloneWhenNotAccepted(t *testing.T) {
	for name, headers := range map[string]map[string]string{
		"no header": {},
		"refused":   {"Accept-Encoding": "gzip;q=0, identity"},
		"other":     {"Accept-Encoding": "zstd"},
	} {
		response := serve(t, text("text/html; charset=utf-8", page), headers)
		if got := response.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("%s: Content-Encoding = %q, want none", name, got)
		}
		if response.Body.String() != page {
			t.Errorf("%s: body was changed", name)
		}
	}
}

// Only text is worth it. A woff2 font is already compressed and a logo's svg
// is text, so the decision is by content type rather than by path, and the
// type is the handler's to state.
func TestCompressesByContentType(t *testing.T) {
	for contentType, want := range map[string]bool{
		"text/html; charset=utf-8":       true,
		"text/css; charset=utf-8":        true,
		"text/javascript; charset=utf-8": true,
		"application/json":               true,
		"application/vnd.cyclonedx+json": true,
		"image/svg+xml":                  true,
		"font/woff2":                     false,
		"application/octet-stream":       false,
		"image/png":                      false,
	} {
		response := serve(t, text(contentType, page), map[string]string{"Accept-Encoding": "gzip"})
		if got := response.Header().Get("Content-Encoding") == "gzip"; got != want {
			t.Errorf("%s: compressed = %v, want %v", contentType, got, want)
		}
	}
}

// A 304 has no body, and a Content-Encoding on it would describe nothing.
func TestLeavesBodilessResponsesAlone(t *testing.T) {
	notModified := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotModified)
	})

	response := serve(t, notModified, map[string]string{"Accept-Encoding": "gzip"})

	if response.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotModified)
	}
	if got := response.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q on a 304", got)
	}
	if response.Body.Len() != 0 {
		t.Errorf("a 304 grew a body: %q", response.Body.String())
	}
}

// A range is a range of the bytes as stored, which the encoded bytes are not.
// So a browser asking for the tail of a file while accepting gzip gets the
// whole file, encoded — never a slice of it that it cannot place.
func TestAnswersRangeRequestsWithTheWholeBody(t *testing.T) {
	files := fstest.MapFS{"directory.css": {Data: []byte(page)}}
	request := httptest.NewRequest(http.MethodGet, "/directory.css", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	request.Header.Set("Range", "bytes=0-99")
	recorder := httptest.NewRecorder()

	compress.Handler(http.FileServerFS(files)).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: a partial body must not be encoded", recorder.Code, http.StatusOK)
	}
	if got := gunzip(t, recorder); got != page {
		t.Errorf("decompressed body is not the whole file")
	}
}

// Below a couple of hundred bytes an encoded body tends to be larger than the
// plain one, and the health check's "ok" is exactly that case.
func TestLeavesTinyBodiesAlone(t *testing.T) {
	response := serve(t, text("text/plain; charset=utf-8", "ok\n"), map[string]string{"Accept-Encoding": "gzip"})

	if got := response.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q on a three-byte body", got)
	}
	if response.Body.String() != "ok\n" {
		t.Errorf("body = %q, want it untouched", response.Body.String())
	}
}

// A browser that prefers brotli gets brotli: smaller than gzip on markup, and
// what every current browser asks for first.
func TestPrefersBrotliWhenTheBrowserDoes(t *testing.T) {
	response := serve(t, text("text/html; charset=utf-8", page), map[string]string{"Accept-Encoding": "gzip, deflate, br, zstd"})

	if got := response.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want br", got)
	}
	if response.Body.Len() >= len(page) {
		t.Errorf("brotli body is %d bytes, original %d", response.Body.Len(), len(page))
	}
}

// The file server states a Content-Length, and the compressed body is not that
// long. A wrong length is a truncated download or a hung connection, so the
// header must go, and the type the file server chose must stay.
func TestDropsTheFileServersContentLength(t *testing.T) {
	files := fstest.MapFS{"directory.css": {Data: []byte(page)}}
	server := http.FileServerFS(files)
	request := httptest.NewRequest(http.MethodGet, "/directory.css", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	recorder := httptest.NewRecorder()

	compress.Handler(server).ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := recorder.Header().Get("Content-Length"); got != "" {
		t.Errorf("Content-Length = %q left over from the file server", got)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
		t.Errorf("Content-Type = %q, want the file server's text/css", got)
	}
	if got := gunzip(t, recorder); got != page {
		t.Errorf("decompressed body differs from the file")
	}
}

// A handler that writes without stating a type leaves the type to sniffing,
// and the sniff has to see the handler's bytes rather than gzip's — or every
// such response would be served as application/gzip.
func TestSniffsTheTypeBeforeCompressing(t *testing.T) {
	untyped := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "<!DOCTYPE html><html><body>"+page+"</body></html>")
	})

	response := serve(t, untyped, map[string]string{"Accept-Encoding": "gzip"})

	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("Content-Type = %q, want text/html from sniffing the handler's bytes", got)
	}
	if got := response.Header().Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}
}
