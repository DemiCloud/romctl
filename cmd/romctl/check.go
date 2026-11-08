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
			dupes, _ := cmd.Flags().GetBool("dupes")
			verbose, _ := cmd.Flags().GetBool("verbose")
			scheme, err := resolveScheme(cmd)
			if err != nil {
				return err
			}
			ok, err := commands.Check(appCtx.lib, systems, dupes, scheme, verbose, appCtx.format, appCtx.rootHint)
			if err != nil {
				return err
			}
			if !ok {
				return errSilentFailure
			}
			return nil
		},
	}
	cmd.Flags().Bool("dupes", false, "")
	cmd.Flags().String("scheme", "", "")
	return cmd
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

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list [SYSTEM...|all]",
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

func newSystemsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "systems",
		Short: "list every known system and its class (not just populated ones)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.Systems(appCtx.format)
		},
	}
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
