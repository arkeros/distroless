package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateSnapshotURL(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		newTimestamp  string
		want         string
	}{
		{
			name:        "standard debian URL",
			url:         "https://snapshot.debian.org/archive/debian/20260320T143128Z",
			newTimestamp: "20260401T120000Z",
			want:        "https://snapshot.debian.org/archive/debian/20260401T120000Z",
		},
		{
			name:        "cloudflare mirror URL",
			url:         "https://snapshot-cloudflare.debian.org/archive/debian/20260320T143128Z",
			newTimestamp: "20260401T120000Z",
			want:        "https://snapshot-cloudflare.debian.org/archive/debian/20260401T120000Z",
		},
		{
			name:        "security URL",
			url:         "https://snapshot-cloudflare.debian.org/archive/debian-security/20260320T001422Z",
			newTimestamp: "20260401T120000Z",
			want:        "https://snapshot-cloudflare.debian.org/archive/debian-security/20260401T120000Z",
		},
		{
			name:        "URL with trailing slash",
			url:         "https://snapshot.debian.org/archive/debian/20260320T143128Z/",
			newTimestamp: "20260401T120000Z",
			want:        "https://snapshot.debian.org/archive/debian/20260401T120000Z/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UpdateSnapshotURL(tt.url, tt.newTimestamp)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseManifest(t *testing.T) {
	manifest, err := ParseManifest("testdata/manifest.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(manifest.Sources) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(manifest.Sources))
	}

	if manifest.Sources[0].Channel != "trixie main contrib" {
		t.Errorf("expected channel 'trixie main contrib', got %q", manifest.Sources[0].Channel)
	}

	if len(manifest.Sources[0].URLs) != 2 {
		t.Errorf("expected 2 URLs, got %d", len(manifest.Sources[0].URLs))
	}

	if manifest.Sources[1].URL != "https://snapshot-cloudflare.debian.org/archive/debian-security/20260320T001422Z" {
		t.Errorf("unexpected security URL: %q", manifest.Sources[1].URL)
	}
}

func TestUpdateTimestampsInFile(t *testing.T) {
	fixture, err := os.ReadFile("testdata/manifest_with_header.yaml")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.yaml")
	if err := os.WriteFile(path, fixture, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	if err := UpdateTimestampsInFile(path, "20260401T120000Z", "20260401T060000Z"); err != nil {
		t.Fatalf("UpdateTimestampsInFile: %v", err)
	}

	result, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read result: %v", err)
	}
	resultStr := string(result)

	if !strings.Contains(resultStr, "archive/debian/20260401T120000Z") {
		t.Error("expected debian URLs to be updated with debian timestamp")
	}
	if !strings.Contains(resultStr, "archive/debian-security/20260401T060000Z") {
		t.Error("expected security URLs to be updated with security timestamp")
	}
	if strings.Contains(resultStr, "20260320T143128Z") {
		t.Error("old debian timestamp should not be present")
	}
	if strings.Contains(resultStr, "20260320T001422Z") {
		t.Error("old security timestamp should not be present")
	}
	if !strings.Contains(resultStr, "Anytime this file is changed") {
		t.Error("header comment should be preserved")
	}
}

func TestUpdateTimestampsInFilePreservesInlineComments(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "manifest.yaml")
	original := `# header comment
version: 1
sources:
  - channel: unstable main contrib
    urls:
      - https://snapshot.debian.org/archive/debian/20260101T000000Z
      - https://snapshot-cloudflare.debian.org/archive/debian/20260101T000000Z
archs:
  - amd64
packages:
  - libc6      # important: pinned for ABI compat
  - bash       # GNU bash, required by util-linux
  # block comment between items
  - util-linux
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	if err := UpdateTimestampsInFile(path, "20260201T120000Z", "20260201T060000Z"); err != nil {
		t.Fatalf("UpdateTimestampsInFile: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	gotStr := string(got)

	wantSubstrings := []string{
		"# header comment",
		"# important: pinned for ABI compat",
		"# GNU bash, required by util-linux",
		"# block comment between items",
		"20260201T120000Z",
	}
	for _, s := range wantSubstrings {
		if !strings.Contains(gotStr, s) {
			t.Errorf("expected output to contain %q, got:\n%s", s, gotStr)
		}
	}
	if strings.Contains(gotStr, "20260101T000000Z") {
		t.Errorf("old timestamp not removed:\n%s", gotStr)
	}
}

func TestParseManifestValidation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "empty document",
			content: "---\n",
			wantErr: "missing or zero version",
		},
		{
			name:    "missing sources",
			content: "version: 1\narchs: [amd64]\npackages: [libc6]\n",
			wantErr: "sources must not be empty",
		},
		{
			name:    "missing archs",
			content: "version: 1\nsources:\n  - channel: trixie\n    url: https://example.com/20260101T000000Z\npackages: [libc6]\n",
			wantErr: "archs must not be empty",
		},
		{
			name:    "missing packages",
			content: "version: 1\nsources:\n  - channel: trixie\n    url: https://example.com/20260101T000000Z\narchs: [amd64]\n",
			wantErr: "packages must not be empty",
		},
		{
			name:    "source missing channel",
			content: "version: 1\nsources:\n  - url: https://example.com/20260101T000000Z\narchs: [amd64]\npackages: [libc6]\n",
			wantErr: "sources[0].channel must not be empty",
		},
		{
			name:    "source missing url and urls",
			content: "version: 1\nsources:\n  - channel: trixie\narchs: [amd64]\npackages: [libc6]\n",
			wantErr: "sources[0] must have url or urls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := ParseManifest(path)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

