package tarballs

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/arkeros/distroless/bazel/mod"
	"github.com/arkeros/distroless/bazel/tarballs"
)

type updateOptions struct {
	LockFile string
}

func newCmdUpdate() *cobra.Command {
	o := &updateOptions{}

	cmd := &cobra.Command{
		Use:   "update <lockfile>",
		Short: "Move every release line in a tarball lockfile to its newest upstream release",
		Long: `Updates a tarball lockfile consumed by the tarballs module extension by:
  1. Asking the lockfile's upstream (nodejs.org, the Adoptium API or
     envoyproxy's GitHub releases) for the newest release of every major
     line already listed
  2. Rewriting the lockfile with the new versions, URLs and checksums
  3. Running bazel mod tidy to refresh MODULE.bazel.lock

Which majors are listed is a policy choice made by hand; this command never
adds or drops a line.

Examples:
  knife tarballs update images/nodejs/nodejs.lock.json
  knife tarballs update images/java/temurin.lock.json
  knife tarballs update images/envoy/envoy.lock.json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.LockFile = args[0]
			return o.Run(cmd.Context())
		},
	}

	return cmd
}

func (o *updateOptions) Run(ctx context.Context) error {
	path := o.LockFile

	// Resolve path relative to BUILD_WORKSPACE_DIRECTORY if available
	if wsDir := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); wsDir != "" {
		if !filepath.IsAbs(path) {
			path = filepath.Join(wsDir, path)
		}
	}

	lock, err := tarballs.ReadLock(path)
	if err != nil {
		return err
	}
	src, err := tarballs.NewSource(lock.Source)
	if err != nil {
		return err
	}

	slog.Info("Resolving latest releases", "source", lock.Source, "lines", len(lock.Lines))
	changes, err := tarballs.Update(ctx, lock, src)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		fmt.Printf("✓ %s is up to date\n", path)
		return nil
	}

	if err := lock.Write(path); err != nil {
		return err
	}
	fmt.Printf("✓ Updated %s\n", path)
	for _, c := range changes {
		fmt.Printf("  %s\n", c)
	}

	slog.Info("Updating MODULE.bazel.lock")
	if err := mod.Tidy(ctx); err != nil {
		return fmt.Errorf("failed to update lockfile: %w", err)
	}
	fmt.Printf("✓ Updated MODULE.bazel.lock\n")

	return nil
}
