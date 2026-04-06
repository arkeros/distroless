package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/arkeros/senku/devtools/bifrost/cli/cmd/cloudrun"
	"github.com/arkeros/senku/devtools/bifrost/cli/cmd/k8s"
	"github.com/arkeros/senku/devtools/bifrost/cli/cmd/terraform"
)

var rootCmd = &cobra.Command{
	Use:   "bifrost",
	Short: "Bifrost - bridge from workload specs to platform-native artifacts",
	Long:  `bifrost translates a unified workload specification into platform-native infrastructure artifacts.`,
}

func init() {
	rootCmd.AddCommand(cloudrun.NewCmdCloudRun())
	rootCmd.AddCommand(k8s.NewCmdK8s())
	rootCmd.AddCommand(terraform.NewCmdTerraform())
}

func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}
