// Command romctl audits and normalizes a RetroDECK/ES-DE ROM library and
// syncs it to a device over rsync/SSH. See usage() (romctl --help) or
// README.md for details.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/commands"
	"github.com/demicloud/romctl/internal/config"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/retrodeck"
	"github.com/demicloud/romctl/internal/sync"
	"github.com/demicloud/romctl/internal/version"
)

var mutatingCommands = map[string]bool{
	"rename": true, "unpack": true, "chd": true, "m3u": true, "dvd": true,
	"n64": true, "snes": true, "xiso": true, "rvz": true, "ps3": true, "clean": true,
	"scrape": true, "saves": true, "relocate": true, "fix": true,
}

var readOnlyCommands = map[string]bool{
	"check": true, "bios": true, "list": true,
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet(cliutil.ProgName(), flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	root := fs.StringP("root", "r", ".", "")
	dryRun := fs.BoolP("dry-run", "n", false, "")
	assumeYes := fs.Bool("yes", false, "")
	showGames := fs.BoolP("games", "g", false, "")
	dupes := fs.Bool("dupes", false, "")
	verbose := fs.BoolP("verbose", "v", false, "")
	pull := fs.Bool("pull", false, "")
	schemeFlag := fs.String("scheme", "", "")
	syncHost := fs.String("sync-host", "", "")
	syncUser := fs.String("sync-user", "", "")
	syncPath := fs.String("sync-path", "", "")
	syncExclude := fs.StringArray("sync-exclude", nil, "")
	scrapeUser := fs.String("scrape-user", "", "")
	scrapeThreads := fs.Int("scrape-threads", 0, "")
	savesDest := fs.String("dest", "", "")
	formatFlag := fs.String("format", "", "")
	help := fs.BoolP("help", "h", false, "")
	showVersion := fs.Bool("version", false, "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cliutil.ProgName(), err)
		return 1
	}

	format, err := cliutil.ParseFormat(*formatFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cliutil.ProgName(), err)
		return 1
	}
	format = format.Resolve(os.Stdout)

	rest := fs.Args()

	if *help {
		if len(rest) > 0 {
			return commandHelp(rest[0])
		}
		usage()
		return 0
	}
	if *showVersion {
		fmt.Println(version.Version)
		return 0
	}

	if len(rest) == 0 {
		usage()
		return 1
	}
	cmd, sysArgs := rest[0], rest[1:]

	switch cmd {
	case "help":
		if len(sysArgs) > 0 {
			return commandHelp(sysArgs[0])
		}
		usage()
		return 0
	case "systems":
		if err := commands.Systems(format); err != nil {
			fmt.Fprintf(os.Stderr, "%s: systems: %v\n", cliutil.ProgName(), err)
			return 1
		}
		return 0
	}

	cfgPath, err := config.DefaultPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cliutil.ProgName(), err)
		return 1
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cliutil.ProgName(), err)
		return 1
	}
	rootChanged := fs.Changed("root")
	lib := library.Open(resolveRoot(*root, rootChanged, cfg.Library.Root))
	rootHint := ""
	if rootChanged {
		rootHint = "-r " + *root
	}

	// Every [sync]/[scrape]/[naming] config value also has a flag, so the
	// config file is never required - an explicit flag always wins.
	if fs.Changed("sync-host") {
		cfg.Sync.Host = *syncHost
	}
	if fs.Changed("sync-user") {
		cfg.Sync.User = *syncUser
	}
	if fs.Changed("sync-path") {
		cfg.Sync.Path = *syncPath
	}
	if fs.Changed("sync-exclude") {
		cfg.Sync.Exclude = *syncExclude
	}
	if fs.Changed("scrape-user") {
		cfg.Scrape.User = *scrapeUser
	}
	if fs.Changed("scrape-threads") {
		cfg.Scrape.Threads = *scrapeThreads
	}
	schemeStr := cfg.Naming.Scheme
	if fs.Changed("scheme") {
		schemeStr = *schemeFlag
	}
	scheme, err := actions.ParseScheme(schemeStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cliutil.ProgName(), err)
		return 1
	}

	if cmd == "sync" {
		return runSync(lib, *dryRun, *pull, cfg, sysArgs)
	}

	if !mutatingCommands[cmd] && !readOnlyCommands[cmd] {
		msg := fmt.Sprintf("unknown command: %s", cmd)
		if suggestion := cliutil.Suggest(cmd, commandOrder); suggestion != "" {
			msg += fmt.Sprintf(" (did you mean %q?)", suggestion)
		} else {
			msg += fmt.Sprintf(" (see %s --help)", cliutil.ProgName())
		}
		fmt.Fprintf(os.Stderr, "%s: %s\n", cliutil.ProgName(), msg)
		return 1
	}

	if err := lib.RequireRoms(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cliutil.ProgName(), err)
		return 1
	}
	systems, err := lib.ResolveSystems(sysArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cliutil.ProgName(), err)
		return 1
	}

	if mutatingCommands[cmd] && cmd != "clean" && !*dryRun && !*assumeYes {
		if !confirmFirstRun(cfgPath, lib.Root) {
			fmt.Fprintf(os.Stderr, "%s: aborted\n", cliutil.ProgName())
			return 1
		}
	}

	runner := &actions.Runner{DryRun: *dryRun, Verbose: *verbose, Format: format}
	runStart := time.Now()
	ok, err := dispatch(cmd, lib, runner, systems, *assumeYes, *showGames, *dupes, *verbose, format, rootHint, scheme, cfg, *savesDest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s: %v\n", cliutil.ProgName(), cmd, err)
		return 1
	}

	// check/bios already encoded their own (richer, per-finding)
	// structured payload inside Check/Bios; every other mutating command
	// just reports counts to the shared runner (see actions.Runner.Count/
	// SetCount), so the one encoded Result for those is built centrally,
	// here, instead of by each command.
	if format.Structured() && mutatingCommands[cmd] {
		result := actions.Result{
			Command: cmd,
			Systems: systems,
			Counts:  runner.Snapshot(),
			Actions: runner.Log(),
			Failed:  !ok,
			Seconds: time.Since(runStart).Seconds(),
		}
		if err := encodeResult(os.Stdout, format, result); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s: %v\n", cliutil.ProgName(), cmd, err)
			return 1
		}
	}

	if !ok {
		return 1
	}
	if mutatingCommands[cmd] && cmd != "clean" && !*dryRun && !format.Structured() {
		if n := actions.RetiredCount(); n > 0 {
			fmt.Printf("%d item(s) pending in _to_delete/; run `%s clean` to remove\n", n, cliutil.ProgName())
		}
		if cfg.Sync.Host != "" {
			fmt.Printf("don't forget `%s sync` to push these changes to the device\n", cliutil.ProgName())
		}
	}
	return 0
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
func encodeResult(w io.Writer, format cliutil.Format, result actions.Result) error {
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

func dispatch(cmd string, lib *library.Library, runner *actions.Runner, systems []string, assumeYes, showGames, dupes, verbose bool, format cliutil.Format, rootHint string, scheme actions.Scheme, cfg config.Config, savesDest string) (bool, error) {
	switch cmd {
	case "check":
		return commands.Check(lib, systems, dupes, scheme, verbose, format, rootHint)
	case "bios":
		return commands.Bios(lib, systems, verbose, format)
	case "list":
		return true, commands.List(lib, systems, showGames, format)
	case "fix":
		return commands.Fix(lib, runner, systems, scheme, savesDest)
	case "rename":
		return true, commands.Rename(lib, runner, systems, scheme)
	case "unpack":
		return commands.Unpack(lib, runner, systems)
	case "chd":
		return commands.CHD(lib, runner, systems, scheme)
	case "m3u":
		return true, commands.M3U(lib, runner, systems)
	case "dvd":
		return commands.DVD(lib, runner, systems)
	case "n64":
		return true, commands.N64(lib, runner, systems)
	case "snes":
		return true, commands.SNES(lib, runner, systems)
	case "xiso":
		return commands.Xiso(lib, runner, systems)
	case "rvz":
		return commands.RVZ(lib, runner, systems)
	case "ps3":
		return commands.PS3(lib, runner, systems, scheme)
	case "saves":
		return commands.Saves(lib, runner, systems, savesDest)
	case "relocate":
		return commands.Relocate(lib, runner, systems)
	case "clean":
		return true, commands.Clean(lib, runner, systems, assumeYes)
	case "scrape":
		return commands.Scrape(lib, runner, systems, cfg)
	}
	return false, fmt.Errorf("unreachable: unknown command %s", cmd)
}

func runSync(lib *library.Library, dryRun, pull bool, cfg config.Config, targets []string) int {
	if err := sync.Run(sync.Options{Root: lib.Root, DryRun: dryRun, Pull: pull, Config: cfg, Targets: targets}); err != nil {
		fmt.Fprintf(os.Stderr, "%s: sync: %v\n", cliutil.ProgName(), err)
		return 1
	}
	return 0
}

// Flag lines shared between the global usage and per-command help, kept
// in one place so they can't drift apart.
const (
	flagRoot          = "  -r, --root DIR          library root containing roms/ and bios/ (default: local RetroDECK install, else .)"
	flagDryRun        = "  -n, --dry-run           preview changes instead of making them (default: apply)"
	flagGames         = "  -g, --games             also print each game's title"
	flagDupes         = "      --dupes             also scan for exact duplicate files"
	flagFormat        = "      --format FMT        fancy, plain, json, yaml, toml, csv (default: fancy on a terminal, else plain)"
	flagVerbose       = "  -v, --verbose           print every finding/action instead of a one-line summary"
	flagYes           = "      --yes               skip confirmation prompts (clean; first-run library notice)"
	flagPull          = "      --pull              pull from the device instead of pushing"
	flagScheme        = "      --scheme SCHEME     naming scheme: notag, keep, no-intro, redump (default: [naming] scheme, else keep)"
	flagSyncHost      = "      --sync-host HOST    device hostname/IP (default: [sync] host)"
	flagSyncUser      = "      --sync-user NAME    device SSH username (default: [sync] user, else deck)"
	flagSyncPath      = "      --sync-path PATH    device library root (default: [sync] path, else auto-discovered)"
	flagSyncExclude   = "      --sync-exclude PAT  extra rsync exclude pattern (repeatable; default: [sync] exclude)"
	flagScrapeUser    = "      --scrape-user ID:PW ScreenScraper account (default: [scrape] user)"
	flagScrapeThreads = "      --scrape-threads N  scrape thread count (default: [scrape] threads, else auto)"
	flagDest          = "      --dest DIR          saves: move into DIR/saves, DIR/states instead of the library root"
	flagHelp          = "  -h, --help              show this help"
	flagVersion       = "      --version           show version"
)

// cmdInfo is one command's entry in the shared table usage() and
// commandHelp() both read from.
type cmdInfo struct {
	desc  string   // one-line description
	usage string   // arguments after "COMMAND", e.g. "[SYSTEM...|all]"
	flags []string // flagRoot/flagDryRun/etc., in display order
}

var commandOrder = []string{
	"check", "bios", "list", "fix", "rename", "unpack", "chd", "m3u", "dvd", "n64",
	"snes", "xiso", "rvz", "ps3", "saves", "relocate", "scrape", "clean", "sync", "systems",
}

// commandGroups orders commandOrder's same 20 names into sections for
// usage()'s display: the 3 commands most sessions actually use, the 12
// individual steps fix chains (kept as real top-level commands - for
// testing one step in isolation, or re-running just the one that failed -
// but visually set apart so the top-level help reads as a handful of
// verbs instead of twenty), then everything else.
var commandGroups = []struct {
	label string
	names []string
}{
	{"", []string{"check", "fix", "sync"}},
	{"Individual fix steps (what `fix` runs, in order):",
		[]string{"relocate", "unpack", "chd", "dvd", "rvz", "xiso", "n64", "snes", "ps3", "m3u", "rename", "saves"}},
	{"Other:", []string{"bios", "list", "scrape", "clean", "systems"}},
}

var commandTable = map[string]cmdInfo{
	"check":    {"audit systems and BIOS files (read-only)", "[SYSTEM...|all]", []string{flagRoot, flagDupes, flagVerbose, flagScheme, flagFormat}},
	"bios":     {"audit BIOS files only (-v for the full inventory)", "[SYSTEM...|all]", []string{flagRoot, flagVerbose, flagFormat}},
	"list":     {"list populated systems and game counts (-g for titles)", "[SYSTEM...|all]", []string{flagRoot, flagGames, flagFormat}},
	"fix":      {"run every local fix step: relocate, unpack, chd, dvd, rvz, xiso, n64, snes, ps3, m3u, rename, saves", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScheme, flagDest, flagFormat}},
	"rename":   {"apply the configured naming scheme to loose files (see [naming])", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScheme, flagFormat}},
	"unpack":   {"extract single-file archives in cartridge systems", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"chd":      {"convert disc images to CHD", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScheme, flagFormat}},
	"m3u":      {"group loose multi-disc CHDs into a playlist directory", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"dvd":      {"re-encode DVD images stored as CD-type CHDs", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"n64":      {"normalize N64 ROMs to big-endian .z64", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"snes":     {"strip copier headers; rename .smc to .sfc", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"xiso":     {"rename Xbox XISO images from .iso to .xiso", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"rvz":      {"convert GameCube/Wii images to RVZ", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"ps3":      {"unpack PS3 archives, then build ISOs from disc folders", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScheme, flagFormat}},
	"saves":    {"move stray save/state files out of roms/ into saves/, states/", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagDest, flagFormat}},
	"relocate": {"move a misfiled cartridge ROM into its correct system directory", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"scrape":   {"fetch game metadata/media via Skyscraper (needs [scrape] user)", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScrapeUser, flagScrapeThreads, flagFormat}},
	"clean":    {"permanently delete _to_delete/ directories", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagYes, flagFormat}},
	"sync":     {"mirror roms/bios/media to a device (needs [sync] host)", "[roms|bios|media...]", []string{flagRoot, flagDryRun, flagPull, flagSyncHost, flagSyncUser, flagSyncPath, flagSyncExclude}},
	"systems":  {"list every known system and its class (not just populated ones)", "", []string{flagFormat}},
}

func usage() {
	prog := cliutil.ProgName()
	fmt.Printf(`romctl - audit and normalize a RetroDECK-style ROM library

Usage: %s COMMAND [SYSTEM...|all] [options]
       %s help COMMAND

Global options (apply to most commands; see '%s help COMMAND' for a
command's own options, e.g. --scheme, --sync-host, --scrape-user):
%s
%s
%s
%s
%s
%s
%s

Commands:
`, prog, prog, prog, flagRoot, flagDryRun, flagVerbose, flagYes, flagFormat, flagHelp, flagVersion)
	for _, g := range commandGroups {
		if g.label != "" {
			fmt.Printf("\n  %s\n", g.label)
		}
		for _, name := range g.names {
			fmt.Printf("  %-8s %s\n", name, commandTable[name].desc)
		}
	}
}

// commandHelp prints man-page-style help for one command (romctl <cmd>
// --help or romctl help <cmd>), returning the process exit status.
func commandHelp(cmd string) int {
	if cmd == "help" {
		usage()
		return 0
	}
	info, ok := commandTable[cmd]
	if !ok {
		msg := fmt.Sprintf("unknown command: %s", cmd)
		if suggestion := cliutil.Suggest(cmd, commandOrder); suggestion != "" {
			msg += fmt.Sprintf(" (did you mean %q?)", suggestion)
		} else {
			msg += fmt.Sprintf(" (see %s --help)", cliutil.ProgName())
		}
		fmt.Fprintf(os.Stderr, "%s: %s\n", cliutil.ProgName(), msg)
		return 1
	}
	prog := cliutil.ProgName()
	line := prog + " " + cmd
	if info.usage != "" {
		line += " " + info.usage
	}
	if len(info.flags) > 0 {
		line += " [options]"
	}
	fmt.Printf("%s - %s\n\nUsage: %s\n", cmd, info.desc, line)
	if len(info.flags) > 0 {
		fmt.Print("\nOptions:\n")
		for _, f := range info.flags {
			fmt.Println(f)
		}
	}
	return 0
}
