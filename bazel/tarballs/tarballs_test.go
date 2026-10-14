package tarballs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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
	for _, name := range []string{"nodejs", "temurin", "envoy"} {
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

// A bare-file entry pins one downloaded file rather than an archive to
// unpack, so `file` is written and `strip_prefix` must not appear — the
// extension dispatches on which of the two is set.
func TestLockRoundTripBinaryEntry(t *testing.T) {
	lock := &Lock{
		SchemaVersion: 1,
		Source:        "envoy",
		Lines: []Line{{
			Major:   "1.39",
			Version: "1.39.1",
			Archives: map[string]Archive{
				"envoy_139_amd64": {URL: "https://example.test/envoy-1.39.1-linux-x86_64", SHA256: "aa", File: "envoy"},
			},
		}},
	}
	path := filepath.Join(t.TempDir(), "envoy.lock.json")
	if err := lock.Write(path); err != nil {
		t.Fatalf("Write: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"strip_prefix"`) {
		t.Errorf("empty strip_prefix must be omitted, got:\n%s", raw)
	}
	got, err := ReadLock(path)
	if err != nil {
		t.Fatalf("ReadLock: %v", err)
	}
	if archive := got.Lines[0].Archives["envoy_139_amd64"]; archive.File != "envoy" || archive.StripPrefix != "" {
		t.Errorf("binary entry lost in round trip: %+v", archive)
	}
}

// envoyReleases is what the fake API publishes, in the order GitHub
// returns them: newest created first. v1.39.0 is listed before v1.39.1 on
// purpose — the resolver must pick the highest patch of the line, not the
// first one it sees — and the 1.36 line is far enough down to fall on a
// later page.
var envoyReleases = []string{
	"v1.39.0",
	"v1.39.1",
	"v1.40.0-rc1",
	"v1.38.4",
	"v1.39.0-nightly",
	"v1.36.9",
}

// envoyAssetSums are keyed by asset arch suffix; the file the fake serves
// keys them by the builder's absolute path, as upstream's does.
var envoyAssetSums = map[string]string{
	"x86_64":   "1111111111111111111111111111111111111111111111111111111111111111",
	"aarch_64": "2222222222222222222222222222222222222222222222222222222222222222",
	"contrib":  "3333333333333333333333333333333333333333333333333333333333333333",
}

// fakeGitHub answers envoyproxy/envoy's releases API and serves its
// assets. It honours `page`/`per_page` so paging is exercised for real,
// records the Authorization header of the last API request, and can be
// told to drop the checksums asset.
type fakeGitHub struct {
	URL string
	// Auth is the Authorization header of the last API request.
	Auth string
	// NoChecksums makes every release's checksums asset 404.
	NoChecksums bool
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	fake := &fakeGitHub{}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/envoyproxy/envoy/releases", func(w http.ResponseWriter, r *http.Request) {
		fake.Auth = r.Header.Get("Authorization")
		perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if perPage == 0 || page == 0 {
			t.Errorf("releases request without paging: %s", r.URL)
		}
		start := min((page-1)*perPage, len(envoyReleases))
		end := min(start+perPage, len(envoyReleases))
		out := []map[string]any{}
		for _, tag := range envoyReleases[start:end] {
			version := strings.TrimPrefix(tag, "v")
			assets := []map[string]any{}
			for _, name := range []string{
				"checksums.txt.asc",
				"envoy-" + version + "-linux-x86_64",
				"envoy-" + version + "-linux-aarch_64",
				"envoy-contrib-" + version + "-linux-x86_64",
			} {
				assets = append(assets, map[string]any{
					"name":                 name,
					"browser_download_url": fake.URL + "/assets/" + version + "/" + name,
				})
			}
			out = append(out, map[string]any{
				"tag_name":   tag,
				"draft":      false,
				"prerelease": strings.Contains(tag, "-rc"),
				"assets":     assets,
			})
		}
		json.NewEncoder(w).Encode(out)
	})
	// The clearsigned checksums, keyed by the builder's own absolute
	// paths, wrapped in the PGP armor upstream ships.
	mux.HandleFunc("/assets/", func(w http.ResponseWriter, r *http.Request) {
		if fake.NoChecksums || !strings.HasSuffix(r.URL.Path, "checksums.txt.asc") {
			http.NotFound(w, r)
			return
		}
		version := strings.Split(strings.TrimPrefix(r.URL.Path, "/assets/"), "/")[0]
		w.Write([]byte("-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA256\n\n" +
			envoyAssetSums["x86_64"] + "  /tmp/tmp.Xj4/envoy-" + version + "-linux-x86_64\n" +
			envoyAssetSums["aarch_64"] + "  /tmp/tmp.Xj4/envoy-" + version + "-linux-aarch_64\n" +
			envoyAssetSums["contrib"] + "  /tmp/tmp.Xj4/envoy-contrib-" + version + "-linux-x86_64\n" +
			"-----BEGIN PGP SIGNATURE-----\n\nc2lnbmF0dXJl\n-----END PGP SIGNATURE-----\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	fake.URL = srv.URL
	return fake
}

// client is the helper under test, pointed at the fake.
func (f *fakeGitHub) client() *GitHub {
	return &GitHub{BaseURL: f.URL, Owner: "envoyproxy", Repo: "envoy"}
}

func TestGitHubLatestReleasePagesAndPicksHighestPatch(t *testing.T) {
	gh := newFakeGitHub(t).client()
	gh.PerPage = 2 // three pages, so the 1.36 line is not on the first

	release, err := gh.LatestRelease(context.Background(), "v1.39.")
	if err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if release.TagName != "v1.39.1" {
		t.Errorf("expected the highest patch of the line, got %q", release.TagName)
	}
	// v1.36.9 is on the last page: reaching it proves the paging.
	old, err := gh.LatestRelease(context.Background(), "v1.36.")
	if err != nil {
		t.Fatalf("LatestRelease(1.36): %v", err)
	}
	if old.TagName != "v1.36.9" {
		t.Errorf("expected v1.36.9 from a later page, got %q", old.TagName)
	}
	if _, err := gh.LatestRelease(context.Background(), "v1.37."); err == nil {
		t.Error("expected an error for a line with no release")
	}
}

// A release candidate is not something to pin, and neither is a tag whose
// remainder past the line is not a plain patch number.
func TestGitHubLatestReleaseSkipsPrereleases(t *testing.T) {
	if _, err := newFakeGitHub(t).client().LatestRelease(context.Background(), "v1.40."); err == nil {
		t.Error("expected no release for a line that has only a prerelease")
	}
}

func TestGitHubSendsTokenWhenSet(t *testing.T) {
	fake := newFakeGitHub(t)
	gh := fake.client()
	if _, err := gh.LatestRelease(context.Background(), "v1.39."); err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if fake.Auth != "" {
		t.Errorf("unauthenticated run should send no Authorization header, got %q", fake.Auth)
	}

	t.Setenv("GITHUB_TOKEN", "ghs_secret")
	if _, err := gh.LatestRelease(context.Background(), "v1.39."); err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if fake.Auth != "Bearer ghs_secret" {
		t.Errorf("expected a bearer token, got %q", fake.Auth)
	}
}

// The checksum file keys its entries by the builder's absolute paths and
// lists the contrib build beside the core one, so a lookup matching on a
// suffix would pin contrib's bytes as the core binary's.
func TestEnvoyLatest(t *testing.T) {
	fake := newFakeGitHub(t)

	line, err := (&Envoy{GitHub: *fake.client()}).Latest(context.Background(), "1.39")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if line.Major != "1.39" || line.Version != "1.39.1" || line.Build != "" {
		t.Errorf("unexpected line: %+v", line)
	}
	if len(line.Archives) != 2 {
		t.Fatalf("expected 2 archives, got %v", line.Archives)
	}
	amd := line.Archives["envoy_139_amd64"]
	if amd.SHA256 != envoyAssetSums["x86_64"] || amd.File != "envoy" || amd.StripPrefix != "" ||
		amd.URL != fake.URL+"/assets/1.39.1/envoy-1.39.1-linux-x86_64" {
		t.Errorf("unexpected amd64 archive: %+v", amd)
	}
	// aarch_64, with the underscore upstream's asset name carries.
	arm := line.Archives["envoy_139_arm64"]
	if arm.SHA256 != envoyAssetSums["aarch_64"] || arm.File != "envoy" ||
		arm.URL != fake.URL+"/assets/1.39.1/envoy-1.39.1-linux-aarch_64" {
		t.Errorf("unexpected arm64 archive: %+v", arm)
	}
}

// An asset without a checksum is not a pin: Bazel would have nothing to
// enforce on the fetch, so the run fails and the previous pin stands.
func TestEnvoyLatestMissingChecksum(t *testing.T) {
	fake := newFakeGitHub(t)
	fake.NoChecksums = true

	if _, err := (&Envoy{GitHub: *fake.client()}).Latest(context.Background(), "1.39"); err == nil {
		t.Error("expected an error when the checksums asset cannot be read")
	}
}
