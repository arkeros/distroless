// Package compress sends what is worth compressing compressed, to a browser
// that asked.
//
// Nothing between the server and a reader does this: Go's file server never
// looks at Accept-Encoding, and Cloud Run's frontend passes bodies through as
// they are. So a page, a stylesheet or an SBOM download went out at full size
// — and markup compresses about tenfold.
//
// The work is httpcompression's. What is decided here is what it is pointed
// at: which encodings, at what effort, for which content.
package compress

import (
	"compress/gzip"
	"net/http"

	"github.com/CAFxX/httpcompression"
)

// Handler wraps next so that a compressible response goes out gzipped or
// brotli-encoded, whichever the request prefers. Everything else passes
// through untouched.
func Handler(next http.Handler) http.Handler {
	adapter, err := httpcompression.Adapter(
		httpcompression.GzipCompressionLevel(gzip.DefaultCompression),
		// Brotli's scale runs to 11, and the pure-Go encoder gets slow well
		// before that. 5 still beats gzip on markup at about gzip's speed —
		// and a page is encoded on every request, so speed is the constraint.
		httpcompression.BrotliCompressionLevel(5),
		// Below this an encoded body tends to come out larger than it went
		// in; "ok\n" from the health check is the typical case.
		httpcompression.MinSize(httpcompression.DefaultMinSize),
		// Text of every kind the directory serves: markup, styles, script,
		// JSON in its plain and CycloneDX flavours, and the svg logos. A
		// woff2 font is compressed already, and is left alone by not being
		// named. Matched on the media type alone, so a charset does not
		// have to be repeated here.
		httpcompression.ContentTypes([]string{
			"text/html",
			"text/plain",
			"text/css",
			"text/javascript",
			"application/json",
			"application/vnd.cyclonedx+json",
			"image/svg+xml",
		}, false),
	)
	if err != nil {
		// Every option above is a constant, so this is a programming error
		// caught the first time the process starts, not a runtime condition.
		panic(err)
	}
	return adapter(next)
}
