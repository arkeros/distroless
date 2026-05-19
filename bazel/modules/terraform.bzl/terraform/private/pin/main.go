// Command pin regenerates the JSON lockfile pointed to by `-lock` from the
// `-provider=<source>@<constraint>` flags repeated on the command line.
//
// For each declared provider it:
//  1. Fetches HashiCorp's `index.json` for the provider type (lists every
//     released version).
//  2. Picks the highest version satisfying the constraint.
//  3. Fetches the `SHA256SUMS` for that version (zip-level sha256s).
//  4. Downloads each platform's zip to compute the `h1:` directory hash
//     terraform records in `.terraform.lock.hcl`.
//  5. Writes a deterministic JSON document keyed by `source`.
//
// Invocation comes via `bazel run @<install>//:pin` — Bazel sets
// `BUILD_WORKSPACE_DIRECTORY` so the tool finds the workspace root
// regardless of the cwd from which it was invoked.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	"golang.org/x/mod/sumdb/dirhash"
)

// Platforms we pin every provider for. Mirrors `_PROVIDER_PLATFORMS` in
// the module extension. Keep in sync.
var platforms = []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"}

type providerSpec struct {
	source     string // e.g. "hashicorp/google"
	constraint string // e.g. "~> 7.0", "1.0.0", "*"
}

func (p providerSpec) ptype() (string, error) {
	parts := strings.SplitN(p.source, "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", fmt.Errorf("source must be `<namespace>/<type>`, got %q", p.source)
	}
	return parts[1], nil
}

// hashEntry is the per-platform pin row written to the lockfile.
type hashEntry struct {
	Sha256 string `json:"sha256"` // hex
	H1     string `json:"h1"`     // "h1:<base64-of-sha256>"
}

// resolved is one provider's locked state: the constraint that resolved
// to `Version`, plus per-platform hashes.
type resolved struct {
	Constraint string               `json:"constraint"`
	Version    string               `json:"version"`
	Platforms  map[string]hashEntry `json:"platforms"`
}

// stringList collects repeated `-provider=…` flags.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func main() {
	var (
		lockRel       string
		providerSpecs stringList
	)
	flag.StringVar(&lockRel, "lock", "", "workspace-relative path of the JSON lockfile to (re)generate")
	flag.Var(&providerSpecs, "provider", "<source>@<constraint>, repeatable; the declared provider set with terraform-style version constraints")
	flag.Parse()

	if lockRel == "" {
		die("missing -lock=<workspace-relative-path>")
	}

	workdir := workspaceRoot()
	lockPath := filepath.Join(workdir, lockRel)

	providers, err := parseProviderFlags(providerSpecs)
	if err != nil {
		die("%v", err)
	}
	if len(providers) == 0 {
		if err := writeLockFile(lockPath, nil); err != nil {
			die("write lock: %v", err)
		}
		fmt.Println("pin: no providers declared; wrote empty lock file.")
		return
	}

	// Dedup by source (last constraint wins) — multiple installs sharing one
	// lockfile may name the same provider twice; refuse contradictions early
	// instead of producing a non-deterministic write order.
	seen := map[string]string{}
	deduped := providers[:0]
	for _, p := range providers {
		if prev, ok := seen[p.source]; ok {
			if prev != p.constraint {
				die("provider %s declared with two constraints: %q and %q", p.source, prev, p.constraint)
			}
			continue
		}
		seen[p.source] = p.constraint
		deduped = append(deduped, p)
	}

	out := map[string]resolved{}
	for _, p := range deduped {
		fmt.Fprintf(os.Stderr, "pin: resolving %s (%s)\n", p.source, p.constraint)
		entry, err := resolveAndFetch(p)
		if err != nil {
			die("resolve %s: %v", p.source, err)
		}
		out[p.source] = entry
	}

	if err := writeLockFile(lockPath, out); err != nil {
		die("write lock: %v", err)
	}
	fmt.Printf("pin: pinned %d provider(s) → %s\n", len(deduped), lockRel)
}

