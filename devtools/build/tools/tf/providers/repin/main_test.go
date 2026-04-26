package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseModule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.MODULE.bazel")
	contents := `
terraform = use_extension("//x:y.bzl", "terraform")
terraform.toolchain(version = "1.14.8")
terraform.provider(source = "hashicorp/google", version = "7.29.0")
terraform.provider(
    source = "hashicorp/random",
    version = "3.6.0",
)
# A non-terraform call should be ignored.
other.provider(source = "foo/bar", version = "1.0.0")
use_repo(terraform, "terraform_toolchains", "terraform_providers")
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseModule(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []provider{
		{source: "hashicorp/google", version: "7.29.0"},
		{source: "hashicorp/random", version: "3.6.0"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d providers, want %d: %+v", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("provider[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseSums(t *testing.T) {
	body := []byte(strings.Join([]string{
		"abc123  terraform-provider-google_7.29.0_darwin_amd64.zip",
		"def456  terraform-provider-google_7.29.0_darwin_arm64.zip",
		"",
		"   ",
		"99aa00  terraform-provider-google_7.29.0_linux_amd64.zip",
	}, "\n"))
	got, err := parseSums(body)
	if err != nil {
		t.Fatal(err)
	}
	if got["terraform-provider-google_7.29.0_darwin_arm64.zip"] != "def456" {
		t.Errorf("darwin_arm64 hex mismatch: %v", got)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 entries, got %d: %v", len(got), got)
	}
}

func TestVerifySha256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob")
	body := []byte("hello world")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	want := hex.EncodeToString(sum[:])

	if err := verifySha256(path, want); err != nil {
		t.Errorf("expected match, got %v", err)
	}
	if err := verifySha256(path, "0000"); err == nil {
		t.Error("expected mismatch error")
	}
}

func TestWriteLockFileEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lock.bzl")
	if err := writeLockFile(path, nil); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "PROVIDER_HASHES = {}") {
		t.Errorf("empty lock should declare PROVIDER_HASHES = {}; got:\n%s", body)
	}
}

func TestWriteLockFileSorted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lock.bzl")
	hashes := map[string]map[string]hashEntry{
		"hashicorp/zzz@1.0.0": {
			"darwin_amd64": {sha256: "z1", h1: "h1:zZ1="},
			"darwin_arm64": {sha256: "z2", h1: "h1:zZ2="},
			"linux_amd64":  {sha256: "z3", h1: "h1:zZ3="},
			"linux_arm64":  {sha256: "z4", h1: "h1:zZ4="},
		},
		"hashicorp/aaa@1.0.0": {
			"darwin_amd64": {sha256: "a1", h1: "h1:aA1="},
			"darwin_arm64": {sha256: "a2", h1: "h1:aA2="},
			"linux_amd64":  {sha256: "a3", h1: "h1:aA3="},
			"linux_arm64":  {sha256: "a4", h1: "h1:aA4="},
		},
	}
	if err := writeLockFile(path, hashes); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	aIdx := strings.Index(string(body), `"hashicorp/aaa@1.0.0"`)
	zIdx := strings.Index(string(body), `"hashicorp/zzz@1.0.0"`)
	if aIdx == -1 || zIdx == -1 {
		t.Fatalf("missing keys in output:\n%s", body)
	}
	if aIdx > zIdx {
		t.Errorf("expected sorted output (aaa before zzz)")
	}
}

func TestProviderPtype(t *testing.T) {
	got, err := provider{source: "hashicorp/google"}.ptype()
	if err != nil || got != "google" {
		t.Errorf("got (%q, %v), want (google, nil)", got, err)
	}
	if _, err := (provider{source: "no-slash"}).ptype(); err == nil {
		t.Error("expected error for source without slash")
	}
	if _, err := (provider{source: "trailing/"}).ptype(); err == nil {
		t.Error("expected error for empty type")
	}
}
