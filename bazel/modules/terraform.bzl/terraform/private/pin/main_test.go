package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProviderFlags(t *testing.T) {
	got, err := parseProviderFlags([]string{
		"hashicorp/google@7.29.0",
		"hashicorp/kubernetes@2.38.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []provider{
		{source: "hashicorp/google", version: "7.29.0"},
		{source: "hashicorp/kubernetes", version: "2.38.0"},
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

func TestParseProviderFlagsRejects(t *testing.T) {
	cases := []string{"missing-at-version", "@no-source", "trailing@"}
	for _, c := range cases {
		if _, err := parseProviderFlags([]string{c}); err == nil {
			t.Errorf("expected error for %q", c)
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
	path := filepath.Join(dir, "lock.json")
	if err := writeLockFile(path, nil); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	var doc lockDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, body)
	}
	if len(doc.Providers) != 0 {
		t.Errorf("empty lock should have empty `providers` map; got %v", doc.Providers)
	}
}

func TestWriteLockFileSorted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lock.json")
	hashes := map[string]map[string]hashEntry{
		"hashicorp/zzz@1.0.0": {
			"darwin_amd64": {Sha256: "z1", H1: "h1:zZ1="},
			"darwin_arm64": {Sha256: "z2", H1: "h1:zZ2="},
			"linux_amd64":  {Sha256: "z3", H1: "h1:zZ3="},
			"linux_arm64":  {Sha256: "z4", H1: "h1:zZ4="},
		},
		"hashicorp/aaa@1.0.0": {
			"darwin_amd64": {Sha256: "a1", H1: "h1:aA1="},
			"darwin_arm64": {Sha256: "a2", H1: "h1:aA2="},
			"linux_amd64":  {Sha256: "a3", H1: "h1:aA3="},
			"linux_arm64":  {Sha256: "a4", H1: "h1:aA4="},
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
		t.Errorf("expected sorted output (aaa before zzz):\n%s", body)
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

func TestSortedKeysUnused(t *testing.T) {
	// Keep `sortedKeys` referenced — it's a future-use seam for custom
	// ordering. Test pins basic correctness so it doesn't bitrot.
	got := sortedKeys(map[string]int{"c": 3, "a": 1, "b": 2})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q want %q", i, got[i], want[i])
		}
	}
}
