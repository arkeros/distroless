// pin: resolves a closed-manifest package list against a live RPM repository
// and writes a JSON lockfile (schema in //rpm/private:lockfile.bzl).
//
// Layout assumptions match Hummingbird's repo (per-arch repodata; noarch
// packages appear in every arch's primary.xml.gz with identical metadata):
//
//	<repo-url>/<arch>/repodata/repomd.xml
//	<repo-url>/<arch>/repodata/repomd.xml.asc      (detached GPG signature)
//	<repo-url>/<arch>/repodata/<sha>-primary.xml.gz
//	<repo-url>/<arch>/Packages/<initial>/<nvra>.rpm
//
// Trust chain at lock time:
//  1. `repomd.xml.asc` is a detached OpenPGP signature over `repomd.xml`,
//     verified against the consumer-supplied keyring. This is the anchor
//     — every digest we pin downstream chains back to it.
//  2. `primary.xml.gz`'s sha256 is taken from the trusted `repomd.xml`.
//  3. Each rpm's sha256 is taken from the trusted `primary.xml`.
//
// Subverting the per-rpm digests in the lockfile therefore requires
// subverting the signature on `repomd.xml` — matching the threat-model
// claim in ADR 0007 §"Threat model and fallbacks". At build time the
// per-rpm signature inside the .rpm header is verified independently by
// rpm-extract against the same keyring.
//
// Some upstreams (notably Hummingbird's RHPG snapshot) publish repomd.xml
// without a detached .asc sibling — only per-RPM signatures and per-package
// sigstore attestations under `metadata/attestations/`. Operators opt those
// upstreams in via `--repomd-signature=optional` (set per-repo through the
// `repomd_signature` attribute on `rpm.install(...)`): a missing .asc is
// treated as TLS-only trust at lock time with a loud stderr warning, and
// the lockfile is still produced. The remaining defenses for that repo
// are HTTPS pinning, per-RPM GPG verification in rpm-extract at build
// time, and the committed lockfile's sha256 chain — which a regressive
// future MITM would have to subvert via a PR. A *bad* signature (tampered
// or signed by a key outside the trust root) still hard-fails under
// `optional`: an actual signature failure is an attack signal, not a
// publisher gap. The default is `required` so repos that do publish .asc
// (e.g. nginx.org) keep the full chain.
//
// For each declared arch we fetch repomd, verify .asc, then primary.xml.gz,
// then walk every <package>. Within (name, arch) the highest version
// (rpmvercmp) wins. Noarch packages are committed on the first declared-arch
// pass and skipped on subsequent passes so the canonical "path" prefix is
// stable. Errors on any declared package without an entry (closed-manifest
// semantics).
package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/arkeros/senku/bazel/modules/rules_rpm/rpm/tools/internal/keyring"
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
	Format   primaryFormat  `xml:"format"`
}

type primaryFormat struct {
	SourceRpm string         `xml:"sourcerpm"`
	Provides  primaryDepList `xml:"provides"`
	Requires  primaryDepList `xml:"requires"`
}

type primaryDepList struct {
	Entries []primaryDepEntry `xml:"entry"`
}