func workspaceRoot() string {
	if w := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); w != "" {
		return w
	}
	w, err := os.Getwd()
	if err != nil {
		die("getwd: %v", err)
	}
	return w
}

// parseProviderFlags turns `["hashicorp/google@~> 7.0", ...]` into the
// internal `providerSpec` list, preserving order.
//
// Uses `strings.Index` on the FIRST `@` (not last) because constraint
// strings can contain `>` / `=` but never `@`, so the boundary is
// unambiguous on the left.
func parseProviderFlags(specs []string) ([]providerSpec, error) {
	out := make([]providerSpec, 0, len(specs))
	for _, s := range specs {
		at := strings.Index(s, "@")
		if at <= 0 || at == len(s)-1 {
			return nil, fmt.Errorf("-provider=%q must look like <source>@<constraint>", s)
		}
		out = append(out, providerSpec{
			source:     s[:at],
			constraint: strings.TrimSpace(s[at+1:]),
		})
	}
	return out, nil
}

// parseConstraint parses a terraform-style constraint string with one
// extension: `*` (or the empty string) means "any released version" —
// go-version doesn't accept those natively, so we expand to `>= 0`.
// Useful for "track latest" pins where the lockfile is the source of
// truth for the resolved version.
func parseConstraint(raw string) (version.Constraints, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "*" {
		trimmed = ">= 0"
	}
	c, err := version.NewConstraint(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid constraint %q: %w", raw, err)
	}
	return c, nil
}

// resolveAndFetch: fetch the provider's release index, pick the newest
// version satisfying the constraint, then download SHA256SUMS + each
// platform zip to compute hashes.
func resolveAndFetch(p providerSpec) (resolved, error) {
	ptype, err := p.ptype()
	if err != nil {
		return resolved{}, err
	}

	constraint, err := parseConstraint(p.constraint)
	if err != nil {
		return resolved{}, err
	}

	available, err := fetchVersions(ptype)
	if err != nil {
		return resolved{}, fmt.Errorf("fetch index: %w", err)
	}

	picked, err := pickHighestMatching(available, constraint)
	if err != nil {
		return resolved{}, err
	}

	entries, err := fetchHashes(ptype, picked)
	if err != nil {
		return resolved{}, err
	}

	return resolved{
		Constraint: p.constraint,
		Version:    picked,
		Platforms:  entries,
	}, nil
}

// releaseIndex is the on-wire shape of releases.hashicorp.com's per-product
// index.json — `versions` is keyed by version string; we only need the keys.
type releaseIndex struct {
	Versions map[string]json.RawMessage `json:"versions"`
}

// fetchVersions returns the list of released version strings for one
// provider type (e.g. "google").
func fetchVersions(ptype string) ([]string, error) {
	url := fmt.Sprintf("https://releases.hashicorp.com/terraform-provider-%s/index.json", ptype)
	body, err := httpGet(url)
	if err != nil {
		return nil, err
	}
	var idx releaseIndex
	if err := json.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("decode index.json: %w", err)
	}
	out := make([]string, 0, len(idx.Versions))
	for v := range idx.Versions {
		out = append(out, v)
	}
	return out, nil
}

// pickHighestMatching returns the highest version string in `available`
// that satisfies the constraint. Prerelease versions (anything with a
// `-` suffix per semver) are skipped — terraform's own resolver does the
// same by default, and pinning a `-rc.1` from a `~> X.Y` constraint
// would be surprising.
func pickHighestMatching(available []string, constraint version.Constraints) (string, error) {
	var best *version.Version
	for _, raw := range available {
		v, err := version.NewVersion(raw)
		if err != nil {
			continue
		}
		if v.Prerelease() != "" {
			continue
		}
		if !constraint.Check(v) {
			continue
		}
		if best == nil || v.GreaterThan(best) {
			best = v
		}
	}
	if best == nil {
		return "", fmt.Errorf("no released version satisfies constraint %s", constraint.String())
	}
	return best.Original(), nil
}

