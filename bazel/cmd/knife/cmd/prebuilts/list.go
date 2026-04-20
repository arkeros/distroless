package prebuilts

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

func newCmdList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List tools with a configured versions.bzl pipeline",
		Long: `Prints one tool name per line. Useful for automation that needs to
decide whether a given tool participates in the prebuilts versioning
pipeline (e.g. CI steps that regenerate versions.bzl after a release).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			names := knownTools()
			sort.Strings(names)
			for _, n := range names {
				fmt.Println(n)
			}
			return nil
		},
	}
}
