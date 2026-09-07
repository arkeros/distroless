package tarballs

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Temurin resolves release lines against the Adoptium v3 API.
type Temurin struct {
	// BaseURL is the API root, "https://api.adoptium.net/v3" in production.
	BaseURL string
}

var temurinArches = []struct{ bazel, adoptium string }{
	{"amd64", "x64"},
	{"arm64", "aarch64"},
}

// Order matters for strip_prefix: the JRE tarball unpacks to
// `jdk-<version>+<build>-jre`, the JDK to `jdk-<version>+<build>`.
var temurinImageTypes = []string{"jre", "jdk"}

// Latest returns the newest Temurin release of the given major, with the
// JRE and JDK for both arches. All four assets must belong to the same
// release: a half-published release (the API serves the new JRE before the
// new JDK, or one arch before the other) is an error, so the caller keeps
// the previous, consistent pin until the next run.
func (t *Temurin) Latest(ctx context.Context, major string) (Line, error) {
	line := Line{Major: major, Archives: map[string]Archive{}}
	release := ""
	for _, imageType := range temurinImageTypes {
		for _, arch := range temurinArches {
			asset, err := t.latestAsset(ctx, major, imageType, arch.adoptium)
			if err != nil {
				return Line{}, err
			}
			if release == "" {
				release = asset.ReleaseName
			} else if asset.ReleaseName != release {
				return Line{}, fmt.Errorf("temurin %s: %s %s is at %s while earlier assets are at %s; release still rolling out",
					major, imageType, arch.adoptium, asset.ReleaseName, release)
			}
			stripPrefix := release
			if imageType == "jre" {
				stripPrefix += "-jre"
			}
			line.Archives[fmt.Sprintf("temurin_%s_%s_%s", imageType, major, arch.bazel)] = Archive{
				URL:         asset.Binary.Package.Link,
				SHA256:      asset.Binary.Package.Checksum,
				StripPrefix: stripPrefix,
			}
		}
	}

	// release_name is `jdk-<version>+<build>`, e.g. jdk-21.0.12.1+1.
	version, build, ok := strings.Cut(strings.TrimPrefix(release, "jdk-"), "+")
	if !ok {
		return Line{}, fmt.Errorf("temurin %s: unexpected release name %q", major, release)
	}
	line.Version, line.Build = version, build
	return line, nil
}

type temurinAsset struct {
	ReleaseName string `json:"release_name"`
	Binary      struct {
		Package struct {
			Checksum string `json:"checksum"`
			Link     string `json:"link"`
		} `json:"package"`
	} `json:"binary"`
}

func (t *Temurin) latestAsset(ctx context.Context, major, imageType, arch string) (temurinAsset, error) {
	q := url.Values{
		"architecture": {arch},
		"image_type":   {imageType},
		"os":           {"linux"},
		"vendor":       {"eclipse"},
	}
	u := fmt.Sprintf("%s/assets/latest/%s/hotspot?%s", t.BaseURL, major, q.Encode())
	var assets []temurinAsset
	if err := getJSON(ctx, u, &assets); err != nil {
		return temurinAsset{}, err
	}
	if len(assets) != 1 {
		return temurinAsset{}, fmt.Errorf("temurin %s: expected one %s %s asset, got %d", major, imageType, arch, len(assets))
	}
	return assets[0], nil
}
