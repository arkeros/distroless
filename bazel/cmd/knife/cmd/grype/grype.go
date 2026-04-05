package grype

import (
	"github.com/spf13/cobra"
)

func NewCmdGrype() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grype",
		Short: "Manage the grype vulnerability database",
	}

	cmd.AddCommand(newCmdUpdate())

	return cmd
}
