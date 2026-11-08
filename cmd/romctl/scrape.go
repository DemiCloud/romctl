package main

import (
	"github.com/spf13/cobra"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/commands"
)

func newScrapeCmd() *cobra.Command {
	cmd := newMutatingCmd("scrape [SYSTEM...|all]", "fetch game metadata/media via Skyscraper (needs [scrape] user)")
	cmd.Flags().String("scrape-user", "", "")
	cmd.Flags().Int("scrape-threads", 0, "")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		cfg := appCtx.cfg
		if cmd.Flags().Changed("scrape-user") {
			cfg.Scrape.User, _ = cmd.Flags().GetString("scrape-user")
		}
		if cmd.Flags().Changed("scrape-threads") {
			cfg.Scrape.Threads, _ = cmd.Flags().GetInt("scrape-threads")
		}
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.Scrape(appCtx.lib, runner, systems, cfg)
		})
	}
	return cmd
}
