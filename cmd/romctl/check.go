package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/demicloud/romctl/internal/commands"
	"github.com/demicloud/romctl/internal/version"
)

func newCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "check [SYSTEM...|all]",
		Short:             "audit systems and BIOS files (read-only)",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeSystems,
		RunE: func(cmd *cobra.Command, args []string) error {
			systems, err := resolveSystems(args)
			if err != nil {
				return err
			}
			verbose, _ := cmd.Flags().GetBool("verbose")
			scheme, err := resolveScheme(cmd)
			if err != nil {
				return err
			}
			ok, err := commands.Check(appCtx.lib, systems, scheme, verbose, appCtx.format, appCtx.rootHint)
			if err != nil {
				return err
			}
			if !ok {
				return errSilentFailure
			}
			return nil
		},
	}
	cmd.Flags().String("naming", "", "")
	return cmd
}

func newDupesCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "dupes [SYSTEM...|all]",
		Short:             "scan for exact (byte-for-byte) duplicate files (read-only)",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeSystems,
		RunE: func(cmd *cobra.Command, args []string) error {
			systems, err := resolveSystems(args)
			if err != nil {
				return err
			}
			verbose, _ := cmd.Flags().GetBool("verbose")
			ok, err := commands.Dupes(appCtx.lib, systems, verbose, appCtx.format)
			if err != nil {
				return err
			}
			if !ok {
				return errSilentFailure
			}
			return nil
		},
	}
}

func newVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "verify [SYSTEM...|all]",
		Short:             "verify every CHD's data against its own checksums (read-only)",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeSystems,
		RunE: func(cmd *cobra.Command, args []string) error {
			systems, err := resolveSystems(args)
			if err != nil {
				return err
			}
			verbose, _ := cmd.Flags().GetBool("verbose")
			ok, err := commands.Verify(appCtx.lib, systems, verbose, appCtx.format)
			if err != nil {
				return err
			}
			if !ok {
				return errSilentFailure
			}
			return nil
		},
	}
}

func newBiosCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "bios [SYSTEM...|all]",
		Short:             "audit BIOS files only (-v for the full inventory)",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeSystems,
		RunE: func(cmd *cobra.Command, args []string) error {
			systems, err := resolveSystems(args)
			if err != nil {
				return err
			}
			verbose, _ := cmd.Flags().GetBool("verbose")
			ok, err := commands.Bios(appCtx.lib, systems, verbose, appCtx.format)
			if err != nil {
				return err
			}
			if !ok {
				return errSilentFailure
			}
			return nil
		},
	}
}

func newCollectionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "collection [SYSTEM...|all]",
		Short:             "compare games against a No-Intro/Redump DAT (read-only)",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeSystems,
		RunE: func(cmd *cobra.Command, args []string) error {
			systems, err := resolveSystems(args)
			if err != nil {
				return err
			}
			verbose, _ := cmd.Flags().GetBool("verbose")
			datDir, _ := cmd.Flags().GetString("dat-dir")
			ok, err := commands.Collection(appCtx.lib, systems, datDir, verbose, appCtx.format)
			if err != nil {
				return err
			}
			if !ok {
				return errSilentFailure
			}
			return nil
		},
	}
	cmd.Flags().String("dat-dir", "", "")
	return cmd
}

func newToolsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tools",
		Short: "report native tool availability",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.Tools(appCtx.format)
		},
	}
}

// newVersionCmd is a subcommand alias for the --version flag.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "version",
		Short:  "show version",
		Args:   cobra.NoArgs,
		Hidden: true,
		Run:    func(cmd *cobra.Command, args []string) { fmt.Println(version.Version) },
	}
}
