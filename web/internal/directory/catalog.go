package directory

import "html/template"

// Family is one kind of base image the Mirror publishes, as the front page
// lists it.
//
// Declared rather than discovered: GHCR has no catalogue API to ask, and the
// mirror_push targets under //images are what decide the list anyway. A
// family missing here is published but unlisted — a gap in the display, not
// in the evidence, which is reached by URL either way.
type Family struct {
	// Name is the family as a reader addresses it on the mirror: the
	// repository its mirror_push target publishes to, which is not always
	// the directory under //images (node lives in //images/nodejs).
	Name string
	// Summary says in one line what is inside, without naming versions.
	// The versions page is the authority on those, and a number here would
	// go stale next to it.
	Summary string
}

// families is what the Mirror publishes for others to build on, in the order
// the README lists them. The web server's own image is published the same
// way but is nobody's base, so it is not here.
var families = []Family{
	{Name: "bash", Summary: "A shell, for entrypoint scripts and build steps"},
	{Name: "java", Summary: "OpenJDK, one tag per LTS line"},
	{Name: "node", Summary: "Node.js, one tag per release line"},
	{Name: "nginx", Summary: "nginx stable and mainline, serving a webroot as nonroot"},
}

// Card is one Family on the front page and where it leads. It names the
// family alone: the mirror host is said once, in the heading, and every
// card lives under it.
type Card struct {
	Family
	// URL is the family's versions page, which is where a question about a
	// family is answered.
	URL string
	// Logo is the family's mark, or empty for a family without one. Every
	// family listed here has one; the test that renders the index counts
	// them.
	Logo template.HTML
}

// Index is the front page of the directory, ready to render.
type Index struct {
	// Mirror is the host the images are published under.
	Mirror string
	Cards  []Card
	// Topbar is the strip every page shares. Set by the handler.
	Topbar Topbar
}

// NewIndex lists every published family under the mirror a reader pulls by.
func NewIndex(mirror string) *Index {
	cards := make([]Card, 0, len(families))
	for _, family := range families {
		cards = append(cards, Card{
			Family: family,
			URL:    familyURL(family.Name, "versions"),
			Logo:   logo(family.Name),
		})
	}
	return &Index{Mirror: mirror, Cards: cards}
}

// logo is the family's mark, inlined so it is drawn in currentColor and so
// follows the page into dark mode — an <img> cannot inherit a colour. The
// files are this package's own, which is what makes them safe to inline as
// HTML.
//
// Empty for a family without one, which a page renders as no mark at all:
// not every published family is on the front page, and a page must not fail
// for lack of decoration. family is a reader-supplied path segment, but the
// embedded FS is read-only and the lookup can only ever find one of its own
// files.
func logo(family string) template.HTML {
	svg, err := assets.ReadFile("static/logos/" + family + ".svg")
	if err != nil {
		return ""
	}
	return template.HTML(svg)
}

// Topbar is the strip above every page: the mirror, as the way home, and a
// search across every published family.
type Topbar struct {
	// Mirror is the host the images are published under, which is also the
	// name of the way home.
	Mirror string
	// Candidates is every published family, each linking to the view the
	// reader is on — the vulnerabilities of java from the vulnerabilities of
	// nginx — at the family's default tag, on the same architecture. The
	// reader asked to change image, not what they were looking at.
	//
	// Shipped in the page rather than fetched, because there are four and
	// the browser narrows them as the reader types. Without JavaScript they
	// are a menu, which is why they are links and not a datalist.
	Candidates []Candidate
}

// Candidate is one family the search can lead to, drawn as its card is: the
// mark beside the name.
type Candidate struct {
	Name string
	URL  string
	Logo template.HTML
}

// topbar builds the strip for a page showing view, carrying query — the
// architecture, or nothing — onto every candidate.
func topbar(mirror, view, query string) Topbar {
	candidates := make([]Candidate, 0, len(families))
	for _, family := range families {
		candidates = append(candidates, Candidate{
			Name: family.Name,
			URL:  familyURL(family.Name, view) + query,
			Logo: logo(family.Name),
		})
	}
	return Topbar{Mirror: mirror, Candidates: candidates}
}
