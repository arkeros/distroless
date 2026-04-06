// Package golden provides test helpers for golden file comparison.
//
// Usage in tests:
//
//	golden.Compare(t, got, "testdata/service.golden.yaml")
//
// To update golden files:
//
//	bazel run //path:test -- --update_goldens
package golden

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arkeros/senku/base/diff/text"
)

var updateGoldens = flag.Bool("update_goldens", false, "update golden files in the source tree (requires bazel run)")

// Compare compares got against the contents of the golden file at path.
// When --update_goldens is passed via bazel run, it writes got back to the source tree.
func Compare(t *testing.T, got []byte, path string) {
	t.Helper()

	if *updateGoldens {
		ws := os.Getenv("BUILD_WORKSPACE_DIRECTORY")
		if ws == "" {
			t.Fatal("--update_goldens requires bazel run (BUILD_WORKSPACE_DIRECTORY not set)")
		}
		pkg := packageFromTestTarget()
		abs := filepath.Join(ws, pkg, path)
		if err := os.WriteFile(abs, got, 0644); err != nil {
			t.Fatalf("writing golden file: %v", err)
		}
		fmt.Printf("updated %s\n", filepath.Join(pkg, path))
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden file %s: %v", path, err)
	}

	if string(got) != string(want) {
		diff := text.Unified(string(want), string(got), "want", "got", 3)
		t.Fatalf("golden file %s is out of date\n\nTo update:\n  bazel run %s -- --update_goldens\n\n%s",
			path, os.Getenv("TEST_TARGET"), diff)
	}
}

// packageFromTestTarget extracts the package path from the TEST_TARGET env var.
// TEST_TARGET is set by bazel to e.g. "//devtools/bifrost/terraform:terraform_test".
func packageFromTestTarget() string {
	target := os.Getenv("TEST_TARGET")
	target = strings.TrimPrefix(target, "//")
	if i := strings.IndexByte(target, ':'); i >= 0 {
		target = target[:i]
	}
	return target
}
