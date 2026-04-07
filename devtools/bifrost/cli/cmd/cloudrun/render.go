package cloudrun

import (
	"os"

	"github.com/spf13/cobra"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
	"github.com/arkeros/senku/devtools/bifrost/cloudrun"
	"github.com/arkeros/senku/devtools/bifrost/internal"
)

type renderOptions struct {
	InputPath       string
	EnvironmentPath string
}

func newCmdRender() *cobra.Command {
	o := &renderOptions{}

	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render Cloud Run manifests from a workload spec",
		Example: `  bifrost cloudrun render -e environment.yaml -f service.yaml
  bifrost cloudrun render -e environment.yaml -f cronjob.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.Run()
		},
	}

	cmd.Flags().StringVarP(&o.InputPath, "file", "f", "", "path to the workload spec YAML or JSON")
	cmd.MarkFlagRequired("file")
	cmd.Flags().StringVarP(&o.EnvironmentPath, "environment", "e", "", "path to an Environment YAML or JSON file")
	cmd.MarkFlagRequired("environment")

	return cmd
}

func (o *renderOptions) Run() error {
	env, err := internal.LoadEnvironment(o.EnvironmentPath)
	if err != nil {
		return err
	}

	f, err := os.Open(o.InputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	spec, err := bifrost.Parse(f, env)
	if err != nil {
		return err
	}

	out, err := cloudrun.Render(spec, env)
	if err != nil {
		return err
	}

	_, err = os.Stdout.Write(out)
	return err
}
