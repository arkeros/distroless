// Command server serves the human-readable view of the Mirror: what is inside
// a published image, read from the signed evidence attached to its Digest.
//
// It is a reader of the Mirror, never a writer. Everything it shows comes from
// an Attestation it verified first.
package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arkeros/distroless/web/internal/attestation"
	"github.com/arkeros/distroless/web/internal/compress"
	"github.com/arkeros/distroless/web/internal/directory"
	"github.com/arkeros/distroless/web/internal/mirror"
	"github.com/arkeros/distroless/web/internal/policy"
)

var (
	port             = flag.String("port", "8080", "port to listen on")
	upstream         = flag.String("upstream", "ghcr.io", "registry holding the mirror's images")
	repositoryPrefix = flag.String("repository-prefix", "arkeros/distroless", "repository prefix to prepend to image names")

	// Display only: the host a reader pulls by, which is not where these
	// bytes are fetched from.
	mirrorHost = flag.String("mirror-host", "distroless.io", "host the images are published under, shown on the page")

	// Defaults come from //oci:cosign_policy.bzl. They are flags so a staging
	// deployment can point at another signer, not so this one can relax.
	certificateIdentity = flag.String("certificate-identity-regexp", policy.CertificateIdentityRegexp,
		"regexp the signing certificate's OIDC subject must match")
	oidcIssuer = flag.String("oidc-issuer", policy.CertificateOIDCIssuer,
		"issuer the signing certificate must come from")
)

func main() {
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// A process that cannot verify has nothing it may legitimately serve, so
	// this is fatal rather than a degraded mode.
	verifier, err := attestation.New(*certificateIdentity, *oidcIssuer)
	if err != nil {
		slog.Error("cannot build attestation verifier", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/", directory.NewHandler(mirror.New(*upstream, *repositoryPrefix, verifier), *mirrorHost))
	// Deliberately not on a registry-backed path: a probe must not fail
	// because GHCR is slow, or Cloud Run will recycle a healthy instance.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, "ok\n")
	})

	server := &http.Server{
		// Nothing in front of this process compresses — Cloud Run's frontend
		// passes bodies through — so it is done here, for every route.
		Handler:           compress.Handler(mux),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		// Longer than the registry proxy's: rendering a page costs a tag
		// resolve, a referrers list, and a blob pull before a byte is written.
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	listener, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		slog.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	slog.Info("starting directory server",
		"addr", listener.Addr(),
		"upstream", *upstream,
		"repository_prefix", *repositoryPrefix,
		"certificate_identity_regexp", *certificateIdentity)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()

	select {
	case err := <-errCh:
		slog.Error("server failed", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown failed", "error", err)
			os.Exit(1)
		}
	}
}
