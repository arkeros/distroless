package main

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestExtract_TzdataContainsUTC drives the first end-to-end slice:
// given the Hummingbird tzdata noarch rpm, Extract must produce a
// content tar containing the UTC zoneinfo entry. UTC is the most stable
// path in tzdata across decades of releases and is what scanners and
// runtimes look for first.
func TestExtract_TzdataContainsUTC(t *testing.T) {
	rpmPath := testdataPath(t, "tzdata.rpm")

	tmp := t.TempDir()
	contentTar := filepath.Join(tmp, "content.tar")
	headerBlob := filepath.Join(tmp, "header.blob")

	if err := Extract(rpmPath, contentTar, headerBlob); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if !tarContainsPath(t, contentTar, "./usr/share/zoneinfo/UTC") {
		t.Errorf("content.tar missing ./usr/share/zoneinfo/UTC")
	}

	hdrInfo, err := os.Stat(headerBlob)
	if err != nil {
		t.Fatalf("stat header.blob: %v", err)
	}
	if hdrInfo.Size() == 0 {
		t.Errorf("header.blob is empty")
	}
}

func testdataPath(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join("testdata", name)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	// rules_go places data files under the runfiles tree.
	if runfiles := os.Getenv("RUNFILES_DIR"); runfiles != "" {
		candidates := []string{
			filepath.Join(runfiles, "_main", "bazel", "modules", "rules_rpm", "rpm", "tools", "rpm-extract", "testdata", name),
			filepath.Join(runfiles, "rules_rpm+", "rpm", "tools", "rpm-extract", "testdata", name),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}
	t.Fatalf("could not locate testdata/%s", name)
	return ""
}

func tarContainsPath(t *testing.T, tarPath, want string) bool {
	t.Helper()
	data, err := os.ReadFile(tarPath)
	if err != nil {
		t.Fatalf("read tar: %v", err)
	}
	r := tar.NewReader(bytes.NewReader(data))
	for {
		hdr, err := r.Next()
		if err == io.EOF {
			return false
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		if hdr.Name == want {
			return true
		}
	}
}

func TestMergedUsr(t *testing.T) {
	type result struct {
		rewritten string
		drop      bool
	}
	cases := map[string]result{
		// Legacy root-prefix files get rewritten under /usr.
		"./lib64/libgcc_s.so.1":            {"./usr/lib64/libgcc_s.so.1", false},
		"./lib/firmware/foo":               {"./usr/lib/firmware/foo", false},
		"./bin/bash":                       {"./usr/bin/bash", false},
		"./sbin/ldconfig":                  {"./usr/sbin/ldconfig", false},
		// The root symlink/dir entries themselves get dropped — the base
		// layer synthesises /lib64 -> usr/lib64 etc.
		"./lib64": {"", true},
		"./lib":   {"", true},
		"./bin":   {"", true},
		"./sbin":  {"", true},
		"lib64":   {"", true},
		// Already-canonical paths are untouched.
		"./usr/lib64/libc.so.6":   {"./usr/lib64/libc.so.6", false},
		"./usr/bin/localedef":     {"./usr/bin/localedef", false},
		"./etc/pki/ca-trust":      {"./etc/pki/ca-trust", false},
		"./usr/share/zoneinfo/UTC": {"./usr/share/zoneinfo/UTC", false},
		// Prefix-match guards: "libexec" must not match "lib".
		"./libexec/foo":           {"./libexec/foo", false},
	}
	for in, want := range cases {
		gotName, gotDrop := mergedUsr(in)
		if gotName != want.rewritten || gotDrop != want.drop {
			t.Errorf("mergedUsr(%q) = (%q, %v), want (%q, %v)", in, gotName, gotDrop, want.rewritten, want.drop)
		}
	}
}

// TestVerifyRpmSignature_ValidPasses asserts the positive case: a real
// Hummingbird-signed tzdata rpm verified against the in-repo keyring
// (which includes Hummingbird's signing key alongside Red Hat's legacy
// keys) returns no error. Companions the tamper test below: if this one
// stays green while the tamper test silently passes, verification has
// regressed to a no-op.
func TestVerifyRpmSignature_ValidPasses(t *testing.T) {
	rpmPath := testdataPath(t, "tzdata.rpm")
	keyPath := testdataPath(t, "hummingbird-release.pgp")
	if err := verifyRpmSignature(rpmPath, keyPath); err != nil {
		t.Fatalf("verifyRpmSignature on untouched rpm: %v", err)
	}
}

// TestVerifyRpmSignature_TamperedFails flips a single byte deep in the
// payload region (after the lead+signature+general headers) of a copy of
// tzdata.rpm. The general-header digest covers the payload bytes, so any
// mutation past the header break must surface as a verification failure.
// If this test passes a clean-signature rpm, verification isn't reaching
// the digest+signature check path.
func TestVerifyRpmSignature_TamperedFails(t *testing.T) {
	rpmBytes, err := os.ReadFile(testdataPath(t, "tzdata.rpm"))
	if err != nil {
		t.Fatal(err)
	}
	keyPath := testdataPath(t, "hummingbird-release.pgp")

	// Flip a byte 256 bytes from the end — comfortably inside the compressed
	// payload, well past any header region.
	if len(rpmBytes) < 512 {
		t.Fatalf("tzdata.rpm unexpectedly small (%d bytes)", len(rpmBytes))
	}
	tampered := append([]byte(nil), rpmBytes...)
	tampered[len(tampered)-256] ^= 0xFF

	tmp := filepath.Join(t.TempDir(), "tampered.rpm")
	if err := os.WriteFile(tmp, tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyRpmSignature(tmp, keyPath); err == nil {
		t.Fatalf("verifyRpmSignature accepted tampered rpm; expected failure")
	}
}

// TestVerifyRpmSignature_EmptyKeyPathSkips documents the opt-out shape:
// passing --gpg-key="" disables verification (so the binary stays usable
// as a one-off CLI). The rpm_package Bazel rule always passes a key, so
// the production path is never empty.
func TestVerifyRpmSignature_EmptyKeyPathSkips(t *testing.T) {
	if err := verifyRpmSignature(testdataPath(t, "tzdata.rpm"), ""); err != nil {
		t.Fatalf("empty keyPath should skip verification, got: %v", err)
	}
}

// TestMergedUsrLink locks the symlink-target companion to mergedUsr.
// Without this rewrite, an absolute target like `/lib/foo` would survive
// past extraction and only resolve at runtime via the base layer's
// `/lib -> usr/lib` symlink (oci/distroless/common:usrmerge_symlinks_hummingbird).
// Rewriting here makes the per-package tar internally consistent and
// removes the cross-layer-ordering dependency.
//
// Empirically (as of 2026-05-18) no package in the senku cc+static
// Hummingbird closure ships an absolute symlink into /lib*, /bin, or
// /sbin — this is a defensive lock-in against future packages.
func TestMergedUsrLink(t *testing.T) {
	cases := map[string]string{
		// Absolute targets into legacy roots get the /usr prefix.
		"/lib/foo":           "/usr/lib/foo",
		"/lib64/libfoo.so.1": "/usr/lib64/libfoo.so.1",
		"/bin/sh":            "/usr/bin/sh",
		"/sbin/ldconfig":     "/usr/sbin/ldconfig",
		// Bare legacy roots — defensive; the per-package tar would
		// almost never ship a symlink pointing at the root dir itself,
		// but if it did we should normalise consistently.
		"/lib":   "/usr/lib",
		"/lib64": "/usr/lib64",
		"/bin":   "/usr/bin",
		"/sbin":  "/usr/sbin",
		// Absolute targets outside the legacy roots are untouched.
		"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem":  "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem",
		"/etc/crypto-policies/back-ends/openssl_fips.config": "/etc/crypto-policies/back-ends/openssl_fips.config",
		"/opt/something":     "/opt/something",
		"/usr/lib64/libc.so": "/usr/lib64/libc.so",
		// Relative targets are untouched — they're location-relative
		// and the path-side rewrite preserves resolution.
		"libfoo.so.1":    "libfoo.so.1",
		"../bin/sh":      "../bin/sh",
		"../../lib/foo":  "../../lib/foo",
		"./bashbug-64":   "./bashbug-64",
		// Prefix-match guards: longer paths that share a prefix with a
		// legacy root must NOT be rewritten. /libexec is the canonical
		// foot-gun for naive `strings.HasPrefix(target, "/lib")` checks.
		"/libexec/foo":  "/libexec/foo",
		"/lib_alt/foo":  "/lib_alt/foo",
		"/binary/thing": "/binary/thing",
	}
	for in, want := range cases {
		if got := mergedUsrLink(in); got != want {
			t.Errorf("mergedUsrLink(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShouldStrip(t *testing.T) {
	cases := map[string]bool{
		"./usr/lib/.build-id/73":                    true,
		"./usr/lib/.build-id/73/abc":                true,
		"usr/lib/.build-id/0f/a568":                 true,
		"./usr/lib/.build-id":                       true,
		"./usr/lib/.build-idx/foo":                  false, // prefix-match guard
		"./usr/bin/localedef":                       false,
		"./usr/share/zoneinfo/UTC":                  false,
		"./etc/pki/ca-trust":                        false,
	}
	for in, want := range cases {
		if got := shouldStrip(in); got != want {
			t.Errorf("shouldStrip(%q) = %v, want %v", in, got, want)
		}
	}
}
