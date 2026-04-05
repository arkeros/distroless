package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arkeros/senku/oci/pkg/proxy"
)

var (
	port             *string
	upstream         *string
	repositoryPrefix *string
)

func init() {
	port = flag.String("port", "8080", "port to listen on")
	upstream = flag.String("upstream", "ghcr.io", "upstream registry host")
	repositoryPrefix = flag.String("repository-prefix", "arkeros/senku", "repository prefix to prepend to image names")
}

func main() {
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handler := proxy.New(*upstream, *repositoryPrefix)
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
