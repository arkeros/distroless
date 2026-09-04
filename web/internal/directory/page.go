// Package directory serves the human-readable view of the Mirror: what is
// inside a published image, read off the evidence attached to its Digest.
//
// What an image *contains* is always read from an Attestation verified first,
// and so is what a scanner *found* in it. What a tag currently *names* cannot
// be: a tag-to-digest mapping is registry metadata that nobody signs. The
// versions page shows that mapping and says so; every other page here is
// rendered from verified evidence.
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
	"slices"
	"strings"
)

//go:embed templates static/directory.css static/directory.mjs static/main.mjs static/fonts
var assets embed.FS

// Parsed once at startup so a broken template fails the process rather than a
// request. The tests in this package execute it against real data, which is
// what catches a renamed field.
var pages = template.Must(template.ParseFS(assets, "templates/*.html"))

// Source resolves an image reference to the evidence attached to its Index.
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
	// Scan returns the verified vulnerability scan attached to an image: what
	// a scanner matched against its SBOM, and when. A Finding that a published
	// VEX statement covers comes back marked as suppressed rather than
	// dropped.
	Scan(ctx context.Context, family, ref string) (digest string, scan *Scan, err error)
	// ScanDocument returns the verified vulnerability attestation exactly as
	// it was signed, as Document does for the SBOM.
	ScanDocument(ctx context.Context, family, ref string) (digest string, document []byte, err error)
	// Versions lists every tag published for a family and the build each one
	// names. Unverified by nature — see the package comment.
	Versions(ctx context.Context, family string) ([]Version, error)
	// Tags lists the tags a family publishes, and nothing else about them.
	// Separate from Versions because a page that only needs the names should
	// not pay for a digest lookup per tag: this is one registry call.
	Tags(ctx context.Context, family string) ([]string, error)
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

// The two views of one build, and the download behind each. A view's download
// is the view's name with .json on the end, which is what lets one redirect
// table and one link builder serve both.
const (
	viewSBOM            = "sbom"
	viewVulnerabilities = "vulnerabilities"
)

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
		serveDocument(w, r, source.Document, mirror, attested{
			what: "SBOM", contentType: "application/vnd.cyclonedx+json", extension: ".cdx.json",
		})
	})
	mux.HandleFunc("GET /directory/image/{family}/{ref}/vulnerabilities", func(w http.ResponseWriter, r *http.Request) {
		serveVulnerabilities(w, r, source, mirror)
	})
	mux.HandleFunc("GET /directory/image/{family}/{ref}/vulnerabilities.json", func(w http.ResponseWriter, r *http.Request) {
		// Cosign's vulnerability predicate has no media type of its own.
		serveDocument(w, r, source.ScanDocument, mirror, attested{
			what: "vulnerability scan", contentType: "application/json", extension: ".vuln.json",
		})
	})
	mux.HandleFunc("GET /directory/image/{family}/versions", func(w http.ResponseWriter, r *http.Request) {
		serveVersions(w, r, source, mirror)
	})
	for _, resource := range []string{viewSBOM, viewSBOM + ".json", viewVulnerabilities, viewVulnerabilities + ".json"} {
		mux.HandleFunc("GET /directory/image/{family}/"+resource, permanentRedirect(func(family string) string {
			return resourceURL(family, defaultRef, resource)
		}))
	}
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
	versions.SBOM = resourceURL(family, defaultRef, viewSBOM)
	versions.Vulnerabilities = resourceURL(family, defaultRef, viewVulnerabilities)
	for i, release := range versions.Releases {
		versions.Releases[i].SBOM = resourceURL(family, release.Digest, viewSBOM)
		versions.Releases[i].Vulnerabilities = resourceURL(family, release.Digest, viewVulnerabilities)
		for j, tag := range release.Tags {
			versions.Releases[i].Tags[j].URL = resourceURL(family, tag.Name, viewSBOM)
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

	writePage(w, "versions.html", versions, family, "")
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
	table.Links = links(r, source, family, ref, digest, arch, viewSBOM)

	// An SBOM is immutable for a Digest, so the digest is the validator: a
	// tag that has not moved costs a request but no body.
	if !revalidated(w, r, ref, digest+"-"+table.Arch) {
		writePage(w, "sbom.html", table, family, ref)
	}
}

// serveVulnerabilities shows what a scanner found in one build.
//
// An image with no scan attached is a 404 and says so, the way an image with
// no SBOM is. It must not be an empty table: an empty table reads as a clean
// result, and "not scanned" is the opposite of one.
func serveVulnerabilities(w http.ResponseWriter, r *http.Request, source Source, mirror string) {
	family, ref := r.PathValue("family"), r.PathValue("ref")
	if !validRef(ref) {
		http.Error(w, "not an image reference: "+ref, http.StatusBadRequest)
		return
	}

	digest, scan, err := source.Scan(r.Context(), family, ref)
	if err != nil {
		slog.Warn("scan lookup failed", "family", family, "ref", ref, "error", err)
		http.Error(w, "no vulnerability scan published for "+pullName(mirror, family, ref), http.StatusNotFound)
		return
	}

	arch := r.URL.Query().Get("arch")
	report := NewReport(pullName(mirror, family, ref), digest, arch, scan)
	report.Links = links(r, source, family, ref, digest, arch, viewVulnerabilities)

	// Unlike an SBOM, what this page shows is not immutable for a Digest: the
	// same build can be scanned again, and its VEX document reissued, and
	// either replaces what the page says. So the validator carries a
	// fingerprint of the content too, or a cache would hold the old page for
	// as long as the digest lived.
	if !revalidated(w, r, ref, digest+"-"+report.Arch+"-"+scan.Fingerprint()) {
		writePage(w, "vulnerabilities.html", report, family, ref)
	}
}

// revalidated sets the validator and cache policy every page for one build
// shares, and reports whether the reader's copy was still good — in which case
// it has already been answered with no body.
//
// validator is whatever the page's content is a pure function of: the digest
// at least, and the architecture, because two architectures are two documents
// and sharing a validator would let a cache serve one for the other.
func revalidated(w http.ResponseWriter, r *http.Request, ref, validator string) bool {
	etag := `"` + validator + `"`
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
		return true
	}
	return false
}

