// pin: resolves a closed-manifest package list against a live RPM repository
// and writes a JSON lockfile (schema in //rpm/private:lockfile.bzl).
//
// Layout assumptions match Hummingbird's repo (per-arch repodata; noarch
// packages appear in every arch's primary.xml.gz with identical metadata):
//
//	<repo-url>/<arch>/repodata/repomd.xml
//	<repo-url>/<arch>/repodata/<sha>-primary.xml.gz
//	<repo-url>/<arch>/Packages/<initial>/<nvra>.rpm
//
// For each declared arch we fetch repomd, then primary.xml.gz, then walk
// every <package>. Within (name, arch) the highest version (rpmvercmp)
// wins. Noarch packages are committed on the first declared-arch pass and
// skipped on subsequent passes so the canonical "path" prefix is stable.
// Errors on any declared package without an entry (closed-manifest semantics).
//
// GPG verification of repomd.xml.asc is currently stubbed — same posture as
// rpm-extract's --gpg-key flag, tracked alongside it in ADR 0007's TODO list.
package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/sassoftware/go-rpmutils"
)

type repomd struct {
	Revision string       `xml:"revision"`
	Data     []repomdData `xml:"data"`
}

type repomdData struct {
	Type     string       `xml:"type,attr"`
	Checksum xmlValue     `xml:"checksum"`
	Location xmlHrefValue `xml:"location"`
}

type xmlValue struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type xmlHrefValue struct {
	Href string `xml:"href,attr"`
}

type primaryMetadata struct {
	Packages []primaryPackage `xml:"package"`
}

type primaryPackage struct {
	Name     string         `xml:"name"`
	Arch     string         `xml:"arch"`
	Version  primaryVersion `xml:"version"`
	Checksum xmlValue       `xml:"checksum"`
	Size     primarySize    `xml:"size"`
	Location xmlHrefValue   `xml:"location"`
}

type primaryVersion struct {
	Epoch string `xml:"epoch,attr"`
	Ver   string `xml:"ver,attr"`
	Rel   string `xml:"rel,attr"`
}

type primarySize struct {
	Package int64 `xml:"package,attr"`
}

