package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/arkeros/senku/base/cache/lru"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

const MaxCacheEntries = 100

type Proxy struct {
	upstream         string
	repositoryPrefix string
	scheme           string

	transports *lru.Cache[string, http.RoundTripper]
}

// Option configures a Proxy.
type Option func(*Proxy)

// Insecure configures the proxy to use plain HTTP instead of HTTPS.
func Insecure() Option {
	return func(p *Proxy) {
		p.scheme = "http"
	}
}

func New(upstream, repositoryPrefix string, opts ...Option) *Proxy {
	p := &Proxy{
		upstream:         upstream,
		repositoryPrefix: repositoryPrefix,
		scheme:           "https",
		transports:       lru.New[string, http.RoundTripper](MaxCacheEntries),
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if r.URL.Path == "/v2/" || r.URL.Path == "/v2" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
		fmt.Fprint(w, "{}")
		return
	}

	if !strings.HasPrefix(r.URL.Path, "/v2/") {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	p.proxyRequest(w, r)
}

func RewritePath(path, repositoryPrefix string) string {
	// /v2/<name>/... → /v2/<prefix>/<name>/...
	const prefix = "/v2/"
	rest := strings.TrimPrefix(path, prefix)
	return prefix + repositoryPrefix + "/" + rest
}

// findOp scans path segments from the tail to find the first OCI operation
// segment ("manifests", "blobs", or "tags") and returns its index.
// Returns -1 if no operation segment is found.
func findOp(segments []string) int {
	for i := len(segments) - 1; i >= 0; i-- {
		switch segments[i] {
		case "manifests", "blobs", "tags":
			return i
		}
	}
	return -1
}

// ExtractRepo extracts the repository name from the request path.
// e.g., /v2/redis/manifests/latest → redis
// e.g., /v2/go/debian13/tags/list → go/debian13
func ExtractRepo(path string) string {
	rest := strings.TrimPrefix(path, "/v2/")
	segments := strings.Split(rest, "/")
	if i := findOp(segments); i > 0 {
		return strings.Join(segments[:i], "/")
	}
	return rest
}

// IsBlob reports whether the path targets a blob endpoint.
func IsBlob(path string) bool {
	rest := strings.TrimPrefix(path, "/v2/")
	segments := strings.Split(rest, "/")
	if i := findOp(segments); i >= 0 {
		return segments[i] == "blobs"
	}
	return false
}

func (p *Proxy) getTransport(repo string) (http.RoundTripper, error) {
	if t, ok := p.transports.Get(repo); ok {
		return t, nil
	}

	fullRepo := p.repositoryPrefix + "/" + repo
	opts := []name.Option{name.WithDefaultRegistry(p.upstream)}
	if p.scheme == "http" {
		opts = append(opts, name.Insecure)
	}
	ref, err := name.NewRepository(fullRepo, opts...)
	if err != nil {
		return nil, fmt.Errorf("parse repository: %w", err)
	}

	t, err := transport.New(
		ref.Registry,
		authn.Anonymous,
		http.DefaultTransport,
		[]string{ref.Scope(transport.PullScope)},
	)
	if err != nil {
		return nil, fmt.Errorf("transport: %w", err)
	}

	p.transports.Put(repo, t)
	return t, nil
}

// CacheLen returns the number of cached transports.
func (p *Proxy) CacheLen() int {
	return p.transports.Len()
}

var proxyHeaders = []string{
	"Content-Type",
	"Content-Length",
	"Docker-Content-Digest",
	"Docker-Distribution-Api-Version",
	"ETag",
	"Link",
	"Location",
}

func (p *Proxy) proxyRequest(w http.ResponseWriter, r *http.Request) {
	repo := ExtractRepo(r.URL.Path)
	t, err := p.getTransport(repo)
	if err != nil {
		slog.Error("transport setup failed", "repo", repo, "error", err)
		http.Error(w, "transport setup failed", http.StatusBadGateway)
		return
	}

	u := *r.URL
	u.Scheme = p.scheme
	u.Host = p.upstream
	u.Path = RewritePath(r.URL.Path, p.repositoryPrefix)

	req, err := http.NewRequestWithContext(r.Context(), r.Method, u.String(), nil)
	if err != nil {
		http.Error(w, "bad request", http.StatusInternalServerError)
		return
	}

	// Forward Accept headers for content negotiation
	for _, v := range r.Header.Values("Accept") {
		req.Header.Add("Accept", v)
	}

	resp, err := t.RoundTrip(req)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if IsBlob(r.URL.Path) && resp.StatusCode < 300 {
		slog.Error("upstream returned blob body instead of redirect", "path", r.URL.Path, "status", resp.StatusCode)
		http.Error(w, "upstream did not redirect blob request", http.StatusBadGateway)
		return
	}

	for _, h := range proxyHeaders {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}

	if isTagsList(r.URL.Path) && resp.StatusCode == http.StatusOK {
		rewriteTagsName(w, resp.Body, repo, resp.StatusCode)
		return
	}

	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Warn("failed to copy response body", "path", req.URL.Path, "error", err)
	}
}

// isTagsList reports whether the path targets a tags/list endpoint.
func isTagsList(path string) bool {
	return strings.HasSuffix(path, "/tags/list")
}

// rewriteTagsName rewrites the "name" field in a tags-list JSON response
// so clients see the vanity repo name instead of the prefixed upstream name.
func rewriteTagsName(w http.ResponseWriter, body io.Reader, repo string, statusCode int) {
	data, err := io.ReadAll(body)
	if err != nil {
		slog.Warn("failed to read tags response", "error", err)
		w.WriteHeader(statusCode)
		return
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(data, &result); err != nil {
		w.WriteHeader(statusCode)
		w.Write(data)
		return
	}

	result["name"], _ = json.Marshal(repo)

	out, err := json.Marshal(result)
	if err != nil {
		w.WriteHeader(statusCode)
		w.Write(data)
		return
	}

	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(out)))
	w.WriteHeader(statusCode)
	if _, err := io.Copy(w, bytes.NewReader(out)); err != nil {
		slog.Warn("failed to write rewritten tags response", "error", err)
	}
}
