package main

import (
	"github.com/spf13/cobra"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/commands"
)

func newRenameCmd() *cobra.Command {
	cmd := newHiddenStepCmd("rename [SYSTEM...|all]", "apply the configured naming scheme to loose files")
	cmd.Flags().String("naming", "", "")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		scheme, err := resolveScheme(cmd)
		if err != nil {
			return err
		}
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return true, commands.Rename(appCtx.lib, runner, systems, scheme)
		})
	}
	return cmd
}

func newIdentifyCmd() *cobra.Command {
	cmd := newMutatingCmd("identify [SYSTEM...|all]", "rename files to their DAT-verified canonical name")
	cmd.Flags().String("naming", "", "")
	cmd.Flags().String("dat-dir", "", "")
	cmd.Flags().Bool("verify-chd", false, "")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		scheme, err := resolveScheme(cmd)
		if err != nil {
			return err
		}
		datDir, _ := cmd.Flags().GetString("dat-dir")
		verifyCHD, _ := cmd.Flags().GetBool("verify-chd")
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.Identify(appCtx.lib, runner, systems, datDir, verifyCHD, scheme)
		})
	}
	return cmd
}

func newRelocateCmd() *cobra.Command {
	cmd := newHiddenStepCmd("relocate [SYSTEM...|all]", "move a misfiled cartridge ROM into its correct system directory")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.Relocate(appCtx.lib, runner, systems)
		})
	}
	return cmd
}

func newSavesCmd() *cobra.Command {
	cmd := newHiddenStepCmd("saves [SYSTEM...|all]", "move stray save/state files out of roms/ into saves/, states/")
	cmd.Flags().String("dest", "", "")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		dest, _ := cmd.Flags().GetString("dest")
		return runMutating(cmd, args, func(systems []string, runner *actions.Runner) (bool, error) {
			return commands.Saves(appCtx.lib, runner, systems, dest)
		})
	}
	return cmd
}
