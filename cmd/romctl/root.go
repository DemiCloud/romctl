package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/config"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/retrodeck"
	"github.com/demicloud/romctl/internal/version"
)

// appCtx is built once, in rootCmd's PersistentPreRunE, from whatever
// combination of -r/--root, the config file and auto-discovery resolved
// to - every subcommand's RunE reads it rather than re-deriving it.
// Building it is side-effect-free even for a command (systems, help,
// completion) that never touches a library at all, so there's nothing to
// special-case: resolveRoot and config.Load never error on a
// nonexistent/irrelevant root, and library.Open just builds path strings.
type appContext struct {
	lib      *library.Library
	cfg      config.Config
	cfgPath  string
	format   cliutil.Format
	rootHint string
}

var appCtx appContext

// errSilentFailure signals "exit 1" without main printing an extra error
// line - for a command that already printed its own failure summary
// (e.g. "chd: 1 converted, 2 failed in 5s") and just needs the process to
// report it via exit code, not a second, redundant "romctl: ..." line.
var errSilentFailure = errors.New("romctl: silent failure")

func main() {
	root := newRootCmd()

	// Bare invocation (no command at all) is handled before Execute,
	// outside cobra's own command-matching: root deliberately has no
	// Run/RunE of its own, so that an actually-mistyped command name
	// (as opposed to no name at all) falls through to cobra's native
	// "unknown command" error and did-you-mean suggestions instead of
	// being silently accepted as a bare positional argument to root.
	if len(os.Args) <= 1 {
		root.Help() //nolint:errcheck
		os.Exit(1)
	}

	if err := root.Execute(); err != nil {
		if !errors.Is(err, errSilentFailure) {
			fmt.Fprintf(os.Stderr, "%s: %v\n", cliutil.ProgName(), err)
		}
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:                   cliutil.ProgName() + " COMMAND [SYSTEM...|all] [options]",
		Short:                 "audit and normalize a RetroDECK-style ROM library",
		Version:               version.Version,
		SilenceErrors:         true,
		SilenceUsage:          true,
		DisableFlagsInUseLine: true,
		PersistentPreRunE:     buildAppContext,
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.SetHelpFunc(printHelp)
	root.SetUsageFunc(func(cmd *cobra.Command) error { printHelp(cmd, nil); return nil })

	root.PersistentFlags().StringP("root", "r", ".", "")
	root.PersistentFlags().BoolP("dry-run", "n", false, "")
	root.PersistentFlags().BoolP("verbose", "v", false, "")
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().String("format", "", "")

	root.AddCommand(newCheckCmd(), newBiosCmd(), newDupesCmd(), newVerifyCmd(), newCollectionCmd(), newIdentifyCmd(), newSystemCmd(), newToolsCmd(), newVersionCmd())
	root.AddCommand(newFixCmd())
	root.AddCommand(newRenameCmd(), newUnpackCmd(), newCHDCmd(), newM3UCmd(),
		newN64Cmd(), newSNESCmd(), newXisoCmd(), newRVZCmd(), newPS3Cmd(), newSavesCmd(), newRelocateCmd())
	root.AddCommand(newScrapeCmd(), newCleanCmd(), newSyncCmd())
	return root
}

// buildAppContext is rootCmd's PersistentPreRunE: it runs before every
// command's own RunE, cobra having already parsed every flag (including
// a leaf command's own local ones) by this point.
func buildAppContext(cmd *cobra.Command, args []string) error {
	cfgPath, err := config.DefaultPath()
	if err != nil {
		return err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	rootFlag, _ := cmd.Flags().GetString("root")
	rootChanged := cmd.Flags().Changed("root")
	lib := library.Open(resolveRoot(rootFlag, rootChanged, cfg.Library.Root))
	rootHint := ""
	if rootChanged {
		rootHint = "-r " + rootFlag
	}

	formatFlag, _ := cmd.Flags().GetString("format")
	format, err := cliutil.ParseFormat(formatFlag)
	if err != nil {
		return err
	}
	format = format.Resolve(os.Stdout)

	appCtx = appContext{lib: lib, cfg: cfg, cfgPath: cfgPath, format: format, rootHint: rootHint}
	return nil
}

// resolveRoot picks the library root: an explicit `-r`/`--root` flag wins
// outright; otherwise `[library] root` in the config file; otherwise a
// roms/ directory already sitting right where romctl was run from (e.g.
// dropped straight into an existing library) wins before anything else is
// even consulted; otherwise a local RetroDECK install, auto-discovered
// the same way `sync` finds one remotely (see internal/retrodeck) - this
// is what lets romctl run directly on a RetroDECK device with no `-r` or
// config needed at all, but it must never override an obvious local
// roms/ that just happens to live somewhere other than the RetroDECK
// install's own resolved path. Falls back to flagRoot (its "." default)
// if none of those apply.
func resolveRoot(flagRoot string, flagChanged bool, cfgRoot string) string {
	if flagChanged {
		return flagRoot
	}
	if cfgRoot != "" {
		return cfgRoot
	}
	if library.HasRoms(flagRoot) {
		return flagRoot
	}
	if resolved, err := retrodeck.ResolveLocal(); err == nil {
		return resolved
	}
	return flagRoot
}

// confirmFirstRun asks a one-time "this will modify files" notice the
// first time any mutating command runs against root, so someone who
// skipped the README still gets a chance to back out before romctl
// touches a library it's never seen before - clean's own narrower,
// every-time confirmation (it's the one step that's actually
// irreversible) already covers that command, so this only gates every
// other mutating command. Roots that have confirmed once are remembered
// in a marker file next to the config file, keyed by absolute path, so
// the notice never repeats for the same root. Skipped entirely (treated
// as already confirmed) when stdin isn't a terminal, so a scripted/CI/cron
// run is never the first thing to hit a prompt it can't answer.
func confirmFirstRun(cfgPath, root string) bool {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	markerPath := filepath.Join(filepath.Dir(cfgPath), "trusted_roots")
	if data, err := os.ReadFile(markerPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if line == abs {
				return true
			}
		}
	}
	if !cliutil.Interactive() {
		return true
	}

	fmt.Printf("romctl hasn't touched %s before.\n", abs)
	fmt.Println("This command will modify files under it. Replaced originals are moved to _to_delete/, not deleted, until `clean` removes them. Pass --yes to skip this and every other confirmation.")
	if !cliutil.Confirm("Continue?") {
		return false
	}

	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err == nil {
		if f, err := os.OpenFile(markerPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintln(f, abs)
			f.Close()
		}
	}
	return true
}

// resolveSystems validates lib.RequireRoms and expands args via
// lib.ResolveSystems - the read-only half of preMutate, also used by
// clean (which has its own, different confirmation) and the read-only
// commands.
func resolveSystems(args []string) ([]string, error) {
	if err := appCtx.lib.RequireRoms(); err != nil {
		return nil, err
	}
	return appCtx.lib.ResolveSystems(args)
}

// preMutate resolves systems and, unless this is a dry run or --yes was
// passed, runs the first-run confirmation gate - the shared prelude every
// mutating command except clean needs.
func preMutate(cmd *cobra.Command, args []string) ([]string, error) {
	systems, err := resolveSystems(args)
	if err != nil {
		return nil, err
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	assumeYes, _ := cmd.Flags().GetBool("yes")
	if !dryRun && !assumeYes {
		if !confirmFirstRun(appCtx.cfgPath, appCtx.lib.Root) {
			return nil, errors.New("aborted")
		}
	}
	return systems, nil
}

// postMutate prints the pending-cleanup/sync-reminder hints after a
// mutating command finishes - skipped on a dry run (nothing was actually
// retired) or under a structured format (its one encoded Result is the
// only thing that belongs on stdout).
func postMutate(cmd *cobra.Command) {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if dryRun || appCtx.format.Structured() {
		return
	}
	if n := actions.RetiredCount(); n > 0 {
		fmt.Printf("%d item(s) pending in _to_delete/; run `%s clean` to remove\n", n, cliutil.ProgName())
	}
	if appCtx.cfg.Sync.Host != "" && actions.ActionsPerformed() > 0 {
		fmt.Printf("don't forget `%s sync` to push these changes to the device\n", cliutil.ProgName())
	}
}

// runMutating is the shape every mutating command's RunE follows:
// resolve systems and run the first-run gate, build a Runner, call fn
// (which does the real work via internal/commands and reports
// counts/actions to the runner), encode a structured Result if --format
// calls for one, and otherwise print the pending-cleanup hints. fn's own
// bool return (ok) becomes a silent exit 1 on false - the command itself
// already printed why (e.g. "3 failed"), so no second error line.
func runMutating(cmd *cobra.Command, args []string, fn func(systems []string, runner *actions.Runner) (bool, error)) error {
	systems, err := preMutate(cmd, args)
	if err != nil {
		return err
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	verbose, _ := cmd.Flags().GetBool("verbose")
	runner := &actions.Runner{DryRun: dryRun, Verbose: verbose, Format: appCtx.format}

	start := time.Now()
	ok, err := fn(systems, runner)
	if err != nil {
		return fmt.Errorf("%s: %w", cmd.Name(), err)
	}

	if appCtx.format.Structured() {
		result := actions.Result{
			Command: cmd.Name(), Systems: systems, Counts: runner.Snapshot(),
			Actions: runner.Log(), Failed: !ok, Seconds: time.Since(start).Seconds(),
		}
		if err := encodeResult(os.Stdout, appCtx.format, result); err != nil {
			return fmt.Errorf("%s: %w", cmd.Name(), err)
		}
	}
	if !ok {
		return errSilentFailure
	}
	postMutate(cmd)
	return nil
}

// resolveScheme applies `--naming` over `[naming] scheme` - only called
// by the commands that take a naming scheme (check, fix, rename, chd, ps3).
func resolveScheme(cmd *cobra.Command) (actions.Scheme, error) {
	schemeStr := appCtx.cfg.Naming.Scheme
	if cmd.Flags().Changed("naming") {
		schemeStr, _ = cmd.Flags().GetString("naming")
	}
	return actions.ParseScheme(schemeStr)
}

// encodeResult writes result as json/yaml/toml via cliutil.Encode, or as
// a single-row CSV (counts sorted by key, for a stable column order)
// since a flat key/value map has no natural row order of its own -
// Actions is left out of the CSV row: free-text descriptions don't fit a
// tabular column, so a CSV consumer gets the same counts/duration/failed
// summary a json/yaml/toml consumer gets, minus the action log. The
// overall pass/fail column is named "overall_failed", not "failed": most
// commands already report their own "failed" count among Counts, and a
// header can't repeat a column name.
func encodeResult(w *os.File, format cliutil.Format, result actions.Result) error {
	if format != cliutil.FormatCSV {
		return cliutil.Encode(w, format, result)
	}
	keys := make([]string, 0, len(result.Counts))
	for k := range result.Counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	header := append([]string{"command", "systems"}, keys...)
	header = append(header, "overall_failed", "seconds")
	row := []string{result.Command, strings.Join(result.Systems, ";")}
	for _, k := range keys {
		row = append(row, strconv.FormatInt(result.Counts[k], 10))
	}
	row = append(row, strconv.FormatBool(result.Failed), strconv.FormatFloat(result.Seconds, 'f', -1, 64))
	return cliutil.EncodeCSV(w, header, [][]string{row})
}

// completeSystems is the shared ValidArgsFunction for every command whose
// positional arguments are system names: it re-resolves the library root
// the same way buildAppContext does (completion runs as its own
// invocation, before any PersistentPreRunE), reads whatever's actually
// under roms/, and offers "all" alongside the real directory names.
func completeSystems(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	rootFlag, _ := cmd.Flags().GetString("root")
	cfgPath, err := config.DefaultPath()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, _ := config.Load(cfgPath)
	lib := library.Open(resolveRoot(rootFlag, cmd.Flags().Changed("root"), cfg.Library.Root))

	systems, err := lib.ResolveSystems(nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return append(systems, "all"), cobra.ShellCompDirectiveNoFileComp
}
