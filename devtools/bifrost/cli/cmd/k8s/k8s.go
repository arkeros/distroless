package k8s

import "github.com/spf13/cobra"

func NewCmdK8s() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "k8s",
		Short: "Manage Kubernetes artifacts",
	}

	cmd.AddCommand(newCmdRender())

	return cmd
}
