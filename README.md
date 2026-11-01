# romctl

Audit and normalize a ROM library laid out for [RetroDECK](https://retrodeck.net/)
(ES-DE directory names under `roms/`, emulator firmware under `bios/`).

`romctl check` reports what is out of convention. The other commands fix it.
Every fixing command is a **dry run unless `--apply` is given**, and nothing is
deleted outright: replaced originals are moved to `<system>/_to_delete/` and
removed later with `romctl clean`.

## Contents

- [Requirements](#requirements)
- [Usage](#usage)
- [Library conventions](#library-conventions)
- [BIOS and firmware](#bios-and-firmware)
- [Emulator configuration notes](#emulator-configuration-notes)
- [Syncing to a device](#syncing-to-a-device)
- [Out of scope](#out-of-scope)
- [License](#license)

## Requirements

| Tool | Used by | Upstream |
|---|---|---|
| bash 4.4+, coreutils, `od`, `md5sum`, `perl` | all | base system |
| `rsync`, `ssh` | `sync` | base system |
| `chdman` | `chd`, `dvd` | [MAME](https://github.com/mamedev/mame) |
| `maxcso` | `chd` (PSP `.cso`) | [maxcso](https://github.com/unknownbrackets/maxcso) |
| `7z` | `unpack`, `chd` (archives), `ps3` | [7-Zip](https://github.com/ip7z/7zip) |
| `makeps3iso` | `ps3` (ISO build) | [ps3iso-utils](https://github.com/bucanero/ps3iso-utils) |
| `dolphin-tool` | `rvz` | [Dolphin](https://github.com/dolphin-emu/dolphin) |

`check` and `bios` need only the base system; CHD, XISO and N64 headers are
parsed directly. The external tools can come from distro packages or be built
from source with the bundled Makefile.

### Building the tools

`make` fetches each tool at a pinned upstream release into `vendor/` and
installs the binaries into `bin/`. romctl puts its own `bin/` first in `PATH`,
so built tools take precedence over system copies (this also works when
romctl is invoked through a symlink).

```sh
make                 # everything
make chdman 7z       # selected tools
make JOBS=8          # parallelism (default: nproc)
make clean           # drop build output, keep sources
make distclean       # drop sources too
```

Build dependencies: a C/C++ toolchain, GNU make, CMake, git, Python 3,
pkg-config, and development headers for SDL2 (MAME's build configuration
requires it even for tools-only builds), libuv, lz4 and zlib.

```sh
# Fedora
sudo dnf install gcc-c++ make cmake git python3 pkgconf-pkg-config \
  SDL2-devel libuv-devel lz4-devel zlib-devel
# Debian/Ubuntu
sudo apt install build-essential cmake git python3 pkg-config \
  libsdl2-dev libuv1-dev liblz4-dev zlib1g-dev
```

MAME and Dolphin are large; expect a first build to take a while. Build on a
native Linux filesystem; building on a mounted Windows or network share is
much slower.

To run romctl from anywhere, symlink it into a directory on `PATH`:

```sh
ln -s "$PWD/romctl" ~/.local/bin/romctl
romctl check -r /path/to/retrodeck
```

To update a tool, bump its `*_REF` in the Makefile and run
`make distclean all`.

## Usage

```
romctl COMMAND [SYSTEM...|all] [--apply] [-r ROOT]
```

Options may appear anywhere on the command line. `ROOT` is the library root containing `roms/` and `bios/` (default: current
directory). Systems are directory names under `roms/`; omit them or pass `all`
for every system.

| Command | Action |
|---|---|
| `check` | Audit systems and BIOS files. Read-only. Exits 1 if anything is `FAIL`. |
| `bios` | Audit BIOS files only. |
| `detag` | Strip region/dump tags, move a leading article to the end, capitalize "- The". |
| `unpack` | Extract single-file archives in cartridge systems. |
| `chd` | Convert game directories, archives and loose `.cue`/`.gdi`/`.iso`/`.cso` to CHD. Multi-disc games become a `.m3u` directory. |
| `m3u` | Group loose `Title (Disc N).chd` files into `Title.m3u/`. |
| `dvd` | Re-encode DVD images stored as CD-type CHDs (e.g. PS2) as DVD CHDs. |
| `n64` | Normalize `.v64`/`.n64` byte orders to big-endian `.z64`. |
| `snes` | Strip 512-byte copier headers; rename `.smc` to `.sfc`. |
| `xiso` | Rename Xbox XISO images from `.iso` to `.xiso`. |
| `rvz` | Convert GameCube/Wii images to RVZ. |
| `ps3` | Unpack PS3 archives into disc folders, then build ISOs from them. |
| `clean` | Delete `_to_delete/` directories. |
| `sync` | Mirror `roms/` and/or `bios/` to the device. See [Syncing to a device](#syncing-to-a-device). |
| `systems` | List known systems and their classes. |

Typical session:

```bash
romctl check                  # what's wrong
romctl chd psx                # what would change
romctl chd psx --apply        # change it
romctl check psx              # confirm
romctl clean psx --apply      # after testing the new files
```

`check` output format: `LEVEL  system  file: message  [suggested command]`.

| Level | Meaning |
|---|---|
| `FAIL` | Broken or won't launch: missing/bad BIOS, broken playlist, invalid image, leftover partial file |
| `WARN` | Works, but out of convention: tags, archives, raw disc images, wrong container |
| `INFO` | Optional improvement or pending cleanup |

A run with no `FAIL` or `WARN` findings ends with `✓ Clean`. Color is used
only on a terminal and honors `NO_COLOR`.

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

- No region or dump tags: `Sonic CD`, not `Sonic CD (USA) [!]`.
- `(Disc N)` is the only tag kept, and only inside `.m3u` directories.
- Leading articles move to the end of the title segment:
  `Legend of Zelda, The - The Minish Cap`.
- Sidecar files (`.ips`, `.sbi`, `.cheats`) share the ROM's base name and are
  renamed with it.
- Arcade systems are never renamed: filenames are set names that the
  emulator resolves.

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

## Syncing to a device

`romctl sync` mirrors the library to the device over SSH with `rsync --delete`,
so files replaced or removed locally are removed on the device too. The local
copy is the source of truth.

```bash
romctl sync                 # dry run: roms/ and bios/; "*deleting" lines are removals
romctl sync --apply         # do it
romctl sync roms --apply    # one target only
```

### Configuration

`${ROMCTL_CONFIG:-${XDG_CONFIG_HOME:-~/.config}/romctl/config}`, plain
`KEY=value` lines (the file is parsed, never sourced):

```ini
SYNC_USER=deck
SYNC_HOST=host1.example.com
# Library root on the device (contains roms/ and bios/)
SYNC_PATH=/path/on/device/retrodeck
# Optional, repeatable: extra rsync exclude patterns
SYNC_EXCLUDE=*.keep
```

Authentication is plain SSH: set up a key and `~/.ssh/config` entry if you
want, or type the password when prompted.

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

- Rescan in ES-DE after a sync. Renamed files or changed extensions appear as
  new entries and need re-scraping.
- The `-rlt` transfer mode (no owner/permission bits) suits filesystems
  without POSIX ownership on either side.

## Out of scope

- **Dump verification.** `check` validates structure, not dump quality. Verify
  ROMs against No-Intro and Redump DATs with a ROM manager such as
  [Igir](https://igir.io/).
- **Full Xbox images.** Rebuild them as XISO with `extract-xiso -r`; `check`
  flags them.
- **Headerless Atari 7800 ROMs.** ProSystem identifies them by checksum; replace
  with headered No-Intro `.a78` dumps if a game misbehaves.

## License

romctl is licensed under the GNU General Public License v3.0 or later; see
[LICENSE](LICENSE). Tools built by the Makefile are fetched from their upstream
projects and remain under their own licenses.
