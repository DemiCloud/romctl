package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/commands"
)

// newCleanCmd is the one mutating command that doesn't go through
// runMutating: it skips the generic first-run gate (it has its own,
// narrower, every-time confirmation instead - see commands.Clean) and
// never prints the pending-cleanup/sync-reminder hints (deleting
// _to_delete/ doesn't create new ones).
func newCleanCmd() *cobra.Command {
	cmd := newMutatingCmd("clean [SYSTEM...|all]", "permanently delete _to_delete/ directories")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		systems, err := resolveSystems(args)
		if err != nil {
			return err
		}
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		verbose, _ := cmd.Flags().GetBool("verbose")
		assumeYes, _ := cmd.Flags().GetBool("yes")
		runner := &actions.Runner{DryRun: dryRun, Verbose: verbose, Format: appCtx.format}

		start := time.Now()
		if err := commands.Clean(appCtx.lib, runner, systems, assumeYes); err != nil {
			return fmt.Errorf("clean: %w", err)
		}
		if appCtx.format.Structured() {
			result := actions.Result{
				Command: "clean", Systems: systems, Counts: runner.Snapshot(),
				Actions: runner.Log(), Seconds: time.Since(start).Seconds(),
			}
			if err := encodeResult(os.Stdout, appCtx.format, result); err != nil {
				return fmt.Errorf("clean: %w", err)
			}
		}
		return nil
	}
	return cmd
}