type lockEntry struct {
	Version string `json:"version"`
	Sha256  string `json:"sha256"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
}

type lockRepo struct {
	URL          string `json:"url"`
	Revision     string `json:"revision"`
	RepomdSha256 string `json:"repomd_sha256"`
}

type lockfile struct {
	SchemaVersion int                             `json:"schema_version"`
	Repo          lockRepo                        `json:"repo"`
	Packages      map[string]map[string]lockEntry `json:"packages"`
}

func main() {
	var (
		repoURL = flag.String("repo-url", "", "base URL of the rpm repo (containing per-arch subdirs)")
		gpgKey  = flag.String("gpg-key", "", "ascii-armored public key for repomd.xml.asc verification")
		pkgs    = flag.String("packages", "", "comma-separated closed package manifest")
		arches  = flag.String("architectures", "", "comma-separated declared arches (e.g. x86_64,aarch64)")
		lockOut = flag.String("lock-out", "", "workspace-relative path to lockfile output (e.g. //:hummingbird_install.json)")
	)
	flag.Parse()
	_ = *gpgKey // TODO: verify <repomd>.xml.asc against the supplied key

	if *repoURL == "" || *pkgs == "" || *arches == "" || *lockOut == "" {
		fmt.Fprintln(os.Stderr, "pin: --repo-url, --packages, --architectures, --lock-out are required")
		os.Exit(2)
	}

	declaredArches := splitCSV(*arches)
	declaredPkgs := splitCSV(*pkgs)
	if len(declaredArches) == 0 || len(declaredPkgs) == 0 {
		fmt.Fprintln(os.Stderr, "pin: --packages and --architectures must be non-empty")
		os.Exit(2)
	}

	lock, err := resolve(*repoURL, declaredArches, declaredPkgs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pin:", err)
		os.Exit(1)
	}
	if err := writeLockfile(stripLabelPrefix(*lockOut), lock); err != nil {
		fmt.Fprintln(os.Stderr, "pin: write lockfile:", err)
		os.Exit(1)
	}
}

// resolve walks each declared arch's repodata, picks the highest version per
// (name, arch) across all primary.xml entries matching the closed manifest,
// and errors if any declared package is missing for an expected arch.
func resolve(repoURL string, declaredArches, declaredPkgs []string) (*lockfile, error) {
	repoURL = strings.TrimRight(repoURL, "/")
	wanted := map[string]bool{}
	for _, p := range declaredPkgs {
		wanted[p] = true
	}

	// best[name][arch] = current highest candidate.
	best := map[string]map[string]lockEntry{}
	bestVer := map[string]map[string]string{} // raw EVR string used for vercmp

	var canonicalRevision, canonicalSha string
	for i, arch := range declaredArches {
		repomdURL := fmt.Sprintf("%s/%s/repodata/repomd.xml", repoURL, arch)
		repomdBytes, err := httpGet(repomdURL)
		if err != nil {
			return nil, fmt.Errorf("fetch repomd %s: %w", arch, err)
		}
		sum := sha256.Sum256(repomdBytes)
		if i == 0 {
			canonicalSha = hex.EncodeToString(sum[:])
		}

		var rm repomd
		if err := xml.Unmarshal(repomdBytes, &rm); err != nil {
			return nil, fmt.Errorf("parse repomd %s: %w", arch, err)
		}
		if i == 0 {
			canonicalRevision = rm.Revision
		}

		var primaryHref, primarySha string
		for _, d := range rm.Data {
			if d.Type == "primary" {
				primaryHref = d.Location.Href
				primarySha = d.Checksum.Value
				break
			}
		}
		if primaryHref == "" {
			return nil, fmt.Errorf("repomd %s missing primary data entry", arch)
		}

		primaryURL := fmt.Sprintf("%s/%s/%s", repoURL, arch, primaryHref)
		primaryGz, err := httpGet(primaryURL)
		if err != nil {
			return nil, fmt.Errorf("fetch primary %s: %w", arch, err)
		}
		got := sha256.Sum256(primaryGz)
		if hex.EncodeToString(got[:]) != primarySha {
			return nil, fmt.Errorf("primary.xml.gz sha mismatch for %s: got %s want %s", arch, hex.EncodeToString(got[:]), primarySha)
		}

		gz, err := gzip.NewReader(bytes.NewReader(primaryGz))
		if err != nil {
			return nil, fmt.Errorf("gzip %s: %w", arch, err)
		}
		primaryBytes, err := io.ReadAll(gz)
		gz.Close()
		if err != nil {
			return nil, fmt.Errorf("gunzip %s: %w", arch, err)
		}

		var pm primaryMetadata
		if err := xml.Unmarshal(primaryBytes, &pm); err != nil {
			return nil, fmt.Errorf("parse primary %s: %w", arch, err)
		}

		for _, pkg := range pm.Packages {
			if !wanted[pkg.Name] {
				continue
			}
			// Drop entries for the wrong arch: a primary.xml carries every package
			// available from that mirror — arch-specific ones for the directory's
			// arch plus all noarch — never some other arch's binaries.
			if pkg.Arch != arch && pkg.Arch != "noarch" {
				continue
			}
			// Noarch packages appear identically in every arch's primary. Commit on
			// the first pass and skip thereafter so the lockfile's `path` prefix
			// stays anchored to declaredArches[0] (= canonical).
			if pkg.Arch == "noarch" && i > 0 {
				continue
			}

			evr := formatEVR(pkg.Version)
			if cur, ok := bestVer[pkg.Name][pkg.Arch]; ok && rpmutils.Vercmp(evr, cur) <= 0 {
				continue
			}
			if _, ok := best[pkg.Name]; !ok {
				best[pkg.Name] = map[string]lockEntry{}
				bestVer[pkg.Name] = map[string]string{}
			}
			bestVer[pkg.Name][pkg.Arch] = evr
			best[pkg.Name][pkg.Arch] = lockEntry{
				Version: evr,
				Sha256:  pkg.Checksum.Value,
				Path:    fmt.Sprintf("%s/%s", arch, pkg.Location.Href),
				Size:    pkg.Size.Package,
			}
		}
	}

	if err := validate(best, declaredArches, declaredPkgs); err != nil {
		return nil, err
	}

	return &lockfile{
		SchemaVersion: 1,
		Repo: lockRepo{
			URL:          repoURL,
			Revision:     canonicalRevision,
			RepomdSha256: canonicalSha,
		},
		Packages: best,
	}, nil
}

// validate enforces closed-manifest semantics: every declared package must
// resolve, and arch-specific packages must resolve for every declared arch.
func validate(best map[string]map[string]lockEntry, declaredArches, declaredPkgs []string) error {
	var missing []string
	for _, name := range declaredPkgs {
		archEntries, ok := best[name]
		if !ok || len(archEntries) == 0 {
			missing = append(missing, name)
			continue
		}
		if _, noarch := archEntries["noarch"]; noarch {
			// One entry is enough for a noarch package.
			continue
		}
		for _, arch := range declaredArches {
			if _, ok := archEntries[arch]; !ok {
				missing = append(missing, fmt.Sprintf("%s [missing %s]", name, arch))
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("closed manifest unresolved: %s", strings.Join(missing, ", "))
	}
	return nil
}

// formatEVR renders an RPM version triple as the lockfile string form:
// epoch-prefixed (`<E>:<V>-<R>`) when epoch is non-zero/non-empty, else
// just `<V>-<R>`. Mirrors what dnf/rpm print as the public version.
func formatEVR(v primaryVersion) string {
	if v.Epoch != "" && v.Epoch != "0" {
		return fmt.Sprintf("%s:%s-%s", v.Epoch, v.Ver, v.Rel)
	}
	return fmt.Sprintf("%s-%s", v.Ver, v.Rel)
}

func splitCSV(s string) []string {
	out := []string{}
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// stripLabelPrefix normalises a Bazel label string to a workspace-relative
// path. Accepts "//:foo.json", "//path:foo.json", "@@//:foo.json"
// (canonical form Bazel hands to repo rules under bzlmod), or a bare path.
// The shell wrapper has already cd'd to BUILD_WORKSPACE_DIRECTORY, so a
// workspace-relative path is what we need.
func stripLabelPrefix(label string) string {
	s := strings.TrimPrefix(label, "@@")
	s = strings.TrimPrefix(s, "//")
	if i := strings.Index(s, ":"); i >= 0 {
		pkg := s[:i]
		name := s[i+1:]
		if pkg == "" {
			return name
		}
		return pkg + "/" + name
	}
	return s
}

func httpGet(url string) ([]byte, error) {
	// Hummingbird's CDN 403s HEAD requests and 302s GETs to S3; net/http follows
	// redirects by default. See ADR 0007 §Implementation note (CDN behavior).
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

func writeLockfile(path string, lock *lockfile) error {
	buf, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	// json.MarshalIndent doesn't append a trailing newline; mirror the
	// existing hand-written lockfile which ends with one.
	buf = append(buf, '\n')
	return os.WriteFile(path, buf, 0o644)
}
