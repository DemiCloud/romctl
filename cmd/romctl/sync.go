package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/demicloud/romctl/internal/sync"
)

// newSyncCmd is excluded from --format (it streams rsync's own live
// output, same reasoning as scrape) and from the first-run/pending-
// cleanup machinery every other mutating command goes through - it
// mirrors against a remote device, not the local library, so none of
// that applies.
func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync [roms|bios|media...]",
		Short: "mirror roms/bios/media to a device (needs [sync] host)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := appCtx.cfg
			if cmd.Flags().Changed("sync-host") {
				cfg.Sync.Host, _ = cmd.Flags().GetString("sync-host")
			}
			if cmd.Flags().Changed("sync-user") {
				cfg.Sync.User, _ = cmd.Flags().GetString("sync-user")
			}
			if cmd.Flags().Changed("sync-path") {
				cfg.Sync.Path, _ = cmd.Flags().GetString("sync-path")
			}
			if cmd.Flags().Changed("sync-exclude") {
				cfg.Sync.Exclude, _ = cmd.Flags().GetStringArray("sync-exclude")
			}
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			pull, _ := cmd.Flags().GetBool("pull")
			if err := sync.Run(sync.Options{Root: appCtx.lib.Root, DryRun: dryRun, Pull: pull, Config: cfg, Targets: args}); err != nil {
				return fmt.Errorf("sync: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().Bool("pull", false, "")
	cmd.Flags().String("sync-host", "", "")
	cmd.Flags().String("sync-user", "", "")
	cmd.Flags().String("sync-path", "", "")
	cmd.Flags().StringArray("sync-exclude", nil, "")
	return cmd
}
