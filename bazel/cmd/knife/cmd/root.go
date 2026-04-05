package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/arkeros/senku/bazel/cmd/knife/cmd/apt"
	"github.com/arkeros/senku/bazel/cmd/knife/cmd/grype"
)

var rootCmd = &cobra.Command{
	Use:   "knife",
	Short: "Knife - a Swiss-army knife for Bazel build management",
	Long:  `knife is a command-line tool for managing Bazel build infrastructure tasks.`,
}

func init() {
	rootCmd.AddCommand(apt.NewCmdApt())
	rootCmd.AddCommand(grype.NewCmdGrype())
}

func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}