type primaryDepEntry struct {
	Name string `xml:"name,attr"`
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
	Version  string `json:"version"`
	Sha256   string `json:"sha256"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Upstream string `json:"upstream,omitempty"`
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
		repoURL    = flag.String("repo-url", "", "base URL of the rpm repo (containing per-arch subdirs)")
		gpgKey     = flag.String("gpg-key", "", "ascii-armored public key for repomd.xml.asc verification")
		pkgs       = flag.String("packages", "", "comma-separated closed package manifest")
		arches     = flag.String("architectures", "", "comma-separated declared arches (e.g. x86_64,aarch64)")
		lockOut    = flag.String("lock-out", "", "workspace-relative path to lockfile output (e.g. //:hummingbird_install.json)")
		sigPolicy  = flag.String("repomd-signature", string(repomdSigRequired), "policy when fetching repomd.xml.asc: 'required' (default — 404 or bad sig aborts) or 'optional' (404 degrades to TLS-only at lock time with a loud warning; bad sig still aborts)")
	)
	flag.Parse()

	if *repoURL == "" || *pkgs == "" || *arches == "" || *lockOut == "" {
		fmt.Fprintln(os.Stderr, "pin: --repo-url, --packages, --architectures, --lock-out are required")
		os.Exit(2)
	}
	policy, err := parseRepomdSigPolicy(*sigPolicy)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pin:", err)
		os.Exit(2)
	}

	declaredArches := splitCSV(*arches)
	declaredPkgs := splitCSV(*pkgs)
	if len(declaredArches) == 0 || len(declaredPkgs) == 0 {
		fmt.Fprintln(os.Stderr, "pin: --packages and --architectures must be non-empty")
		os.Exit(2)
	}

	// Load the trust root once. The same keyring verifies repomd.xml.asc
	// for every arch (repos are arch-multiplexed under one signing chain).
	// Empty --gpg-key degrades to TLS-only trust and is loud on stderr so
	// it can't accidentally ride through in a Bazel-rule context — the
	// `rpm.install(...)` extension requires gpg_key, so production calls
	// always populate it.
	var trustRoot openpgp.EntityList
	if *gpgKey != "" {
		var err error
		trustRoot, err = keyring.ReadMultiBlock(*gpgKey)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pin: load gpg key:", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintln(os.Stderr, "pin: warning: --gpg-key is empty; repomd.xml.asc verification skipped (lock-time trust root degrades to TLS only)")
	}

	lock, err := resolve(*repoURL, declaredArches, declaredPkgs, trustRoot, policy)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pin:", err)
		os.Exit(1)
	}
	if err := writeLockfile(stripLabelPrefix(*lockOut), lock); err != nil {
		fmt.Fprintln(os.Stderr, "pin: write lockfile:", err)
		os.Exit(1)
	}
}

// repomdSigPolicy controls how resolve() reacts to a missing repomd.xml.asc.
// `required` is the default and the right choice for any repo that
// actually publishes detached signatures (e.g. nginx.org). `optional`
// is the explicit opt-out for upstreams that don't (e.g. Hummingbird's
// RHPG snapshot): a 404 on .asc degrades to TLS-only trust at lock time
// with a loud stderr warning, and the lockfile is still written. A
// signature that's *present but invalid* (tampered payload or wrong
// signer) still aborts under `optional` — that's an attack signal, not
// a publisher gap.
type repomdSigPolicy string

const (
	repomdSigRequired repomdSigPolicy = "required"
	repomdSigOptional repomdSigPolicy = "optional"
)

func parseRepomdSigPolicy(s string) (repomdSigPolicy, error) {
	switch repomdSigPolicy(s) {
	case repomdSigRequired, repomdSigOptional:
		return repomdSigPolicy(s), nil
	default:
		return "", fmt.Errorf("--repomd-signature: unknown value %q (want 'required' or 'optional')", s)
	}
}

// pkgKey is a (name, arch) addressing key. arch is either a declared arch
// like "x86_64"/"aarch64" or "noarch".
type pkgKey struct {
	name string
	arch string
}

// candidate is a parsed primary.xml package entry plus the dependency edges
// (requires/provides). One candidate per (name, arch) — when multiple
// versions exist upstream we keep the highest per rpmvercmp.
type candidate struct {
	evr      string
	sha256   string
	path     string
	size     int64
	upstream string
	requires []string
	provides []string
}

// resolve walks each declared arch's repodata, indexes every package (not
// just the declared ones) with its requires/provides edges, then closes the
// dependency graph starting from the declared seed. The lockfile receives
// the *closure* — same posture as @debian's apt resolver: the user names
// the roots, the lockfile carries every transitively reachable package.
//
// Errors when a declared package itself can't be found. Unresolved
// requires (e.g. file-path deps that would need filelists.xml.gz to
// resolve) are warned to stderr and skipped — runtime failure is the
// backstop, but most cc-grade packages declare soname requires that
// primary.xml fully covers.
func resolve(repoURL string, declaredArches, declaredPkgs []string, trustRoot openpgp.EntityList, policy repomdSigPolicy) (*lockfile, error) {
	repoURL = strings.TrimRight(repoURL, "/")

	candidates := map[pkgKey]candidate{}
	bestEVR := map[pkgKey]string{}

	var canonicalRevision, canonicalSha string
	for i, arch := range declaredArches {
		repomdURL := fmt.Sprintf("%s/%s/repodata/repomd.xml", repoURL, arch)
		repomdBytes, err := httpGet(repomdURL)
		if err != nil {
			return nil, fmt.Errorf("fetch repomd %s: %w", arch, err)
		}
		// Anchor the trust chain before any byte from repomd is used:
		// every primary.xml sha and per-rpm sha we pin downstream chains
		// back to this signature. trustRoot is nil only when --gpg-key
		// was empty (one-off CLI use); the main() banner already warned.
		if trustRoot != nil {
			sigURL := repomdURL + ".asc"
			sigBytes, err := httpGet(sigURL)
			if err != nil {
				var statusErr *httpStatusError
				if policy == repomdSigOptional && errors.As(err, &statusErr) && statusErr.status == http.StatusNotFound {
					// `optional` opt-in: upstream doesn't publish a detached
					// signature for this arch. Operator-chosen state — log a
					// one-line note (so the policy choice is visible in run
					// logs) and proceed. Per-RPM signatures still verify at
					// build time via rpm-extract.
					fmt.Fprintf(os.Stderr, "pin: %s: repomd.xml.asc not published (repomd_signature=optional)\n", arch)
				} else {
					return nil, fmt.Errorf("fetch repomd.xml.asc %s: %w", arch, err)
				}
			} else if err := verifyDetachedSignature(repomdBytes, sigBytes, trustRoot); err != nil {
				// A *present* signature that fails to verify is a hard
				// error under either policy. `optional` permits absence,
				// not a forged or tampered signature — that's an attack
				// signal we never want to silently accept.
				return nil, fmt.Errorf("verify repomd.xml.asc %s: %w", arch, err)
			}
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

			key := pkgKey{name: pkg.Name, arch: pkg.Arch}
			evr := formatEVR(pkg.Version)
			if cur, ok := bestEVR[key]; ok && rpmutils.Vercmp(evr, cur) <= 0 {
				continue
			}
			bestEVR[key] = evr
			candidates[key] = candidate{
				evr:      evr,
				sha256:   pkg.Checksum.Value,
				path:     fmt.Sprintf("%s/%s", arch, pkg.Location.Href),
				size:     pkg.Size.Package,
				upstream: pkg.Format.SourceRpm,
				requires: depNames(pkg.Format.Requires.Entries),
				provides: depNames(pkg.Format.Provides.Entries),
			}
		}
	}

	// Provides index: capability name -> set of keys offering it. Every
	// candidate implicitly provides itself by package name so a `requires
	// = "glibc"` entry resolves to the glibc candidate without needing the
	// explicit self-provide.
	providesIndex := map[string][]pkgKey{}
	for key, c := range candidates {
		providesIndex[key.name] = append(providesIndex[key.name], key)
		for _, p := range c.provides {
			providesIndex[p] = append(providesIndex[p], key)
		}
	}

	closure, err := closeDeps(candidates, providesIndex, declaredArches, declaredPkgs)
	if err != nil {
		return nil, err
	}

	out := map[string]map[string]lockEntry{}
	for key := range closure {
		c := candidates[key]
		if _, ok := out[key.name]; !ok {
			out[key.name] = map[string]lockEntry{}
		}
		out[key.name][key.arch] = lockEntry{
			Version:  c.evr,
			Sha256:   c.sha256,
			Path:     c.path,
			Size:     c.size,
			Upstream: c.upstream,
		}
	}

	if err := validate(out, declaredArches, declaredPkgs); err != nil {
		return nil, err
	}

	return &lockfile{
		SchemaVersion: 1,
		Repo: lockRepo{
			URL:          repoURL,
			Revision:     canonicalRevision,
			RepomdSha256: canonicalSha,
		},
		Packages: out,
	}, nil
}

