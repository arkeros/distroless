package snapshots

import (
	"github.com/spf13/cobra"
)

func NewCmdSnapshots() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshots",
		Short: "Manage Debian snapshot timestamps",
	}

	cmd.AddCommand(newCmdUpdate())

	return cmd
}
