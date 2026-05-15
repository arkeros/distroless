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
