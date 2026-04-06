package terraform

import "github.com/spf13/cobra"

func NewCmdTerraform() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "terraform",
		Short: "Manage Terraform artifacts",
	}

	cmd.AddCommand(newCmdRender())

	return cmd
}
