package main

import (
	"testing"
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
