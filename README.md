# romctl

Audit and normalize a ROM library laid out for [RetroDECK](https://retrodeck.net/)
(ES-DE directory names under `roms/`, emulator firmware under `bios/`).

`romctl check` reports what is out of convention. The other commands fix it.
Every fixing command **applies its changes by default**; pass `--dry-run` to
preview instead. Nothing is deleted outright: replaced originals are moved to
`<system>/_to_delete/` and removed later with `romctl clean` (which asks for
confirmation first, since it's the one step that can't be undone). A run
that retires anything prints a reminder of how many items are waiting.

## Contents

- [Requirements](#requirements)
- [Usage](#usage)
- [Library conventions](#library-conventions)
- [BIOS and firmware](#bios-and-firmware)
- [Emulator configuration notes](#emulator-configuration-notes)
- [Scraping](#scraping)
- [Syncing to a device](#syncing-to-a-device)
- [Out of scope](#out-of-scope)
- [License](#license)

## Requirements

A single `romctl` binary for your platform (Linux, Windows or macOS) —
nothing else to install for most commands. `check`, `bios`, `rename`,
`unpack`, `m3u`, `n64`, `snes`, `clean` and `system` are pure Go: archive
handling, CSO decompression and ISO9660 reading/writing are built in. Four
commands still shell out to an external tool: three for formats (CHD, RVZ
and the Xbox XDVDFS disc format) complex and emulator-sensitive enough not
to reimplement, one for scraping (a mature, already-compliant scraper not
worth reimplementing against ScreenScraper's own quota/approval rules — see
[Scraping](#scraping)):

| Tool | Used by | Upstream |
|---|---|---|
| `chdman` | `chd` | [MAME](https://github.com/mamedev/mame) |
| `dolphin-tool` | `rvz` | [Dolphin](https://github.com/dolphin-emu/dolphin) |
| `extract-xiso` | `xiso` (full/redump disc images only; renaming an already-valid XISO is pure Go) | [extract-xiso](https://github.com/XboxDev/extract-xiso) |
| `skyscraper` | `scrape` | [Skyscraper](https://github.com/muldjord/skyscraper) |

A release binary has these **embedded** for its platform and extracts them
to a local cache on first use — nothing to build yourself. If a release
doesn't carry one for your platform (see `make tools` below), romctl falls
back to a copy on `PATH`. `romctl tools` reports, for the actual binary
you're running, which of the four are embedded, found on `PATH`, or
neither — so a missing tool shows up before a long `chd`/`rvz` run, not
partway through it.

Release binaries aren't code-signed. On macOS, a binary downloaded from a
browser is quarantined and Gatekeeper will refuse to run it ("cannot be
opened because it is from an unidentified developer", or "is damaged and
can't be opened" on newer macOS); clear the quarantine flag with
`xattr -d com.apple.quarantine ./romctl` before running it. On Windows,
SmartScreen may warn about an unrecognized app; choose "More info" ->
"Run anyway".

`sync` additionally needs `rsync` and `ssh` on `PATH` (on Windows: via WSL,
MSYS2/Git for Windows, or similar — reimplementing the rsync protocol is out
of scope).

### Building from source

```sh
go build -o romctl ./cmd/romctl    # or: make build
```

### Building the embedded tools yourself

Only needed if you're building a release, or your platform doesn't have one
embedded. `make tools` fetches `chdman`, `dolphin-tool`, `extract-xiso` and
`skyscraper` at a pinned upstream release and builds them for the host
platform, staging them where the next `go build`/`make build` will embed
them:

```sh
make tools                  # all four tools for the host platform
make tools-windows-amd64    # cross-build chdman + extract-xiso for Windows via MinGW-w64
make JOBS=8                 # parallelism (default: nproc)
make clean                  # drop build output, keep fetched sources
make distclean               # drop sources too
```

Build dependencies: a C/C++ toolchain, GNU make, CMake, git, Python 3,
pkg-config, development headers for SDL2 (MAME's build configuration
requires it even for a tools-only build) and zlib, and Qt5 (`qmake`) for
Skyscraper.

```sh
# Fedora
sudo dnf install gcc-c++ make cmake git python3 pkgconf-pkg-config SDL2-devel zlib-devel qt5-qtbase-devel
# Debian/Ubuntu
sudo apt install build-essential cmake git python3 pkg-config libsdl2-dev zlib1g-dev qtbase5-dev qt5-qmake qtbase5-dev-tools
```

MAME and Dolphin are large; expect a first build to take a while. Build on a
native Linux filesystem; building on a mounted Windows or network share is
much slower. To update a tool, bump its `*_REF` in the Makefile and run
`make distclean tools`.

#### Embedding DAT snapshots

Only needed to build a release that has `romctl collection`'s have/missing
data for one or more systems built in (`--dat-dir` works against your own
DAT files either way, with no build step). Unlike the tools above, `make
dats` doesn't fetch anything itself:

1. Download the current bulk pack - No-Intro's Dat-o-Matic "Daily"
   aggregate ([datomatic.no-intro.org](https://datomatic.no-intro.org)) is
   a single zip covering every system at once ("No-Intro Love Pack (DAT)
   (*date*).zip"); use whatever [Redump's downloads
   page](http://redump.org/downloads/) offers for disc-based systems.
2. Unzip it, as-is, under `thirdparty/dats/no-intro/` and/or
   `thirdparty/dats/redump/`. No need to pick out or rename individual
   files - the pack bundles hundreds of systems romctl doesn't track, and
   each file's own name carries a timestamp that changes release to
   release; `make dats` matches and renames only what's actually needed.
3. Run it:

```sh
make dats    # validate and stage into internal/datdb/data/ for embedding
```

See `AGENTS.md`'s "DAT snapshot status" for the exact matching mechanism
(`internal/datdb.Stage`) and why this step is manual rather than
automated.

## Usage

```
romctl COMMAND [SYSTEM...|all] [--dry-run] [-r ROOT]
```

### Shell completion

```sh
romctl completion bash > /etc/bash_completion.d/romctl   # or see each shell's own instructions:
romctl completion zsh --help
romctl completion fish --help
romctl completion powershell --help
```

Tab-completes commands, flags, and system names - the last of these reads
whatever's actually under the resolved library's `roms/` (honoring an
already-typed `-r`/`--root`), not a fixed list.

Options may appear anywhere on the command line. `ROOT` is the library root
containing `roms/` and `bios/`. If `-r`/`--root` isn't passed, romctl looks
for `[library] root` in the config file, then for a `roms/` directory right
where it's being run from, then for a local RetroDECK install to
auto-discover it from (see [Configuration](#configuration)) - the last of
these never overrides an actual `roms/` sitting in the current directory,
even if it's not the RetroDECK install's own resolved path. Systems are
directory names under `roms/`; omit them or pass `all` for every system.
Every mutating command below applies its changes immediately; pass
`--dry-run`/`-n` to preview instead. The first time any mutating command
(other than `clean`, which always confirms for itself) runs against a
library root it hasn't touched before, romctl prints a one-time notice -
what it's about to do and that replaced files aren't actually deleted - and
asks for confirmation; `--yes` skips it (and any `clean` confirmation too).
Once confirmed, a root is remembered (see [Configuration](#configuration)'s
directory) and never asked about again. The check is skipped entirely in a
non-interactive session (no controlling terminal, e.g. CI/cron), so a
scripted run is never the first thing to hit a prompt it can't answer.

By default every command prints a one-line summary when it finishes (e.g.
`chd: 3 converted, 1 skipped, 0 failed in 12s`) instead of a line per file;
pass `-v`/`--verbose` to also print each individual action as it happens.
`--dry-run` always previews in full regardless of `-v` - that's the point
of a dry run. In between, the long-running conversion commands (`chd`,
`rvz`, `xiso`, `n64`, `snes`, `unpack`) animate a live `done/total`
counter on stderr so a run across hundreds of files doesn't look stalled;
like `check`'s own spinner, it's skipped under `-v`/`--dry-run` (which
already print per-file) and when stderr isn't a terminal. After a mutating
command finishes, if `[sync] host`/`--sync-host` is configured, romctl also
prints a one-line reminder to run `sync` and push the changes up.

### Output format

`--format FMT` controls how every command (`scrape`/`sync` excepted - see
below) renders its result:

| `FMT` | Behavior |
|---|---|
| `fancy` | Colors, unicode symbols (`✓`/`!`/`✗`) and live spinners/progress counters. Default on a terminal. |
| `plain` | The same text, but bare ASCII (`OK`/`WARN`/`FAIL`) and no color or animation. Default when output is piped/redirected. |
| `json`, `yaml`, `toml` | Every finding (`check`/`bios`) or a result object (every mutating command: counts, the action log, pass/fail, elapsed seconds) - for scripting, not for reading in a terminal. |
| `csv` | The same data as a table: one row per finding for `check`/`bios`, one row per command invocation otherwise (so running the same command repeatedly and appending builds a history). |

Auto-detection (`fancy` vs `plain`) is based on whether stdout is a
terminal, same as `NO_COLOR` (https://no-color.org/, always wins even over
an explicit `--format=fancy`) and the historical per-command color/spinner
behavior this replaces. A structured format silences the normal text
output entirely (no `plan:`/`do:` lines, no spinner, no one-line summary) -
exactly one encoded result lands on stdout; `clean`'s confirmation prompt
and listing still go to stderr either way, so it stays interactive even
under `--format=json`. `scrape` and `sync` are the two exceptions: both
stream another process's own live/interactive output (Skyscraper, rsync)
that can't be captured as structured data without losing the thing that
makes it useful, so `scrape --format=json` (etc.) falls back to plain text
with a note, and `sync` doesn't take `--format` at all.
Conversion commands convert every matching file across every system given
in parallel, scaled to available CPU cores - pass one large system to
convert, not one call per system, to get the full benefit. On Linux,
`chd` also stages the candidate CHD on `/dev/shm` (tmpfs) when there's
room, skipping a redundant read-back from a slow ROM library (e.g. an SD
card) during chdman's own verify step; this is size-aware and silently
falls back to disk when it isn't safe to use.

| Command | Action |
|---|---|
| `check` | Audit systems and BIOS files (naming, structure, containers, BIOS). Read-only. Exits 1 if anything is `FAIL`. `-v`/`--verbose` prints every finding instead of the Checks/Fixes tables. See [Output format](#output-format) for `--format`. |
| `bios` | Audit BIOS files only. `-v`/`--verbose` prints the full inventory (every known file, not just problems) instead. See [BIOS and firmware](#bios-and-firmware). |
| `dupes` | Scan for exact (byte-for-byte) duplicate files. Read-only, its own command rather than a `check` flag - see below for why. |
| `verify` | Verify every CHD's data against its own checksums, via chdman. Read-only, its own command rather than a `check` flag - see below for why. |
| `collection` | Compare games against an embedded (or `--dat-dir`-overridden) No-Intro/Redump DAT, reporting have/missing/unverifiable per game. Read-only. See [Collection: have/missing against a DAT](#collection-havemissing-against-a-dat). |
| `identify` | Identify one or more arbitrary files - not a system - against every embedded (or `--dat-dir`-overridden) No-Intro/Redump DAT at once. Read-only; never renames anything. See [Identify: what is this file?](#identify-what-is-this-file). |
| `system list` | List every known system, its `roms/` directory, and its class, whether or not it's populated. |
| `system games` | List populated systems (full name, game count). `-g`/`--games` also lists each title. Full names come from the library's own `systems.txt` (see below) when present, else romctl's own table. |
| `fix` | Run every local fix step below in order (`relocate`, `unpack`, `chd`, `rvz`, `xiso`, `n64`, `snes`, `ps3`, `m3u`, `rename`, `saves`) in one command. Excludes `scrape`/`sync`/`clean`, which need credentials, a device, or confirmation. |
| `rename` | Apply the configured naming scheme (`[naming] scheme`) to loose files. `--verify-dat` hash-confirms against a DAT first and uses its name when confirmed, falling back to the plain scheme cleanup for anything it can't confirm. See [Naming](#naming). |
| `unpack` | Extract single-file archives in cartridge, N64, SNES, GameCube/Wii and Xbox systems. CD/DVD/UMD-class systems and PS3 extract their own archives as part of conversion (`chd`, `ps3`); arcade sets are never unpacked. |
| `chd` | Convert game directories, archives and loose `.cue`/`.gdi`/`.iso`/`.cso` to CHD. Multi-disc games become a `.m3u` directory. Also re-encodes an existing CHD that was made CD-type before a DVD-class system (e.g. PS2) had that distinction, as proper DVD-type - same "make this system's CHDs correct" pass, not a separate command. |
| `m3u` | Group loose `Title (Disc N).chd`/`Title (Disc N).rvz` files into `Title.m3u/`. |
| `n64` | Normalize `.v64`/`.n64` byte orders to big-endian `.z64`. |
| `snes` | Strip 512-byte copier headers; rename `.smc` to `.sfc`. |
| `xiso` | Convert Xbox disc images to XISO: rename an already-valid one, or rebuild a full/redump dump. |
| `rvz` | Convert GameCube/Wii images to RVZ. |
| `ps3` | Unpack PS3 archives into disc folders, then build ISOs from them. |
| `saves` | Move stray save/save-state files out of `roms/` into RetroDECK's own `saves/`/`states/` at the library root. See [Stray save files](#stray-save-files). |
| `relocate` | Move a cartridge/handheld ROM out of the wrong system's directory (identified by its own header, not its extension) into the correct one. See [Misfiled ROMs](#misfiled-roms). |
| `scrape` | Fetch game metadata/media via Skyscraper (needs `[scrape] user` in the config file). See [Scraping](#scraping). |
| `clean` | Permanently delete `_to_delete/` directories (asks for confirmation once, for everything it's about to delete). |
| `sync` | Mirror `roms/`/`bios/`/`media` to the device, or `--pull` to reverse direction. See [Syncing to a device](#syncing-to-a-device). |
| `tools` | Report whether `chdman`/`dolphin-tool`/`extract-xiso`/`skyscraper` are embedded in *this* binary, found on `PATH`, or neither (see [Requirements](#requirements)), plus which systems have a No-Intro/Redump DAT snapshot embedded and its version/date (see [Collection](#collection-havemissing-against-a-dat)). |

Typical session:

```bash
romctl check                  # what's wrong
romctl fix --dry-run          # what fix would change, across every local step
romctl fix                    # or one step at a time:
romctl chd psx --dry-run      #   what would change
romctl chd psx                #   change it
romctl check psx              # confirm
romctl clean psx              # after testing the new files (asks to confirm)
```

Every suggested-fix hint `check` prints (and every example above) uses
whatever command you actually typed to invoke romctl — `./romctl`, a bare
`romctl` from `PATH`, or an absolute path — plus an explicit `-r PATH` if
you passed one, so it can be copy-pasted as-is and will still target the
right library root even from a different directory or shell session.

By default `check` prints a live status line per category while it runs
(naming, structure, containers, bios - each category scans every system
in parallel), then two rich tables
(unicode box-drawing in `fancy` format, plain ASCII under `--format=plain`
- see [Output format](#output-format)):

- **Checks**: one row per category - a pass/fail glyph, and how many
  files came back Pass/Warn/Fail/Info. A category that wasn't run at all
  (`bios` with no `bios/` directory) gets a row of dashes instead of real
  counts, since nothing there is actually knowable either way.
- **Fixes** (`Issue Type` / `System` / `Fix`): how to resolve what was
  found - `INFO` findings need no action, so only `FAIL`/`WARN` ones get
  a row. One row per command-backed fix type, naming every system it
  actually affects (or `all` once more than one does, since every fix
  command also accepts `all`), e.g. `romctl rename snes` when only SNES
  needs it. Every other finding is grouped by what's wrong: a kind with
  only one instance still names its own specific file (e.g. "acquire
  bios/scph5501.bin and place it in bios/"); more than one collapses into
  a single row instead of one per file, pointing at the command that
  shows them all (`romctl check -v`). BIOS findings always collapse this
  way, however many there are - `System` reads `BIOS` rather than any one
  console, and the row points at `romctl bios -v`, since that's the
  dedicated command for "what's wrong with my BIOS files", not something
  `check` itself should try to enumerate. The table is omitted entirely
  on a clean run.

Pass `-v`/`--verbose` to skip the live status lines and tables and print
every individual finding as it's found instead (the previous, only
behavior):

`LEVEL  system  file: message  [suggested command]`.

| Level | Meaning |
|---|---|
| `FAIL` | Broken or won't launch: missing/bad BIOS, broken playlist, invalid image, leftover partial file |
| `WARN` | Works, but out of convention: tags, archives, raw disc images, wrong container |
| `INFO` | Optional improvement or pending cleanup |

### Duplicate and CHD data checks

`dupes` and `verify` are their own commands rather than `check` flags
(an earlier version of `verify` was exactly that - `check --verify`), for
the same reason `bios` already is: unlike every other check, both mean
reading every file's full content, which can take long enough on a
library with large CHD/ISO files that folding either into check's own
instant, all-categories-at-once output made a long run indistinguishable
from a hang. As their own commands, they get their own progress UX that
actually fits a long scan, without changing what `check -v` prints for
everything else.

`dupes [SYSTEM...|all]` scans each given system for exact (byte-for-byte)
duplicate files, flagging every copy after the first. It only catches
true duplicates - a game present under two different region dumps
(different hashes, same underlying game) isn't detected; doing that
precisely needs No-Intro/Redump-style DAT clone-group data, which is out
of scope for now.

`verify [SYSTEM...|all]` hands every `.chd` (bare files and every disc
inside a `Title.m3u/` playlist directory) to chdman's own `verify`
subcommand - a full pass over every compressed hunk's checksum, not just
the header/metadata read `check`'s `containers` category already does for
free. This catches corruption the header-only check can't: a `.chd` whose
header is intact but whose data got corrupted afterward (bitrot, a
transfer interrupted partway through). It needs chdman itself available
(embedded or on `PATH`; see [Requirements](#requirements)), failing up
front with the same actionable message any other chdman-backed command
would if it isn't.

Both default to a live "done/total" counter on a real terminal while they
hash/verify across a worker pool (the same `cliutil.Progress` every
mutating command already uses for a long batch), then print the same
Checks/Fixes tables `check` does.

`-v`/`--verbose` on a real Fancy terminal instead shows a live, per-file
view: one spinning line per active worker, naming the file it's
processing right now, turning into a permanent colored `✓`/`✗` line the
moment that worker finishes and immediately picking up its next file -
so a long run actually shows multiple files in progress at once, not
just a bare count or a single name sitting still. The number of visible
worker rows is capped to what the terminal can actually display (its
current row count, not raw CPU core count): a block taller than the
terminal can't be redrawn in place - "move the cursor up N rows" clamps
at row 1 once N exceeds what's visible, so every redraw ends up printing
another copy instead of overwriting the last one. Capping the row count
fixes that directly while keeping real concurrency, rather than either
risking that mess again or dropping to one file at a time to avoid it.
Verbose output with no real terminal to animate on (plain/structured
output, or redirected to a file) falls back to one plain
`[i/total] hash sys/file` / `[i/total] verify sys/file` line per file
instead, processed one at a time there to keep the printed order sane.

### Collection: have/missing against a DAT

`collection [SYSTEM...|all]` compares a system's games against an embedded
No-Intro (cartridge/handheld) or Redump (disc-based) DAT snapshot, reporting
one row per known game: `have`, `missing`, or `unverifiable`. A DAT
typically lists far more games than any one library actually has, so by
default only `have`/`unverifiable` rows are reported; pass `--missing` to
also list every game you don't have, for the full picture.

No DAT snapshot ships with romctl by default - see
[Building the embedded tools yourself](#building-the-embedded-tools-yourself)'s
sibling step, `make dats`, which stages DAT files the maintainer downloaded by
hand (from [Dat-o-Matic](https://datomatic.no-intro.org) and
[Redump](http://redump.org/downloads/)) for embedding. `romctl tools`'s
second table reports which systems actually have a snapshot embedded in a
given build, and that snapshot's own version/date. Pass `--dat-dir DIR` to
use your own DAT files (same `<provider>/<System name>.dat` layout) instead
of - or in addition to systems the embedded set doesn't cover.

A game only counts as `have` when every ROM it needs (a multi-disc or
multi-chip game can need more than one) is found among local files by
CRC32/MD5/SHA1, matched whichever way the DAT entry itself provides -
including inside a `.zip`/`.7z`/`.rar`, hashed without extracting to disk.
A `have` row's `Hash` column is the CRC32(s) that actually confirmed it -
the same value(s) `identify` would show for the same file(s).
`.chd` files report as `unverifiable` by default, not matched or missing:
no CHD metadata stores a hash that lines up with a Redump DAT entry
(`chdman verify` only checks a CHD against itself). Pass `--verify-chd`
for a real verdict instead: it extracts the `.chd` (`chdman extractcd`/
`extractdvd`) into scratch space and hashes the result - per-track for a
multi-track CD, parsed from chdman's own generated cue sheet - at a real
cost, roughly what a full `verify` pass already costs, and it's a new
dependency on `chdman` that `collection` otherwise doesn't need at all.
A Redump DAT's own `.cue` entry (commonly hashed alongside the binary
tracks) is ignored on this path: chdman's regenerated cue sheet can never
byte-match Redump's distributed one, even with perfectly correct track
data, so a game counts as matched once every actual track is found.
Cartridge systems are unaffected either way - they're never
CHD-converted, so they always get a real have/missing verdict regardless
of this flag.

Every DAT entry - including every region/revision/translation variant - is
its own row; `collection` doesn't group regional variants of the same game
(1G1R-style) the way a full ROM manager would.

### Identify: what is this file?

`collection` answers "for each known game, what's its status" - DAT-first.
`identify PATH...` answers the opposite question, file-first: "what is
this?" Point it at one or more arbitrary files (not a system - it doesn't
need to be inside `roms/`, or even sorted into the right folder at all)
and it hashes each one and checks it against *every* embedded (or
`--dat-dir`-overridden) No-Intro/Redump DAT, not just one system's -
that's the whole point, you don't have to already know what it is.

A `.chd` is handled automatically: `identify` reads its own header to
tell a DVD-type CHD from a CD/GD-ROM-shaped one (the same byte-level
detection `check` already uses to classify CHDs, see
[Formats by system](#formats-by-system)) and extracts it with the right
`chdman` command accordingly - no `--verify-chd` flag or system hint
needed, since this command only ever handles a handful of files at a
time, not a whole library's worth.

Every path gets a row - `System`/`Game` blank when nothing matched, so a
miss is still visible rather than silently dropped from a multi-file run.
Read-only: it never touches the checked file or anything else. To rename
a confirmed match *inside your library*, that's `rename --verify-dat`
(see [Naming](#naming)) - identify answers "what is this file," rename
answers "what should it be called."

### Stray save files

Many emulators default to writing save/save-state files right next to the
ROM instead of to RetroDECK's own `saves/`/`states/` directories, so a
library built up over years - especially one migrated from a standalone
emulator setup - can end up with thousands of `.sav`/`.srm`/`.sh`/`.state`
files loose in `roms/`. `check` flags these (`structure`, "does not belong
in roms/") and collapses them into a single Fixes-table row rather than
one per file, since one command fixes every instance:

```
romctl saves all
```

This moves each one to `saves/<system>/` (`.sav`/`.srm`/`.sh`) or
`states/<system>/` (`.state`), creating those directories as needed -
RetroDECK's own per-system layout since v0.5.0b, so an emulator pointed at
a standard RetroDECK install picks them up right where it already looks.
Pass `--dest DIR` to relocate into `DIR/saves/`, `DIR/states/` instead -
useful if you'd rather keep save data somewhere other than the
RetroDECK-managed tree entirely.

Nothing is ever deleted or overwritten: this is the user's actual save
data, not a replaceable ROM dump. If the destination already has a
same-named file, the source is left exactly where it was and the
conflict is reported at the end instead, with both copies' size and
modification time and whether their content actually matches:

```
Conflicts (destination already exists; left in place untouched):
  n64   Game.srm   roms/n64: 83.0KiB, 2026-09-01 14:03   vs   saves/n64: 83.0KiB, 2026-09-01 14:03   [identical, safe to delete either copy]
  snes  Zelda.srm  roms/snes: 2.0KiB, 2026-01-02 11:00   vs   saves/snes: 2.1KiB, 2025-11-20 08:40   [DIFFERENT - resolve manually]
```

### Misfiled ROMs

A ROM dropped into the wrong system's directory - the actual trigger for
this feature: a Genesis cartridge ROM zipped up alone with no
`.cue`/`.gdi`/`.iso`, sitting in `roms/segacd/` - fails confusingly rather
than obviously: `chd` can only report "no cue/gdi/iso in: ..." for it,
with nothing pointing at the real problem. `check` identifies a cartridge/
handheld ROM by its own header bytes (not its extension or which directory
it happens to be in), the same magic offsets maintained by the
[`file(1)`/libmagic project](https://github.com/file/file) rather than
guessed, and flags one that doesn't match the system it's actually sitting
in:

```
romctl relocate all
```

This moves each one into its correct system directory - e.g. out of
`roms/segacd/` into `roms/genesis/`. Coverage is deliberately partial:
only formats with a strong, unambiguous signature are recognized (NES,
Famicom Disk System, Game Boy/Color, Game Boy Advance, N64, Genesis/Mega
Drive, Sega 32X, Sega Master System, Game Gear, Atari Lynx, Neo Geo
Pocket) - most pre-16-bit cartridge systems have no reliable header at
all, and SNES's own header check is a weak heuristic even in `file(1)`
itself, so neither is guessed at. Only bare files are checked, not archive
contents - a misfiled ROM still inside an archive is instead named by
`chd`/`unpack`'s own conversion errors, as above.

Like stray saves, nothing is ever overwritten: a destination that already
exists is left in both places and reported as a conflict instead, with
the same size/modification-time/content comparison shown above.

### systems.txt cross-check

ES-DE itself generates a `systems.txt` at the library root (one line per
system, `<dir>: <full name>`) the first time it builds out the ROM
directory tree - a convenience file for users, straight from whatever
RetroDECK version is actually installed. When it's present, romctl
prefers it over its own hand-ported system-name table (`system games`'s
full names) and uses it to flag a `roms/<dir>/` that neither romctl nor that
install's own system list recognizes at all - most likely a typo or a
leftover from a system you no longer use:

```
WARN  totally-bogus-system  roms/totally-bogus-system: not a system directory RetroDECK or romctl recognizes
```

This finding only fires when `systems.txt` exists: romctl's own system
table is a hand-ported subset, so without something to cross-check
against, an unrecognized directory could just as easily be a newer
RetroDECK system romctl hasn't been updated for yet.

## Library conventions

### One file per game

Each game is a single file, except multi-disc games, which are a directory
named `Title.m3u/` containing the discs and a playlist of the same name. ES-DE
treats a directory with a valid extension as one game.

```
roms/psx/Final Fantasy VII.m3u/
├── Final Fantasy VII.m3u
├── Final Fantasy VII (Disc 01).chd
├── Final Fantasy VII (Disc 02).chd
└── Final Fantasy VII (Disc 03).chd
```

The playlist lists one disc filename per line, relative to the directory.
Disc numbers are zero-padded so they sort correctly.

### Naming

How `rename`, `chd`/`ps3`'s output naming, and `check`'s naming audit treat
region/dump tags depends on `[naming] scheme` in the config file, or
`--naming` on the command line (see [Configuration](#configuration)):

| `scheme` | Behavior |
|---|---|
| `keep` (default) | Leave names exactly as found; `rename` becomes a no-op. romctl never rewrites your naming unless you ask it to. |
| `notag` | One definitive version per game, no tags: `Sonic CD`, not `Sonic CD (USA) [!]`. `(Disc N)` is the only tag kept, and only inside `.m3u` directories. |
| `no-intro` | Reorder recognized `Region`/`Rev N` tags into [No-Intro's](https://wiki.no-intro.org/index.php?title=Naming_Convention) own order: `Super Mario World (Japan) (Rev 1)`. |
| `redump` | Reorder recognized `Region`/`Disc N`/`Rev N` tags into [Redump's](http://redump.org/) own order: `Final Fantasy VII (USA) (Disc 1)`. |

`no-intro` and `redump` only reorder what's already in the filename; by
default, `rename` never checks a file's actual content against a DAT (see
[Out of scope](#out-of-scope)), so anything a tag it doesn't recognize
(language lists, `Beta`/`Proto`/`Unl` flags, bad-dump markers, ...) is
left as found rather than guessed at, appended after the reordered tags.
`rename --verify-dat` is the exception: for a file it can hash-confirm
against an embedded (or `--dat-dir`-overridden) No-Intro/Redump DAT, it
uses that DAT game's own name directly - still run through `scheme` -
instead of reordering tags already in the filename; a file it can't
confirm falls back to the same tag-reordering behavior as always. See
[Collection: have/missing against a DAT](#collection-havemissing-against-a-dat)
for the same matching logic's read-only form.

Independent of `scheme`:

- Leading articles move to the end of the title segment:
  `Legend of Zelda, The - The Minish Cap` (skipped entirely under `keep`).
- Sidecar files (`.ips`, `.sbi`, `.cheats`) share the ROM's base name and are
  renamed with it.
- Arcade systems are never renamed: filenames are set names that the
  emulator resolves.
- romctl never picks one region's dump over another on your behalf; see
  [Out of scope](#out-of-scope).

### Formats by system

| Systems | Format | Notes |
|---|---|---|
| CD-based: PS1, Saturn, Sega CD, PC Engine CD, Dreamcast, 3DO, ... | CHD (`createcd`) | Lossless; FLAC for audio tracks. Default codecs. |
| PS2 | CHD (`createdvd -hs 2048`) | DVD titles must be DVD-type CHDs. The few CD-based PS2 titles stay CD-type. |
| PSP | CHD (`createdvd -hs 2048`) | Recommended by RetroDECK; needs PPSSPP 1.17+. CSO is decompressed with `maxcso` first. |
| GameCube, Wii | RVZ | Dolphin's native compressed format. Dolphin does not read CHD. An [NKit](https://wiki.dolphin-emu.org/index.php?title=NKit)-trimmed `Title.nkit.iso` converts the same way as a plain `.iso` - `rvz` matches on the file's final extension, which NKit's own naming leaves as `.iso` - and `dolphin-tool` (built from the same Dolphin tree that can already read an NKit image directly) reconstructs the trimmed junk data during conversion, so the resulting RVZ isn't itself an NKit image and carries none of its caveats (the NKit warning dialog, netplay/savestate incompatibility with a non-NKit copy). |
| Xbox | XISO (`.xiso` or `.iso`) | Trimmed Xbox filesystem image. xemu does not read CHD or full Redump images. |
| PS3 | Disc folder `Title.ps3/` or decrypted ISO | See [PS3](#ps3). |
| N64 | `.z64` | Big-endian (native) byte order. |
| SNES | `.sfc` | No copier header. |
| Other cartridge systems | Bare ROM | Archives add a decompression step and gain nothing at these sizes. |
| Arcade (FBNeo, MAME, Naomi, ...) | `.zip` per set | Never unzip. Keep sets non-merged so each zip is self-contained. |

Why CHD over CUE/BIN: one file instead of many, internal checksums
(`chdman verify`), and no CUE sheets pointing at renamed BIN files.

Why bare cartridge ROMs over 7z/zip: the entire set is tiny, solid 7z must be
decompressed in full on every launch, and some standalone emulators handle
archives inconsistently.

### Arcade

Use a set built for the emulator version actually shipped; ROM definitions
change between releases, and a mismatched set fails in unpredictable ways.
Filter to parent sets with working drivers, and keep required BIOS sets
(`neogeo.zip`, `pgm.zip`, ...) in the same directory as the games - MAME/FBNeo
treat these as just another ROM set (a parent other sets point at), not
firmware, so they belong in `roms/<system>/`, never in `bios/`. See
[BIOS and firmware](#bios-and-firmware) for what actually goes in `bios/`.

Neo Geo games run best under FBNeo from standard zipped sets. Flash-cart
formats such as `.neo` are redundant once an FBNeo set is present.

## BIOS and firmware

RetroArch cores read BIOS files from one flat system directory by fixed
filename; standalone emulators get subdirectories. The layout looks messy, but
every location is load-bearing. Do not reorganize it.

`romctl bios` (and `check`'s `bios` category) audits every file below
against the library's actual systems, at one of three severities:

| Level | When |
|---|---|
| `FAIL` | A **required** file is missing or wrong, and the owning system already has games - broken right now. |
| `WARN` | A **required** file is missing or wrong, but the owning system has no games yet - not broken yet, but will be the moment one's added. Also used for a wrong (not missing) **optional** file - its presence means it's actually in use, so a bad copy is worth a look even though nothing's required. |
| `INFO` | An **optional** file is simply missing - nothing needs it, ever, regardless of whether the system has games. |

A file with a single canonical dump is verified by MD5; one that's
inherently per-console or per-revision (PS2, Xbox's `xbox_hdd.qcow2`,
NDS's `firmware.bin`) is checked by presence (and, for `firmware.bin`,
size) only - hashing those would flag every genuine console's own copy as
"wrong." Every MD5 below has been cross-checked against the
independently-maintained RetroDECK BIOS pack's own per-file manifest, not
hand-typed from one source.

Both commands normally print only what's actually wrong (same as every
other check category); `romctl bios -v` instead prints the **full
inventory** - every file below, including the ones already present and
correct - so there's one place to see exactly what a given library has,
is missing, or doesn't need. `-v`'s table stays terse on purpose: a short
name and a couple-word issue ("missing", "hash mismatch"), not an exact
hash or MAC-address aside - most people running this don't need either.
`--format=json`/`yaml`/`toml` carries the full detail (exact hash values,
the complete technical note) instead, for scripting or a closer look;
`--format=csv` stays as terse as the table, since it's still a columns
format. All four work on both the problems-only and `-v` full-inventory
output.

| System | File(s) in `bios/` | Required | Notes |
|---|---|---|---|
| PS1 | `scph5501.bin` (US), `scph5500.bin` (JP), `scph5502.bin` (EU) | One of them | |
| PS2 | `ps2-*.bin` | Yes | Several valid revisions; no single hash. |
| PS3 | `rpcs3/dev_flash/` | Yes | Installed PS3 system firmware, from `PS3UPDAT.PUP` via RPCS3 - not a single file to hash. |
| Saturn | `mpr-17933.bin` (US/EU v1.01a), `sega_101.bin` (JP) | US/EU file | Names are lowercase and exact. |
| Sega CD | `bios_CD_U.bin`, `bios_CD_E.bin`, `bios_CD_J.bin` | One of them | |
| PC Engine CD / TurboGrafx-CD | `syscard3.pce` | Yes | `syscard1/2.pce` and `gexpress.pce` for a few titles only - optional. |
| Dreamcast | `dc/dc_boot.bin` | Yes | `dc/dc_flash.bin` holds settings and varies; never checked. |
| Naomi / Atomiswave | `dc/naomi.zip`, `dc/awbios.zip` | Yes | Presence-checked only - a zip's own byte layout varies between legitimate sources even when the ROM content inside is identical, so no single hash is correct. |
| Xbox | `mcpx_1.0.bin`, `Complex.bin`, `xbox_hdd.qcow2` | Yes | At the top level of `bios/`, under exactly these names. `mcpx_1.0.bin` (the fixed MCPX boot ROM) is hashed; `Complex.bin` (at least two legitimately different builds circulate under this name) and `xbox_hdd.qcow2` (holds game saves once used) are presence-checked only. |
| Nintendo DS | `bios7.bin`, `bios9.bin`, `firmware.bin` | No | melonDS has free replacements. `firmware.bin` is per-console (256 KiB, size-checked only - it holds a real console's own Wi-Fi MAC address, so it has no canonical hash). |
| GB / GBC / GBA / SGB | `gb_bios.bin`, `gbc_bios.bin`, `gba_bios.bin`, `sgb_bios.bin` | No | Boot logos and accuracy edge cases only. |
| Famicom Disk System | `disksys.rom` | Yes | |
| Atari 5200 | `5200.rom` | Yes | |
| Atari Lynx | `lynxboot.img` | Yes | |
| Atari 7800 | `7800 BIOS (U).rom` | No | A handful of specific titles only; most don't need it. |
| 3DO | `panafz10.bin` | Yes | |
| GameCube / Wii | `GC/USA/IPL.bin`, `GC/EUR/IPL.bin`, `GC/JAP/IPL.bin` | No | Dolphin's per-region boot ROM; HLE works fine without it for almost every game. |

Arcade BIOS sets (`neogeo.zip`, `pgm.zip`, ...) are not audited here at
all: they live inside `roms/<system>/` alongside the game sets (see
[Arcade](#arcade)), and which specific set a given ROM needs is
clone/DAT-level information romctl doesn't have - the same reason dump
verification itself is [out of scope](#out-of-scope).

`bios/` is also checked for the reverse case: a file that doesn't match
anything in the table above at all (a typo'd name, leftover from a system
you no longer use, stray OS/editor clutter) is flagged INFO, same
severity as a missing optional file - it's a heads-up, never a problem.
A file under an installed, multi-file firmware tree (`rpcs3/dev_flash/`)
is never flagged this way, since that's not a single file to begin with.

RetroDECK's Configurator also has a BIOS checker; it is authoritative for the
exact names a given RetroDECK release expects.

## Emulator configuration notes

### PS3

ES-DE's PS3 system offers several launch methods; the default
(shortcut/`.desktop`) does not launch disc folders or ISOs directly. In ES-DE:
**Other Settings → Alternative Emulators → Sony PlayStation 3**:

| Storage | Alternative emulator |
|---|---|
| `Title.ps3/` disc folders | RPCS3 Directory (Standalone) |
| `Title.iso` (decrypted) | RPCS3 ISO (Standalone) |

The system-wide choice can be overridden per game (metadata editor →
Alternative emulator), which allows migrating one game at a time.

- **Disc dumps vs. digital titles.** Disc dumps have a `BLxx`/`BCxx` title ID
  and a `PS3_GAME/` folder. Digital (PSN) titles have an `NPxx` title ID, need
  their license (`.rap`) to decrypt, and cannot become ISOs. Install those
  through RPCS3 instead of storing them under `roms/`.
- **Folders to ISO.** `romctl ps3` builds an ISO from each disc folder with
  `makeps3iso`. Test one game with a per-game override before converting all.
- **Firmware** lives under `bios/rpcs3/`, so it travels with the library.
- **Per-device state.** Controller configuration, shader caches, and installed
  updates/DLC live in the emulator's own config directory on each device.
  Controllers usually work with Configuration → Pads → handler **SDL**.

### Xbox

- BIOS file names and locations must match xemu's configured paths (Machine →
  Settings → System). See the BIOS table.
- Set System Memory to **64 MiB** (retail). 128 MiB is for debug kits and
  homebrew.
- Game saves live inside `xbox_hdd.qcow2`. Treat it as save data, not
  firmware: back it up, and never overwrite it from an older copy.

### Saturn

The default core (Beetle Saturn) will not boot without `mpr-17933.bin`
(US/EU) or `sega_101.bin` (JP). Kronos and YabaSanshiro are alternatives.

### Nintendo DS

Real BIOS/firmware files are optional. If melonDS ignores them, enable
"Use external BIOS/firmware" in its emulation settings. Touch-heavy games
work best on a device with a touchscreen.

### Xbox 360

Check that the frontend actually exposes an Xbox 360 system before collecting
games: every supported system gets a generated `systeminfo.txt` in its ROM
directory.

## Scraping

`romctl scrape` fetches game metadata and media from ScreenScraper and
writes it straight into `ES-DE/gamelists/<system>/gamelist.xml` and
`ES-DE/downloaded_media/<system>/`, via an embedded
[Skyscraper](https://github.com/muldjord/skyscraper). romctl never talks to
a scraper API itself: Skyscraper's own ScreenScraper developer credentials
are baked into it, so only your personal account is needed.

```toml
# In the config file (see Configuration below, or config.example.toml)
[scrape]
user = "ssid:sspassword"
#threads = 6   # optional - see below
```

```sh
romctl scrape --dry-run     # preview: which populated systems would be scraped
romctl scrape               # scrape every populated system with a Skyscraper equivalent
romctl scrape psx snes      # just these systems
```

Leave `threads` unset and Skyscraper auto-detects your ScreenScraper
account's real thread allowance itself; only set it to use fewer threads
than you're allowed. Skyscraper handles the rest of ScreenScraper's quota
compliance itself too (backing off on rate limits, falling back to a single
thread for an unregistered/invalid account), so romctl doesn't reimplement
any of that.

A system needs a `-p` platform Skyscraper actually supports; not every
romctl system has one (original Xbox and PS3 aren't scraped by Skyscraper
at all) - those are skipped with a clear message rather than guessed at.

Multi-disc games (`Title.m3u/` directories) are scraped correctly as one
game, not once per disc: Skyscraper has no native concept of "a directory
is the game," so a regular directory scan would otherwise wander into
`Title.m3u/` and scrape each disc file inside as its own unrelated game
(duplicate entries, wasted ScreenScraper lookups). romctl scrapes each
`Title.m3u/` game by its playlist file alone, in a separate pass, and
corrects Skyscraper's resulting `<path>`/media naming to match the
`Title.m3u.<ext>` convention afterward. Confirmed end-to-end against real
ScreenScraper matches (both a regular single-file game and a multi-disc
one), not just synthetic/unmatched data.

**Known v1 gap**: romctl ships a minimal `artwork.xml` that gets
`covers`/`screenshots`/`marquees`/`wheels` as raw, uncomposited images;
manuals, videos and a few other media types ES-DE can use aren't wired up
yet.

## Syncing to a device

`romctl sync` mirrors the library to the device over SSH with `rsync --delete`,
so files replaced or removed locally are removed on the device too. The local
copy is the source of truth. A third target, `media`, covers ES-DE's scraped
data (`ES-DE/gamelists/` and `ES-DE/downloaded_media/`), which RetroDECK
stores as a sibling of `roms/` and `bios/` under the same device path.

```bash
romctl sync --dry-run       # preview: roms/ and bios/; "*deleting" lines are removals
romctl sync                 # do it
romctl sync roms            # one target only
romctl sync --pull          # initial population: pull roms/bios/media from the device
romctl sync --pull media    # pull just one target, e.g. media you don't have locally yet
```

`--pull` reverses direction (device → local) and never deletes locally, so
it's safe to run anytime, including to populate a brand-new local working
copy from an existing device. With no target named, a push defaults to
`roms`/`bios` (pushing `media` is left explicit, see below); a pull
defaults to all three, since a pull is usually "get me everything I don't
have yet."

### Keeping scraped data in sync

Scraping can happen on-device in ES-DE, or locally via `romctl scrape` (see
[Scraping](#scraping)) - either way, the intended workflow is:

1. `romctl sync --pull media` to bring the device's `gamelist.xml` and
   `downloaded_media/` down locally (or just `romctl sync --pull` if you
   don't have a local working copy at all yet). Skip this if you're
   scraping locally with `romctl scrape` instead of on-device.
2. Run the normal normalization commands (`rename`, `chd`, `n64`, `snes`,
   `xiso`, `rvz`, `m3u`). Any command that renames or converts a ROM that
   was already scraped updates the matching `<path>` in `gamelist.xml` and
   moves the corresponding media files automatically, so nothing is
   orphaned. A ROM that hasn't been scraped yet is untouched (there's
   nothing to update).
3. `romctl sync` to push `roms/`/`bios/` back up (add `media` if you want
   to push local gamelist/media edits back too - including anything
   `romctl scrape` just wrote).

This is pull-then-normalize-then-push by design: there's no automatic
conflict detection between a local pull and new on-device scrapes, so
pulling again before your next push is on you.

The multi-disc (`Title.m3u/`) media-naming convention above (flat
`Title.m3u.<ext>`, no subfolder) is confirmed against a real scraped
library, across multiple systems and every media type ES-DE writes.

### Configuration

`${ROMCTL_CONFIG:-${XDG_CONFIG_HOME:-~/.config}/romctl/config.toml}`, a
TOML file (parsed, never sourced). See
[config.example.toml](config.example.toml) for every key in one place,
including `[scrape]` (see [Scraping](#scraping)) and `[naming]` (see
[Naming](#naming)). A `trusted_roots` file next to it (same directory)
records which library roots have already confirmed the one-time
first-run notice above - delete a line from it (or the whole file) to be
asked again for that root.

**The config file is entirely optional.** Every key below also has a CLI
flag, so a config-free setup (e.g. a first run, or a one-off invocation)
works for every command, not just the ones without a `[...]` table:

| Config key | Flag | Default when neither is set |
|---|---|---|
| `[library] root` | `-r`/`--root` | local RetroDECK install, else `.` |
| `[naming] scheme` | `--naming` | `keep` |
| `[sync] host` | `--sync-host` | *(required for `sync`)* |
| `[sync] user` | `--sync-user` | `deck` |
| `[sync] path` | `--sync-path` | auto-discovered from the device |
| `[sync] exclude` | `--sync-exclude` (repeatable) | *(none)* |
| `[scrape] user` | `--scrape-user` | *(required for `scrape`)* |
| `[scrape] threads` | `--scrape-threads` | auto-detected by Skyscraper |

A flag always wins over the matching config value when both are set.

```toml
[library]
root = "/path/to/library"   # optional override for the -r default (see below)

[sync]
host = "host1.example.com"
exclude = ["*.keep"]   # optional, extra rsync exclude patterns
```

`[library] root` overrides the library root when `-r`/`--root` isn't
passed on the command line. Leave it unset when running romctl directly
on a RetroDECK device: romctl auto-discovers the local install's root the
same way it auto-discovers a remote one for `sync` (see below), by
reading RetroDECK's own `retrodeck.json`, just from the local filesystem
instead of over SSH. Full precedence, highest first: `-r`/`--root` flag,
`[library] root`, a `roms/` directory already in the current directory
(e.g. romctl was placed straight into an existing library), then the
RetroDECK autodiscovery - that last step only runs when the current
directory doesn't already look like a library, so it never redirects away
from a `roms/` that's actually right there.

`host` is the only setting required. `user` defaults to `deck`, the
standard Steam Deck/RetroDECK username, if not set. The device's library
path is auto-discovered: romctl reads RetroDECK's own `retrodeck.json`
over SSH (`paths.rd_home_path`) to find it, so it's correct whether the
install lives on internal storage or an SD card, without any config —
including across the card being moved to a different mount point. Set
`path` only to skip that lookup, e.g. for a non-RetroDECK setup or a
customized install location:

```toml
[sync]
path = "/path/on/device/retrodeck"
```

Authentication is plain SSH: set up a key and `~/.ssh/config` entry if you
want, or type the password when prompted. Without a key, `sync` still only
prompts once per run: it opens a multiplexed SSH connection (OpenSSH
`ControlMaster`) up front and reuses it for the `retrodeck.json` lookup and
every target's rsync transfer. If that connection can't be established for
any reason, each step just falls back to prompting on its own, same as
before.

If the card is mounted locally instead — plugged into the machine running
romctl, not the device — skip `sync` entirely and just point `-r` at it
directly.

Running romctl directly on the device itself (e.g. the Steam Deck's own
desktop mode) needs no `-r` or config at all: with no `-r` and no
`[library] root` set, romctl looks for a local RetroDECK install the same
way `sync` finds one remotely, and uses its library root automatically.

### What is never synced or deleted

rsync does not delete excluded files on the receiver, so every excluded path
is both skipped and protected on the device:

- **Everywhere:** save and state files (`*.sav`, `*.srm`, `*.state*`),
  conversion leftovers, `_to_delete/`.
- **bios/:** emulator-written data: the Xbox HDD image (game saves), PS2
  memory cards and NVRAM, Dreamcast VMU saves and NVRAM, Wii U saves,
  installed Switch content, shader caches.

`saves/` and `states/` live outside `roms/` and `bios/` and are never touched.

Notes:

- A renamed/converted ROM that was already scraped keeps its metadata (see
  [Keeping scraped data in sync](#keeping-scraped-data-in-sync)); a rescan is
  only needed for genuinely new, never-scraped games.
- The `-rlt` transfer mode (no owner/permission bits) suits filesystems
  without POSIX ownership on either side.

## Out of scope

- **Full ROM-manager duties.** `collection` (see
  [Collection: have/missing against a DAT](#collection-havemissing-against-a-dat))
  hash-matches a library against embedded No-Intro/Redump DAT snapshots,
  `identify` (see [Identify: what is this file?](#identify-what-is-this-file))
  identifies an arbitrary file against every embedded DAT at once, and
  `rename --verify-dat` renames a confirmed match to its DAT name - but
  none of these 1G1R-group, dedupe across regions, or otherwise actively
  manage a collection the way a full ROM manager would. Use a full ROM
  manager such as
  [Igir](https://igir.io/) for that.
- **Headerless Atari 7800 ROMs.** ProSystem identifies them by checksum; replace
  with headered No-Intro `.a78` dumps if a game misbehaves.
- **Picking one region's dump over another.** `dupes` only catches
  byte-for-byte identical files; it doesn't single out one region's dump
  of the same game, which needs No-Intro/Redump-style clone-group data.
  That's on you, same as naming tags - see [Naming](#naming).

## License

romctl is licensed under the GNU General Public License v3.0 or later; see
[LICENSE](LICENSE). Tools built by the Makefile are fetched from their upstream
projects and remain under their own licenses; see
[THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md) for what those are and
why embedding them is GPL-compatible.
