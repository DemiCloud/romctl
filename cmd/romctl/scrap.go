package main

import (
	"github.com/spf13/cobra"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/commands"
)

func newScrapCmd() *cobra.Command {
	cmd := newMutatingCmd("scrap [SYSTEM...|all]", "retire confirmed-dead files; flag unmanaged formats for review")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.Scrap(appCtx.lib, runner, systems)
		})
	}
	return cmd
}