// closeDeps walks the requires graph starting from declared roots until
// fixpoint. Per arch, roots are the declared package names resolved to
// either the arch-specific or noarch candidate (preferring arch-specific
// when both exist). Self-references (glibc requires libc.so.6 which it
// also provides) terminate via the closure-seen check.
func closeDeps(candidates map[pkgKey]candidate, providesIndex map[string][]pkgKey, declaredArches, declaredPkgs []string) (map[pkgKey]bool, error) {
	closure := map[pkgKey]bool{}
	for _, arch := range declaredArches {
		var worklist []pkgKey
		for _, name := range declaredPkgs {
			// Prefer arch-specific over noarch when both are present (rare).
			if _, ok := candidates[pkgKey{name, arch}]; ok {
				worklist = append(worklist, pkgKey{name, arch})
				continue
			}
			if _, ok := candidates[pkgKey{name, "noarch"}]; ok {
				worklist = append(worklist, pkgKey{name, "noarch"})
				continue
			}
			return nil, fmt.Errorf("declared package %q not found in repo for arch %s", name, arch)
		}
		for len(worklist) > 0 {
			cur := worklist[0]
			worklist = worklist[1:]
			if closure[cur] {
				continue
			}
			closure[cur] = true
			c := candidates[cur]
			for _, req := range c.requires {
				if skipRequire(req) {
					continue
				}
				chosen, ok := pickProvider(providesIndex[req], arch)
				if !ok {
					// File-path requires (e.g. /usr/sbin/ldconfig) need
					// filelists.xml.gz to resolve; pin only parses primary.
					// Many file-path requires are scriptlet-only and have no
					// runtime consequence, so warn instead of fail.
					fmt.Fprintf(os.Stderr, "pin: warning: no provider for %q (required by %s.%s)\n", req, cur.name, cur.arch)
					continue
				}
				worklist = append(worklist, chosen)
			}
		}
	}
	return closure, nil
}

