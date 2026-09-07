package tarballs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLockRoundTrip(t *testing.T) {
	lock := &Lock{
		SchemaVersion: 1,
		Source:        "nodejs",
		Lines: []Line{{
			Major:   "24",
			Version: "24.19.0",
			Archives: map[string]Archive{
				"nodejs_24_amd64": {URL: "https://nodejs.org/dist/v24.19.0/node-v24.19.0-linux-x64.tar.xz", SHA256: "aa", StripPrefix: "node-v24.19.0-linux-x64"},
			},
		}},
	}
	path := filepath.Join(t.TempDir(), "nodejs.lock.json")
	if err := lock.Write(path); err != nil {
		t.Fatalf("Write: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(raw), "}\n") {
		t.Errorf("lockfile should end with a newline, got %q", raw[len(raw)-3:])
	}
	if strings.Contains(string(raw), `"build"`) {
		t.Errorf("empty build must be omitted, got:\n%s", raw)
	}
	got, err := ReadLock(path)
	if err != nil {
		t.Fatalf("ReadLock: %v", err)
	}
	if got.Source != "nodejs" || len(got.Lines) != 1 || got.Lines[0].Version != "24.19.0" {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if got.Lines[0].Archives["nodejs_24_amd64"].StripPrefix != "node-v24.19.0-linux-x64" {
		t.Errorf("archive lost in round trip: %+v", got.Lines[0].Archives)
	}
}

func TestReadLockRejectsUnknownSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock.json")
	os.WriteFile(path, []byte(`{"schema_version": 2, "source": "nodejs", "lines": []}`), 0o644)
	if _, err := ReadLock(path); err == nil {
		t.Error("expected error for schema_version 2")
	}
}

func nodejsServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		// nodejs.org lists newest first; 26.x entries precede 24.x.
		w.Write([]byte(`[
  {"version":"v26.8.1","date":"2026-08-26","lts":false},
  {"version":"v26.8.0","date":"2026-08-25","lts":false},
  {"version":"v24.20.0","date":"2026-08-26","lts":"Krypton"},
  {"version":"v24.19.0","date":"2026-08-03","lts":"Krypton"}
]`))
	})
	mux.HandleFunc("/v24.20.0/SHASUMS256.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("deadbeef01  node-v24.20.0-linux-x64.tar.xz\n" +
			"deadbeef02  node-v24.20.0-linux-arm64.tar.xz\n" +
			"deadbeef03  node-v24.20.0-linux-x64.tar.gz\n"))
	})
	return httptest.NewServer(mux)
}

func TestNodeJSLatest(t *testing.T) {
	srv := nodejsServer(t)
	defer srv.Close()

	src := &NodeJS{BaseURL: srv.URL}
	line, err := src.Latest(context.Background(), "24")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if line.Major != "24" || line.Version != "24.20.0" || line.Build != "" {
		t.Errorf("unexpected line: %+v", line)
	}
	amd := line.Archives["nodejs_24_amd64"]
	if amd.SHA256 != "deadbeef01" || amd.StripPrefix != "node-v24.20.0-linux-x64" ||
		amd.URL != srv.URL+"/v24.20.0/node-v24.20.0-linux-x64.tar.xz" {
		t.Errorf("unexpected amd64 archive: %+v", amd)
	}
	arm := line.Archives["nodejs_24_arm64"]
	if arm.SHA256 != "deadbeef02" || arm.StripPrefix != "node-v24.20.0-linux-arm64" {
		t.Errorf("unexpected arm64 archive: %+v", arm)
	}
	if len(line.Archives) != 2 {
		t.Errorf("expected 2 archives, got %d", len(line.Archives))
	}
}

func TestNodeJSLatestUnknownMajor(t *testing.T) {
	srv := nodejsServer(t)
	defer srv.Close()

	if _, err := (&NodeJS{BaseURL: srv.URL}).Latest(context.Background(), "22"); err == nil {
		t.Error("expected error for a major absent from index.json")
	}
}

// temurinServer answers Adoptium's assets/latest endpoint. releaseByType
// lets a test publish different releases for jre and jdk to simulate a
// half-published release.
func temurinServer(t *testing.T, releaseByType map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/assets/latest/21/hotspot") {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		imageType, arch := q.Get("image_type"), q.Get("architecture")
		release := releaseByType[imageType]
		version := strings.TrimPrefix(release, "jdk-")
		fileVersion := strings.ReplaceAll(version, "+", "_")
		json.NewEncoder(w).Encode([]map[string]any{{
			"release_name": release,
			"binary": map[string]any{
				"image_type":   imageType,
				"architecture": arch,
				"package": map[string]any{
					"checksum": "sha-" + imageType + "-" + arch,
					"link": "https://github.com/adoptium/temurin21-binaries/releases/download/" +
						strings.ReplaceAll(release, "+", "%2B") + "/OpenJDK21U-" + imageType + "_" + arch + "_linux_hotspot_" + fileVersion + ".tar.gz",
				},
			},
		}})
	}))
}

