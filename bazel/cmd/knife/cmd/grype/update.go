package grype

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/anchore/grype/grype/db/v6/distribution"
	"github.com/spf13/cobra"

	grypedbpkg "github.com/arkeros/senku/bazel/pkg/grypedb"
	"github.com/arkeros/senku/bazel/pkg/mod"
)

type updateOptions struct {
	ModuleFile string
}

func newCmdUpdate() *cobra.Command {
	o := &updateOptions{
		ModuleFile: "bazel/include/oci.MODULE.bazel",
	}

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update the grype vulnerability database to the latest version",
		Long: `Updates the grype vulnerability database to the latest version by:
  1. Fetching the latest database metadata (URL and SHA256) from grype.anchore.io
  2. Updating the MODULE.bazel include file with the new URL and SHA256
  3. Running bazel mod tidy to update the lockfile

Examples:
  knife grype update
  knife grype update --module-file bazel/include/oci.MODULE.bazel`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.Run(cmd.Context())
		},
	}

	cmd.Flags().StringVar(&o.ModuleFile, "module-file", o.ModuleFile, "path to the MODULE.bazel file containing the grype database config")

	return cmd
}

func (o *updateOptions) Run(ctx context.Context) error {
	path := o.ModuleFile

	// Resolve path relative to BUILD_WORKSPACE_DIRECTORY if available
	if wsDir := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); wsDir != "" {
		if !filepath.IsAbs(path) {
			path = filepath.Join(wsDir, path)
		}
	}

	slog.Info("Fetching latest grype database metadata")

	client, err := distribution.NewClient(distribution.Config{
		LatestURL: "https://grype.anchore.io/databases/v6/latest.json",
	})
	if err != nil {
		return fmt.Errorf("failed to create grype client: %w", err)
	}

	latest, err := client.Latest()
	if err != nil {
		return fmt.Errorf("failed to fetch latest grype metadata: %w", err)
	}

	fullURL := fmt.Sprintf("https://grype.anchore.io/databases/v6/%s", latest.Archive.Path)
	sha256sum := strings.TrimPrefix(latest.Archive.Checksum, "sha256:")

	slog.Info("Latest grype database found",
		"built", latest.Archive.Built.String(),
		"url", fullURL,
		"sha256", sha256sum)

	if err := grypedbpkg.UpdateModuleFile(path, fullURL, sha256sum); err != nil {
		return err
	}

	fmt.Printf("✓ Updated grype database in %s\n", path)
	fmt.Printf("  URL: %s\n", fullURL)
	fmt.Printf("  SHA256: %s\n", sha256sum)

	// Update lockfile
	slog.Info("Updating MODULE.bazel.lock")
	if err := mod.Tidy(ctx); err != nil {
		return fmt.Errorf("failed to update lockfile: %w", err)
	}
	fmt.Printf("✓ Updated MODULE.bazel.lock\n")

	return nil
}
