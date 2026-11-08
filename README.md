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
`unpack`, `m3u`, `n64`, `snes`, `clean` and `systems` are pure Go: archive
handling, CSO decompression and ISO9660 reading/writing are built in. Four
commands still shell out to an external tool: three for formats (CHD, RVZ
and the Xbox XDVDFS disc format) complex and emulator-sensitive enough not
to reimplement, one for scraping (a mature, already-compliant scraper not
worth reimplementing against ScreenScraper's own quota/approval rules — see
[Scraping](#scraping)):

| Tool | Used by | Upstream |
|---|---|---|
| `chdman` | `chd`, `dvd` | [MAME](https://github.com/mamedev/mame) |
| `dolphin-tool` | `rvz` | [Dolphin](https://github.com/dolphin-emu/dolphin) |
| `extract-xiso` | `xiso` (full/redump disc images only; renaming an already-valid XISO is pure Go) | [extract-xiso](https://github.com/XboxDev/extract-xiso) |
| `skyscraper` | `scrape` | [Skyscraper](https://github.com/muldjord/skyscraper) |

A release binary has these **embedded** for its platform and extracts them
to a local cache on first use — nothing to build yourself. If a release
doesn't carry one for your platform (see `make tools` below), romctl falls
back to a copy on `PATH`.

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

## Usage

```
romctl COMMAND [SYSTEM...|all] [--dry-run] [-r ROOT]
```

Options may appear anywhere on the command line. `ROOT` is the library root
containing `roms/` and `bios/`. If `-r`/`--root` isn't passed, romctl looks
for `[library] root` in the config file, then for a `roms/` directory right
where it's being run from, then for a local RetroDECK install to
auto-discover it from (see [Configuration](#configuration)) - the last of
these never overrides an actual `roms/` sitting in the current directory,
even if it's not the RetroDECK install's own resolved path. Systems are
directory names under `roms/`; omit them or pass `all` for every system.
Every mutating command below applies its changes immediately; pass
`--dry-run`/`-n` to preview instead. By default every command prints a
one-line summary when it finishes (e.g. `chd: 3 converted, 1 skipped, 0
failed in 12s`) instead of a line per file; pass `-v`/`--verbose` to also
print each individual action as it happens. `--dry-run` always previews in
full regardless of `-v` - that's the point of a dry run. After a mutating
command finishes, if `[sync] host`/`--sync-host` is configured, romctl also
prints a one-line reminder to run `sync` and push the changes up.
Conversion commands (`chd`, `dvd`, `rvz`, `xiso`, `n64`, `snes`, `unpack`)
convert every matching file across every system given in parallel, scaled
to available CPU cores - pass one large system to convert, not one call per
system, to get the full benefit. On Linux, `chd`/`dvd` also stage the
candidate CHD on `/dev/shm` (tmpfs) when there's room, skipping a
redundant read-back from a slow ROM library (e.g. an SD card) during
chdman's own verify step; this is size-aware and silently falls back to
disk when it isn't safe to use.

| Command | Action |
|---|---|
| `check` | Audit systems and BIOS files. Read-only. Exits 1 if anything is `FAIL`. `--dupes` adds an exact-duplicate-file scan; `-v`/`--verbose` prints every finding instead of the `Checks:`/`Fixes:`/summary sections. |
| `bios` | Audit BIOS files only. |
| `list` | List populated systems (full name, game count). `-g`/`--games` also lists each title. Full names come from the library's own `systems.txt` (see below) when present, else romctl's own table. |
| `fix` | Run every local fix step below in order (`relocate`, `unpack`, `chd`, `dvd`, `rvz`, `xiso`, `n64`, `snes`, `ps3`, `m3u`, `rename`, `saves`) in one command. Excludes `scrape`/`sync`/`clean`, which need credentials, a device, or confirmation. |
| `rename` | Apply the configured naming scheme (`[naming] scheme`) to loose files. See [Naming](#naming). |
| `unpack` | Extract single-file archives in cartridge systems. |
| `chd` | Convert game directories, archives and loose `.cue`/`.gdi`/`.iso`/`.cso` to CHD. Multi-disc games become a `.m3u` directory. |
| `m3u` | Group loose `Title (Disc N).chd` files into `Title.m3u/`. |
| `dvd` | Re-encode DVD images stored as CD-type CHDs (e.g. PS2) as DVD CHDs. |
| `n64` | Normalize `.v64`/`.n64` byte orders to big-endian `.z64`. |
| `snes` | Strip 512-byte copier headers; rename `.smc` to `.sfc`. |
| `xiso` | Convert Xbox disc images to XISO: rename an already-valid one, or rebuild a full/redump dump. |
| `rvz` | Convert GameCube/Wii images to RVZ. |
| `ps3` | Unpack PS3 archives into disc folders, then build ISOs from them. |
| `saves` | Move stray save/save-state files out of `roms/` into RetroDECK's own `saves/`/`states/` at the library root. See [Stray save files](#stray-save-files). |
| `relocate` | Move a cartridge/handheld ROM out of the wrong system's directory (identified by its own header, not its extension) into the correct one. See [Misfiled ROMs](#misfiled-roms). |
| `scrape` | Fetch game metadata/media via Skyscraper (needs `[scrape] user` in the config file). See [Scraping](#scraping). |
| `clean` | Permanently delete `_to_delete/` directories (asks for confirmation first). |
| `sync` | Mirror `roms/`/`bios/`/`media` to the device, or `--pull` to reverse direction. See [Syncing to a device](#syncing-to-a-device). |
| `systems` | List every known system and its class, whether or not it's populated. |

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

By default `check` prints a live spinner per category while it runs
(naming, structure, containers, bios, and duplicates if `--dupes` is set -
each category scans every system in parallel), then three sections:
`Checks:` (a one-line tally per category), `Fixes:` (how to resolve what
was found - one collapsed line per command-backed fix, e.g. "12 finding(s)
-> [romctl rename all]", since running it fixes every instance; one line per
finding for anything without a command, naming the actual file, e.g.
"psx bios/scph5501.bin: acquire bios/scph5501.bin and place it in bios/"),
and the colorized summary. Pass `-v`/`--verbose` to skip the spinners and
print every individual finding as it's found instead (the previous,
only behavior):

`LEVEL  system  file: message  [suggested command]`.

| Level | Meaning |
|---|---|
| `FAIL` | Broken or won't launch: missing/bad BIOS, broken playlist, invalid image, leftover partial file |
| `WARN` | Works, but out of convention: tags, archives, raw disc images, wrong container |
| `INFO` | Optional improvement or pending cleanup |

A run with no `FAIL` or `WARN` findings ends with `✓ Clean`. Color is used
only on a terminal and honors `NO_COLOR`; the spinner likewise only
appears on a terminal (stderr) and never corrupts piped/redirected output.

`check --dupes` additionally scans each system for exact (byte-for-byte)
duplicate files, flagging every copy after the first. This is off by
default: unlike every other check, finding exact duplicates means reading
every file's full content, which takes meaningfully longer on a library
with large CHD/ISO files - though every file within a system is now
hashed in parallel, so the wall-clock cost is lower than it used to be.
It only catches true duplicates - a game present under two different
region dumps (different hashes, same underlying game) isn't detected;
doing that precisely needs No-Intro/Redump-style DAT clone-group data,
which is out of scope for now.

### Stray save files

Many emulators default to writing save/save-state files right next to the
ROM instead of to RetroDECK's own `saves/`/`states/` directories, so a
library built up over years - especially one migrated from a standalone
emulator setup - can end up with thousands of `.sav`/`.srm`/`.sh`/`.state`
files loose in `roms/`. `check` flags these (`structure`, "does not belong
in roms/") and collapses them into a single `Fixes:` line rather than one
per file, since one command fixes every instance:

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
prefers it over its own hand-ported system-name table (`list`'s full
names) and uses it to flag a `roms/<dir>/` that neither romctl nor that
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
`--scheme` on the command line (see [Configuration](#configuration)):

| `scheme` | Behavior |
|---|---|
| `keep` (default) | Leave names exactly as found; `rename` becomes a no-op. romctl never rewrites your naming unless you ask it to. |
| `notag` | One definitive version per game, no tags: `Sonic CD`, not `Sonic CD (USA) [!]`. `(Disc N)` is the only tag kept, and only inside `.m3u` directories. |
| `no-intro` | Reorder recognized `Region`/`Rev N` tags into [No-Intro's](https://wiki.no-intro.org/index.php?title=Naming_Convention) own order: `Super Mario World (Japan) (Rev 1)`. |
| `redump` | Reorder recognized `Region`/`Disc N`/`Rev N` tags into [Redump's](http://redump.org/) own order: `Final Fantasy VII (USA) (Disc 1)`. |

`no-intro` and `redump` only reorder what's already in the filename -
romctl has no DAT data to check against (see
[Out of scope](#out-of-scope)), so anything they don't recognize (language
lists, `Beta`/`Proto`/`Unl` flags, bad-dump markers, ...) is left as found
rather than guessed at, appended after the reordered tags.

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
| GameCube, Wii | RVZ | Dolphin's native compressed format. Dolphin does not read CHD. |
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
(`neogeo.zip`, `pgm.zip`, ...) in the same directory as the games.

Neo Geo games run best under FBNeo from standard zipped sets. Flash-cart
formats such as `.neo` are redundant once an FBNeo set is present.

## BIOS and firmware

RetroArch cores read BIOS files from one flat system directory by fixed
filename; standalone emulators get subdirectories. The layout looks messy, but
every location is load-bearing. Do not reorganize it.

`romctl bios` reports missing required files for systems that have games, and
verifies MD5s for files with a single canonical dump.

| System | File(s) in `bios/` | Required | Notes |
|---|---|---|---|
| PS1 | `scph5501.bin` (US), `scph5500.bin` (JP), `scph5502.bin` (EU) | One of them | |
| PS2 | `ps2-*.bin` | Yes | Several valid revisions; no single hash. |
| PS3 | `rpcs3/dev_flash/` | Yes | Installed PS3 system firmware. |
| Saturn | `mpr-17933.bin` (US/EU v1.01a), `sega_101.bin` (JP) | US/EU file | Names are lowercase and exact. |
| Sega CD | `bios_CD_U.bin`, `bios_CD_E.bin`, `bios_CD_J.bin` | One of them | |
| PC Engine CD | `syscard3.pce` | Yes | `syscard1/2.pce` and `gexpress.pce` for a few titles. |
| Dreamcast | `dc/dc_boot.bin` | Yes | `dc/dc_flash.bin` holds settings and varies. |
| Naomi / Atomiswave | `dc/naomi.zip`, `dc/awbios.zip` | Yes | |
| Xbox | `mcpx_1.0.bin`, `Complex.bin`, `xbox_hdd.qcow2` | Yes | At the top level of `bios/`, under exactly these names. |
| Nintendo DS | `bios7.bin`, `bios9.bin`, `firmware.bin` | No | melonDS has free replacements. `firmware.bin` is per-console (256 KiB) and has no canonical hash. |
| GB / GBC / GBA / SGB | `gb_bios.bin`, `gbc_bios.bin`, `gba_bios.bin`, `sgb_bios.bin` | No | Boot logos and accuracy edge cases only. |
| Famicom Disk System | `disksys.rom` | Yes | |
| Atari 5200 | `5200.rom` | Yes | |
| Atari Lynx | `lynxboot.img` | Yes | |
| 3DO | `panafz10.bin` | Yes | |

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
[Naming](#naming)).

**The config file is entirely optional.** Every key below also has a CLI
flag, so a config-free setup (e.g. a first run, or a one-off invocation)
works for every command, not just the ones without a `[...]` table:

| Config key | Flag | Default when neither is set |
|---|---|---|
| `[library] root` | `-r`/`--root` | local RetroDECK install, else `.` |
| `[naming] scheme` | `--scheme` | `keep` |
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

- **Dump verification.** `check` validates structure, not dump quality. Verify
  ROMs against No-Intro and Redump DATs with a ROM manager such as
  [Igir](https://igir.io/).
- **Headerless Atari 7800 ROMs.** ProSystem identifies them by checksum; replace
  with headered No-Intro `.a78` dumps if a game misbehaves.
- **Picking one region's dump over another.** `--dupes` only catches
  byte-for-byte identical files; it doesn't single out one region's dump
  of the same game, which needs No-Intro/Redump-style clone-group data.
  That's on you, same as naming tags - see [Naming](#naming).

## License

romctl is licensed under the GNU General Public License v3.0 or later; see
[LICENSE](LICENSE). Tools built by the Makefile are fetched from their upstream
projects and remain under their own licenses; see
[THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md) for what those are and
why embedding them is GPL-compatible.
