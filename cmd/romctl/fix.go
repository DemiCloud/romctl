package main

import (
	"github.com/spf13/cobra"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/commands"
)

func newFixCmd() *cobra.Command {
	cmd := newMutatingCmd("fix [SYSTEM...|all]",
		"run every local fix step: relocate, unpack, chd, dvd, rvz, xiso, n64, snes, ps3, m3u, rename, saves")
	cmd.Flags().String("naming", "", "")
	cmd.Flags().String("dest", "", "")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		scheme, err := resolveScheme(cmd)
		if err != nil {
			return err
		}
		dest, _ := cmd.Flags().GetString("dest")
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.Fix(appCtx.lib, runner, systems, scheme, dest)
		})
	}
	return cmd
}
