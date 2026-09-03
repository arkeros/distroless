// Package directory serves the human-readable view of the Mirror: what is
// inside a published image, read off the evidence attached to its Digest.
//
// What an image *contains* is always read from an Attestation verified first.
// What a tag currently *names* cannot be: a tag-to-digest mapping is registry
// metadata that nobody signs. The versions page shows that mapping and says
// so; every other page here is rendered from verified evidence.
package directory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

//go:embed templates static/directory.css static/directory.js static/fonts
var assets embed.FS

// Parsed once at startup so a broken template fails the process rather than a
// request. The tests in this package execute it against real data, which is
// what catches a renamed field.
var pages = template.Must(template.ParseFS(assets, "templates/*.html"))

// Source resolves an image reference to the SBOM attached to its Index.
//
// family and ref are handed over separately rather than pre-joined: a tag and
// a digest are two different kinds of reference, and building one is the job
// of the component that talks to the registry.
type Source interface {
	SBOM(ctx context.Context, family, ref string) (digest string, components []Component, err error)
	// Document returns the verified CycloneDX attestation exactly as it was
	// signed. The page renders a projection of it; this is the evidence
	// itself, which is what a reader feeds to their own tooling.
	Document(ctx context.Context, family, ref string) (digest string, document []byte, err error)
	// Versions lists every tag published for a family and the build each one
	// names. Unverified by nature — see the package comment.
	Versions(ctx context.Context, family string) ([]Version, error)
}

// defaultRef is what a reader gets when they name no reference. It is only
// ever reached by redirect: every page has one canonical URL, and that URL
// spells out what it is showing.
const defaultRef = "latest"

// An OCI tag and a digest are told apart by the colon alone — a tag may not
// contain one — so the rule is total and needs no prefix sniffing.
var (
	tagPattern    = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)
	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// isDigest reports whether ref names a Digest rather than a tag.
func isDigest(ref string) bool { return strings.Contains(ref, ":") }

// validRef reports whether ref could name anything at all. It is checked
// before the Source is asked, so syntactic nonsense costs no registry round
// trip and never reaches a response header.
func validRef(ref string) bool {
	if isDigest(ref) {
		return digestPattern.MatchString(ref)
	}
	return tagPattern.MatchString(ref)
}

// NewHandler serves the directory pages and the assets they reference.
//
// mirror is the host the images are published under — the name a reader pulls
// by. It is display only: what gets fetched is decided by the Source, which
// may well read from the registry behind the mirror rather than the mirror
// itself.
//
// Nothing is registered for /directory/image/{family}/{ref} itself. That URL
// is where a page describing the image — its entrypoint, user, labels — would
// belong, and answering it with the SBOM today would make the SBOM the
// permanent default view of an image.
func NewHandler(source Source, mirror string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /directory/static/", http.StripPrefix("/directory", staticHeaders(http.FileServerFS(assets))))
	mux.HandleFunc("GET /directory/image/{family}/{ref}/sbom", func(w http.ResponseWriter, r *http.Request) {
		serveSBOM(w, r, source, mirror)
	})
	mux.HandleFunc("GET /directory/image/{family}/{ref}/sbom.json", func(w http.ResponseWriter, r *http.Request) {
		serveDocument(w, r, source)
	})
	mux.HandleFunc("GET /directory/image/{family}/versions", func(w http.ResponseWriter, r *http.Request) {
		serveVersions(w, r, source, mirror)
	})
	mux.HandleFunc("GET /directory/image/{family}/sbom", permanentRedirect(func(family string) string {
		return resourceURL(family, defaultRef, "sbom")
	}))
	mux.HandleFunc("GET /directory/image/{family}/sbom.json", permanentRedirect(func(family string) string {
		return resourceURL(family, defaultRef, "sbom.json")
	}))
	// A bare family is a question about the family rather than about one
	// build, which is what the versions page answers. Registered twice
	// because ServeMux strips a trailing slash but will not add one: without
	// the `{$}` form, /directory/image/java/ is a 404 next to a working
	// /directory/image/java.
	for _, pattern := range []string{"GET /directory/image/{family}", "GET /directory/image/{family}/{$}"} {
		mux.HandleFunc(pattern, permanentRedirect(func(family string) string {
			return familyURL(family, "versions")
		}))
	}
	return mux
}

