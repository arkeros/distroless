package grypedb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateModuleFile(t *testing.T) {
	// Copy fixture to tmpdir since UpdateModuleFile modifies the file in place
	fixture, err := os.ReadFile("testdata/module.bazel")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "oci.MODULE.bazel")
	if err := os.WriteFile(path, fixture, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	newURL := "https://grype.anchore.io/databases/v6/vulnerability-db_v6.1.4_2026-03-27T00:34:41Z_1774593488.tar.zst"
	newSHA := "newsha256hash"

	if err := UpdateModuleFile(path, newURL, newSHA); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read result: %v", err)
	}

	resultStr := string(result)

	if !strings.Contains(resultStr, newURL) {
		t.Errorf("expected new URL in output, got:\n%s", resultStr)
	}
	if !strings.Contains(resultStr, newSHA) {
		t.Errorf("expected new SHA in output, got:\n%s", resultStr)
	}
	if strings.Contains(resultStr, "oldsha256hash") {
		t.Error("old SHA should not be present")
	}
	if strings.Contains(resultStr, "v6.1.3") {
		t.Error("old URL should not be present")
	}
	// Structure should be preserved
	if !strings.Contains(resultStr, "grype_db.database(") {
		t.Error("grype_db.database call should be preserved")
	}
	if !strings.Contains(resultStr, `name = "grype_database"`) {
		t.Error("name argument should be preserved")
	}
}

func TestUpdateModuleFileNotFound(t *testing.T) {
	err := UpdateModuleFile("/nonexistent/file.bazel", "url", "sha")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestUpdateModuleFileNoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.MODULE.bazel")
	content := `bazel_dep(name = "rules_go", version = "0.60.0")
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err := UpdateModuleFile(path, "url", "sha")
	if err == nil {
		t.Error("expected error when no sha256/url found")
	}
}
