package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/bazelbuild/buildtools/build"
)

// TestPinFileMatchesModule fails when bazel/include/terraform.providers.lock.bzl
// is out of sync with the providers declared in terraform.MODULE.bazel —
// the most common stale-pin bug ("I added a terraform.provider(...) and
// forgot to run :repin").
//
// Hermetic. Doesn't fetch from the network or verify hash *values*; it
// only checks that the set of `<source>@<version>` keys in the pin file
// equals the set of declared providers, and that each pinned entry
// covers every supported platform. A wrong hash value would still slip
// through here, but would be caught immediately by `terraform init`
// against the local mirror — the failure mode this guards against is
// "the file isn't there at all", not "the file is subtly wrong".
func TestPinFileMatchesModule(t *testing.T) {
	workspace := workspaceFromRunfiles(t)

	declared, err := parseModule(filepath.Join(workspace, moduleRel))
	if err != nil {
		t.Fatalf("parse %s: %v", moduleRel, err)
	}
	pinned, err := parseLockKeys(filepath.Join(workspace, lockRel))
	if err != nil {
		t.Fatalf("parse %s: %v", lockRel, err)
	}

	declaredKeys := map[string]bool{}
	for _, p := range declared {
		declaredKeys[p.key()] = true
	}

	for k := range declaredKeys {
		if _, ok := pinned[k]; !ok {
			t.Errorf("provider %s declared in %s but missing from %s.\n"+
				"  Run `bazel run //devtools/build/tools/tf/providers/repin`.",
				k, moduleRel, lockRel)
		}
	}
	for k := range pinned {
		if !declaredKeys[k] {
			t.Errorf("provider %s pinned in %s but not declared in %s.\n"+
				"  Either drop the entry from the lock file or add the "+
				"corresponding terraform.provider(...) tag.",
				k, lockRel, moduleRel)
		}
	}
}

// TestPinEntriesCoverAllPlatforms guards against a pin file with
// per-platform gaps (e.g., a hand-edit that dropped one platform's
// entry). Without all four platforms, the module extension would fail
// to materialize the archive repos and tf_root downstream would
// silently lose the platform.
func TestPinEntriesCoverAllPlatforms(t *testing.T) {
	workspace := workspaceFromRunfiles(t)
	pinned, err := parseLockKeysWithPlatforms(filepath.Join(workspace, lockRel))
	if err != nil {
		t.Fatal(err)
	}
	for key, gotPlatforms := range pinned {
		missing := []string{}
		for _, p := range platforms {
			if _, ok := gotPlatforms[p]; !ok {
				missing = append(missing, p)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Errorf("provider %s in %s is missing platforms: %v.\n"+
				"  Re-run `bazel run //devtools/build/tools/tf/providers/repin`.",
				key, lockRel, missing)
		}
	}
}

// parseLockKeys extracts the set of top-level keys (e.g.
// "hashicorp/google@7.29.0") from the PROVIDER_HASHES dict.
func parseLockKeys(path string) (map[string]bool, error) {
	dict, err := loadProviderHashes(path)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, entry := range dict.List {
		keyStr, ok := entry.Key.(*build.StringExpr)
		if !ok {
			continue
		}
		out[keyStr.Value] = true
	}
	return out, nil
}

// parseLockKeysWithPlatforms returns map[<source>@<version>] →
// map[<platform>] of which platforms have entries. Used for the
// per-platform-coverage check.
func parseLockKeysWithPlatforms(path string) (map[string]map[string]bool, error) {
	dict, err := loadProviderHashes(path)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]bool{}
	for _, entry := range dict.List {
		keyStr, ok := entry.Key.(*build.StringExpr)
		if !ok {
			continue
		}
		platDict, ok := entry.Value.(*build.DictExpr)
		if !ok {
			continue
		}
		out[keyStr.Value] = map[string]bool{}
		for _, platEntry := range platDict.List {
			ks, ok := platEntry.Key.(*build.StringExpr)
			if !ok {
				continue
			}
			out[keyStr.Value][ks.Value] = true
		}
	}
	return out, nil
}

// loadProviderHashes parses the .bzl file and returns the DictExpr
// assigned to PROVIDER_HASHES.
func loadProviderHashes(path string) (*build.DictExpr, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := build.ParseBzl(path, data)
	if err != nil {
		return nil, err
	}
	for _, stmt := range f.Stmt {
		assign, ok := stmt.(*build.AssignExpr)
		if !ok {
			continue
		}
		ident, ok := assign.LHS.(*build.Ident)
		if !ok || ident.Name != "PROVIDER_HASHES" {
			continue
		}
		dict, ok := assign.RHS.(*build.DictExpr)
		if !ok {
			return nil, &parseError{msg: "PROVIDER_HASHES is not a dict"}
		}
		return dict, nil
	}
	return nil, &parseError{msg: "PROVIDER_HASHES not found in " + path}
}

type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }

// workspaceFromRunfiles resolves the workspace root from inside a
// `bazel test` invocation. bazel sets TEST_SRCDIR (the runfiles
// container) and TEST_WORKSPACE (the canonical name); the workspace
// root in runfiles sits at $TEST_SRCDIR/$TEST_WORKSPACE.
func workspaceFromRunfiles(t *testing.T) string {
	t.Helper()
	srcDir := os.Getenv("TEST_SRCDIR")
	wsName := os.Getenv("TEST_WORKSPACE")
	if srcDir == "" || wsName == "" {
		t.Fatal("TEST_SRCDIR / TEST_WORKSPACE not set; run via `bazel test`")
	}
	return filepath.Join(srcDir, wsName)
}