// permanentRedirect sends one URL to the canonical one for what it asks.
//
// Permanent, because these mappings are this routing table rather than
// anything the registry decides — but with an explicit Cache-Control, because
// a 301 with no directive is cached by browsers heuristically and forever, and
// there would be no way to take it back.
func permanentRedirect(target func(family string) string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		to := target(r.PathValue("family"))
		if r.URL.RawQuery != "" {
			to += "?" + r.URL.RawQuery
		}
		w.Header().Set("Cache-Control", "public, max-age=300")
		http.Redirect(w, r, to, http.StatusMovedPermanently)
	}
}

// serveVersions lists what a family publishes right now.
//
// The one page here that is inherently mutable: it answers a question about
// tags, and tags move. Nothing on it can be keyed by a content address.
func serveVersions(w http.ResponseWriter, r *http.Request, source Source, mirror string) {
	family := r.PathValue("family")

	published, err := source.Versions(r.Context(), family)
	if err != nil {
		slog.Warn("versions lookup failed", "family", family, "error", err)
		http.Error(w, "nothing published for "+mirror+"/"+family, http.StatusNotFound)
		return
	}

	versions := NewVersions(mirror+"/"+family, published)
	for i, release := range versions.Releases {
		versions.Releases[i].SBOM = resourceURL(family, release.Digest, "sbom")
		for j, tag := range release.Tags {
			versions.Releases[i].Tags[j].SBOM = resourceURL(family, tag.Name, "sbom")
		}
	}

	// The mapping itself is the validator: a tag that moved must not be served
	// from a cache, and no single digest here would notice that it had.
	etag := `"` + mappingDigest(versions.Releases) + `"`
	w.Header().Set("ETag", etag)
	// Far shorter than the digest-addressed pages. A fresh push showing up
	// promptly is the whole value of this page.
	w.Header().Set("Cache-Control", "public, max-age=60")
	if strings.Contains(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	var page bytes.Buffer
	if err := pages.ExecuteTemplate(&page, "versions.html", versions); err != nil {
		slog.Error("rendering versions page failed", "family", family, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(page.Bytes()); err != nil {
		slog.Warn("writing versions page failed", "family", family, "error", err)
	}
}

// mappingDigest summarises the tag-to-build mapping a versions page was
// rendered from, so that any tag moving changes the validator.
func mappingDigest(releases []Release) string {
	sum := sha256.New()
	for _, release := range releases {
		for _, tag := range release.Tags {
			sum.Write([]byte(tag.Name + " " + release.Digest + "\n"))
		}
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func serveSBOM(w http.ResponseWriter, r *http.Request, source Source, mirror string) {
	family, ref := r.PathValue("family"), r.PathValue("ref")
	if !validRef(ref) {
		http.Error(w, "not an image reference: "+ref, http.StatusBadRequest)
		return
	}

	digest, components, err := source.SBOM(r.Context(), family, ref)
	if err != nil {
		// Nothing here distinguishes an unknown family from an image with no
		// SBOM attached, and to a reader they are the same absence.
		slog.Warn("sbom lookup failed", "family", family, "ref", ref, "error", err)
		http.Error(w, "no SBOM published for "+pullName(mirror, family, ref), http.StatusNotFound)
		return
	}

	// Shown as the reader would pull it, not as it is stored upstream.
	arch := r.URL.Query().Get("arch")
	table := NewTable(pullName(mirror, family, ref), digest, arch, components)
	table.Download = resourceURL(family, ref, "sbom.json")
	// A page reached by tag can name the exact build it is showing; one
	// reached by digest already is that name.
	if !isDigest(ref) {
		table.Permalink = resourceURL(family, digest, "sbom")
		// The requested architecture, not the resolved one: a permalink that
		// silently changes what is on screen is a worse permalink, and an
		// unqualified page should stay unqualified.
		if arch != "" {
			table.Permalink += "?arch=" + url.QueryEscape(arch)
		}
	}

	// An SBOM is immutable for a Digest, so the digest is the validator: a tag
	// that has not moved costs a request but no body. The architecture is part
	// of it because two architectures are two documents — sharing a validator
	// would let a cache serve one for the other.
	etag := `"` + digest + "-" + table.Arch + `"`
	w.Header().Set("ETag", etag)
	// A page at a digest is immutable data, but not an immutable page: the
	// template changes on deploy, and a digest URL has no cache-busting lever
	// — the digest is the point. So it is long-lived rather than immutable.
	if isDigest(ref) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	if strings.Contains(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Render before writing, so a template failure is a 500 rather than a
	// truncated page under a 200.
	var page bytes.Buffer
	if err := pages.ExecuteTemplate(&page, "sbom.html", table); err != nil {
		slog.Error("rendering sbom page failed", "family", family, "ref", ref, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(page.Bytes()); err != nil {
		slog.Warn("writing sbom page failed", "family", family, "ref", ref, "error", err)
	}
}

// serveDocument hands over the attestation's own bytes.
//
// Not filtered to the architecture the page happens to be showing: the
// signature covers the whole document, and a subset of it is something nobody
// attested to.
func serveDocument(w http.ResponseWriter, r *http.Request, source Source) {
	family, ref := r.PathValue("family"), r.PathValue("ref")
	if !validRef(ref) {
		http.Error(w, "not an image reference: "+ref, http.StatusBadRequest)
		return
	}

	digest, document, err := source.Document(r.Context(), family, ref)
	if err != nil {
		slog.Warn("sbom document lookup failed", "family", family, "ref", ref, "error", err)
		http.Error(w, "no SBOM published for "+family, http.StatusNotFound)
		return
	}

	// One document per Digest, covering every architecture, so unlike the page
	// the architecture is not part of the validator.
	etag := `"` + digest + `"`
	w.Header().Set("ETag", etag)
	// Unlike the page, the document has no presentation to go stale, so at a
	// digest it can be cached for as long as anyone will keep it.
	if isDigest(ref) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	if strings.Contains(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.cyclonedx+json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+downloadName(family, ref, digest)+`"`)
	if _, err := w.Write(document); err != nil {
		slog.Warn("writing sbom document failed", "family", family, "ref", ref, "error", err)
	}
}

// pullName is the image as a reader would pull it. A tag joins with a colon
// and a digest with an at-sign; getting that wrong names an image nobody can
// fetch.
func pullName(mirror, family, ref string) string {
	if isDigest(ref) {
		return mirror + "/" + family + "@" + ref
	}
	return mirror + "/" + family + ":" + ref
}

// familyURL addresses a resource of a whole family rather than of one build.
func familyURL(family, resource string) string {
	return "/directory/image/" + url.PathEscape(family) + "/" + resource
}

// resourceURL addresses one resource of one reference of one image. Escaping
// is per path segment, which leaves the colon of a digest alone — it is legal
// in a segment, and encoding it would make the URL unreadable for no gain.
func resourceURL(family, ref, resource string) string {
	return "/directory/image/" + url.PathEscape(family) + "/" + url.PathEscape(ref) + "/" + resource
}

// downloadName names the file the reader ends up with: what they were looking
// at, plus enough digest to keep two builds of a moving tag from colliding in
// one directory. At a digest the reference already is the digest, so repeating
// it names nothing extra.
func downloadName(family, ref, digest string) string {
	name := nameForFile(family)
	if !isDigest(ref) {
		name += "-" + nameForFile(ref)
	}
	return name + "-" + shortDigest(digest) + ".cdx.json"
}

// nameForFile reduces a reader-supplied path segment to something that can
// only be a filename. It reaches a response header, where a quote or a newline
// would be someone else's syntax rather than a name.
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
