package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/arkeros/senku/bazel/cmd/knife/cmd"
)

func main() {
	ctx := context.Background()

	if err := cmd.Execute(ctx); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}
