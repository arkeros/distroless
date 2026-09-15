package tarballs

import (
	"bufio"
	"context"
	"fmt"
	"path"
	"strings"
)

// Envoy resolves release lines against envoyproxy/envoy's GitHub releases.
//
// The releases attach the envoy binary itself — the same bytes as
// /usr/bin/envoy in that release's deb, verified byte for byte at 1.39.1 —
// so the lockfile pins a bare file rather than an archive. apt.envoyproxy.io
// is not an option: its Release is dated 2024-12-08 and its newest package
// is 1.32.2.
type Envoy struct {
	GitHub GitHub
}

// envoyArches maps the Bazel arch used in repo names to the suffix of the
// release asset. Note `aarch_64`, with the underscore upstream writes.
var envoyArches = []struct{ bazel, asset string }{
	{"amd64", "x86_64"},
	{"arm64", "aarch_64"},
}

// envoyChecksums is the asset carrying every asset's SHA256, clearsigned.
// The signature is not verified, the same posture nodejs.go takes with the
// .asc beside SHASUMS256.txt: trust is TLS to github.com plus a checksum
// reviewed into the lockfile, which Bazel then enforces on every fetch.
const envoyChecksums = "checksums.txt.asc"

// Latest returns the newest release of the given line. `major` is the
// line as the lockfile writes it, "1.39", and the repo names strip its dot
// — `envoy_139_amd64` — since a Bazel repo name may not contain one.
func (e *Envoy) Latest(ctx context.Context, major string) (Line, error) {
	release, err := e.GitHub.LatestRelease(ctx, "v"+major+".")
	if err != nil {
		return Line{}, err
	}
	version := strings.TrimPrefix(release.TagName, "v")

	sums, err := e.checksums(ctx, release)
	if err != nil {
		return Line{}, err
	}
	line := Line{Major: major, Version: version, Archives: map[string]Archive{}}
	for _, arch := range envoyArches {
		name := fmt.Sprintf("envoy-%s-linux-%s", version, arch.asset)
		asset, err := release.Asset(name)
		if err != nil {
			return Line{}, fmt.Errorf("envoy %s: %w", major, err)
		}
		sha, ok := sums[name]
		if !ok {
			return Line{}, fmt.Errorf("envoy %s: %s missing from %s of %s", major, name, envoyChecksums, release.TagName)
		}
		line.Archives[fmt.Sprintf("envoy_%s_%s", strings.ReplaceAll(major, ".", ""), arch.bazel)] = Archive{
			URL:    asset.URL,
			SHA256: sha,
			File:   "envoy",
		}
	}
	return line, nil
}

// checksums parses the release's clearsigned checksum file into asset name
// -> sha256. Its entries are keyed by the builder's own absolute paths
// (`/tmp/tmp.XXXX/envoy-1.39.1-linux-x86_64`), so the key is the base name;
// the armor around them has no line of the shape a checksum takes.
func (e *Envoy) checksums(ctx context.Context, release Release) (map[string]string, error) {
	asset, err := release.Asset(envoyChecksums)
	if err != nil {
		return nil, err
	}
	body, err := e.GitHub.Fetch(ctx, asset)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	sums := map[string]string{}
	sc := bufio.NewScanner(body)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && isSHA256(fields[0]) {
			sums[path.Base(fields[1])] = fields[0]
		}
	}
	return sums, sc.Err()
}

func isSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	return strings.Trim(s, "0123456789abcdef") == ""
}
