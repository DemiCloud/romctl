package main

import (
	"github.com/spf13/cobra"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/commands"
)

// addMutatingFlags registers the flags every job-based conversion/
// normalization command shares (-n/-v/--format are already persistent on
// root; this is just the systems-taking Args/ValidArgsFunction wiring
// every one of them also shares).
func newMutatingCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:               use,
		Short:             short,
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeSystems,
	}
}

func newCHDCmd() *cobra.Command {
	cmd := newMutatingCmd("chd [SYSTEM...|all]", "convert disc images to CHD")
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

func newDVDCmd() *cobra.Command {
	cmd := newMutatingCmd("dvd [SYSTEM...|all]", "re-encode DVD images stored as CD-type CHDs")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.DVD(appCtx.lib, runner, systems)
		})
	}
	return cmd
}

func newRVZCmd() *cobra.Command {
	cmd := newMutatingCmd("rvz [SYSTEM...|all]", "convert GameCube/Wii images to RVZ")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.RVZ(appCtx.lib, runner, systems)
		})
	}
	return cmd
}

func newXisoCmd() *cobra.Command {
	cmd := newMutatingCmd("xiso [SYSTEM...|all]", "rename Xbox XISO images from .iso to .xiso")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.Xiso(appCtx.lib, runner, systems)
		})
	}
	return cmd
}

func newN64Cmd() *cobra.Command {
	cmd := newMutatingCmd("n64 [SYSTEM...|all]", "normalize N64 ROMs to big-endian .z64")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return true, commands.N64(appCtx.lib, runner, systems)
		})
	}
	return cmd
}

func newSNESCmd() *cobra.Command {
	cmd := newMutatingCmd("snes [SYSTEM...|all]", "strip copier headers; rename .smc to .sfc")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return true, commands.SNES(appCtx.lib, runner, systems)
		})
	}
	return cmd
}

func newUnpackCmd() *cobra.Command {
	cmd := newMutatingCmd("unpack [SYSTEM...|all]", "extract single-file archives in cartridge systems")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.Unpack(appCtx.lib, runner, systems)
		})
	}
	return cmd
}

func newM3UCmd() *cobra.Command {
	cmd := newMutatingCmd("m3u [SYSTEM...|all]", "group loose multi-disc CHDs into a playlist directory")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return true, commands.M3U(appCtx.lib, runner, systems)
		})
	}
	return cmd
}

func newPS3Cmd() *cobra.Command {
	cmd := newMutatingCmd("ps3 [SYSTEM...|all]", "unpack PS3 archives, then build ISOs from disc folders")
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
