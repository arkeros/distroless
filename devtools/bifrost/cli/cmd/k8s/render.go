package k8s

import (
	"os"

	"github.com/spf13/cobra"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
	"github.com/arkeros/senku/devtools/bifrost/k8s"
)

type renderOptions struct {
	InputPath string
}

func newCmdRender() *cobra.Command {
	o := &renderOptions{}

	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render Kubernetes manifests from a workload spec",
		Example: `  bifrost k8s render -f service.yaml
  bifrost k8s render -f cronjob.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.Run()
		},
	}

	cmd.Flags().StringVarP(&o.InputPath, "file", "f", "", "path to the workload spec YAML or JSON")
	cmd.MarkFlagRequired("file")

	return cmd
}

func (o *renderOptions) Run() error {
	f, err := os.Open(o.InputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	spec, err := bifrost.Parse(f)
	if err != nil {
		return err
	}

	out, err := k8s.Render(spec)
	if err != nil {
		return err
	}

	_, err = os.Stdout.Write(out)
	return err
}
