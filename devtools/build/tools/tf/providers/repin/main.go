// Command repin regenerates `bazel/include/terraform.providers.lock.bzl`
// from the `terraform.provider(...)` tags declared in
// `bazel/include/terraform.MODULE.bazel`.
//
// For each declared provider it fetches HashiCorp's `SHA256SUMS` file
// for the zip-level sha256, downloads each platform's zip to compute
// the `h1:` directory hash terraform records in `.terraform.lock.hcl`,
// and writes the resulting `PROVIDER_HASHES` dict in alphabetical
// order. Idempotent on a correctly-pinned spec.
//
// Run via `bazel run //devtools/build/tools/tf/providers/repin` —
// Bazel sets `BUILD_WORKSPACE_DIRECTORY` so the tool finds the workspace
// root regardless of the cwd from which it was invoked.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bazelbuild/buildtools/build"
	"golang.org/x/mod/sumdb/dirhash"
)

const (
	moduleRel = "bazel/include/terraform.MODULE.bazel"
	lockRel   = "bazel/include/terraform.providers.lock.bzl"
)

// Platforms we pin every provider for. Mirrors
// `_PROVIDER_PLATFORMS` in the toolchain extension. Keep in sync.
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
	sha256 string // hex
	h1     string // "h1:<base64-of-sha256>"
}

func main() {
	workdir := workspaceRoot()

	providers, err := parseModule(filepath.Join(workdir, moduleRel))
	if err != nil {
		die("parse module: %v", err)
	}
	if len(providers) == 0 {
		// Empty PROVIDER_HASHES — still need to emit the file so the
		// load() in extensions.bzl resolves.
		if err := writeLockFile(filepath.Join(workdir, lockRel), nil); err != nil {
			die("write lock: %v", err)
		}
		fmt.Println("repin: no terraform.provider tags declared; wrote empty lock file.")
		return
	}

	// Dedup so two modules pinning the same provider aren't fetched twice.
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
		fmt.Fprintf(os.Stderr, "repin: fetching %s\n", p.key())
		entries, err := fetchHashes(p)
		if err != nil {
			die("fetch %s: %v", p.key(), err)
		}
		hashes[p.key()] = entries
	}

	if err := writeLockFile(filepath.Join(workdir, lockRel), hashes); err != nil {
		die("write lock: %v", err)
	}
	fmt.Printf("repin: pinned %d provider(s).\n", len(deduped))
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

// parseModule walks the AST of a .MODULE.bazel file and extracts every
// `terraform.provider(source=…, version=…)` call. Order preserved.
func parseModule(path string) ([]provider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := build.ParseModule(path, data)
	if err != nil {
		return nil, err
	}
	var out []provider
	build.Walk(f, func(expr build.Expr, _ []build.Expr) {
		call, ok := expr.(*build.CallExpr)
		if !ok {
			return
		}
		dot, ok := call.X.(*build.DotExpr)
		if !ok || dot.Name != "provider" {
			return
		}
		ident, ok := dot.X.(*build.Ident)
		if !ok || ident.Name != "terraform" {
			return
		}
		p := extractProvider(call.List)
		if p.source != "" && p.version != "" {
			out = append(out, p)
		}
	})
	return out, nil
}

func extractProvider(args []build.Expr) provider {
	var p provider
	for _, arg := range args {
		assign, ok := arg.(*build.AssignExpr)
		if !ok {
			continue
		}
		ident, ok := assign.LHS.(*build.Ident)
		if !ok {
			continue
		}
		str, ok := assign.RHS.(*build.StringExpr)
		if !ok {
			continue
		}
		switch ident.Name {
		case "source":
			p.source = str.Value
		case "version":
			p.version = str.Value
		}
	}
	return p
}

// fetchHashes downloads the official SHA256SUMS for the pinned
// version (for the zip-level sha256 that download_and_extract
// consumes), then downloads each platform's zip into a temp dir to
// compute the `h1:` directory hash that terraform's lockfile records.
//
// The two hashes attest different things and are NOT
// interconvertible: sha256 hashes the zip bytes; h1 (Go's directory
// hash format, also used by terraform) hashes the unpacked-file
// manifest. Both are needed downstream — keep them paired in the lock
// file so neither side has to recompute.
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

	tmp, err := os.MkdirTemp("", "repin-"+ptype+"-")
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
		out[platform] = hashEntry{sha256: hexSum, h1: h1}
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
	// Sized for the worst case caller: a provider zip can be ~150 MB
	// (hashicorp/google), which on a slow link runs well past 30s. The
	// SHA256SUMS fetch sharing this client doesn't care — it's a few
	// hundred bytes either way, so a longer ceiling never bites it.
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

// writeLockFile renders the new PROVIDER_HASHES dict deterministically
// (alphabetic key order) and writes it to path.
func writeLockFile(path string, hashes map[string]map[string]hashEntry) error {
	var buf bytes.Buffer
	buf.WriteString(lockHeader)
	buf.WriteString("PROVIDER_HASHES = {")
	if len(hashes) == 0 {
		buf.WriteString("}\n")
		return os.WriteFile(path, buf.Bytes(), 0o644)
	}
	buf.WriteString("\n")

	keys := make([]string, 0, len(hashes))
	for k := range hashes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		fmt.Fprintf(&buf, "    %q: {\n", key)
		for _, platform := range platforms {
			entry := hashes[key][platform]
			fmt.Fprintf(&buf, "        %q: {\n", platform)
			fmt.Fprintf(&buf, "            \"sha256\": %q,\n", entry.sha256)
			fmt.Fprintf(&buf, "            \"h1\": %q,\n", entry.h1)
			buf.WriteString("        },\n")
		}
		buf.WriteString("    },\n")
	}
	buf.WriteString("}\n")
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

const lockHeader = `"""Pinned provider hashes for the terraform module extension.

Generated by ` + "`//devtools/build/tools/tf/providers:repin`" + `. **Do not edit
by hand** — bump versions in ` + "`bazel/include/terraform.MODULE.bazel`" + `,
then run the pin tool.

Each entry maps ` + "`<source>@<version>`" + ` to a per-platform dict of
hashes. ` + "`sha256`" + ` is the hex-encoded sha256 of the provider zip,
consumed by ` + "`download_and_extract`" + ` for the bazel-side download
integrity check. ` + "`h1`" + ` is terraform's directory-hash format
(base64 of sha256 over the unpacked-file manifest, ` + "`golang.org/x/mod" + `
` + "/sumdb/dirhash`" + ` Hash1) and goes verbatim into the generated
` + "`.terraform.lock.hcl`" + `.
"""

`

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "repin: "+format+"\n", args...)
	os.Exit(1)
}