// writePage renders a template and sends it. Rendered before writing, so a
// template failure is a 500 rather than a truncated page under a 200.
func writePage(w http.ResponseWriter, name string, data any, family, ref string) {
	var page bytes.Buffer
	if err := pages.ExecuteTemplate(&page, name, data); err != nil {
		slog.Error("rendering page failed", "template", name, "family", family, "ref", ref, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(page.Bytes()); err != nil {
		slog.Warn("writing page failed", "template", name, "family", family, "ref", ref, "error", err)
	}
}

// attested describes one kind of downloadable evidence.
type attested struct {
	// what it is called in an error, e.g. "SBOM".
	what string
	// contentType it is served as.
	contentType string
	// extension the downloaded file gets, e.g. ".cdx.json".
	extension string
}

// serveDocument hands over an attestation's own bytes.
//
// Not filtered to the architecture the page happens to be showing: the
// signature covers the whole document, and a subset of it is something nobody
// attested to.
func serveDocument(w http.ResponseWriter, r *http.Request, fetch func(context.Context, string, string) (string, []byte, error), mirror string, kind attested) {
	family, ref := r.PathValue("family"), r.PathValue("ref")
	if !validRef(ref) {
		http.Error(w, "not an image reference: "+ref, http.StatusBadRequest)
		return
	}

	digest, document, err := fetch(r.Context(), family, ref)
	if err != nil {
		slog.Warn("document lookup failed", "what", kind.what, "family", family, "ref", ref, "error", err)
		http.Error(w, "no "+kind.what+" published for "+pullName(mirror, family, ref), http.StatusNotFound)
		return
	}

	// One document per Digest, covering every architecture, so unlike the page
	// the architecture is not part of the validator. The content is, because a
	// scan record can be replaced by a re-scan without the digest changing;
	// for an SBOM, immutable for its digest, the two say the same thing.
	sum := sha256.Sum256(document)
	etag := `"` + digest + "-" + hex.EncodeToString(sum[:6]) + `"`
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

	w.Header().Set("Content-Type", kind.contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+downloadName(family, ref, digest, kind.extension)+`"`)
	if _, err := w.Write(document); err != nil {
		slog.Warn("writing document failed", "what", kind.what, "family", family, "ref", ref, "error", err)
	}
}

// links builds the ways in and out of a page for one build.
//
// arch is the architecture the reader asked for, not the one resolved: a
// permalink that silently changes what is on screen is a worse permalink, and
// an unqualified page should stay unqualified. It is carried on every link so
// that switching view or tag does not silently switch architecture too.
func links(r *http.Request, source Source, family, ref, digest, arch, view string) Links {
	query := ""
	if arch != "" {
		query = "?arch=" + url.QueryEscape(arch)
	}
	built := Links{
		Download:        resourceURL(family, ref, view+".json"),
		SBOM:            resourceURL(family, ref, viewSBOM) + query,
		Vulnerabilities: resourceURL(family, ref, viewVulnerabilities) + query,
		Versions:        familyURL(family, "versions"),
		Showing:         ref,
	}
	// A page reached by tag can name the exact build it is showing; one
	// reached by digest already is that name.
	if isDigest(ref) {
		built.Showing = shortDigest(ref)
	} else {
		built.Permalink = resourceURL(family, digest, view) + query
	}
	built.Siblings = siblings(r, source, family, view, query)
	return built
}

// siblings lists the family's other tags as links to this same view.
//
// A failure here is deliberately swallowed: the reader came for the evidence,
// and losing the ability to jump between tags is not worth turning a rendered
// page into an error. Nothing is returned for a single tag, which is not a
// choice.
func siblings(r *http.Request, source Source, family, view, query string) []Tag {
	tags, err := source.Tags(r.Context(), family)
	if err != nil {
		slog.Warn("tag listing failed", "family", family, "error", err)
		return nil
	}
	if len(tags) < 2 {
		return nil
	}

	// A registry lists tags alphabetically, which puts 17 above 25. Ordered
	// by the same rules as the versions page, so the two agree about what
	// "first" means.
	slices.SortFunc(tags, compareTag)

	siblings := make([]Tag, 0, len(tags))
	for _, tag := range tags {
		siblings = append(siblings, Tag{Name: tag, URL: resourceURL(family, tag, view) + query})
	}
	return siblings
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
func downloadName(family, ref, digest, extension string) string {
	name := nameForFile(family)
	if !isDigest(ref) {
		name += "-" + nameForFile(ref)
	}
	return name + "-" + shortDigest(digest) + extension
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
		switch {
		case strings.HasSuffix(r.URL.Path, ".woff2"):
			w.Header().Set("Content-Type", "font/woff2")
		case strings.HasSuffix(r.URL.Path, ".mjs"):
			// A browser will not execute a module served as anything else,
			// and Go's built-in table has no .mjs entry at all.
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		}

		next.ServeHTTP(w, r)
	})
}
