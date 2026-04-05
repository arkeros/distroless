package debian

import (
	"github.com/spf13/cobra"
)

func NewCmdDebian() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "debian",
		Short: "Manage Debian packages",
	}

	cmd.AddCommand(newCmdVersions())

	return cmd
}
