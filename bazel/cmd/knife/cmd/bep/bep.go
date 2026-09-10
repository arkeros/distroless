// Package bep exposes //bazel/bep as `knife bep`.
package bep

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/arkeros/distroless/bazel/bep"
)

func NewCmdBep() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bep",
		Short: "Read a Build Event Protocol stream",
	}
	cmd.AddCommand(newCmdDigests())
	return cmd
}

func newCmdDigests() *cobra.Command {
	return &cobra.Command{
		Use:   "digests <bep.json>",
		Short: "Report `{target: hash}` over the outputs each target produced",
		Long: `Reduces the stream that --build_event_json_file writes to one hash per
target, over the digests of that target's default outputs.

Two of these maps, from two revisions, say which targets' bytes actually
changed -- as opposed to which will merely be rebuilt.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			file, err := os.Open(args[0])
			if err != nil {
				return fmt.Errorf("opening the build event file: %w", err)
			}
			defer file.Close()

			digests, err := bep.OutputDigests(file)
			if err != nil {
				return err
			}
			// Compact and key-sorted, so two maps can be diffed as text as
			// well as parsed.
			encoder := json.NewEncoder(cmd.OutOrStdout())
			return encoder.Encode(digests)
		},
	}
}
