package cloudrun

import "github.com/spf13/cobra"

func NewCmdCloudRun() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cloudrun",
		Short: "Manage Cloud Run artifacts",
	}

	cmd.AddCommand(newCmdRender())

	return cmd
}
