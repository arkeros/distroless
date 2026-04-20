package prebuilts

import "github.com/spf13/cobra"

func NewCmdPrebuilts() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prebuilts",
		Short: "Manage pinned prebuilt CLI binaries sourced from GitHub Releases",
	}
	cmd.AddCommand(newCmdUpdate())
	return cmd
}
