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

// TestRun_DeterministicAcrossInputOrders enforces the determinism contract
// from docs/adr/0007-hummingbird-rpm-base.md §Determinism: the output tar
// must be byte-identical regardless of the order headers appear in the
// config JSON. Same logical packages, different list order → same bytes.
//
// Uses synthetic blobs with the 0x8eade801 magic prefix and distinct tails
// so that insertion order is actually observable in the rowid → blob
// mapping. Reusing one identical fixture would mask the property: same
// rows are bytewise-identical regardless of which order assigned them
// rowid 1 vs 3.
func TestRun_DeterministicAcrossInputOrders(t *testing.T) {
	fixtures := t.TempDir()
	mkBlob := func(name string, tail byte) string {
		p := filepath.Join(fixtures, name+".header.blob")
		body := append([]byte{}, rpmHeaderMagic...)
		// 256 bytes of distinguishable filler so each blob occupies a
		// different b-tree slot if insertion order is preserved.
		filler := make([]byte, 256)
		for i := range filler {
			filler[i] = tail
		}
		body = append(body, filler...)
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	entries := []headerEntry{
		{Package: "aaa", Version: "1.0-1", Arch: "noarch", HeaderPath: mkBlob("aaa", 0xAA)},
		{Package: "mmm", Version: "2.0-1", Arch: "noarch", HeaderPath: mkBlob("mmm", 0xBB)},
		{Package: "zzz", Version: "3.0-1", Arch: "noarch", HeaderPath: mkBlob("zzz", 0xCC)},
	}
	reversed := []headerEntry{entries[2], entries[1], entries[0]}

	first := runOnce(t, entries)
	second := runOnce(t, reversed)

	if !bytes.Equal(first, second) {
		t.Fatalf("rpmdb tar not deterministic across input orderings (len(first)=%d, len(second)=%d)", len(first), len(second))
	}
}

// TestRun_ReproducibleSameInputs is the broader determinism test: two
// invocations with byte-identical inputs in identical order must produce
// byte-identical output tars. Catches any residual non-determinism
// (random allocation paths, time-of-day leakage, etc.) that the
// order-reversal test wouldn't see.
func TestRun_ReproducibleSameInputs(t *testing.T) {
	headerPath := testdataPath(t, "tzdata.header.blob")
	entries := []headerEntry{
		{Package: "tzdata", Version: "2026a-1.1.hum1", Arch: "noarch", HeaderPath: headerPath},
	}

	first := runOnce(t, entries)
	second := runOnce(t, entries)

	if !bytes.Equal(first, second) {
		t.Fatalf("rpmdb tar not reproducible across identical runs (len(first)=%d, len(second)=%d)", len(first), len(second))
	}
}

// TestRun_PageSizePinned reads the page_size back from the produced sqlite
// to confirm the URL-level `_pragma=page_size(4096)` actually applied,
// rather than being silently parsed and ignored (in which case the file
// would default to whatever the SQLite build prefers — usually but not
// always 4096).
func TestRun_PageSizePinned(t *testing.T) {
	headerPath := testdataPath(t, "tzdata.header.blob")
	tarBytes := runOnce(t, []headerEntry{{
		Package: "tzdata", Version: "2026a-1.1.hum1", Arch: "noarch", HeaderPath: headerPath,
	}})

	dbPath := filepath.Join(t.TempDir(), "rpmdb.sqlite")
	if err := os.WriteFile(dbPath, extractSqlite(t, tarBytes), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&immutable=1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var pageSize int
	if err := db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		t.Fatalf("read PRAGMA page_size: %v", err)
	}
	if pageSize != 4096 {
		t.Errorf("page_size = %d, want 4096", pageSize)
	}
}

// TestRun_NoSqliteSequenceTable confirms `INTEGER PRIMARY KEY` was used
// (not `AUTOINCREMENT`) by asserting the absence of the sqlite_sequence
// table. AUTOINCREMENT pulls in sqlite_sequence and writes a row per
// inserted package; dropping it is the cheapest reduction in the file's
// determinism surface.
func TestRun_NoSqliteSequenceTable(t *testing.T) {
	headerPath := testdataPath(t, "tzdata.header.blob")
	tarBytes := runOnce(t, []headerEntry{{
		Package: "tzdata", Version: "2026a-1.1.hum1", Arch: "noarch", HeaderPath: headerPath,
	}})

	dbPath := filepath.Join(t.TempDir(), "rpmdb.sqlite")
	if err := os.WriteFile(dbPath, extractSqlite(t, tarBytes), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&immutable=1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var name string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='sqlite_sequence'`).Scan(&name)
	if err == nil {
		t.Errorf("sqlite_sequence table present — AUTOINCREMENT must have crept back in")
	} else if err != sql.ErrNoRows {
		t.Fatalf("query sqlite_master: %v", err)
	}
}

func extractSqlite(t *testing.T, tarBytes []byte) []byte {
	t.Helper()
	tmp := t.TempDir()
	tarPath := filepath.Join(tmp, "rpmdb.tar")
	if err := os.WriteFile(tarPath, tarBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	return readTarEntry(t, tarPath, "./usr/lib/sysimage/rpm/rpmdb.sqlite")
}

func runOnce(t *testing.T, entries []headerEntry) []byte {
	t.Helper()
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")
	outPath := filepath.Join(tmp, "rpmdb.tar")

	cfgBytes, err := json.Marshal(config{Headers: entries})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, cfgBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(cfgPath, outPath, "none"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	return out
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
