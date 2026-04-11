package mod

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Tidy runs `bazel mod tidy` in the workspace directory.
// It uses BUILD_WORKSPACE_DIRECTORY when available (i.e., running via `bazel run`).
func Tidy(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "bazel", "mod", "tidy", "--lockfile_mode=update")
	if wsDir := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); wsDir != "" {
		cmd.Dir = wsDir
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bazel mod tidy failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}
