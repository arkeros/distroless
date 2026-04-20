// Package toolversions rewrites a versions.bzl-style file describing the
// prebuilt platform binaries of a CLI tool published as GitHub Releases
// (tag `<tool>/vX.Y.Z`, asset name `<tool>-<os>-<arch>`), using
// bazelbuild/buildtools for formatting.
//
// The generated file has the shape:
//
//	DEFAULT_VERSION = "X.Y.Z"
//
//	<TOOL_UPPER>_VERSIONS = {
//	    "X.Y.Z-<os>_<arch>": ("<tool>-<os>-<arch>", "<sha256>"),
//	    ...
//	}
//
//	def get_<tool>_url(version, filename):
//	    return "<url_template>".format(version, filename)
//
// It matches the shape used by grype.bzl/syft.bzl and is consumed by a
// module extension that downloads the binary and registers a toolchain.
package toolversions

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/bazelbuild/buildtools/build"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Config describes one tool's versions.bzl layout.
type Config struct {
	// Tool is the lowercase tool name, e.g. "bifrost".
	Tool string
	// URLTemplate is the Go format string for release asset URLs; takes
	// (version, filename) as in fmt.Sprintf. The corresponding Starlark
	// get_<tool>_url() function re-emits this via "".format(version, filename).
	URLTemplate string
}

// Release describes one release of the tool and its per-platform artifacts.
type Release struct {
	Version   string
	Artifacts []Artifact
}

// Artifact is one platform-specific binary of a release.
type Artifact struct {
	// Platform is the versions.bzl key suffix, e.g. "darwin_amd64".
	Platform string
	// Filename is the release-asset name, e.g. "bifrost-darwin-amd64".
	Filename string
	// SHA256 is the hex-encoded SHA-256 of the asset bytes.
	SHA256 string
}

// Write rewrites path with the given releases. defaultVersion becomes the
// top-level DEFAULT_VERSION; it must match one of releases.
//
// The file is regenerated from scratch — manual edits to DEFAULT_VERSION
// and the VERSIONS dict are not preserved. Other top-level declarations
// (docstring, utility functions) are preserved via the AST.
func Write(path string, cfg Config, releases []Release, defaultVersion string) error {
	if cfg.Tool == "" {
		return fmt.Errorf("tool name is required")
	}
	if cfg.URLTemplate == "" {
		return fmt.Errorf("url template is required")
	}
	if len(releases) == 0 {
		return fmt.Errorf("no releases provided")
	}
	found := false
	for _, r := range releases {
		if r.Version == defaultVersion {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("default version %q not in releases", defaultVersion)
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var f *build.File
	if len(existing) > 0 {
		f, err = build.Parse(path, existing)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	} else {
		f, err = build.Parse(path, []byte(bootstrap(cfg)))
		if err != nil {
			return fmt.Errorf("parse bootstrap: %w", err)
		}
	}

	setStringAssign(f, "DEFAULT_VERSION", defaultVersion)
	setDictAssign(f, versionsVar(cfg.Tool), releasesToDict(releases))

	out := build.Format(f)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func versionsVar(tool string) string {
	return strings.ToUpper(tool) + "_VERSIONS"
}

// CompareVersions orders version strings by numeric component, not lexically.
// Handles the YYYY.WEEK.INCREMENT CalVer scheme used by release-cli.yaml as
// well as plain X.Y.Z semver; purely non-numeric components fall back to
// string compare. Returns negative if a < b, positive if a > b, 0 if equal.
func CompareVersions(a, b string) int {
	ap, bp := strings.Split(a, "."), strings.Split(b, ".")
	n := len(ap)
	if len(bp) < n {
		n = len(bp)
	}
	for i := 0; i < n; i++ {
		ai, aerr := strconv.Atoi(ap[i])
		bi, berr := strconv.Atoi(bp[i])
		if aerr == nil && berr == nil {
			if ai != bi {
				return ai - bi
			}
			continue
		}
		if c := strings.Compare(ap[i], bp[i]); c != 0 {
			return c
		}
	}
	return len(ap) - len(bp)
}

func bootstrap(cfg Config) string {
	return fmt.Sprintf(`"""%[1]s version definitions with filenames and checksums.

Binaries are published to GitHub Releases at:

    %[2]s

Format: "VERSION-PLATFORM": (filename, sha256)
Platforms: darwin_amd64, darwin_arm64, linux_amd64, linux_arm64

Regenerate with: bazel run //bazel/cmd/knife -- prebuilts update --tool %[3]s
"""

DEFAULT_VERSION = ""

%[4]s = {}

def get_%[3]s_url(version, filename):
    """Returns the download URL for a %[3]s release."""
    return "%[2]s".format(version, filename)
`,
		cases.Title(language.English).String(cfg.Tool),
		urlTemplateForStarlark(cfg.URLTemplate),
		cfg.Tool,
		versionsVar(cfg.Tool),
	)
}

// urlTemplateForStarlark rewrites a Go fmt template (%s %s) into the
// Starlark-style "{}/{}" placeholders that get_<tool>_url consumes via
// str.format(version, filename).
func urlTemplateForStarlark(goTmpl string) string {
	return strings.ReplaceAll(goTmpl, "%s", "{}")
}

func releasesToDict(releases []Release) *build.DictExpr {
	sorted := append([]Release(nil), releases...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return CompareVersions(sorted[i].Version, sorted[j].Version) < 0
	})

	dict := &build.DictExpr{ForceMultiLine: true}
	for _, r := range sorted {
		arts := append([]Artifact(nil), r.Artifacts...)
		sort.Slice(arts, func(i, j int) bool { return arts[i].Platform < arts[j].Platform })
		for _, a := range arts {
			key := r.Version + "-" + a.Platform
			dict.List = append(dict.List, &build.KeyValueExpr{
				Key: &build.StringExpr{Value: key},
				Value: &build.TupleExpr{
					ForceMultiLine: true,
					List: []build.Expr{
						&build.StringExpr{Value: a.Filename},
						&build.StringExpr{Value: a.SHA256},
					},
				},
			})
		}
	}
	return dict
}

func setStringAssign(f *build.File, name, value string) {
	rhs := &build.StringExpr{Value: value}
	if a, ok := findAssign(f, name); ok {
		a.RHS = rhs
		return
	}
	f.Stmt = append(f.Stmt, &build.AssignExpr{
		LHS: &build.Ident{Name: name},
		Op:  "=",
		RHS: rhs,
	})
}

func setDictAssign(f *build.File, name string, dict *build.DictExpr) {
	if a, ok := findAssign(f, name); ok {
		a.RHS = dict
		return
	}
	f.Stmt = append(f.Stmt, &build.AssignExpr{
		LHS: &build.Ident{Name: name},
		Op:  "=",
		RHS: dict,
	})
}

func findAssign(f *build.File, name string) (*build.AssignExpr, bool) {
	for _, stmt := range f.Stmt {
		a, ok := stmt.(*build.AssignExpr)
		if !ok {
			continue
		}
		id, ok := a.LHS.(*build.Ident)
		if !ok || id.Name != name {
			continue
		}
		return a, true
	}
	return nil, false
}
