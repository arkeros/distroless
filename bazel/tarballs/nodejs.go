package tarballs

import (
	"bufio"
	"context"
	"fmt"
	"strings"
)

// NodeJS resolves release lines against nodejs.org/dist.
type NodeJS struct {
	// BaseURL is the dist root, "https://nodejs.org/dist" in production.
	BaseURL string
}

// nodeArches maps the Bazel arch used in repo names to nodejs.org's
// tarball suffix.
var nodeArches = []struct{ bazel, node string }{
	{"amd64", "x64"},
	{"arm64", "arm64"},
}

// Latest returns the newest release of the given major line. nodejs.org's
// index.json is sorted newest first, so the first entry on the line wins.
func (n *NodeJS) Latest(ctx context.Context, major string) (Line, error) {
	var index []struct {
		Version string `json:"version"`
	}
	if err := getJSON(ctx, n.BaseURL+"/index.json", &index); err != nil {
		return Line{}, err
	}
	version := ""
	for _, r := range index {
		if strings.HasPrefix(r.Version, "v"+major+".") {
			version = strings.TrimPrefix(r.Version, "v")
			break
		}
	}
	if version == "" {
		return Line{}, fmt.Errorf("nodejs: no release of the %s line in index.json", major)
	}

	sums, err := n.shasums(ctx, version)
	if err != nil {
		return Line{}, err
	}
	line := Line{Major: major, Version: version, Archives: map[string]Archive{}}
	for _, a := range nodeArches {
		prefix := fmt.Sprintf("node-v%s-linux-%s", version, a.node)
		file := prefix + ".tar.xz"
		sha, ok := sums[file]
		if !ok {
			return Line{}, fmt.Errorf("nodejs: %s missing from SHASUMS256.txt of v%s", file, version)
		}
		line.Archives["nodejs_"+major+"_"+a.bazel] = Archive{
			URL:         fmt.Sprintf("%s/v%s/%s", n.BaseURL, version, file),
			SHA256:      sha,
			StripPrefix: prefix,
		}
	}
	return line, nil
}

// shasums parses SHASUMS256.txt into filename -> sha256.
func (n *NodeJS) shasums(ctx context.Context, version string) (map[string]string, error) {
	body, err := get(ctx, fmt.Sprintf("%s/v%s/SHASUMS256.txt", n.BaseURL, version))
	if err != nil {
		return nil, err
	}
	defer body.Close()
	sums := map[string]string{}
	sc := bufio.NewScanner(body)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 {
			sums[fields[1]] = fields[0]
		}
	}
	return sums, sc.Err()
}
