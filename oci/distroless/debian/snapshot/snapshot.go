package snapshot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v4"
)

var (
	timestampRegex         = regexp.MustCompile(`\d{8}T\d{6}Z`)
	debianURLRegex         = regexp.MustCompile(`(/archive/debian/)\d{8}T\d{6}Z`)
	debianSecurityURLRegex = regexp.MustCompile(`(/archive/debian-security/)\d{8}T\d{6}Z`)
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

// Purger sends a Fastly-style HTTP PURGE for a single URL. snapshot.debian.org
// is Fastly-fronted; PURGE invalidates the cached redirect across all edges
// within ~150ms, ensuring the next fetch goes back to origin instead of
// returning a stale `Packages.xz` blob from one edge while another edge
// already has the newer one.
type Purger interface {
	Purge(ctx context.Context, url string) error
}

// HTTPPurger sends `PURGE` requests via an http.Client. Default timeout 15s.
type HTTPPurger struct {
	Client *http.Client
}

func (p *HTTPPurger) Purge(ctx context.Context, url string) error {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, "PURGE", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PURGE returned %d", resp.StatusCode)
	}
	return nil
}

// PackageIndexURLs derives the per-(channel, component, arch, format) Packages
// URLs that rules_distroless will fetch when resolving the manifest. Sources
// with a single URL are treated identically to single-element URLs lists.
// Format extensions match rules_distroless's `_INDEX_FORMATS` preference order.
func PackageIndexURLs(m *Manifest) []string {
	var out []string
	for _, src := range m.Sources {
		bases := src.URLs
		if src.URL != "" {
			bases = append(bases, src.URL)
		}
		dist, components := splitChannel(src.Channel)
		if dist == "" || len(components) == 0 {
			continue
		}
		for _, base := range bases {
			base = strings.TrimRight(base, "/")
			for _, comp := range components {
				for _, arch := range m.Archs {
					for _, ext := range []string{".xz", ".gz", ".bz2"} {
						out = append(out, fmt.Sprintf(
							"%s/dists/%s/%s/binary-%s/Packages%s",
							base, dist, comp, arch, ext))
					}
				}
			}
		}
	}
	return out
}

// PurgePackagesIndexes fan-outs PURGE for every package index URL derivable
// from the manifest, in parallel. Best-effort: returns the joined errors
// (if any) but the caller may choose to continue regardless. Only URLs that
// are CDN-fronted by a service that accepts anonymous PURGE are worth
// purging — currently only `snapshot.debian.org` (Fastly).
func PurgePackagesIndexes(ctx context.Context, m *Manifest, purger Purger) error {
	urls := PackageIndexURLs(m)

	var wg sync.WaitGroup
	errs := make([]error, len(urls))
	for i, url := range urls {
		if !purgeable(url) {
			continue
		}
		wg.Add(1)
		go func(i int, url string) {
			defer wg.Done()
			if err := purger.Purge(ctx, url); err != nil {
				errs[i] = fmt.Errorf("PURGE %s: %w", url, err)
			}
		}(i, url)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// purgeable reports whether anonymous Fastly-style PURGE is known to work for
// this URL. snapshot.debian.org is Fastly. snapshot-cloudflare.debian.org is
// Cloudflare and rejects anonymous purge. nginx.org and other arbitrary hosts
// are skipped.
func purgeable(url string) bool {
	return strings.Contains(url, "://snapshot.debian.org/")
}

// splitChannel splits an apt channel string into (dist, components). The first
// whitespace-separated token is the dist; the rest are components. Returns
// ("", nil) for empty input.
func splitChannel(channel string) (dist string, components []string) {
	parts := strings.Fields(channel)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}
