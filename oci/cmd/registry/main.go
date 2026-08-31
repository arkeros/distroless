package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arkeros/distroless/oci/pkg/proxy"
)

// repeatedString collects a flag given more than once, e.g. `--repo=a --repo=b`.
type repeatedString []string

func (r *repeatedString) String() string { return fmt.Sprintf("%v", *r) }

func (r *repeatedString) Set(s string) error {
	*r = append(*r, s)
	return nil
}

var (
	port             *string
	upstream         *string
	repositoryPrefix *string
	repos            repeatedString
)

func init() {
	port = flag.String("port", "8080", "port to listen on")
	upstream = flag.String("upstream", "ghcr.io", "upstream registry host")
	repositoryPrefix = flag.String("repository-prefix", "arkeros/distroless", "repository prefix to prepend to image names")
	flag.Var(&repos, "repo", "repository to expose (may be repeated)")
}

func main() {
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handler := proxy.New(*upstream, *repositoryPrefix, proxy.WithRepos(repos))
	server := &http.Server{
		Handler:           handler,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	listener, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		slog.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	slog.Info("starting registry proxy",
		"addr", listener.Addr(),
		"upstream", *upstream,
		"repository_prefix", *repositoryPrefix)

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
