// Package ocitest provides test helpers for OCI registry interactions.
package ocitest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Response defines a canned response for a given path in FakeRegistry.
type Response struct {
	Status  int
	Headers map[string]string
	Body    string
}

// FakeRegistry implements a minimal OCI registry with standard auth challenge flow.
// On /v2/ it returns a 401 with Www-Authenticate pointing to /auth/token.
// On /auth/token it issues bearer tokens scoped per repository.
// On /v2/<repo>/... it validates the bearer token and serves content.
type FakeRegistry struct {
	t        *testing.T
	contents map[string]Response // path → response
}

func (f *FakeRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		for k, v := range resp.Headers {
			w.Header().Set(k, v)
		}
		status := resp.Status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		fmt.Fprint(w, resp.Body)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprint(w, `{"errors":[{"code":"NAME_UNKNOWN"}]}`)
}

// NewServer creates an httptest.Server running a FakeRegistry with the given canned responses.
func NewServer(t *testing.T, contents map[string]Response) *httptest.Server {
	return httptest.NewServer(&FakeRegistry{t: t, contents: contents})
}
