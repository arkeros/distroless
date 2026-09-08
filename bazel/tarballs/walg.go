package tarballs

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
)

// WalG resolves release lines against the GitHub releases API of
// wal-g/wal-g. Unlike nodejs.org and Adoptium there is no checksum index:
// each archive has a sibling `<archive>.sha256` asset, fetched per archive.
type WalG struct {
	// BaseURL is the repository API root,
	// "https://api.github.com/repos/wal-g/wal-g" in production.
	BaseURL string
}

// walgUbuntu is the builder image whose binaries we take. wal-g publishes the
// same commit built on 20.04, 22.04 and 24.04; they differ only in the glibc
// they were linked against, and 24.04's 2.39 is below both distros' (Debian
// sid 2.43, Hummingbird 2.42), so the newest build is also the portable one.
const walgUbuntu = "24.04"

// walgArches maps the Bazel arch used in repo names to wal-g's asset suffix.
var walgArches = []struct{ bazel, asset string }{
	{"amd64", "amd64"},
	{"arm64", "aarch64"},
}

// githubRelease is the subset of the GitHub releases API this resolver reads.
type githubRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Latest returns the newest published release of the given major line.
// Drafts and prereleases are skipped, and the highest version wins rather
// than the first listed — the API orders by creation date, which a
// backported patch release on an older line would put out of order.
func (w *WalG) Latest(ctx context.Context, major string) (Line, error) {
	var releases []githubRelease
	if err := getJSON(ctx, w.BaseURL+"/releases", &releases); err != nil {
		return Line{}, err
	}

	var best *githubRelease
	var bestParts []int
	for i, r := range releases {
		if r.Draft || r.Prerelease {
			continue
		}
		parts, ok := walgVersion(r.TagName, major)
		if !ok {
			continue
		}
		if best == nil || compareParts(parts, bestParts) > 0 {
			best, bestParts = &releases[i], parts
		}
	}
	if best == nil {
		return Line{}, fmt.Errorf("wal-g: no published release of the %s line", major)
	}
	version := strings.TrimPrefix(best.TagName, "v")

	assets := map[string]string{}
	for _, a := range best.Assets {
		assets[a.Name] = a.URL
	}

	line := Line{Major: major, Version: version, Archives: map[string]Archive{}}
	for _, a := range walgArches {
		// The Postgres build. wal-g ships one binary per database it backs
		// up; `pg` is the only one this repo has a use for.
		name := fmt.Sprintf("wal-g-pg-%s-%s.tar.gz", walgUbuntu, a.asset)
		url, ok := assets[name]
		if !ok {
			return Line{}, fmt.Errorf("wal-g: %s missing from release %s", name, best.TagName)
		}
		sumURL, ok := assets[name+".sha256"]
		if !ok {
			return Line{}, fmt.Errorf("wal-g: %s.sha256 missing from release %s", name, best.TagName)
		}
		sha, err := walgChecksum(ctx, sumURL)
		if err != nil {
			return Line{}, err
		}
		// No strip_prefix: the archive holds the bare binary, no directory.
		line.Archives["walg_"+major+"_"+a.bazel] = Archive{URL: url, SHA256: sha}
	}
	return line, nil
}

// walgVersion parses `v3.0.9` into its numeric parts, reporting false for a
// tag that is not a plain release of the wanted major line.
func walgVersion(tag, major string) ([]int, bool) {
	fields := strings.Split(strings.TrimPrefix(tag, "v"), ".")
	if len(fields) != 3 || fields[0] != major {
		return nil, false
	}
	parts := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil, false
		}
		parts = append(parts, n)
	}
	return parts, true
}

func compareParts(a, b []int) int {
	for i := range a {
		if a[i] != b[i] {
			return a[i] - b[i]
		}
	}
	return 0
}

// walgChecksum reads the `<sha256>  <filename>` line of a sibling .sha256
// asset.
func walgChecksum(ctx context.Context, url string) (string, error) {
	body, err := get(ctx, url)
	if err != nil {
		return "", err
	}
	defer body.Close()
	sc := bufio.NewScanner(body)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) == 2 {
			return fields[0], nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("wal-g: %s holds no checksum line", url)
}
