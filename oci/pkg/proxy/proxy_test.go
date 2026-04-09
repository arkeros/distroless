package proxy_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arkeros/senku/oci/pkg/proxy"
	"github.com/arkeros/senku/oci/ocitest"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func TestV2Base(t *testing.T) {
	p := proxy.New("ghcr.io", "arkeros/senku")
	srv := httptest.NewServer(p)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v2/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /v2/ status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestV2BaseIncludesDistributionHeader(t *testing.T) {
	p := proxy.New("ghcr.io", "arkeros/senku")
	srv := httptest.NewServer(p)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v2/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if v := resp.Header.Get("Docker-Distribution-Api-Version"); v != "registry/2.0" {
		t.Errorf("Docker-Distribution-Api-Version = %q, want %q", v, "registry/2.0")
	}
}

func TestRewritePath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/v2/redis/manifests/latest", "/v2/arkeros/senku/redis/manifests/latest"},
		{"/v2/redis/blobs/sha256:abc123", "/v2/arkeros/senku/redis/blobs/sha256:abc123"},
		{"/v2/redis/tags/list", "/v2/arkeros/senku/redis/tags/list"},
		{"/v2/go/debian13/manifests/v1.0.0", "/v2/arkeros/senku/go/debian13/manifests/v1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := proxy.RewritePath(tt.path, "arkeros/senku")
			if got != tt.want {
				t.Errorf("RewritePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestExtractRepo(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/v2/redis/manifests/latest", "redis"},
		{"/v2/redis/blobs/sha256:abc123", "redis"},
		{"/v2/redis/tags/list", "redis"},
		{"/v2/go/debian13/manifests/v1.0.0", "go/debian13"},
		// Repos with op-like names must not be misclassified.
		{"/v2/org/manifests/manifests/latest", "org/manifests"},
		{"/v2/org/blobs/blobs/sha256:abc", "org/blobs"},
		{"/v2/org/tags/tags/list", "org/tags"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := proxy.ExtractRepo(tt.path)
			if got != tt.want {
				t.Errorf("ExtractRepo(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsBlob(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/v2/redis/blobs/sha256:abc", true},
		{"/v2/org/blobs/blobs/sha256:abc", true},
		{"/v2/redis/manifests/latest", false},
		{"/v2/redis/tags/list", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := proxy.IsBlob(tt.path)
			if got != tt.want {
				t.Errorf("IsBlob(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestNonV2PathReturns404(t *testing.T) {
	p := proxy.New("ghcr.io", "arkeros/senku")
	srv := httptest.NewServer(p)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func newTestProxy(t *testing.T, upstream *ocitest.Server) *httptest.Server {
	t.Helper()
	p := proxy.New(upstream.Listener.Addr().String(), "arkeros/senku", proxy.Insecure())
	srv := httptest.NewServer(p)
	t.Cleanup(srv.Close)
	return srv
}

// firstLayerDigest returns the digest of the first layer in the image.
func firstLayerDigest(t *testing.T, img v1.Image) v1.Hash {
	t.Helper()
	layers, err := img.Layers()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := layers[0].Digest()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestProxyManifest(t *testing.T) {
	upstream := ocitest.NewServer(t)
	img := upstream.MustPushImage(t, "arkeros/senku/redis", "latest")

	srv := newTestProxy(t, upstream)

	resp, err := http.Get(srv.URL + "/v2/redis/manifests/latest")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	mt, _ := img.MediaType()
	if ct := resp.Header.Get("Content-Type"); ct != string(mt) {
		t.Errorf("Content-Type = %q, want %q", ct, mt)
	}
	wantDigest, _ := img.Digest()
	if digest := resp.Header.Get("Docker-Content-Digest"); digest != wantDigest.String() {
		t.Errorf("Docker-Content-Digest = %q, want %q", digest, wantDigest)
	}
	body, _ := io.ReadAll(resp.Body)
	wantManifest, _ := img.RawManifest()
	if string(body) != string(wantManifest) {
		t.Errorf("body mismatch")
	}
}

func TestProxyBlobDirectResponseReturns502(t *testing.T) {
	upstream := ocitest.NewServer(t)
	img := upstream.MustPushImage(t, "arkeros/senku/redis", "latest")
	digest := firstLayerDigest(t, img)

	srv := newTestProxy(t, upstream)

	resp, err := http.Get(fmt.Sprintf("%s/v2/redis/blobs/%s", srv.URL, digest))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
}

func TestProxyBlobRedirect(t *testing.T) {
	const redirectURL = "https://storage.example.com/blob/sha256:abc123"

	upstream := ocitest.NewServer(t)
	redirector := upstream.WithBlobRedirect(t, redirectURL)

	p := proxy.New(redirector.Listener.Addr().String(), "arkeros/senku", proxy.Insecure())
	srv := httptest.NewServer(p)
	t.Cleanup(srv.Close)

	// Don't follow redirects — we want to verify the proxy passes through the 307 + Location
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.Get(srv.URL + "/v2/redis/blobs/sha256:abc123")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}
	if loc := resp.Header.Get("Location"); loc != redirectURL {
		t.Errorf("Location = %q, want storage URL", loc)
	}
}

func TestUpstream404(t *testing.T) {
	upstream := ocitest.NewServer(t)

	srv := newTestProxy(t, upstream)

	resp, err := http.Get(srv.URL + "/v2/nonexistent/manifests/latest")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestTagsList(t *testing.T) {
	upstream := ocitest.NewServer(t)
	upstream.MustPushImage(t, "arkeros/senku/redis", "latest")
	upstream.MustPushImage(t, "arkeros/senku/redis", "v1.0.0")

	srv := newTestProxy(t, upstream)

	resp, err := http.Get(srv.URL + "/v2/redis/tags/list")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Name != "redis" {
		t.Errorf("name = %q, want %q", result.Name, "redis")
	}
	if len(result.Tags) != 2 {
		t.Errorf("tags count = %d, want 2", len(result.Tags))
	}
}

func TestTagsListMultiSegmentRepo(t *testing.T) {
	upstream := ocitest.NewServer(t)
	upstream.MustPushImage(t, "arkeros/senku/go/debian13", "v1.0.0")

	srv := newTestProxy(t, upstream)

	resp, err := http.Get(srv.URL + "/v2/go/debian13/tags/list")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Name != "go/debian13" {
		t.Errorf("name = %q, want %q", result.Name, "go/debian13")
	}
}

func TestPerRepoTokenScoping(t *testing.T) {
	upstream := ocitest.NewServer(t)
	upstream.MustPushImage(t, "arkeros/senku/redis", "latest")
	upstream.MustPushImage(t, "arkeros/senku/nginx", "latest")

	srv := newTestProxy(t, upstream)

	// Pull redis first
	resp, err := http.Get(srv.URL + "/v2/redis/manifests/latest")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("redis: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Pull nginx — must work with its own token
	resp, err = http.Get(srv.URL + "/v2/nginx/manifests/latest")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("nginx: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestTransportCacheIsBounded(t *testing.T) {
	upstream := ocitest.NewServer(t)
	for i := range 200 {
		upstream.MustPushImage(t, fmt.Sprintf("arkeros/senku/repo%d", i), "latest")
	}

	p := proxy.New(upstream.Listener.Addr().String(), "arkeros/senku", proxy.Insecure())
	srv := httptest.NewServer(p)
	defer srv.Close()

	// Request 200 unique repos
	for i := range 200 {
		resp, err := http.Get(fmt.Sprintf("%s/v2/repo%d/manifests/latest", srv.URL, i))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("repo%d: status = %d, want %d", i, resp.StatusCode, http.StatusOK)
		}
	}

	// Cache should not have grown beyond the limit
	if size := p.CacheLen(); size > proxy.MaxCacheEntries {
		t.Errorf("transport cache size = %d, want <= %d", size, proxy.MaxCacheEntries)
	}
}

func TestCatalogEmpty(t *testing.T) {
	p := proxy.New("ghcr.io", "arkeros/senku")
	srv := httptest.NewServer(p)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v2/_catalog")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	want := `{"repositories":[]}`
	got := strings.TrimSpace(string(body))
	if got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestCatalog(t *testing.T) {
	p := proxy.New("ghcr.io", "arkeros/senku", proxy.WithRepos([]string{"redis", "nginx"}))
	srv := httptest.NewServer(p)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v2/_catalog")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var result struct {
		Repositories []string `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Repositories) != 2 || result.Repositories[0] != "redis" || result.Repositories[1] != "nginx" {
		t.Errorf("repositories = %v, want [redis nginx]", result.Repositories)
	}
}

func TestNonExistentRepoForwardsUpstreamStatus(t *testing.T) {
	upstream := ocitest.NewServerDenyAuth(t)
	upstream.MustPushImage(t, "arkeros/senku/nginx", "latest")

	p := proxy.New(upstream.Listener.Addr().String(), "arkeros/senku", proxy.Insecure())
	srv := httptest.NewServer(p)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v2/apache/manifests/latest")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestQueryStringForwarded(t *testing.T) {
	upstream := ocitest.NewServer(t)
	upstream.MustPushImage(t, "arkeros/senku/redis", "v1.0.0")
	upstream.MustPushImage(t, "arkeros/senku/redis", "v1.0.1")
	upstream.MustPushImage(t, "arkeros/senku/redis", "v1.0.2")

	srv := newTestProxy(t, upstream)

	// Request with n=2 to paginate — the proxy must forward the query string.
	resp, err := http.Get(srv.URL + "/v2/redis/tags/list?n=2")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Name != "redis" {
		t.Errorf("name = %q, want %q", result.Name, "redis")
	}
	if len(result.Tags) != 2 {
		t.Errorf("tags count = %d, want 2 (paginated)", len(result.Tags))
	}
}
