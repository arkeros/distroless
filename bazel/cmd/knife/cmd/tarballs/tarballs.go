package tarballs

import (
	"github.com/spf13/cobra"
)

func NewCmdTarballs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tarballs",
		Short: "Manage upstream prebuilt runtimes (nodejs.org, Temurin, envoy)",
	}

	cmd.AddCommand(newCmdUpdate())

	return cmd
}
