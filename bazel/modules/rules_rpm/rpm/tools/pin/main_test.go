package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
)

func TestFormatEVR(t *testing.T) {
	cases := []struct {
		in   primaryVersion
		want string
	}{
		// epoch=0 is the common case and is suppressed in the public EVR string.
		{primaryVersion{Epoch: "0", Ver: "2026a", Rel: "1.1.hum1"}, "2026a-1.1.hum1"},
		{primaryVersion{Epoch: "", Ver: "2.42", Rel: "13.hum1"}, "2.42-13.hum1"},
		// epoch != 0 must be preserved (busybox is the canonical example).
		{primaryVersion{Epoch: "1", Ver: "1.37.0", Rel: "7.2.hum1"}, "1:1.37.0-7.2.hum1"},
	}
	for _, c := range cases {
		got := formatEVR(c.in)
		if got != c.want {
			t.Errorf("formatEVR(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStripLabelPrefix(t *testing.T) {
	cases := map[string]string{
		"//:hummingbird_install.json":      "hummingbird_install.json",
		"//path/to:lockfile.json":          "path/to/lockfile.json",
		"@@//:hummingbird_install.json":    "hummingbird_install.json",
		"@@//path/to:lockfile.json":        "path/to/lockfile.json",
		"hummingbird_install.json":         "hummingbird_install.json",
		"path/to/hummingbird_install.json": "path/to/hummingbird_install.json",
	}
	for in, want := range cases {
		if got := stripLabelPrefix(in); got != want {
			t.Errorf("stripLabelPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateMissingArch(t *testing.T) {
	// glibc resolved for x86_64 only; aarch64 missing must surface.
	best := map[string]map[string]lockEntry{
		"glibc": {
			"x86_64": {Version: "2.42-13.hum1"},
		},
	}
	err := validate(best, []string{"x86_64", "aarch64"}, []string{"glibc"})
	if err == nil {
		t.Fatalf("validate: expected error, got nil")
	}
}

func TestValidateNoarchOnly(t *testing.T) {
	// Noarch is sufficient — no per-arch demand.
	best := map[string]map[string]lockEntry{
		"tzdata": {
			"noarch": {Version: "2026a-1.1.hum1"},
		},
	}
	if err := validate(best, []string{"x86_64", "aarch64"}, []string{"tzdata"}); err != nil {
		t.Fatalf("validate noarch: unexpected error %v", err)
	}
}

func TestValidateUnknownPackage(t *testing.T) {
	if err := validate(map[string]map[string]lockEntry{}, []string{"x86_64"}, []string{"glibc"}); err == nil {
		t.Fatalf("validate: expected error for unknown package, got nil")
	}
}

func TestSkipRequire(t *testing.T) {
	cases := map[string]bool{
		// Skip — pin can't / shouldn't resolve these via primary.xml.
		"rpmlib(PayloadIsZstd)":      true,
		"rpmlib(CompressedFileNames)": true,
		"config(crypto-policies)":    true,
		"solvable:fips":              true,
		"/usr/sbin/ldconfig":         true,
		"/bin/sh":                    true,
		// Rich/boolean conditional deps (rpm-4.13+ "X if Y" syntax). Out of scope.
		"(glibc-gconv-extra(x86-64) = 2.42-13.hum1 if redhat-rpm-config)": true,
		// Keep — real soname / package-name deps that pin must resolve.
		"libc.so.6(GLIBC_2.38)(64bit)": false,
		"libz.so.1()(64bit)":           false,
		"rtld(GNU_HASH)":               false,
		"crypto-policies":              false,
		"ca-certificates":              false,
	}
	for in, want := range cases {
		if got := skipRequire(in); got != want {
			t.Errorf("skipRequire(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCloseDepsTransitive(t *testing.T) {
	// Model: openssl-libs depends on glibc (libc.so.6) and zlib-ng-compat
	// (libz.so.1). zlib-ng-compat depends on glibc. Declaring just
	// openssl-libs must close over all three.
	candidates := map[pkgKey]candidate{
		{"openssl-libs", "x86_64"}: {
			requires: []string{"libc.so.6()(64bit)", "libz.so.1()(64bit)"},
			provides: []string{"libssl.so.3()(64bit)"},
		},
		{"glibc", "x86_64"}: {
			requires: []string{},
			provides: []string{"libc.so.6()(64bit)"},
		},
		{"zlib-ng-compat", "x86_64"}: {
			requires: []string{"libc.so.6()(64bit)"},
			provides: []string{"libz.so.1()(64bit)"},
		},
		// Unrelated package — must NOT end up in the closure.
		{"unrelated", "x86_64"}: {
			provides: []string{"some-other-thing"},
		},
	}
	providesIndex := map[string][]pkgKey{
		"openssl-libs":              {{"openssl-libs", "x86_64"}},
		"glibc":                     {{"glibc", "x86_64"}},
		"zlib-ng-compat":            {{"zlib-ng-compat", "x86_64"}},
		"unrelated":                 {{"unrelated", "x86_64"}},
		"libc.so.6()(64bit)":        {{"glibc", "x86_64"}},
		"libz.so.1()(64bit)":        {{"zlib-ng-compat", "x86_64"}},
		"libssl.so.3()(64bit)":      {{"openssl-libs", "x86_64"}},
		"some-other-thing":          {{"unrelated", "x86_64"}},
	}
	closure, err := closeDeps(candidates, providesIndex, []string{"x86_64"}, []string{"openssl-libs"})
	if err != nil {
		t.Fatalf("closeDeps: %v", err)
	}
	want := map[pkgKey]bool{
		{"openssl-libs", "x86_64"}:   true,
		{"glibc", "x86_64"}:          true,
		{"zlib-ng-compat", "x86_64"}: true,
	}
	if len(closure) != len(want) {
		t.Errorf("closure size = %d, want %d (got %v)", len(closure), len(want), closure)
	}
	for k := range want {
		if !closure[k] {
			t.Errorf("closure missing %v", k)
		}
	}
	if closure[pkgKey{"unrelated", "x86_64"}] {
		t.Errorf("closure should not contain unrelated package")
	}
}

// withFastBackoff shrinks the retry backoff so retry tests don't pay
// real wall-clock sleeps. Restored on test teardown so a parallel test
// can't observe the override.
func withFastBackoff(t *testing.T) {
	t.Helper()
	prev := httpGetBaseBackoff
	httpGetBaseBackoff = 1 * time.Millisecond
	t.Cleanup(func() { httpGetBaseBackoff = prev })
}

// TestHttpGet_Retries5xxThenSucceeds is the headline retry property:
// two consecutive 5xx responses followed by a 200 must produce a
// successful fetch, not propagate the first 5xx. A regression here
// would mean the daily lockfile-update cron starts failing on any
// transient CDN hiccup.
func TestHttpGet_Retries5xxThenSucceeds(t *testing.T) {
	withFastBackoff(t)
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	body, err := httpGet(srv.URL)
	if err != nil {
		t.Fatalf("httpGet: %v", err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

// TestHttpGet_GivesUpAfterMaxAttempts asserts the upper bound: a
// permanently-broken endpoint fails after httpGetMaxAttempts and no
// more — the retry loop must not turn a daily cron into an infinite
// retry storm if Hummingbird actually does go offline.
func TestHttpGet_GivesUpAfterMaxAttempts(t *testing.T) {
	withFastBackoff(t)
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "always-broken", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := httpGet(srv.URL); err == nil {
		t.Fatal("httpGet: expected error after exhausting retries, got nil")
	}
	if attempts != httpGetMaxAttempts {
		t.Errorf("attempts = %d, want %d", attempts, httpGetMaxAttempts)
	}
}

// TestHttpGet_4xxIsTerminal asserts the non-retry property: a 404 is
// deterministic — retrying just burns time and obscures the real
// error. A regression here would mean a typo in the repo URL takes
// 3× as long to surface in CI.
func TestHttpGet_4xxIsTerminal(t *testing.T) {
	withFastBackoff(t)
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := httpGet(srv.URL); err == nil {
		t.Fatal("httpGet: expected error on 404, got nil")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (4xx is terminal)", attempts)
	}
}

// TestHttpGet_TransportErrorRetries asserts the transport-failure
// retry path (companion to the 5xx case above). Pointing at a closed
// port produces an immediate connect error; the retry loop must treat
// that as transient too, since on a real CI box DNS/conn-refused can
// be momentary.
func TestHttpGet_TransportErrorRetries(t *testing.T) {
	withFastBackoff(t)
	// Bind+close to harvest a guaranteed-closed port number.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	start := time.Now()
	if _, err := httpGet("http://" + addr); err == nil {
		t.Fatal("httpGet: expected error against closed port, got nil")
	}
	// 1ms + 2ms backoff between the three attempts; with millisecond
	// base backoff the total wall-clock should be dominated by the
	// connect timeouts themselves, but we assert the test took *some*
	// time so a regression to "no retry" would show as ~0ms.
	if elapsed := time.Since(start); elapsed < 3*time.Millisecond {
		t.Errorf("httpGet returned in %v — expected at least one retry backoff to elapse", elapsed)
	}
}

// armoredDetachSign produces an ASCII-armored detached signature, the
// on-disk shape repomd.xml.asc takes in every YUM/DNF repo.
func armoredDetachSign(t *testing.T, signer *openpgp.Entity, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&buf, signer, bytes.NewReader(data), nil); err != nil {
		t.Fatalf("ArmoredDetachSign: %v", err)
	}
	return buf.Bytes()
}

// TestVerifyDetachedSignature_ValidPasses is the positive case. Pair it
// with the tampered/wrong-key tests below: if this stays green while
// either of those passes, verification has regressed to a no-op.
func TestVerifyDetachedSignature_ValidPasses(t *testing.T) {
	signer, err := openpgp.NewEntity("Test Signer", "", "signer@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("<repomd>fake repomd payload</repomd>\n")
	sig := armoredDetachSign(t, signer, data)
	if err := verifyDetachedSignature(data, sig, openpgp.EntityList{signer}); err != nil {
		t.Fatalf("verifyDetachedSignature on untouched data: %v", err)
	}
}

// TestVerifyDetachedSignature_TamperedFails proves the verification
// actually inspects bytes: one flipped byte in the signed payload must
// fail the check. If this test ever passes a tampered payload, the
// signature is being parsed but its hash isn't being compared.
func TestVerifyDetachedSignature_TamperedFails(t *testing.T) {
	signer, err := openpgp.NewEntity("Test Signer", "", "signer@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("<repomd>fake repomd payload</repomd>\n")
	sig := armoredDetachSign(t, signer, data)

	tampered := append([]byte(nil), data...)
	tampered[5] ^= 0x01
	if err := verifyDetachedSignature(tampered, sig, openpgp.EntityList{signer}); err == nil {
		t.Fatal("verifyDetachedSignature accepted tampered payload; expected failure")
	}
}

// TestVerifyDetachedSignature_WrongKeyFails proves trust-root selectivity:
// a signature from a key that isn't in the trust root must fail. This is
// the property a hostile-CDN attack scenario relies on — even if the
// attacker substitutes both repomd.xml *and* repomd.xml.asc, the .asc
// signs under their own key, not the Hummingbird vendor key, and the
// keyring rejects it.
func TestVerifyDetachedSignature_WrongKeyFails(t *testing.T) {
	attacker, err := openpgp.NewEntity("Attacker", "", "attacker@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	vendor, err := openpgp.NewEntity("Vendor", "", "vendor@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("<repomd>spoofed repomd payload</repomd>\n")
	attackerSig := armoredDetachSign(t, attacker, data)

	if err := verifyDetachedSignature(data, attackerSig, openpgp.EntityList{vendor}); err == nil {
		t.Fatal("verifyDetachedSignature accepted signature from a key not in trust root; expected failure")
	}
}

// TestResolve_VerifiesRepomdAgainstTrustRoot is the integration test for
// the fix: end-to-end through resolve() against an in-process repo
// server, asserting that swapping the signing key (= an attacker
// substituting a self-signed metadata bundle) causes pin to refuse to
// produce a lockfile. The positive control (same key signs as is in
// the trust root) succeeds in the companion test below.
func TestResolve_VerifiesRepomdAgainstTrustRoot(t *testing.T) {
	vendor, err := openpgp.NewEntity("Vendor", "", "vendor@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	attacker, err := openpgp.NewEntity("Attacker", "", "attacker@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	srv := newFakeRpmRepo(t, []string{"x86_64"}, []fakePackage{
		{name: "tzdata", arch: "noarch", ver: "2026a", rel: "1.hum1"},
	}, attacker)
	defer srv.Close()

	// Vendor is in the trust root; the metadata is signed by attacker.
	// resolve() must refuse to proceed.
	_, err = resolve(srv.URL, []string{"x86_64"}, []string{"tzdata"}, openpgp.EntityList{vendor})
	if err == nil {
		t.Fatal("resolve accepted attacker-signed repomd against vendor trust root; expected failure")
	}
	if !strings.Contains(err.Error(), "verify repomd.xml.asc") {
		t.Errorf("error did not mention signature verification: %v", err)
	}
}

// TestResolve_AcceptsValidSignature is the positive control: the same
// fake-repo machinery, but the trust root contains the signing key, so
// resolve() walks through to a complete lockfile. Without this
// companion, the negative test above could pass on a resolve() that
// errors for unrelated reasons.
func TestResolve_AcceptsValidSignature(t *testing.T) {
	vendor, err := openpgp.NewEntity("Vendor", "", "vendor@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	srv := newFakeRpmRepo(t, []string{"x86_64"}, []fakePackage{
		{name: "tzdata", arch: "noarch", ver: "2026a", rel: "1.hum1"},
	}, vendor)
	defer srv.Close()

	lock, err := resolve(srv.URL, []string{"x86_64"}, []string{"tzdata"}, openpgp.EntityList{vendor})
	if err != nil {
		t.Fatalf("resolve with valid signature: %v", err)
	}
	if _, ok := lock.Packages["tzdata"]; !ok {
		t.Fatalf("lockfile missing tzdata: %+v", lock.Packages)
	}
}

// fakePackage is the minimum (name, arch, version) shape needed to
// synthesize a primary.xml entry. fakeRpmRepo serves the metadata
// triplet (repomd.xml, repomd.xml.asc, primary.xml.gz) over HTTP,
// signed by the supplied key.
type fakePackage struct {
	name, arch, ver, rel string
}

func newFakeRpmRepo(t *testing.T, arches []string, pkgs []fakePackage, signer *openpgp.Entity) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	for _, arch := range arches {
		primary := buildPrimaryXML(pkgs, arch)
		primaryGz := gzipBytes(t, primary)
		primarySha := sha256Hex(primaryGz)
		repomd := buildRepomdXML(primarySha)
		var sigBuf bytes.Buffer
		if err := openpgp.ArmoredDetachSign(&sigBuf, signer, bytes.NewReader(repomd), nil); err != nil {
			t.Fatal(err)
		}

		mux.HandleFunc(fmt.Sprintf("/%s/repodata/repomd.xml", arch), func(w http.ResponseWriter, r *http.Request) {
			w.Write(repomd)
		})
		mux.HandleFunc(fmt.Sprintf("/%s/repodata/repomd.xml.asc", arch), func(w http.ResponseWriter, r *http.Request) {
			w.Write(sigBuf.Bytes())
		})
		mux.HandleFunc(fmt.Sprintf("/%s/repodata/primary.xml.gz", arch), func(w http.ResponseWriter, r *http.Request) {
			w.Write(primaryGz)
		})
	}
	return httptest.NewServer(mux)
}

func buildRepomdXML(primarySha string) []byte {
	type loc struct {
		Href string `xml:"href,attr"`
	}
	type sum struct {
		Type  string `xml:"type,attr"`
		Value string `xml:",chardata"`
	}
	type data struct {
		Type     string `xml:"type,attr"`
		Checksum sum    `xml:"checksum"`
		Location loc    `xml:"location"`
	}
	type rm struct {
		XMLName  xml.Name `xml:"repomd"`
		Revision string   `xml:"revision"`
		Data     []data   `xml:"data"`
	}
	out, _ := xml.Marshal(rm{
		Revision: "1700000000",
		Data: []data{{
			Type:     "primary",
			Checksum: sum{Type: "sha256", Value: primarySha},
			Location: loc{Href: "repodata/primary.xml.gz"},
		}},
	})
	return out
}

func buildPrimaryXML(pkgs []fakePackage, arch string) []byte {
	var b bytes.Buffer
	fmt.Fprint(&b, `<?xml version="1.0" encoding="UTF-8"?><metadata xmlns="http://linux.duke.edu/metadata/common" xmlns:rpm="http://linux.duke.edu/metadata/rpm">`)
	for _, p := range pkgs {
		// The pin parser is namespace-loose (it reads child element
		// names without a namespace prefix), so this minimal shape
		// without xmlns prefixes still drives every required field.
		fmt.Fprintf(&b, `<package><name>%s</name><arch>%s</arch>`, p.name, p.arch)
		fmt.Fprintf(&b, `<version epoch="0" ver="%s" rel="%s"/>`, p.ver, p.rel)
		fmt.Fprintf(&b, `<checksum type="sha256">%s</checksum>`, strings.Repeat("0", 64))
		fmt.Fprint(&b, `<size package="1"/>`)
		// path is `<arch>/<href>`; tests don't fetch this so any href is fine.
		fmt.Fprintf(&b, `<location href="Packages/%s/%s-%s-%s.%s.rpm"/>`, p.name[:1], p.name, p.ver, p.rel, p.arch)
		fmt.Fprintf(&b, `<format><sourcerpm>%s-%s-%s.src.rpm</sourcerpm></format>`, p.name, p.ver, p.rel)
		fmt.Fprint(&b, `</package>`)
	}
	fmt.Fprint(&b, `</metadata>`)
	return b.Bytes()
}

func gzipBytes(t *testing.T, in []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(in); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(in []byte) string {
	sum := sha256.Sum256(in)
	return hex.EncodeToString(sum[:])
}

func TestCloseDepsSelfReference(t *testing.T) {
	// glibc requires libc.so.6 which glibc itself provides. The closure
	// must terminate (not infinite-loop on self-reference).
	candidates := map[pkgKey]candidate{
		{"glibc", "x86_64"}: {
			requires: []string{"libc.so.6()(64bit)"},
			provides: []string{"libc.so.6()(64bit)"},
		},
	}
	providesIndex := map[string][]pkgKey{
		"glibc":              {{"glibc", "x86_64"}},
		"libc.so.6()(64bit)": {{"glibc", "x86_64"}},
	}
	closure, err := closeDeps(candidates, providesIndex, []string{"x86_64"}, []string{"glibc"})
	if err != nil {
		t.Fatalf("closeDeps: %v", err)
	}
	if !closure[pkgKey{"glibc", "x86_64"}] || len(closure) != 1 {
		t.Errorf("closure = %v, want {glibc/x86_64: true}", closure)
	}
}