func TestTemurinLatest(t *testing.T) {
	srv := temurinServer(t, map[string]string{"jre": "jdk-21.0.12.1+1", "jdk": "jdk-21.0.12.1+1"})
	defer srv.Close()

	line, err := (&Temurin{BaseURL: srv.URL}).Latest(context.Background(), "21")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if line.Major != "21" || line.Version != "21.0.12.1" || line.Build != "1" {
		t.Errorf("unexpected line: %+v", line)
	}
	if len(line.Archives) != 4 {
		t.Fatalf("expected 4 archives, got %v", line.Archives)
	}
	jre := line.Archives["temurin_jre_21_arm64"]
	if jre.SHA256 != "sha-jre-aarch64" || jre.StripPrefix != "jdk-21.0.12.1+1-jre" ||
		jre.URL != "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.12.1%2B1/OpenJDK21U-jre_aarch64_linux_hotspot_21.0.12.1_1.tar.gz" {
		t.Errorf("unexpected jre arm64 archive: %+v", jre)
	}
	jdk := line.Archives["temurin_jdk_21_amd64"]
	if jdk.SHA256 != "sha-jdk-x64" || jdk.StripPrefix != "jdk-21.0.12.1+1" {
		t.Errorf("unexpected jdk amd64 archive: %+v", jdk)
	}
}

func TestTemurinLatestRejectsLockstepMismatch(t *testing.T) {
	srv := temurinServer(t, map[string]string{"jre": "jdk-21.0.12.1+1", "jdk": "jdk-21.0.12+8"})
	defer srv.Close()

	if _, err := (&Temurin{BaseURL: srv.URL}).Latest(context.Background(), "21"); err == nil {
		t.Error("expected error when jre and jdk are at different releases")
	}
}

type fakeSource map[string]Line

func (f fakeSource) Latest(_ context.Context, major string) (Line, error) {
	return f[major], nil
}

func TestUpdateReplacesOnlyChangedLines(t *testing.T) {
	lock := &Lock{SchemaVersion: 1, Source: "nodejs", Lines: []Line{
		{Major: "24", Version: "24.19.0", Archives: map[string]Archive{"nodejs_24_amd64": {SHA256: "old"}}},
		{Major: "26", Version: "26.8.1", Archives: map[string]Archive{"nodejs_26_amd64": {SHA256: "same"}}},
	}}
	src := fakeSource{
		"24": {Major: "24", Version: "24.20.0", Archives: map[string]Archive{"nodejs_24_amd64": {SHA256: "new"}}},
		"26": {Major: "26", Version: "26.8.1", Archives: map[string]Archive{"nodejs_26_amd64": {SHA256: "same"}}},
	}
	changes, err := Update(context.Background(), lock, src)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(changes) != 1 || changes[0].Major != "24" || changes[0].From != "24.19.0" || changes[0].To != "24.20.0" {
		t.Errorf("unexpected changes: %+v", changes)
	}
	if lock.Lines[0].Version != "24.20.0" || lock.Lines[0].Archives["nodejs_24_amd64"].SHA256 != "new" {
		t.Errorf("line 24 not replaced: %+v", lock.Lines[0])
	}
	if lock.Lines[1].Version != "26.8.1" {
		t.Errorf("line 26 should be untouched: %+v", lock.Lines[1])
	}
}

func TestNewSource(t *testing.T) {
	for _, name := range []string{"nodejs", "temurin"} {
		if _, err := NewSource(name); err != nil {
			t.Errorf("NewSource(%q): %v", name, err)
		}
	}
	if _, err := NewSource("gopher"); err == nil {
		t.Error("expected error for unknown source")
	}
}

func TestStalledUpstreamTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // never answers within the client's timeout
	}))
	// Deferred in this order so the handler is released before Close waits
	// for it; the other way round deadlocks the test.
	defer srv.Close()
	defer close(release)

	old := httpClient.Timeout
	httpClient.Timeout = 50 * time.Millisecond
	defer func() { httpClient.Timeout = old }()

	if _, err := (&NodeJS{BaseURL: srv.URL}).Latest(context.Background(), "24"); err == nil {
		t.Error("expected a timeout error from a stalled upstream")
	}
}
