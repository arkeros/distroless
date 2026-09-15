package tarballs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// GitHub reads one repository's releases and the files attached to them.
// It knows about GitHub and nothing about any particular upstream: which
// tag belongs to a release line, what the assets are called and how their
// checksums are published are the resolver's business (see envoy.go).
type GitHub struct {
	// BaseURL is the API root, "https://api.github.com" in production.
	BaseURL     string
	Owner, Repo string
	// PerPage is the release page size; 100 (the API's maximum) when
	// unset. Tests lower it to exercise paging.
	PerPage int
}

// Release is one published release and what is attached to it.
type Release struct {
	TagName    string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// Asset is one file attached to a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// releasePageCap bounds the walk. A repository with more releases than
// this fails loudly rather than pinning whatever the walk happened to
// reach: envoyproxy/envoy is at a few hundred, so hitting the cap means
// the shape of the API changed, not that the project got busy.
const releasePageCap = 10

// LatestRelease returns the release of one line: the highest patch among
// the tags starting with prefix, e.g. "v1.39." for envoy's 1.39 line.
//
// Every page is read rather than trusting the API's newest-first order,
// so a release republished out of order cannot win. Drafts (whose assets
// are not public) and prereleases are not releases to pin, and a tag whose
// remainder past the prefix is not a plain number is not a patch of the
// line.
func (g *GitHub) LatestRelease(ctx context.Context, prefix string) (Release, error) {
	perPage := g.PerPage
	if perPage == 0 {
		perPage = 100
	}

	best, bestPatch := Release{}, -1
	for page := 1; ; page++ {
		if page > releasePageCap {
			return Release{}, fmt.Errorf("github %s/%s: no release matching %q in the first %d pages of releases",
				g.Owner, g.Repo, prefix, releasePageCap)
		}
		url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=%d&page=%d", g.BaseURL, g.Owner, g.Repo, perPage, page)
		var releases []Release
		if err := getJSON(ctx, url, &releases, g.authorize); err != nil {
			return Release{}, err
		}
		for _, release := range releases {
			if release.Draft || release.Prerelease || !strings.HasPrefix(release.TagName, prefix) {
				continue
			}
			patch, err := strconv.Atoi(strings.TrimPrefix(release.TagName, prefix))
			if err != nil || patch <= bestPatch {
				continue
			}
			best, bestPatch = release, patch
		}
		if len(releases) < perPage {
			break
		}
	}
	if bestPatch < 0 {
		return Release{}, fmt.Errorf("github %s/%s: no release tagged %s<patch>", g.Owner, g.Repo, prefix)
	}
	return best, nil
}

// Asset returns the release's asset of that exact name.
func (r Release) Asset(name string) (Asset, error) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, nil
		}
	}
	return Asset{}, fmt.Errorf("release %s has no asset named %s", r.TagName, name)
}

// Fetch reads an asset's bytes. Asset downloads redirect to a storage host
// that takes no token, and Go drops the Authorization header across that
// hop, so they go out unauthenticated either way.
func (g *GitHub) Fetch(ctx context.Context, asset Asset) (io.ReadCloser, error) {
	return get(ctx, asset.URL)
}

// authorize attaches a token when the environment has one. api.github.com
// allows 60 requests an hour unauthenticated and 5000 with a token; the
// update workflow runs with one, a developer running `knife tarballs
// update` by hand may not.
func (g *GitHub) authorize(req *http.Request) {
	for _, name := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if token := os.Getenv(name); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
			return
		}
	}
}
