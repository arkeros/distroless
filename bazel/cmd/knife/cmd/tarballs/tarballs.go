package tarballs

import (
	"github.com/spf13/cobra"
)

func NewCmdTarballs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tarballs",
		Short: "Manage upstream prebuilt runtime tarballs (nodejs.org, Temurin)",
	}

	cmd.AddCommand(newCmdUpdate())

	return cmd
}
