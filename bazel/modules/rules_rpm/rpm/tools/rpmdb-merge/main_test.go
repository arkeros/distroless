package main

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestRun_TzdataHeaderProducesScannableRpmdb drives the first slice:
// given one header.blob (extracted from the Hummingbird tzdata rpm), the
// emitted tar must contain /usr/lib/sysimage/rpm/rpmdb.sqlite, the sqlite
// must be openable, `SELECT blob FROM Packages` must return one row, and
// that row must equal the input bytes verbatim. That's the entire contract
// syft's rpm-db cataloger relies on (see knqyf263/go-rpmdb sqlite3.go).
func TestRun_TzdataHeaderProducesScannableRpmdb(t *testing.T) {
	headerPath := testdataPath(t, "tzdata.header.blob")
	wantHeader, err := os.ReadFile(headerPath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")
	outPath := filepath.Join(tmp, "rpmdb.tar")

	cfgBytes, err := json.Marshal(config{
		Headers: []headerEntry{{
			Package:    "tzdata",
			Version:    "2026a-1.1.hum1",
			Arch:       "noarch",
			HeaderPath: headerPath,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, cfgBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	// Test uses uncompressed tar so the assertion below can read it with
	// archive/tar directly. The Bazel rule passes --compress=zstd (default)
	// when wiring into image layers.
	if err := run(cfgPath, outPath, "none"); err != nil {
		t.Fatalf("run: %v", err)
	}

	sqliteBytes := readTarEntry(t, outPath, "./usr/lib/sysimage/rpm/rpmdb.sqlite")
	if len(sqliteBytes) == 0 {
		t.Fatalf("rpmdb.sqlite missing from tar")
	}

	dbPath := filepath.Join(tmp, "rpmdb.sqlite")
	if err := os.WriteFile(dbPath, sqliteBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&immutable=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT blob FROM Packages")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var blobs [][]byte
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			t.Fatal(err)
		}
		blobs = append(blobs, b)
	}
	if len(blobs) != 1 {
		t.Fatalf("Packages row count: got %d, want 1", len(blobs))
	}
	// librpm strips the 0x8eade801 magic prefix when storing headers in the
	// rpmdb sqlite. The Packages.blob column should equal our input minus
	// the 8-byte magic.
	wantStored := bytes.TrimPrefix(wantHeader, rpmHeaderMagic)
	if len(wantStored) == len(wantHeader) {
		t.Fatal("fixture header.blob missing 0x8eade801 magic prefix — test setup is wrong")
	}
	if !bytes.Equal(blobs[0], wantStored) {
		t.Errorf("Packages.blob != stripped header.blob (got %d bytes, want %d)", len(blobs[0]), len(wantStored))
	}
}

func testdataPath(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join("testdata", name)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	t.Fatalf("could not locate testdata/%s (cwd-only — Bazel runfiles lookup not yet wired)", name)
	return ""
}

func readTarEntry(t *testing.T, tarPath, want string) []byte {
	t.Helper()
	data, err := os.ReadFile(tarPath)
	if err != nil {
		t.Fatalf("read tar: %v", err)
	}
	r := tar.NewReader(bytes.NewReader(data))
	for {
		hdr, err := r.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		if hdr.Name == want {
			b, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("read tar body: %v", err)
			}
			return b
		}
	}
}
