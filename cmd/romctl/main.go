// Command romctl audits and normalizes a RetroDECK/ES-DE ROM library and
// syncs it to a device over rsync/SSH. See usage() (romctl --help) or
// README.md for details.
package main

import (
	"fmt"
	"io"
	"os"

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
	"scrape": true,
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
	help := fs.BoolP("help", "h", false, "")
	showVersion := fs.Bool("version", false, "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cliutil.ProgName(), err)
		return 1
	}

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
		commands.Systems()
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
		fmt.Fprintf(os.Stderr, "%s: unknown command: %s (see %s --help)\n", cliutil.ProgName(), cmd, cliutil.ProgName())
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

	ok, err := dispatch(cmd, lib, &actions.Runner{DryRun: *dryRun, Verbose: *verbose}, systems, *assumeYes, *showGames, *dupes, *verbose, rootHint, scheme, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s: %v\n", cliutil.ProgName(), cmd, err)
		return 1
	}
	if !ok {
		return 1
	}
	if mutatingCommands[cmd] && cmd != "clean" && !*dryRun {
		if n := actions.RetiredCount(); n > 0 {
			fmt.Printf("%d item(s) pending in _to_delete/; run `%s clean` to remove\n", n, cliutil.ProgName())
		}
	}
	return 0
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

func dispatch(cmd string, lib *library.Library, runner *actions.Runner, systems []string, assumeYes, showGames, dupes, verbose bool, rootHint string, scheme actions.Scheme, cfg config.Config) (bool, error) {
	switch cmd {
	case "check":
		return commands.Check(lib, systems, dupes, scheme, verbose, rootHint)
	case "bios":
		return commands.Bios(lib, systems)
	case "list":
		return true, commands.List(lib, systems, showGames)
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
	flagVerbose       = "  -v, --verbose           print every finding/action instead of a one-line summary"
	flagYes           = "      --yes               skip the confirmation prompt"
	flagPull          = "      --pull              pull from the device instead of pushing"
	flagScheme        = "      --scheme SCHEME     naming scheme: notag, keep, no-intro, redump (default: [naming] scheme, else keep)"
	flagSyncHost      = "      --sync-host HOST    device hostname/IP (default: [sync] host)"
	flagSyncUser      = "      --sync-user NAME    device SSH username (default: [sync] user, else deck)"
	flagSyncPath      = "      --sync-path PATH    device library root (default: [sync] path, else auto-discovered)"
	flagSyncExclude   = "      --sync-exclude PAT  extra rsync exclude pattern (repeatable; default: [sync] exclude)"
	flagScrapeUser    = "      --scrape-user ID:PW ScreenScraper account (default: [scrape] user)"
	flagScrapeThreads = "      --scrape-threads N  scrape thread count (default: [scrape] threads, else auto)"
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
	"check", "bios", "list", "rename", "unpack", "chd", "m3u", "dvd", "n64",
	"snes", "xiso", "rvz", "ps3", "scrape", "clean", "sync", "systems",
}

var commandTable = map[string]cmdInfo{
	"check":   {"audit systems and BIOS files (read-only)", "[SYSTEM...|all]", []string{flagRoot, flagDupes, flagVerbose, flagScheme}},
	"bios":    {"audit BIOS files only", "[SYSTEM...|all]", []string{flagRoot}},
	"list":    {"list populated systems and game counts (-g for titles)", "[SYSTEM...|all]", []string{flagRoot, flagGames}},
	"rename":  {"apply the configured naming scheme to loose files (see [naming])", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScheme}},
	"unpack":  {"extract single-file archives in cartridge systems", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose}},
	"chd":     {"convert disc images to CHD", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScheme}},
	"m3u":     {"group loose multi-disc CHDs into a playlist directory", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose}},
	"dvd":     {"re-encode DVD images stored as CD-type CHDs", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose}},
	"n64":     {"normalize N64 ROMs to big-endian .z64", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose}},
	"snes":    {"strip copier headers; rename .smc to .sfc", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose}},
	"xiso":    {"rename Xbox XISO images from .iso to .xiso", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose}},
	"rvz":     {"convert GameCube/Wii images to RVZ", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose}},
	"ps3":     {"unpack PS3 archives, then build ISOs from disc folders", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScheme}},
	"scrape":  {"fetch game metadata/media via Skyscraper (needs [scrape] user)", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScrapeUser, flagScrapeThreads}},
	"clean":   {"permanently delete _to_delete/ directories", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagYes}},
	"sync":    {"mirror roms/bios/media to a device (needs [sync] host)", "[roms|bios|media...]", []string{flagRoot, flagDryRun, flagPull, flagSyncHost, flagSyncUser, flagSyncPath, flagSyncExclude}},
	"systems": {"list every known system and its class (not just populated ones)", "", nil},
}

func usage() {
	prog := cliutil.ProgName()
	fmt.Printf(`romctl - audit and normalize a RetroDECK-style ROM library

Usage: %s COMMAND [SYSTEM...|all] [options]
       %s help COMMAND

Options:
%s
%s
%s
%s
%s
%s
%s
%s
%s
%s
%s
%s
%s
%s
%s
%s

Commands:
`, prog, prog, flagRoot, flagDryRun, flagGames, flagDupes, flagVerbose, flagYes, flagPull, flagScheme,
		flagSyncHost, flagSyncUser, flagSyncPath, flagSyncExclude, flagScrapeUser, flagScrapeThreads,
		flagHelp, flagVersion)
	for _, name := range commandOrder {
		fmt.Printf("  %-8s %s\n", name, commandTable[name].desc)
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
		fmt.Fprintf(os.Stderr, "%s: unknown command: %s (see %s --help)\n", cliutil.ProgName(), cmd, cliutil.ProgName())
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
