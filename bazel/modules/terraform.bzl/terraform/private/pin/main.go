// Command pin regenerates the JSON lockfile pointed to by `-lock` from the
// `-provider=<source>@<version>` flags repeated on the command line.
//
// For each declared provider it fetches HashiCorp's `SHA256SUMS` file for
// the zip-level sha256, downloads each platform's zip to compute the `h1:`
// directory hash terraform records in `.terraform.lock.hcl`, and writes
// the resulting JSON document deterministically (alphabetical keys).
// Idempotent on a correctly-pinned spec.
//
// Invocation comes via `bazel run @<install>//:pin` — Bazel sets
// `BUILD_WORKSPACE_DIRECTORY` so the tool finds the workspace root
// regardless of the cwd from which it was invoked.
package main

import (
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

	"golang.org/x/mod/sumdb/dirhash"
)

// Platforms we pin every provider for. Mirrors `_PROVIDER_PLATFORMS` in
// the module extension. Keep in sync.
var platforms = []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"}

type provider struct {
	source  string // e.g. "hashicorp/google"
	version string // e.g. "7.29.0"
}

func (p provider) key() string { return p.source + "@" + p.version }

func (p provider) ptype() (string, error) {
	parts := strings.SplitN(p.source, "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", fmt.Errorf("source must be `<namespace>/<type>`, got %q", p.source)
	}
	return parts[1], nil
}

type hashEntry struct {
	Sha256 string `json:"sha256"` // hex
	H1     string `json:"h1"`     // "h1:<base64-of-sha256>"
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
	flag.Var(&providerSpecs, "provider", "<source>@<version>, repeatable; the declared provider set")
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

	// Dedup by `<source>@<version>` in case the same install declared a
	// provider twice (or two installs share one lockfile).
	seen := map[string]bool{}
	deduped := providers[:0]
	for _, p := range providers {
		if seen[p.key()] {
			continue
		}
		seen[p.key()] = true
		deduped = append(deduped, p)
	}

	hashes := map[string]map[string]hashEntry{}
	for _, p := range deduped {
		fmt.Fprintf(os.Stderr, "pin: fetching %s\n", p.key())
		entries, err := fetchHashes(p)
		if err != nil {
			die("fetch %s: %v", p.key(), err)
		}
		hashes[p.key()] = entries
	}

	if err := writeLockFile(lockPath, hashes); err != nil {
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

// parseProviderFlags turns `["hashicorp/google@7.29.0", ...]` into the
// internal `provider` struct list, preserving order.
func parseProviderFlags(specs []string) ([]provider, error) {
	out := make([]provider, 0, len(specs))
	for _, s := range specs {
		at := strings.LastIndex(s, "@")
		if at <= 0 || at == len(s)-1 {
			return nil, fmt.Errorf("-provider=%q must look like <source>@<version>", s)
		}
		out = append(out, provider{source: s[:at], version: s[at+1:]})
	}
	return out, nil
}

// fetchHashes downloads the official SHA256SUMS for the pinned version
// (for the zip-level sha256 that download_and_extract consumes), then
// downloads each platform's zip into a temp dir to compute the `h1:`
// directory hash that terraform's lockfile records.
//
// The two hashes attest different things and are NOT interconvertible:
// sha256 hashes the zip bytes; h1 (Go's directory hash format, also
// used by terraform) hashes the unpacked-file manifest. Both are
// needed downstream — keep them paired in the lock file so neither
// side has to recompute.
func fetchHashes(p provider) (map[string]hashEntry, error) {
	ptype, err := p.ptype()
	if err != nil {
		return nil, err
	}
	base := fmt.Sprintf("https://releases.hashicorp.com/terraform-provider-%s/%s", ptype, p.version)

	sumsBody, err := httpGet(base + fmt.Sprintf("/terraform-provider-%s_%s_SHA256SUMS", ptype, p.version))
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
		zipName := fmt.Sprintf("terraform-provider-%s_%s_%s.zip", ptype, p.version, platform)
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
		// dirhash.HashZip with Hash1 returns the canonical `h1:…`
		// string that terraform writes into .terraform.lock.hcl.
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

// lockDoc is the JSON shape of the lockfile. Top-level key `providers`
// maps `<source>@<version>` → per-platform hash entries.
type lockDoc struct {
	Providers map[string]map[string]hashEntry `json:"providers"`
}

// writeLockFile renders the new providers map deterministically
// (alphabetic key order) and writes it to path.
func writeLockFile(path string, hashes map[string]map[string]hashEntry) error {
	doc := lockDoc{Providers: hashes}
	if doc.Providers == nil {
		doc.Providers = map[string]map[string]hashEntry{}
	}

	// json.Marshal sorts map keys alphabetically — what we want for the
	// top-level `<source>@<version>` map. For the per-platform inner map
	// the same alphabetical sort produces darwin_*, linux_* in a stable
	// order matching the platforms slice we declare elsewhere.
	body, err := marshalIndent(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func marshalIndent(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	// json.Marshal sorts keys, but we additionally rewrite the
	// per-platform inner map order to match `platforms` slice order
	// (darwin_amd64, darwin_arm64, linux_amd64, linux_arm64) — which
	// already happens to be alphabetical, so MarshalIndent's default
	// suffices. Keep this seam for future custom ordering needs.
	return append(b, '\n'), nil
}

// sortedKeys returns the alphabetically-sorted keys of a string-keyed
// map. Kept handy for potential future custom-ordered marshaling.
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
