// rpmdb-merge: N raw RPM header blobs -> tar containing
// /usr/lib/sysimage/rpm/rpmdb.sqlite with one Packages row per header.
//
// Minimum-viable schema mirrors what syft / trivy actually read:
// `SELECT blob FROM Packages` (see knqyf263/go-rpmdb pkg/sqlite3/sqlite3.go).
// The full librpm schema also writes secondary indexes (Name, Basenames,
// Requirename, ...) keyed off RPM header tags — those drive librpm's
// transactional install/remove operations but aren't read by SBOM scanners,
// so we omit them until something demands them.
//
// modernc.org/sqlite keeps the toolchain hermetic (pure Go, no cgo).
//
// Inputs are described by a config JSON written by the Bazel `rpmdb_merge`
// rule: a list of {package, version, arch, header_path} records. The
// header_path entries are paths relative to the action's exec root.
package main

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/klauspost/compress/zstd"
	_ "modernc.org/sqlite"
)

// The librpm on-disk header magic plus its 4-byte reserved padding. rpm-extract
// emits header.blob with this prefix (it's the in-rpm-file general-header byte
// range). librpm's rpmdb storage format strips it — the sqlite Packages.blob
// column begins directly with the big-endian index_count. anchore/go-rpmdb's
// header parser (which syft uses) assumes the stripped format.
var rpmHeaderMagic = []byte{0x8e, 0xad, 0xe8, 0x01, 0x00, 0x00, 0x00, 0x00}

type config struct {
	Headers []headerEntry `json:"headers"`
}

type headerEntry struct {
	Package    string `json:"package"`
	Version    string `json:"version"`
	Arch       string `json:"arch"`
	HeaderPath string `json:"header_path"`
}

const rpmdbPath = "./usr/lib/sysimage/rpm/rpmdb.sqlite"

func main() {
	configPath := flag.String("config", "", "path to JSON config listing header inputs")
	outPath := flag.String("out", "", "output tar path")
	compressFlag := flag.String("compress", "zstd", "tar compression: zstd | none")
	flag.Parse()

	if *configPath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "rpmdb-merge: --config and --out are required")
		os.Exit(2)
	}

	if err := run(*configPath, *outPath, *compressFlag); err != nil {
		fmt.Fprintln(os.Stderr, "rpmdb-merge:", err)
		os.Exit(1)
	}
}

func run(configPath, outPath, compress string) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	sqliteBytes, err := buildRpmdb(cfg.Headers)
	if err != nil {
		return fmt.Errorf("build rpmdb: %w", err)
	}

	return writeTar(outPath, sqliteBytes, compress)
}

func loadConfig(path string) (*config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("decode config json: %w", err)
	}
	return &cfg, nil
}

// buildRpmdb materializes a fresh rpmdb.sqlite into a temp file, populates
// the Packages table, then reads the bytes back. We round-trip through the
// filesystem because modernc.org/sqlite doesn't expose an in-memory-with-
// readback path that yields valid on-disk bytes for tar emission.
func buildRpmdb(headers []headerEntry) ([]byte, error) {
	tmp, err := os.CreateTemp("", "rpmdb-*.sqlite")
	if err != nil {
		return nil, err
	}
	dbPath := tmp.Name()
	tmp.Close()
	defer os.Remove(dbPath)

	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(`CREATE TABLE Packages (hnum INTEGER PRIMARY KEY AUTOINCREMENT, blob BLOB NOT NULL)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create Packages: %w", err)
	}

	stmt, err := db.Prepare(`INSERT INTO Packages (blob) VALUES (?)`)
	if err != nil {
		db.Close()
		return nil, err
	}
	for _, h := range headers {
		blob, err := os.ReadFile(h.HeaderPath)
		if err != nil {
			stmt.Close()
			db.Close()
			return nil, fmt.Errorf("read header %s: %w", h.HeaderPath, err)
		}
		stored := bytes.TrimPrefix(blob, rpmHeaderMagic)
		if len(stored) == len(blob) {
			stmt.Close()
			db.Close()
			return nil, fmt.Errorf("header %s missing expected 0x8eade801 magic prefix", h.Package)
		}
		if _, err := stmt.Exec(stored); err != nil {
			stmt.Close()
			db.Close()
			return nil, fmt.Errorf("insert %s: %w", h.Package, err)
		}
	}
	stmt.Close()
	if err := db.Close(); err != nil {
		return nil, err
	}

	return os.ReadFile(dbPath)
}

// writeTar wraps the sqlite bytes in a tar at the rpmdb path expected by
// syft/trivy/librpm. Parent dirs are emitted so `tar -x` on hosts that
// don't auto-synthesize them (stereoscope) sees a navigable tree.
//
// When `compress == "zstd"`, the tar stream is wrapped in zstd so the
// resulting layer ships with `tar+zstd` media type matching the rest of
// senku's distroless layers. `none` writes raw tar bytes (useful for
// in-process consumption / tests).
func writeTar(outPath string, sqliteBytes []byte, compress string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var w io.Writer = f
	var closer io.Closer
	switch compress {
	case "", "none":
		// no-op
	case "zstd":
		zw, err := zstd.NewWriter(f)
		if err != nil {
			return fmt.Errorf("init zstd writer: %w", err)
		}
		w = zw
		closer = zw
	default:
		return fmt.Errorf("unsupported --compress=%q (expected zstd|none)", compress)
	}

	tw := tar.NewWriter(w)
	defer func() {
		tw.Close()
		if closer != nil {
			closer.Close()
		}
	}()

	epoch := time.Unix(0, 0)
	dirs := []string{
		"./usr",
		"./usr/lib",
		"./usr/lib/sysimage",
		"./usr/lib/sysimage/rpm",
	}
	for _, d := range dirs {
		if err := tw.WriteHeader(&tar.Header{
			Name:     d + "/",
			Mode:     0o755,
			Typeflag: tar.TypeDir,
			ModTime:  epoch,
		}); err != nil {
			return err
		}
	}

	if err := tw.WriteHeader(&tar.Header{
		Name:     rpmdbPath,
		Mode:     0o644,
		Size:     int64(len(sqliteBytes)),
		Typeflag: tar.TypeReg,
		ModTime:  epoch,
	}); err != nil {
		return err
	}
	_, err = tw.Write(sqliteBytes)
	return err
}
