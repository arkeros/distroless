package snapshot

import (
	"fmt"
	"os"
	"regexp"

	"go.yaml.in/yaml/v4"
)

var (
	timestampRegex          = regexp.MustCompile(`\d{8}T\d{6}Z`)
	debianURLRegex          = regexp.MustCompile(`(/archive/debian/)\d{8}T\d{6}Z`)
	debianSecurityURLRegex  = regexp.MustCompile(`(/archive/debian-security/)\d{8}T\d{6}Z`)
)

type Source struct {
	Channel string   `yaml:"channel"`
	URL     string   `yaml:"url,omitempty"`
	URLs    []string `yaml:"urls,omitempty"`
}

type Manifest struct {
	Version  int      `yaml:"version"`
	Sources  []Source `yaml:"sources"`
	Archs    []string `yaml:"archs"`
	Packages []string `yaml:"packages"`
}

func ParseManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	if m.Version == 0 {
		return nil, fmt.Errorf("invalid manifest: missing or zero version")
	}
	if len(m.Sources) == 0 {
		return nil, fmt.Errorf("invalid manifest: sources must not be empty")
	}
	if len(m.Archs) == 0 {
		return nil, fmt.Errorf("invalid manifest: archs must not be empty")
	}
	if len(m.Packages) == 0 {
		return nil, fmt.Errorf("invalid manifest: packages must not be empty")
	}
	for i, src := range m.Sources {
		if src.Channel == "" {
			return nil, fmt.Errorf("invalid manifest: sources[%d].channel must not be empty", i)
		}
		if src.URL == "" && len(src.URLs) == 0 {
			return nil, fmt.Errorf("invalid manifest: sources[%d] must have url or urls", i)
		}
	}

	return &m, nil
}

func UpdateSnapshotURL(url, newTimestamp string) string {
	return timestampRegex.ReplaceAllString(url, newTimestamp)
}

// UpdateTimestampsInFile reads the manifest at path, replaces snapshot
// timestamps in URLs (debianTimestamp for /archive/debian/ URLs,
// securityTimestamp for /archive/debian-security/ URLs), and writes the
// file back. Surgical text replacement preserves all comments and
// formatting.
func UpdateTimestampsInFile(path, debianTimestamp, securityTimestamp string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}
	updated := debianURLRegex.ReplaceAll(data, []byte("${1}"+debianTimestamp))
	updated = debianSecurityURLRegex.ReplaceAll(updated, []byte("${1}"+securityTimestamp))
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}
	return nil
}
