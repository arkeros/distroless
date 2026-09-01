// Package directory serves the human-readable view of the Mirror: what is
// inside a published image, read off the evidence attached to its Digest.
package directory

import (
	"bytes"
	"context"
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

//go:embed templates/sbom.html static/sbom.css static/sbom.js static/fonts
var assets embed.FS

// Parsed once at startup so a broken template fails the process rather than a
// request. The tests in this package execute it against real data, which is
// what catches a renamed field.
var pages = template.Must(template.ParseFS(assets, "templates/*.html"))

// Source resolves an image reference to the SBOM attached to its Index.
type Source interface {
	SBOM(ctx context.Context, image string) (digest string, components []Component, err error)
	// Document returns the verified CycloneDX attestation exactly as it was
	// signed. The page renders a projection of it; this is the evidence
	// itself, which is what a reader feeds to their own tooling.
	Document(ctx context.Context, image string) (digest string, document []byte, err error)
}

// NewHandler serves the directory pages and the assets they reference.
//
// mirror is the host the images are published under — the name a reader pulls
// by. It is display only: what gets fetched is decided by the Source, which
// may well read from the registry behind the mirror rather than the mirror
// itself.
func NewHandler(source Source, mirror string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /directory/static/", http.StripPrefix("/directory", staticHeaders(http.FileServerFS(assets))))
	mux.HandleFunc("GET /directory/image/{family}/sbom", func(w http.ResponseWriter, r *http.Request) {
		serveSBOM(w, r, source, mirror)
	})
	mux.HandleFunc("GET /directory/image/{family}/sbom.json", func(w http.ResponseWriter, r *http.Request) {
		serveDocument(w, r, source)
	})
	return mux
}

func serveSBOM(w http.ResponseWriter, r *http.Request, source Source, mirror string) {
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		tag = "latest"
	}
	image := r.PathValue("family") + ":" + tag

	digest, components, err := source.SBOM(r.Context(), image)
	if err != nil {
		// Nothing here distinguishes an unknown family from an image with no
		// SBOM attached, and to a reader they are the same absence.
		slog.Warn("sbom lookup failed", "image", image, "error", err)
		http.Error(w, "no SBOM published for "+image, http.StatusNotFound)
		return
	}

	// Shown as the reader would pull it, not as it is stored upstream.
	table := NewTable(mirror+"/"+image, digest, r.URL.Query().Get("arch"), components)
	table.Download = "/directory/image/" + r.PathValue("family") + "/sbom.json?tag=" + url.QueryEscape(tag)

	// An SBOM is immutable for a Digest, so the digest is the validator: a tag
	// that has not moved costs a request but no body. The architecture is part
	// of it because two architectures are two documents — sharing a validator
	// would let a cache serve one for the other.
	etag := `"` + digest + "-" + table.Arch + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=300")
	if strings.Contains(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Render before writing, so a template failure is a 500 rather than a
	// truncated page under a 200.
	var page bytes.Buffer
	if err := pages.ExecuteTemplate(&page, "sbom.html", table); err != nil {
		slog.Error("rendering sbom page failed", "image", image, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(page.Bytes()); err != nil {
		slog.Warn("writing sbom page failed", "image", image, "error", err)
	}
}

// serveDocument hands over the attestation's own bytes.
//
// Not filtered to the architecture the page happens to be showing: the
// signature covers the whole document, and a subset of it is something nobody
// attested to.
func serveDocument(w http.ResponseWriter, r *http.Request, source Source) {
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		tag = "latest"
	}
	family := r.PathValue("family")
	image := family + ":" + tag

	digest, document, err := source.Document(r.Context(), image)
	if err != nil {
		slog.Warn("sbom document lookup failed", "image", image, "error", err)
		http.Error(w, "no SBOM published for "+image, http.StatusNotFound)
		return
	}

	// One document per Digest, covering every architecture, so unlike the page
	// the architecture is not part of the validator.
	etag := `"` + digest + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=300")
	if strings.Contains(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Named for what the reader was looking at, and carrying the digest so two
	// downloads of a tag that moved do not collide in one directory.
	filename := nameForFile(family) + "-" + nameForFile(tag) + "-" + shortDigest(digest) + ".cdx.json"
	w.Header().Set("Content-Type", "application/vnd.cyclonedx+json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	if _, err := w.Write(document); err != nil {
		slog.Warn("writing sbom document failed", "image", image, "error", err)
	}
}

// nameForFile reduces a reader-supplied tag to something that can only be a
// filename. It reaches a response header, where a quote or a newline would be
// someone else's syntax rather than a name.
func nameForFile(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.', r == '_', r == '-':
			return r
		default:
			return '-'
		}
	}, s)
}

// shortDigest is the leading hex of a digest: enough to tell two builds apart
// in a downloads folder without spending 64 characters on it.
func shortDigest(digest string) string {
	if _, hex, found := strings.Cut(digest, ":"); found {
		digest = hex
	}
	if len(digest) > 12 {
		digest = digest[:12]
	}
	return digest
}

func staticHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Short, because these are served under unversioned names.
		w.Header().Set("Cache-Control", "public, max-age=3600")

		// The file server would otherwise ask mime.TypeByExtension, which
		// answers from the host's /etc/mime.types — a file the image we ship
		// in does not have. Stating it keeps the type the same everywhere.
		if strings.HasSuffix(r.URL.Path, ".woff2") {
			w.Header().Set("Content-Type", "font/woff2")
		}

		next.ServeHTTP(w, r)
	})
}
