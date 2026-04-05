package apt

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/arkeros/senku/oci/distroless/debian/snapshot"
)

type updateOptions struct {
	Path string
}

func newCmdUpdate() *cobra.Command {
	o := &updateOptions{}

	cmd := &cobra.Command{
		Use:   "update <yaml-file>",
		Short: "Update Debian snapshot timestamps in a manifest YAML file",
		Long: `Updates the Debian snapshot timestamps to the latest available by:
  1. Fetching the latest snapshot timestamp from snapshot.debian.org
  2. Updating all source URLs in the specified YAML file with the new timestamp

Examples:
  knife apt update oci/distroless/debian13.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Path = args[0]
			return o.Run()
		},
	}

	return cmd
}

func (o *updateOptions) Run() error {
	path := o.Path

	// Resolve path relative to BUILD_WORKSPACE_DIRECTORY if available
	if wsDir := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); wsDir != "" {
		if !filepath.IsAbs(path) {
			path = filepath.Join(wsDir, path)
		}
	}

	slog.Info("Fetching latest Debian snapshot timestamps")

	debianTimestamp, err := snapshot.FetchLatestSnapshot("https://snapshot.debian.org/archive/debian/")
	if err != nil {
		return fmt.Errorf("failed to fetch debian snapshot: %w", err)
	}
	slog.Info("Found latest debian snapshot", "timestamp", debianTimestamp)

	securityTimestamp, err := snapshot.FetchLatestSnapshot("https://snapshot.debian.org/archive/debian-security/")
	if err != nil {
		return fmt.Errorf("failed to fetch debian-security snapshot: %w", err)
	}
	slog.Info("Found latest debian-security snapshot", "timestamp", securityTimestamp)

	manifest, err := snapshot.ParseManifest(path)
	if err != nil {
		return err
	}

	manifest.UpdateTimestamps(debianTimestamp, securityTimestamp)

	if err := manifest.WriteFile(path); err != nil {
		return err
	}

	// Derive the lock target name from the manifest filename (e.g., debian13.yaml -> @debian13//:lock)
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	fmt.Printf("✓ Updated %s\n", path)
	fmt.Printf("  debian: %s\n", debianTimestamp)
	fmt.Printf("  debian-security: %s\n", securityTimestamp)
	fmt.Printf("\nRemember to regenerate the lockfile:\n")
	fmt.Printf("  bazel run @%s//:lock\n", name)

	return nil
}
