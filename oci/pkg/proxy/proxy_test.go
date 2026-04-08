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

func newTestProxy(upstream *httptest.Server) *httptest.Server {
	p := proxy.New(upstream.Listener.Addr().String(), "arkeros/senku", proxy.Insecure())
	return httptest.NewServer(p)
}

func TestProxyManifest(t *testing.T) {
	upstream := ocitest.NewServer(t, map[string]ocitest.Response{
		"/v2/arkeros/senku/redis/manifests/latest": {
			Headers: map[string]string{
				"Content-Type":          "application/vnd.oci.image.index.v1+json",
				"Docker-Content-Digest": "sha256:deadbeef",
			},
			Body: `{"schemaVersion":2}`,
		},
	})
	defer upstream.Close()

	srv := newTestProxy(upstream)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v2/redis/manifests/latest")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/vnd.oci.image.index.v1+json" {
		t.Errorf("Content-Type = %q, want OCI index", ct)
	}
	if digest := resp.Header.Get("Docker-Content-Digest"); digest != "sha256:deadbeef" {
		t.Errorf("Docker-Content-Digest = %q, want sha256:deadbeef", digest)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"schemaVersion":2}` {
		t.Errorf("body = %q", body)
	}
}

func TestProxyBlobDirectResponseReturns502(t *testing.T) {
	upstream := ocitest.NewServer(t, map[string]ocitest.Response{
		"/v2/arkeros/senku/redis/blobs/sha256:abc123": {
			Headers: map[string]string{
				"Content-Type": "application/octet-stream",
			},
			Body: "blob-content",
		},
	})
	defer upstream.Close()

	srv := newTestProxy(upstream)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v2/redis/blobs/sha256:abc123")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
}

func TestProxyBlobRedirect(t *testing.T) {
	upstream := ocitest.NewServer(t, map[string]ocitest.Response{
		"/v2/arkeros/senku/redis/blobs/sha256:abc123": {
			Status: http.StatusTemporaryRedirect,
			Headers: map[string]string{
				"Location": "https://storage.example.com/blob/sha256:abc123",
			},
		},
	})
	defer upstream.Close()

	srv := newTestProxy(upstream)
	defer srv.Close()

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
	if loc := resp.Header.Get("Location"); loc != "https://storage.example.com/blob/sha256:abc123" {
		t.Errorf("Location = %q, want storage URL", loc)
	}
}

func TestUpstream404(t *testing.T) {
	upstream := ocitest.NewServer(t, nil)
	defer upstream.Close()

	srv := newTestProxy(upstream)
	defer srv.Close()

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
	upstream := ocitest.NewServer(t, map[string]ocitest.Response{
		"/v2/arkeros/senku/redis/tags/list": {
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: `{"name":"arkeros/senku/redis","tags":["latest","v1.0.0"]}`,
		},
	})
	defer upstream.Close()

	srv := newTestProxy(upstream)
	defer srv.Close()

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
	upstream := ocitest.NewServer(t, map[string]ocitest.Response{
		"/v2/arkeros/senku/go/debian13/tags/list": {
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: `{"name":"arkeros/senku/go/debian13","tags":["v1.0.0"]}`,
		},
	})
	defer upstream.Close()

	srv := newTestProxy(upstream)
	defer srv.Close()

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
	upstream := ocitest.NewServer(t, map[string]ocitest.Response{
		"/v2/arkeros/senku/redis/manifests/latest": {
			Headers: map[string]string{"Content-Type": "application/vnd.oci.image.index.v1+json"},
			Body:   `{"schemaVersion":2}`,
		},
		"/v2/arkeros/senku/nginx/manifests/latest": {
			Headers: map[string]string{"Content-Type": "application/vnd.oci.image.index.v1+json"},
			Body:   `{"schemaVersion":2}`,
		},
	})
	defer upstream.Close()

	srv := newTestProxy(upstream)
	defer srv.Close()

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
	// Generate content for many unique repos
	contents := make(map[string]ocitest.Response)
	for i := range 200 {
		path := fmt.Sprintf("/v2/arkeros/senku/repo%d/manifests/latest", i)
		contents[path] = ocitest.Response{
			Headers: map[string]string{"Content-Type": "application/vnd.oci.image.index.v1+json"},
			Body:   `{"schemaVersion":2}`,
		}
	}

	upstream := ocitest.NewServer(t, contents)
	defer upstream.Close()

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

func TestQueryStringForwarded(t *testing.T) {
	upstream := ocitest.NewServer(t, map[string]ocitest.Response{
		"/v2/arkeros/senku/redis/tags/list?n=10&last=v1.0.0": {
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: `{"name":"arkeros/senku/redis","tags":["v1.0.1","v1.0.2"]}`,
		},
	})
	defer upstream.Close()

	srv := newTestProxy(upstream)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v2/redis/tags/list?n=10&last=v1.0.0")
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
	if len(result.Tags) != 2 || result.Tags[0] != "v1.0.1" {
		t.Errorf("tags = %v, want [v1.0.1 v1.0.2]", result.Tags)
	}
}
