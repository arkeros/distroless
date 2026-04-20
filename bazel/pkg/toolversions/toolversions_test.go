package toolversions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arkeros/senku/base/cmp"
)

var testCfg = Config{
	Tool:        "bifrost",
	URLTemplate: "https://github.com/arkeros/senku/releases/download/bifrost/v%s/%s",
}

func TestWriteBootstrap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "versions.bzl")

	releases := []Release{
		{
			Version: "1.0.0",
			Artifacts: []Artifact{
				{Platform: "darwin_arm64", Filename: "bifrost-darwin-arm64", SHA256: "aaa"},
				{Platform: "linux_amd64", Filename: "bifrost-linux-amd64", SHA256: "bbb"},
			},
		},
		{
			Version: "2.0.0",
			Artifacts: []Artifact{
				{Platform: "darwin_arm64", Filename: "bifrost-darwin-arm64", SHA256: "ccc"},
			},
		},
	}
	if err := Write(path, testCfg, releases, "2.0.0"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(got)

	for _, want := range []string{
		`DEFAULT_VERSION = "2.0.0"`,
		`BIFROST_VERSIONS = {`,
		`"1.0.0-darwin_arm64"`,
		`"1.0.0-linux_amd64"`,
		`"2.0.0-darwin_arm64"`,
		`"bifrost-darwin-arm64"`,
		`"aaa"`,
		`"bbb"`,
		`"ccc"`,
		`def get_bifrost_url(version, filename):`,
		`"https://github.com/arkeros/senku/releases/download/bifrost/v{}/{}"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q\n---\n%s", want, s)
		}
	}
}

func TestWriteIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "versions.bzl")
	releases := []Release{{
		Version: "1.0.0",
		Artifacts: []Artifact{
			{Platform: "linux_amd64", Filename: "bifrost-linux-amd64", SHA256: "xyz"},
		},
	}}
	if err := Write(path, testCfg, releases, "1.0.0"); err != nil {
		t.Fatalf("first write: %v", err)
	}
	first, _ := os.ReadFile(path)
	if err := Write(path, testCfg, releases, "1.0.0"); err != nil {
		t.Fatalf("second write: %v", err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Errorf("output not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestWriteUnknownDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "versions.bzl")
	releases := []Release{{
		Version: "1.0.0",
		Artifacts: []Artifact{
			{Platform: "linux_amd64", Filename: "bifrost-linux-amd64", SHA256: "xyz"},
		},
	}}
	if err := Write(path, testCfg, releases, "9.9.9"); err == nil {
		t.Fatal("expected error for unknown default version")
	}
}

func TestCompareVersionsCalVer(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2026.16.6", "2026.16.43", -1},
		{"2026.16.43", "2026.16.6", 1},
		{"2026.16.6", "2026.16.6", 0},
		{"2026.15.76", "2026.16.6", -1},
		{"1.0.0", "2.0.0", -1},
		{"10.0.0", "2.0.0", 1},
	}
	for _, c := range cases {
		got := CompareVersions(c.a, c.b)
		if cmp.Sign(got) != cmp.Sign(c.want) {
			t.Errorf("CompareVersions(%q, %q) = %d, want sign %d", c.a, c.b, got, c.want)
		}
	}
}

func TestWriteOrdersNumericVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "versions.bzl")
	releases := []Release{
		{Version: "2026.16.43", Artifacts: []Artifact{{Platform: "linux_amd64", Filename: "bifrost-linux-amd64", SHA256: "new"}}},
		{Version: "2026.16.6", Artifacts: []Artifact{{Platform: "linux_amd64", Filename: "bifrost-linux-amd64", SHA256: "old"}}},
		{Version: "2026.15.76", Artifacts: []Artifact{{Platform: "linux_amd64", Filename: "bifrost-linux-amd64", SHA256: "older"}}},
	}
	if err := Write(path, testCfg, releases, "2026.16.43"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, _ := os.ReadFile(path)
	s := string(got)

	idxOlder := strings.Index(s, `"2026.15.76-linux_amd64"`)
	idxOld := strings.Index(s, `"2026.16.6-linux_amd64"`)
	idxNew := strings.Index(s, `"2026.16.43-linux_amd64"`)
	if !(idxOlder < idxOld && idxOld < idxNew) {
		t.Errorf("expected newest last: 2026.15.76 < 2026.16.6 < 2026.16.43, got offsets %d %d %d", idxOlder, idxOld, idxNew)
	}
}

func TestWriteKnifeTool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "versions.bzl")
	cfg := Config{
		Tool:        "knife",
		URLTemplate: "https://github.com/arkeros/senku/releases/download/knife/v%s/%s",
	}
	releases := []Release{{
		Version: "1.0.0",
		Artifacts: []Artifact{
			{Platform: "linux_amd64", Filename: "knife-linux-amd64", SHA256: "abc"},
		},
	}}
	if err := Write(path, cfg, releases, "1.0.0"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, _ := os.ReadFile(path)
	s := string(got)
	for _, want := range []string{
		`KNIFE_VERSIONS = {`,
		`def get_knife_url(version, filename):`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q\n---\n%s", want, s)
		}
	}
}
