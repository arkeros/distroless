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
