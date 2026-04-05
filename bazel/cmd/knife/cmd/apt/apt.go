package apt

import (
	"github.com/spf13/cobra"
)

func NewCmdApt() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apt",
		Short: "Manage apt package manifests and lockfiles",
	}

	cmd.AddCommand(newCmdUpdate())
	cmd.AddCommand(newCmdVersions())

	return cmd
}
