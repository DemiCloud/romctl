package main

import (
	"github.com/spf13/cobra"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/commands"
)

// newMutatingCmd builds the shared shape every mutating command uses
// (-n/-v/--format are already persistent on root; this is just the
// systems-taking Args/ValidArgsFunction wiring most of them also share).
// fix/clean/scrape also use this but aren't hidden by newHiddenStepCmd
// below, which every individual fix step (chd, rename, ...) goes through
// instead.
func newMutatingCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:               use,
		Short:             short,
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeSystems,
	}
}

// newHiddenStepCmd is newMutatingCmd for the steps `fix` already chains
// (its own description names each one): kept as real, independently-
// runnable commands - for testing one step in isolation, or re-running
// just the one that failed - but left out of the default command listing
// to keep it to the handful of verbs most sessions actually reach for.
// `romctl help <name>` or `romctl <name> --help` still works for any of
// them directly.
func newHiddenStepCmd(use, short string) *cobra.Command {
	cmd := newMutatingCmd(use, short)
	cmd.Hidden = true
	return cmd
}

func newCHDCmd() *cobra.Command {
	cmd := newHiddenStepCmd("chd [SYSTEM...|all]", "convert disc images to CHD")
	cmd.Flags().String("scheme", "", "")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		scheme, err := resolveScheme(cmd)
		if err != nil {
			return err
		}
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.CHD(appCtx.lib, runner, systems, scheme)
		})
	}
	return cmd
}

func newRVZCmd() *cobra.Command {
	cmd := newHiddenStepCmd("rvz [SYSTEM...|all]", "convert GameCube/Wii images to RVZ")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.RVZ(appCtx.lib, runner, systems)
		})
	}
	return cmd
}

func newXisoCmd() *cobra.Command {
	cmd := newHiddenStepCmd("xiso [SYSTEM...|all]", "convert Xbox disc images to XISO")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.Xiso(appCtx.lib, runner, systems)
		})
	}
	return cmd
}

func newN64Cmd() *cobra.Command {
	cmd := newHiddenStepCmd("n64 [SYSTEM...|all]", "normalize N64 ROMs to big-endian .z64")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return true, commands.N64(appCtx.lib, runner, systems)
		})
	}
	return cmd
}

func newSNESCmd() *cobra.Command {
	cmd := newHiddenStepCmd("snes [SYSTEM...|all]", "strip copier headers; rename .smc to .sfc")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return true, commands.SNES(appCtx.lib, runner, systems)
		})
	}
	return cmd
}

func newUnpackCmd() *cobra.Command {
	cmd := newHiddenStepCmd("unpack [SYSTEM...|all]", "extract single-file archives in cartridge systems")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.Unpack(appCtx.lib, runner, systems)
		})
	}
	return cmd
}

func newM3UCmd() *cobra.Command {
	cmd := newHiddenStepCmd("m3u [SYSTEM...|all]", "group loose multi-disc CHDs into a playlist directory")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return true, commands.M3U(appCtx.lib, runner, systems)
		})
	}
	return cmd
}

func newPS3Cmd() *cobra.Command {
	cmd := newHiddenStepCmd("ps3 [SYSTEM...|all]", "unpack PS3 archives, then build ISOs from disc folders")
	cmd.Flags().String("scheme", "", "")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		scheme, err := resolveScheme(cmd)
		if err != nil {
			return err
		}
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.PS3(appCtx.lib, runner, systems, scheme)
		})
	}
	return cmd
}
