// Package ocitest provides test helpers for OCI registry interactions.
package ocitest

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// Server is an httptest.Server running a real in-memory OCI registry
// fronted by a token-based auth layer.
type Server struct {
	*httptest.Server
}

// FakeRegistry implements a minimal OCI auth layer in front of registry.New().
// On /v2/ it returns a 401 with Www-Authenticate pointing to /auth/token.
// On /auth/token it issues bearer tokens scoped per repository.
// On /v2/<repo>/... it validates the bearer token and delegates to the real registry.
type FakeRegistry struct {
	backend http.Handler
	// DenyAuth, when true, makes the token endpoint return 403 for
	// scopes that don't match any pushed repository.
	DenyAuth bool
	// pushedRepos tracks which repos have been pushed to, for DenyAuth scope checking.
	pushedRepos map[string]bool
}

func (f *FakeRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Token endpoint at a non-standard path — only discoverable via Www-Authenticate challenge.
	// This ensures the proxy follows the OCI auth spec rather than hardcoding a token URL.
	if r.URL.Path == "/auth/token" {
		scope := r.URL.Query().Get("scope")
		if f.DenyAuth && !f.scopeMatchesPushed(scope) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"errors":[{"code":"DENIED","message":"requested access to the resource is denied"}]}`)
			return
		}
		token := "token-" + scope
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"token": token})
		return
	}

	// /v2/ ping — return 401 with challenge pointing to our token endpoint.
	// Use "localhost" instead of the raw IP to avoid go-containerregistry's
	// SSRF protection rejecting loopback IP literals in realm URLs.
	if r.URL.Path == "/v2/" || r.URL.Path == "/v2" {
		_, port, _ := net.SplitHostPort(r.Host)
		host := net.JoinHostPort("localhost", port)
		w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="http://%s/auth/token",service="%s"`, host, host))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Write operations (push) bypass token scoping — they are only used
	// by MustPushImage during test setup and the upload paths don't follow
	// the simple /v2/<repo>/<op>/<ref> layout that our scope check expects.
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		f.backend.ServeHTTP(w, r)
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
	// The scope in the token is "repository:<repo>:<actions>" — we only
	// verify the repo part matches, not the specific actions.
	if strings.HasPrefix(r.URL.Path, "/v2/") {
		rest := strings.TrimPrefix(r.URL.Path, "/v2/")
		parts := strings.Split(rest, "/")
		if len(parts) >= 3 {
			repo := strings.Join(parts[:len(parts)-2], "/")
			if repo != "" {
				scopeRepo := extractRepoFromScope(scope)
				if scopeRepo != repo {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
			}
		}
	}

	// Delegate to the real registry backend.
	f.backend.ServeHTTP(w, r)
}

// extractRepoFromScope extracts the repository name from an OCI scope string.
// Scope format: "repository:<repo>:<actions>"
func extractRepoFromScope(scope string) string {
	parts := strings.SplitN(scope, ":", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// scopeMatchesPushed returns true if the scope references a repo that has been pushed to.
func (f *FakeRegistry) scopeMatchesPushed(scope string) bool {
	return f.pushedRepos[extractRepoFromScope(scope)]
}

// NewServer creates an httptest.Server running a real OCI registry with auth.
func NewServer(t *testing.T) *Server {
	t.Helper()
	fr := &FakeRegistry{
		backend:     registry.New(),
		pushedRepos: make(map[string]bool),
	}
	srv := httptest.NewServer(fr)
	t.Cleanup(srv.Close)
	return &Server{Server: srv}
}

// NewServerDenyAuth creates an httptest.Server that denies auth tokens for
// repos that haven't been pushed to, mimicking GHCR behavior for non-existent repos.
func NewServerDenyAuth(t *testing.T) *Server {
	t.Helper()
	fr := &FakeRegistry{
		backend:     registry.New(),
		DenyAuth:    true,
		pushedRepos: make(map[string]bool),
	}
	srv := httptest.NewServer(fr)
	t.Cleanup(srv.Close)
	return &Server{Server: srv}
}

// WithBlobRedirect returns a new httptest.Server that redirects all blob
// requests to the given URL, mimicking how production registries (e.g. GHCR)
// redirect blob downloads to external storage. All other requests are
// delegated to the underlying registry with auth.
func (s *Server) WithBlobRedirect(t *testing.T, redirectURL string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/blobs/sha256:") {
			w.Header().Set("Location", redirectURL)
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		s.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// MustPushImage pushes a random image to the given repo:tag and returns it.
// The repo should be the full path as the upstream sees it (e.g. "arkeros/senku/redis").
func (s *Server) MustPushImage(t *testing.T, repo, tag string) v1.Image {
	t.Helper()
	ref, err := name.ParseReference(
		fmt.Sprintf("%s/%s:%s", s.Listener.Addr().String(), repo, tag),
		name.Insecure,
	)
	if err != nil {
		t.Fatal(err)
	}
	img, err := random.Image(256, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Temporarily disable DenyAuth so the push token request succeeds.
	fr := s.Config.Handler.(*FakeRegistry)
	savedDeny := fr.DenyAuth
	fr.DenyAuth = false
	if err := remote.Write(ref, img); err != nil {
		t.Fatal(err)
	}
	fr.DenyAuth = savedDeny
	fr.pushedRepos[repo] = true
	return img
}
