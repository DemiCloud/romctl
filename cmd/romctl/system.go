package main

import (
	"github.com/spf13/cobra"

	"github.com/demicloud/romctl/internal/commands"
)

// newSystemCmd groups every system-information command: `system list`
// (every known system, its roms/ directory, and its class - works with no
// library at all) and `system games` (the systems actually populated in
// this library, with game counts - -g for full titles). Parented under one
// command instead of two unrelated top-level ones, since both answer "what
// systems", just at different scopes.
func newSystemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "system information (see 'system list', 'system games')",
	}
	cmd.AddCommand(newSystemListCmd(), newSystemGamesCmd())
	return cmd
}

func newSystemListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list every known system, its directory, and its class",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.Systems(appCtx.format)
		},
	}
}

func newSystemGamesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "games [SYSTEM...|all]",
		Short:             "list populated systems and game counts (-g for titles)",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeSystems,
		RunE: func(cmd *cobra.Command, args []string) error {
			systems, err := resolveSystems(args)
			if err != nil {
				return err
			}
			showGames, _ := cmd.Flags().GetBool("games")
			return commands.List(appCtx.lib, systems, showGames, appCtx.format)
		},
	}
	cmd.Flags().BoolP("games", "g", false, "")
	return cmd
}
