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

// fakeRegistry implements a minimal OCI registry with standard auth challenge flow.
// On /v2/ it returns a 401 with Www-Authenticate pointing to /auth/token.
// On /auth/token it issues bearer tokens scoped per repository.
// On /v2/<repo>/... it validates the bearer token and serves content.
type fakeRegistry struct {
	t        *testing.T
	contents map[string]fakeResponse // path → response
}

type fakeResponse struct {
	status  int
	headers map[string]string
	body    string
}

func (f *fakeRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Token endpoint at a non-standard path — only discoverable via Www-Authenticate challenge.
	// This ensures the proxy follows the OCI auth spec rather than hardcoding a token URL.
	if r.URL.Path == "/auth/token" {
		scope := r.URL.Query().Get("scope")
		token := "token-" + scope
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"token": token})
		return
	}

	// /v2/ ping — return 401 with challenge pointing to our token endpoint
	if r.URL.Path == "/v2/" || r.URL.Path == "/v2" {
		host := r.Host
		w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="http://%s/auth/token",service="%s"`, host, host))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Registry API — require valid bearer token
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	if !strings.HasPrefix(token, "token-") {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	scope := strings.TrimPrefix(token, "token-")

	// Enforce that the token scope matches the requested repository.
	if strings.HasPrefix(r.URL.Path, "/v2/") {
		rest := strings.TrimPrefix(r.URL.Path, "/v2/")
		parts := strings.Split(rest, "/")
		if len(parts) >= 3 {
			repo := strings.Join(parts[:len(parts)-2], "/")
			if repo != "" {
				expectedScope := "repository:" + repo + ":pull"
				if scope != expectedScope {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
			}
		}
	}

	key := r.URL.Path
	if r.URL.RawQuery != "" {
		key += "?" + r.URL.RawQuery
	}
	if resp, ok := f.contents[key]; ok {
		for k, v := range resp.headers {
			w.Header().Set(k, v)
		}
		status := resp.status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		fmt.Fprint(w, resp.body)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprint(w, `{"errors":[{"code":"NAME_UNKNOWN"}]}`)
}

func newFakeRegistry(t *testing.T, contents map[string]fakeResponse) *httptest.Server {
	return httptest.NewServer(&fakeRegistry{t: t, contents: contents})
}

func newTestProxy(upstream *httptest.Server) *httptest.Server {
	p := proxy.New(upstream.Listener.Addr().String(), "arkeros/senku", proxy.Insecure())
	return httptest.NewServer(p)
}

func TestProxyManifest(t *testing.T) {
	upstream := newFakeRegistry(t, map[string]fakeResponse{
		"/v2/arkeros/senku/redis/manifests/latest": {
			headers: map[string]string{
				"Content-Type":          "application/vnd.oci.image.index.v1+json",
				"Docker-Content-Digest": "sha256:deadbeef",
			},
			body: `{"schemaVersion":2}`,
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
	upstream := newFakeRegistry(t, map[string]fakeResponse{
		"/v2/arkeros/senku/redis/blobs/sha256:abc123": {
			headers: map[string]string{
				"Content-Type": "application/octet-stream",
			},
			body: "blob-content",
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
	upstream := newFakeRegistry(t, map[string]fakeResponse{
		"/v2/arkeros/senku/redis/blobs/sha256:abc123": {
			status: http.StatusTemporaryRedirect,
			headers: map[string]string{
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
	upstream := newFakeRegistry(t, nil)
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
	upstream := newFakeRegistry(t, map[string]fakeResponse{
		"/v2/arkeros/senku/redis/tags/list": {
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body: `{"name":"arkeros/senku/redis","tags":["latest","v1.0.0"]}`,
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
	upstream := newFakeRegistry(t, map[string]fakeResponse{
		"/v2/arkeros/senku/go/debian13/tags/list": {
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body: `{"name":"arkeros/senku/go/debian13","tags":["v1.0.0"]}`,
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
	upstream := newFakeRegistry(t, map[string]fakeResponse{
		"/v2/arkeros/senku/redis/manifests/latest": {
			headers: map[string]string{"Content-Type": "application/vnd.oci.image.index.v1+json"},
			body:    `{"schemaVersion":2}`,
		},
		"/v2/arkeros/senku/nginx/manifests/latest": {
			headers: map[string]string{"Content-Type": "application/vnd.oci.image.index.v1+json"},
			body:    `{"schemaVersion":2}`,
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
	contents := make(map[string]fakeResponse)
	for i := range 200 {
		path := fmt.Sprintf("/v2/arkeros/senku/repo%d/manifests/latest", i)
		contents[path] = fakeResponse{
			headers: map[string]string{"Content-Type": "application/vnd.oci.image.index.v1+json"},
			body:    `{"schemaVersion":2}`,
		}
	}

	upstream := newFakeRegistry(t, contents)
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

func TestQueryStringForwarded(t *testing.T) {
	upstream := newFakeRegistry(t, map[string]fakeResponse{
		"/v2/arkeros/senku/redis/tags/list?n=10&last=v1.0.0": {
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body: `{"name":"arkeros/senku/redis","tags":["v1.0.1","v1.0.2"]}`,
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