// pickProvider chooses one candidate from a list of providers, preferring
// the targetArch over noarch and picking the lexically-first name as the
// tiebreaker. Producing a deterministic result keeps lockfiles diff-friendly.
func pickProvider(providers []pkgKey, targetArch string) (pkgKey, bool) {
	var compatible []pkgKey
	for _, p := range providers {
		if p.arch == targetArch || p.arch == "noarch" {
			compatible = append(compatible, p)
		}
	}
	if len(compatible) == 0 {
		return pkgKey{}, false
	}
	sort.Slice(compatible, func(i, j int) bool {
		if compatible[i].name != compatible[j].name {
			return compatible[i].name < compatible[j].name
		}
		// Prefer arch-specific over noarch when both exist for the same name.
		return compatible[i].arch == targetArch
	})
	return compatible[0], true
}

// skipRequire filters out dependency entries that are not real package
// edges: rpmlib() machinery features handled by the package manager,
// config() pseudo-deps for noarch config packages, file-path requires
// that need filelists.xml.gz to resolve (we warn and continue),
// solvable() runtime fingerprints, and rich/boolean conditional
// expressions (`(X if Y)` form — rpm-4.13+ syntax used for
// weak/conditional deps that we treat as out of scope).
func skipRequire(name string) bool {
	switch {
	case strings.HasPrefix(name, "rpmlib("):
		return true
	case strings.HasPrefix(name, "config("):
		return true
	case strings.HasPrefix(name, "solvable:"):
		return true
	case strings.HasPrefix(name, "/"):
		return true
	case strings.HasPrefix(name, "("):
		return true
	}
	return false
}

