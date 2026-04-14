package gcp

import (
	"net/url"
	"strings"
	"testing"
)

func TestSecretPattern(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"projects/my-project/secrets/db-pass/versions/1", true},
		{"projects/my-project/secrets/db-pass/versions/42", true},
		{"projects/my-project/secrets/db-pass/versions/100", true},
		{"projects/my-project/secrets/db-pass/versions/latest", false},
		{"projects/my-project/secrets/db-pass/versions/", false},
		{"projects/my-project/secrets/db-pass/versions/1a", false},
		{"projects//secrets/db-pass/versions/1", false},
		{"projects/my-project/secrets//versions/1", false},
		{"not-a-ref", false},
		{"", false},
	}
	for _, tt := range tests {
		got := SecretPattern.MatchString(tt.input)
		if got != tt.valid {
			t.Errorf("SecretPattern.MatchString(%q) = %v, want %v", tt.input, got, tt.valid)
		}
	}
}

func TestURI(t *testing.T) {
	uri := URI("my-project", "db-pass", "42")
	if uri != "gcp:///projects/my-project/secrets/db-pass/versions/42" {
		t.Errorf("got %q", uri)
	}
}

func TestURI_Roundtrip(t *testing.T) {
	tests := []struct {
		project string
		name    string
		version string
	}{
		{"my-project", "db-pass", "1"},
		{"senku-prod", "registry-env", "42"},
		{"project-123", "my-secret-name", "100"},
	}
	for _, tt := range tests {
		uri := URI(tt.project, tt.name, tt.version)

		u, err := url.Parse(uri)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", uri, err)
		}
		if u.Scheme != "gcp" {
			t.Errorf("scheme = %q, want gcp", u.Scheme)
		}

		ref := strings.TrimPrefix(u.Path, "/")
		if !SecretPattern.MatchString(ref) {
			t.Errorf("URI %q does not match SecretPattern", uri)
		}
	}
}
