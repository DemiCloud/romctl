package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/demicloud/romctl/internal/cliutil"
)

// Flag lines shared between the root command's help and each
// subcommand's, kept in one place so they can't drift apart. Unlike
// cobra's own auto-generated flag usage, these are the exact,
// hand-tuned, man-page-brief lines romctl has always printed - printHelp
// renders these instead of cobra's defaults so the migration to cobra
// (for completion/POSIX-flag handling) doesn't change a single line of
// output text.
const (
	flagRoot          = "  -r, --root DIR          library root containing roms/ and bios/ (default: local RetroDECK install, else .)"
	flagDryRun        = "  -n, --dry-run           preview changes instead of making them (default: apply)"
	flagGames         = "  -g, --games             also print each game's title"
	flagDupes         = "      --dupes             also scan for exact duplicate files"
	flagFormat        = "      --format FMT        fancy, plain, json, yaml, toml, csv"
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

// cmdInfo is one command's entry in the shared table printHelp reads
// from for both the root overview and a single command's own help.
type cmdInfo struct {
	desc  string   // one-line description
	usage string   // arguments after "COMMAND", e.g. "[SYSTEM...|all]"
	flags []string // flagRoot/flagDryRun/etc., in display order
}

var commandOrder = []string{
	"check", "bios", "list", "fix", "rename", "unpack", "chd", "m3u", "n64",
	"snes", "xiso", "rvz", "ps3", "saves", "relocate", "scrape", "clean", "sync", "systems", "tools",
}

// commandGroups orders commandOrder's same names into sections for the
// root overview: the 3 commands most sessions actually use, then
// everything else. The individual steps `fix` chains (chd, rename, ...)
// are deliberately not listed here at all - see newHiddenStepCmd.
var commandGroups = []struct {
	label string
	names []string
}{
	{"", []string{"check", "fix", "sync"}},
	{"Other:", []string{"bios", "list", "scrape", "clean", "systems", "tools"}},
}

var commandTable = map[string]cmdInfo{
	"check":    {"audit systems and BIOS files (read-only)", "[SYSTEM...|all]", []string{flagRoot, flagDupes, flagVerbose, flagScheme, flagFormat}},
	"bios":     {"audit BIOS files only", "[SYSTEM...|all]", []string{flagRoot, flagVerbose, flagFormat}},
	"list":     {"list populated systems and game counts", "[SYSTEM...|all]", []string{flagRoot, flagGames, flagFormat}},
	"fix":      {"run every local fix step", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScheme, flagDest, flagFormat}},
	"rename":   {"apply the configured naming scheme to loose files", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScheme, flagFormat}},
	"unpack":   {"extract single-file archives in cartridge systems", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"chd":      {"convert disc images to CHD", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScheme, flagFormat}},
	"m3u":      {"group loose multi-disc CHDs into a playlist directory", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"n64":      {"normalize N64 ROMs to big-endian .z64", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"snes":     {"strip copier headers; rename .smc to .sfc", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"xiso":     {"convert Xbox disc images to XISO", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"rvz":      {"convert GameCube/Wii images to RVZ", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"ps3":      {"unpack PS3 archives, then build ISOs from disc folders", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScheme, flagFormat}},
	"saves":    {"move stray save/state files out of roms/ into saves/, states/", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagDest, flagFormat}},
	"relocate": {"move a misfiled cartridge ROM into its correct system directory", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"scrape":   {"fetch game metadata/media via Skyscraper", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScrapeUser, flagScrapeThreads, flagFormat}},
	"clean":    {"permanently delete _to_delete/ directories", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagYes, flagFormat}},
	"sync":     {"mirror roms/bios/media to a device", "[roms|bios|media...]", []string{flagRoot, flagDryRun, flagPull, flagSyncHost, flagSyncUser, flagSyncPath, flagSyncExclude}},
	"systems":  {"list every known system and its class", "", []string{flagFormat}},
	"tools":    {"report native tool availability", "", []string{flagFormat}},
}

// defaultHelpFunc is cobra's own stock renderer, captured before
// printHelp replaces it on root (inherited by every subcommand) - used as
// printHelp's fallback for a command not in commandTable (cobra's own
// built-in help/completion subtree), so those still render sensibly
// instead of printing nothing.
var defaultHelpFunc func(*cobra.Command, []string)

// printHelp is root's HelpFunc/UsageFunc: the root overview (grouped
// commands) for the root command itself, man-page-brief per-command help
// (commandTable) for anything listed there, and cobra's own default
// rendering for anything else (its built-in help/completion commands).
func printHelp(cmd *cobra.Command, args []string) {
	if !cmd.HasParent() {
		printRootHelp()
		return
	}
	info, ok := commandTable[cmd.Name()]
	if !ok {
		defaultHelpFunc(cmd, args)
		return
	}
	prog := cliutil.ProgName()
	line := prog + " " + cmd.Name()
	if info.usage != "" {
		line += " " + info.usage
	}
	if len(info.flags) > 0 {
		line += " [options]"
	}
	fmt.Printf("%s - %s\n\nUsage: %s\n", cmd.Name(), info.desc, line)
	if len(info.flags) > 0 {
		fmt.Print("\nOptions:\n")
		for _, f := range info.flags {
			fmt.Println(f)
		}
	}
}

func printRootHelp() {
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
