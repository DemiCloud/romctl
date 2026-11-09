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
	flagFormat        = "      --format FMT        fancy, plain, json, yaml, toml, csv"
	flagVerbose       = "  -v, --verbose           print every finding/action instead of a one-line summary"
	flagYes           = "      --yes               skip confirmation prompts (clean; first-run library notice)"
	flagPull          = "      --pull              pull from the device instead of pushing"
	flagNaming        = "      --naming SCHEME     naming scheme: notag, keep, no-intro, redump (default: [naming] scheme, else keep)"
	flagSyncHost      = "      --sync-host HOST    device hostname/IP (default: [sync] host)"
	flagSyncUser      = "      --sync-user NAME    device SSH username (default: [sync] user, else deck)"
	flagSyncPath      = "      --sync-path PATH    device library root (default: [sync] path, else auto-discovered)"
	flagSyncExclude   = "      --sync-exclude PAT  extra rsync exclude pattern (repeatable; default: [sync] exclude)"
	flagScrapeUser    = "      --scrape-user ID:PW ScreenScraper account (default: [scrape] user)"
	flagScrapeThreads = "      --scrape-threads N  scrape thread count (default: [scrape] threads, else auto)"
	flagDest          = "      --dest DIR          saves: move into DIR/saves, DIR/states instead of the library root"
	flagDatDir        = "      --dat-dir DIR       use DAT files from DIR instead of the embedded snapshot"
	flagVerifyCHD     = "      --verify-chd        extract and hash-verify .chd files against the DAT too (needs chdman; slower)"
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
	"check", "bios", "dupes", "verify", "collection", "identify", "system", "fix", "rename", "unpack", "chd", "m3u", "n64",
	"snes", "xiso", "rvz", "ps3", "saves", "relocate", "scrape", "clean", "sync", "tools",
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
	{"Other:", []string{"bios", "dupes", "verify", "collection", "identify", "system", "scrape", "clean", "tools"}},
}

var commandTable = map[string]cmdInfo{
	"check":      {"audit systems and BIOS files (read-only)", "[SYSTEM...|all]", []string{flagRoot, flagVerbose, flagNaming, flagFormat}},
	"bios":       {"audit BIOS files only", "[SYSTEM...|all]", []string{flagRoot, flagVerbose, flagFormat}},
	"dupes":      {"scan for exact (byte-for-byte) duplicate files (read-only)", "[SYSTEM...|all]", []string{flagRoot, flagVerbose, flagFormat}},
	"verify":     {"verify every CHD's data against its own checksums (read-only)", "[SYSTEM...|all]", []string{flagRoot, flagVerbose, flagFormat}},
	"collection": {"compare games against a No-Intro/Redump DAT (read-only)", "[SYSTEM...|all]", []string{flagRoot, flagVerbose, flagDatDir, flagVerifyCHD, flagFormat}},
	"identify":   {"rename files to their DAT-verified canonical name", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagNaming, flagDatDir, flagVerifyCHD, flagFormat}},
	"system":     {"system information (see 'system list', 'system games')", "", nil},
	"list":       {"list every known system, its directory, and its class", "", []string{flagFormat}},
	"games":      {"list populated systems and game counts", "[SYSTEM...|all]", []string{flagRoot, flagGames, flagFormat}},
	"fix":        {"run every local fix step", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagNaming, flagDest, flagFormat}},
	"rename":     {"apply the configured naming scheme to loose files", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagNaming, flagFormat}},
	"unpack":     {"extract single-file archives (cartridge/N64/SNES/GC/Wii/Xbox)", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"chd":        {"convert disc images to CHD", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagNaming, flagFormat}},
	"m3u":        {"group loose multi-disc CHDs into a playlist directory", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"n64":        {"normalize N64 ROMs to big-endian .z64", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"snes":       {"strip copier headers; rename .smc to .sfc", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"xiso":       {"convert Xbox disc images to XISO", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"rvz":        {"convert GameCube/Wii images to RVZ", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"ps3":        {"unpack PS3 archives, then build ISOs from disc folders", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagNaming, flagFormat}},
	"saves":      {"move stray save/state files out of roms/ into saves/, states/", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagDest, flagFormat}},
	"relocate":   {"move a misfiled cartridge ROM into its correct system directory", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagFormat}},
	"scrape":     {"fetch game metadata/media via Skyscraper", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagScrapeUser, flagScrapeThreads, flagFormat}},
	"clean":      {"permanently delete _to_delete/ directories", "[SYSTEM...|all]", []string{flagRoot, flagDryRun, flagVerbose, flagYes, flagFormat}},
	"sync":       {"mirror roms/bios/media to a device", "[roms|bios|media...]", []string{flagRoot, flagDryRun, flagPull, flagSyncHost, flagSyncUser, flagSyncPath, flagSyncExclude}},
	"tools":      {"report native tool availability", "", []string{flagFormat}},
}

// printHelp is root's HelpFunc/UsageFunc: the root overview (grouped
// commands) for the root command itself, man-page-brief per-command help
// (commandTable) for anything listed there, and printFallbackHelp for
// anything else (a grouping command like `system`, or cobra's own
// built-in help/completion subtree). This deliberately never calls into
// cobra's own template-based default HelpFunc: that renderer calls
// UsageString(), which resolves (by walking up to root, same as HelpFunc
// itself does) to the UsageFunc set below - which is printHelp - so
// reaching it from here would call back into printHelp and recurse
// forever instead of returning.
func printHelp(cmd *cobra.Command, args []string) {
	if !cmd.HasParent() {
		printRootHelp()
		return
	}
	if cmd.HasSubCommands() {
		printFallbackHelp(cmd)
		return
	}
	info, ok := commandTable[cmd.Name()]
	if !ok {
		printFallbackHelp(cmd)
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

// printFallbackHelp renders man-page-brief help for a command with no
// commandTable entry: a grouping command like `system` (short description
// plus its subcommands), or one of cobra's own built-in commands (help,
// completion) - still just Short/UseLine/child names, not cobra's own
// template renderer (see printHelp's doc comment on why that's avoided).
func printFallbackHelp(cmd *cobra.Command) {
	fmt.Printf("%s - %s\n\nUsage: %s\n", cmd.Name(), cmd.Short, cmd.UseLine())
	children := cmd.Commands()
	if len(children) == 0 {
		return
	}
	fmt.Print("\nCommands:\n")
	for _, c := range children {
		if !c.IsAvailableCommand() {
			continue
		}
		fmt.Printf("  %-12s %s\n", c.Name(), c.Short)
	}
}

func printRootHelp() {
	prog := cliutil.ProgName()
	fmt.Printf(`romctl - audit and normalize a RetroDECK-style ROM library

Usage: %s COMMAND [SYSTEM...|all] [options]
       %s help COMMAND

Global options (apply to most commands; see '%s help COMMAND' for a
command's own options, e.g. --naming, --sync-host, --scrape-user):
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