func depNames(entries []primaryDepEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name)
	}
	return out
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

// verifyDetachedSignature checks signedBytes against the armored detached
// signature in sigBytes using the supplied trust root. Returns nil iff at
// least one key in the trust root produced a valid signature; any failure
// — bad signature, no matching key, malformed armor — surfaces as a
// non-nil error so the caller can refuse to proceed with untrusted bytes.
func verifyDetachedSignature(signedBytes, sigBytes []byte, trustRoot openpgp.EntityList) error {
	_, err := openpgp.CheckArmoredDetachedSignature(
		trustRoot,
		bytes.NewReader(signedBytes),
		bytes.NewReader(sigBytes),
		nil,
	)
	return err
}

// httpGetMaxAttempts and httpGetBaseBackoff bound the retry loop in
// httpGet. The daily lockfile-refresh cron hits Hummingbird's CDN
// once per arch per repo; a single transient flake (occasional 5xx,
// connection reset, body-read EOF) should not break the run and let
// the lockfile silently stale until the next cron tick 24h later.
// Three attempts with 1s, 2s backoff is the right floor: long enough
// to ride out CDN hiccups, short enough that a genuinely-down repo
// fails in ~7s wall-clock instead of dragging out a half-broken job.
//
// httpGetBaseBackoff is a var (not const) so tests can shrink it; the
// retry semantics themselves are exercised in main_test.go without
// paying real sleeps.
const httpGetMaxAttempts = 3

var httpGetBaseBackoff = 1 * time.Second

// httpGet fetches url with up to httpGetMaxAttempts attempts. Retries
// on transport failures (DNS, connect, TLS, read mid-body) and HTTP
// 5xx; 4xx responses are terminal because a 404 won't become a 200
// on retry. Wraps the final error with the URL so the caller's error
// chain stays diagnosable.
func httpGet(url string) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= httpGetMaxAttempts; attempt++ {
		body, err, retryable := httpGetOnce(url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable || attempt == httpGetMaxAttempts {
			break
		}
		// 1s before attempt 2, 2s before attempt 3. (Exponential, but
		// capped at the loop bound — no need for a max-backoff knob.)
		backoff := httpGetBaseBackoff << (attempt - 1)
		fmt.Fprintf(os.Stderr, "pin: %s failed (attempt %d/%d), retrying in %v: %v\n",
			url, attempt, httpGetMaxAttempts, backoff, err)
		time.Sleep(backoff)
	}
	return nil, fmt.Errorf("%s: %w", url, lastErr)
}

// httpStatusError carries the HTTP status code through the error chain so
// callers can branch on a specific 4xx with errors.As — used by resolve()
// to distinguish "upstream genuinely doesn't publish this file" (HTTP 404
// on every arch's .asc) from "transport flake" or other 4xx classes.
type httpStatusError struct{ status int }

func (e *httpStatusError) Error() string { return fmt.Sprintf("HTTP %d", e.status) }

// httpGetOnce is a single attempt. retryable distinguishes transient
// failures (caller should back off and try again) from terminal ones
// (caller should stop). Hummingbird's CDN 403s HEAD and 302s GETs to
// S3-backed URLs; net/http follows redirects by default. 30s total
// timeout covers connect+TLS+body for repomd.xml (~kB) and
// primary.xml.gz (multi-MB on ~17K-package repos) with headroom for
// slow CI links while still catching stalled connections.
func httpGetOnce(url string) (body []byte, err error, retryable bool) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err, true
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 && resp.StatusCode < 600 {
		return nil, &httpStatusError{status: resp.StatusCode}, true
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{status: resp.StatusCode}, false
	}
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err, true
	}
	return body, nil, false
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