// fetchHashes downloads the SHA256SUMS for the resolved version (for the
// zip-level sha256 that download_and_extract consumes), then downloads
// each platform's zip to compute the `h1:` directory hash terraform's
// lockfile records.
//
// The two hashes attest different things and are NOT interconvertible:
// sha256 hashes the zip bytes; h1 (Go's directory hash format) hashes the
// unpacked-file manifest. Both end up in the lockfile so neither side
// has to recompute downstream.
func fetchHashes(ptype, ver string) (map[string]hashEntry, error) {
	base := fmt.Sprintf("https://releases.hashicorp.com/terraform-provider-%s/%s", ptype, ver)

	sumsBody, err := httpGet(base + fmt.Sprintf("/terraform-provider-%s_%s_SHA256SUMS", ptype, ver))
	if err != nil {
		return nil, err
	}
	sums, err := parseSums(sumsBody)
	if err != nil {
		return nil, fmt.Errorf("parse SHA256SUMS: %w", err)
	}

	tmp, err := os.MkdirTemp("", "pin-"+ptype+"-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	out := map[string]hashEntry{}
	for _, platform := range platforms {
		zipName := fmt.Sprintf("terraform-provider-%s_%s_%s.zip", ptype, ver, platform)
		hexSum, ok := sums[zipName]
		if !ok {
			return nil, fmt.Errorf("SHA256SUMS missing entry for %s", zipName)
		}
		zipPath := filepath.Join(tmp, zipName)
		if err := download(base+"/"+zipName, zipPath); err != nil {
			return nil, fmt.Errorf("download %s: %w", zipName, err)
		}
		if err := verifySha256(zipPath, hexSum); err != nil {
			return nil, fmt.Errorf("sha256 mismatch for %s: %w", zipName, err)
		}
		h1, err := dirhash.HashZip(zipPath, dirhash.Hash1)
		if err != nil {
			return nil, fmt.Errorf("h1 hash %s: %w", zipName, err)
		}
		out[platform] = hashEntry{Sha256: hexSum, H1: h1}
	}
	return out, nil
}

func download(url, dest string) error {
	body, err := httpGet(url)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, body, 0o644)
}

func verifySha256(path, wantHex string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != wantHex {
		return fmt.Errorf("got %s want %s", got, wantHex)
	}
	return nil
}

func httpGet(url string) ([]byte, error) {
	// Sized for the worst case: a provider zip can be ~150 MB on a slow link.
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// parseSums consumes a SHA256SUMS file body. Each line is
// `<hex-sha256>  <filename>` (two spaces). Empty lines ignored.
func parseSums(body []byte) (map[string]string, error) {
	out := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("malformed line: %q", line)
		}
		out[fields[1]] = fields[0]
	}
	return out, nil
}

// lockDoc is the JSON shape of the lockfile. Top-level `providers` maps
// `<source>` (not `<source>@<version>`) → its resolved entry.
type lockDoc struct {
	Providers map[string]resolved `json:"providers"`
}

// writeLockFile renders the new providers map deterministically
// (alphabetic source key order) and writes it to path. Per-platform
// keys inside each entry are also sorted by json.Marshal's standard
// alphabetic order, matching the `platforms` slice order coincidentally
// (darwin_*, linux_*).
func writeLockFile(path string, hashes map[string]resolved) error {
	doc := lockDoc{Providers: hashes}
	if doc.Providers == nil {
		doc.Providers = map[string]resolved{}
	}

	// Avoid the default `<`/`>`/`&` HTML-escaping — constraint strings like
	// `>= 7.0` are far more readable as-is than `>= 7.0`. Lockfile is
	// a build artifact, not HTML output.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	body := buf.Bytes()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

// sortedKeys returns the alphabetically-sorted keys of a string-keyed
// map. Kept as a future-use seam for any custom-ordered marshaling.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "pin: "+format+"\n", args...)
	os.Exit(1)
}
