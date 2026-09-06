package main

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/sassoftware/go-rpmutils/cpio"
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

// TestExtract_ContentTarIsByteDeterministic anchors the per-rpm Bazel-
// cache contract: two extractions of the same .rpm must produce
// byte-identical content tars. Same property rpmdb-merge enforces via
// TestRun_ReproducibleSameInputs — locks down format selection
// (tar.FormatUSTAR pinned in writeCpioEntryAsTar) and preserves any
// upstream cpio mtime/uid/gid drift as a single-source-of-truth check.
// If this regresses, the Bazel action cache silently produces different
// outputs for the same input and image layer hashes start moving for
// no reason.
func TestExtract_ContentTarIsByteDeterministic(t *testing.T) {
	rpmPath := testdataPath(t, "tzdata.rpm")
	tmp := t.TempDir()

	out1 := filepath.Join(tmp, "1.content.tar")
	hdr1 := filepath.Join(tmp, "1.header.blob")
	if err := Extract(rpmPath, out1, hdr1); err != nil {
		t.Fatalf("Extract (1): %v", err)
	}
	out2 := filepath.Join(tmp, "2.content.tar")
	hdr2 := filepath.Join(tmp, "2.header.blob")
	if err := Extract(rpmPath, out2, hdr2); err != nil {
		t.Fatalf("Extract (2): %v", err)
	}

	b1, err := os.ReadFile(out1)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := os.ReadFile(out2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("content.tar not byte-deterministic across two extractions (len(b1)=%d, len(b2)=%d)", len(b1), len(b2))
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
		"./lib64/libgcc_s.so.1": {"./usr/lib64/libgcc_s.so.1", false},
		"./lib/firmware/foo":    {"./usr/lib/firmware/foo", false},
		"./bin/bash":            {"./usr/bin/bash", false},
		"./sbin/ldconfig":       {"./usr/sbin/ldconfig", false},
		// The root symlink/dir entries themselves get dropped — the base
		// layer synthesises /lib64 -> usr/lib64 etc.
		"./lib64": {"", true},
		"./lib":   {"", true},
		"./bin":   {"", true},
		"./sbin":  {"", true},
		"lib64":   {"", true},
		// Already-canonical paths are untouched.
		"./usr/lib64/libc.so.6":    {"./usr/lib64/libc.so.6", false},
		"./usr/bin/localedef":      {"./usr/bin/localedef", false},
		"./etc/pki/ca-trust":       {"./etc/pki/ca-trust", false},
		"./usr/share/zoneinfo/UTC": {"./usr/share/zoneinfo/UTC", false},
		// Prefix-match guards: "libexec" must not match "lib".
		"./libexec/foo": {"./libexec/foo", false},
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
// `/lib -> usr/lib` symlink (images/common:usrmerge_symlinks_hummingbird).
// Rewriting here makes the per-package tar internally consistent and
// removes the cross-layer-ordering dependency.
//
// Empirically (as of 2026-05-18) no package in the distroless cc+static
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
		"libfoo.so.1":   "libfoo.so.1",
		"../bin/sh":     "../bin/sh",
		"../../lib/foo": "../../lib/foo",
		"./bashbug-64":  "./bashbug-64",
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
		"./usr/lib/.build-id/73":     true,
		"./usr/lib/.build-id/73/abc": true,
		"usr/lib/.build-id/0f/a568":  true,
		"./usr/lib/.build-id":        true,
		"./usr/lib/.build-idx/foo":   false, // prefix-match guard
		"./usr/bin/localedef":        false,
		"./usr/share/zoneinfo/UTC":   false,
		"./etc/pki/ca-trust":         false,
	}
	for in, want := range cases {
		if got := shouldStrip(in); got != want {
			t.Errorf("shouldStrip(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestExtract_HardlinksCarryContent locks the cpio hardlink contract. rpm's
// cpio stores a set of hardlinked files as one entry per path with a zero
// filesize, and the payload only once, on the last path of the set. tzdata
// is almost entirely such sets (Europe/Madrid, UTC and Etc/UTC among them),
// and an extractor that copies entries one by one ships them as empty files:
// zoneinfo.ZoneInfo("Europe/Madrid") then fails with "Invalid TZif file".
// Every regular file in the tar must carry its bytes, whether as data or as
// a tar hardlink to an entry that does.
func TestExtract_HardlinksCarryContent(t *testing.T) {
	rpmPath := testdataPath(t, "tzdata.rpm")

	tmp := t.TempDir()
	contentTar := filepath.Join(tmp, "content.tar")
	if err := Extract(rpmPath, contentTar, filepath.Join(tmp, "header.blob")); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	f, err := os.Open(contentTar)
	if err != nil {
		t.Fatalf("open content.tar: %v", err)
	}
	defer f.Close()

	sizes := map[string]int64{}
	links := map[string]string{}
	var empty []string
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read content.tar: %v", err)
		}
		switch hdr.Typeflag {
		case tar.TypeReg:
			sizes[hdr.Name] = hdr.Size
			if hdr.Size == 0 {
				empty = append(empty, hdr.Name)
			}
		case tar.TypeLink:
			links[hdr.Name] = hdr.Linkname
		}
	}

	if len(empty) > 0 {
		t.Errorf("%d regular files have no content, first: %v", len(empty), empty[:min(5, len(empty))])
	}
	for name, target := range links {
		if sizes[target] == 0 {
			t.Errorf("hardlink %s -> %s points at an entry with no content", name, target)
		}
	}
	for _, zone := range []string{"./usr/share/zoneinfo/Europe/Madrid", "./usr/share/zoneinfo/UTC"} {
		if _, isLink := links[zone]; !isLink && sizes[zone] == 0 {
			t.Errorf("%s is neither a file with content nor a hardlink to one", zone)
		}
	}
	if len(links) == 0 {
		t.Errorf("tzdata's hardlink sets were not preserved as tar hardlinks")
	}
}

// newcEntry is one file of a hand-built cpio stream: what rpm's payload
// looks like before rpm-extract sees it.
type newcEntry struct {
	name  string
	ino   int
	nlink int
	data  string
}

// newc writes entries in the SVR4 "newc" format rpm uses: a 110-byte
// ASCII header, the NUL-terminated name, then the data, each padded to
// four bytes, and the TRAILER!!! entry last.
func newc(entries ...newcEntry) []byte {
	var buf bytes.Buffer
	pad := func() {
		for buf.Len()%4 != 0 {
			buf.WriteByte(0)
		}
	}
	write := func(e newcEntry) {
		fmt.Fprintf(&buf, "070701%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x",
			e.ino, 0o100644, 0, 0, e.nlink, 0, len(e.data), 0, 0, 0, 0, len(e.name)+1, 0)
		buf.WriteString(e.name)
		buf.WriteByte(0)
		pad()
		buf.WriteString(e.data)
		pad()
	}
	for _, e := range entries {
		write(e)
	}
	write(newcEntry{name: "TRAILER!!!", nlink: 1})
	return buf.Bytes()
}

type tarEntry struct {
	typeflag byte
	linkname string
	data     string
}

func readTar(t *testing.T, b []byte) map[string]tarEntry {
	t.Helper()
	out := map[string]tarEntry{}
	tr := tar.NewReader(bytes.NewReader(b))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		out[hdr.Name] = tarEntry{typeflag: hdr.Typeflag, linkname: hdr.Linkname, data: string(data)}
	}
}

// TestWritePayloadAsTar_Hardlinks covers the two shapes a hardlink set
// takes in rpm's cpio: content on the last member, and no content at all
// because the file is empty — python3.13-libs and ca-certificates both
// carry sets of empty files, which the first fix refused as truncated.
func TestWritePayloadAsTar_Hardlinks(t *testing.T) {
	stream := newc(
		newcEntry{name: "./usr/share/zoneinfo/Etc/UTC", ino: 7, nlink: 3},
		newcEntry{name: "./usr/share/zoneinfo/README", ino: 8, nlink: 1, data: "readme"},
		newcEntry{name: "./usr/share/zoneinfo/Zulu", ino: 7, nlink: 3},
		newcEntry{name: "./usr/lib64/python3.13/email/mime/__init__.py", ino: 9, nlink: 2},
		newcEntry{name: "./usr/share/zoneinfo/UTC", ino: 7, nlink: 3, data: "TZif2"},
		newcEntry{name: "./usr/lib64/python3.13/json/__init__.py", ino: 9, nlink: 2},
	)

	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	if err := writePayloadAsTar(tw, cpio.NewReader(bytes.NewReader(stream))); err != nil {
		t.Fatalf("writePayloadAsTar: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	got := readTar(t, out.Bytes())

	want := map[string]tarEntry{
		"./usr/share/zoneinfo/README":  {typeflag: tar.TypeReg, data: "readme"},
		"./usr/share/zoneinfo/UTC":     {typeflag: tar.TypeReg, data: "TZif2"},
		"./usr/share/zoneinfo/Etc/UTC": {typeflag: tar.TypeLink, linkname: "./usr/share/zoneinfo/UTC"},
		"./usr/share/zoneinfo/Zulu":    {typeflag: tar.TypeLink, linkname: "./usr/share/zoneinfo/UTC"},
		// An empty set: the first path is the file, the rest link to it.
		"./usr/lib64/python3.13/email/mime/__init__.py": {typeflag: tar.TypeReg},
		"./usr/lib64/python3.13/json/__init__.py":       {typeflag: tar.TypeLink, linkname: "./usr/lib64/python3.13/email/mime/__init__.py"},
	}
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("%s missing from tar", name)
			continue
		}
		if g != w {
			t.Errorf("%s = %+v, want %+v", name, g, w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("tar has %d entries, want %d: %v", len(got), len(want), got)
	}
}

// TestWritePayloadAsTar_StrippedPayloadMember: when the member carrying a
// set's bytes is one we strip, the members we keep still get the bytes.
// The end-of-stream fallback for empty sets must not turn that into silent
// data loss.
func TestWritePayloadAsTar_StrippedPayloadMember(t *testing.T) {
	stream := newc(
		newcEntry{name: "./usr/share/foo/data", ino: 5, nlink: 3},
		newcEntry{name: "./usr/share/foo/data.link", ino: 5, nlink: 3},
		newcEntry{name: "./usr/lib/.build-id/ab/cdef", ino: 5, nlink: 3, data: "bytes"},
	)

	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	if err := writePayloadAsTar(tw, cpio.NewReader(bytes.NewReader(stream))); err != nil {
		t.Fatalf("writePayloadAsTar: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	got := readTar(t, out.Bytes())

	want := map[string]tarEntry{
		"./usr/share/foo/data":      {typeflag: tar.TypeReg, data: "bytes"},
		"./usr/share/foo/data.link": {typeflag: tar.TypeLink, linkname: "./usr/share/foo/data"},
	}
	for name, w := range want {
		if g := got[name]; g != w {
			t.Errorf("%s = %+v, want %+v", name, g, w)
		}
	}
	if _, ok := got["./usr/lib/.build-id/ab/cdef"]; ok {
		t.Errorf("stripped path was written")
	}
	if len(got) != len(want) {
		t.Errorf("tar has %d entries, want %d: %v", len(got), len(want), got)
	}
}
