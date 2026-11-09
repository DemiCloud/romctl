# AGENTS.md

Guidance for coding agents working in this repository.

## Project

`romctl` is a Go CLI that audits and normalizes a ROM library laid out for
RetroDECK/ES-DE (`roms/<system>/`, `bios/`), and syncs it to a device over
rsync/SSH. It was rewritten from a single Bash script to a Go module so a
released binary embeds everything it needs instead of requiring the user to
compile a pile of external tools first. `README.md` is the user-facing
reference; keep it in step with behavior changes.

| Path | Purpose |
|---|---|
| `cmd/romctl/` | `spf13/cobra` command tree (one file per command/group - `root.go` is shared infra: `appContext`, the first-run/pending-cleanup machinery, `--format` resolution; `help.go` is the hand-tuned man-page-brief help renderer, bypassing cobra's own verbose default; everything else wires a `*cobra.Command` to its `internal/commands` function). `completion`/dynamic system-name completion come from cobra for free. The individual steps `fix` chains (`chd`, `rename`, ...) are built via `newHiddenStepCmd`, not `newMutatingCmd`: real, independently-runnable commands, just left out of the default `--help` listing (`fix`'s own one-liner already covers "what it runs"; `romctl help <name>` still works for any of them directly). |
| `cmd/stage-dats/` | `make dats`'s thin wrapper over `internal/datdb.Stage` - a build-time maintainer tool, not part of any release, same "thin wrapper, real logic lives in `internal/`" convention as `cmd/romctl` itself. |
| `internal/` | Everything else, one package per concern — see below. |
| `Makefile` | `go build`/`test`/`vet` wrappers, plus fetching and building the four native tools (`chdman`, `dolphin-tool`, `extract-xiso`, `skyscraper`) into `internal/tools/data/<goos>_<goarch>/` for embedding. |
| `.github/workflows/` | `ci.yml` (test/build matrix), `tools.yml` (native tool builds per platform), `release.yml` (assembles per-platform binaries from both, then publishes an actual GitHub Release with every one attached, via `softprops/action-gh-release`, on an actual tag push). |
| `README.md` | User documentation. |
| `vendor/`, `build/` | Build output; git-ignored. |

Key `internal/` packages:

| Package | Purpose |
|---|---|
| `actions` | Dry-run/apply framework (`Runner.Act`), `retire`, cross-platform case-only rename, title-cleaning (`CleanName`). |
| `library`, `systemdb`, `config` | Library root resolution, the system/class/BIOS tables (plus `SkyscraperPlatform`, a system→Skyscraper-platform-name map built from Skyscraper's real `--help` output, not guessed - some niche/regional systems fall back to the closest real platform, noted inline), the TOML config file parser (`[sync]`/`[scrape]` tables; `github.com/BurntSushi/toml`). |
| `sync` | rsync/SSH mirroring (`cmd/romctl/sync.go`'s `RunE` is a thin wrapper). Device path auto-discovery: unless `[sync] path` overrides it, `sync` reads RetroDECK's own `retrodeck.json` (`paths.rd_home_path`) over SSH, so it's correct whether the install is on internal storage or an SD card with no config needed. `[sync] user` defaults to `deck`. Opens one multiplexed SSH connection (`sshMux`, `ControlMaster`/`ControlPath`) up front, reused by the `retrodeck.json` lookup and every target's rsync, so a password-auth user is prompted once per run instead of once per connection; `sshMux`'s zero value is inactive, so a failure to establish it is a silent, harmless fallback to one prompt per connection. |
| `formats/chd`, `formats/iso9660`, `formats/xiso`, `formats/n64`, `formats/snes`, `formats/cso` | Pure-Go byte-level format detection, plus the ISO9660 writer (replaces `makeps3iso`) and CSO decompressor (replaces `maxcso`). `formats/chd` also exposes `LogicalSize` (the v5 header's uncompressed byte count), used to size a scratch-space request before `--verify-chd` extraction. |
| `formats/cuesheet` | Parses the one specific cue-sheet shape `chdman extractcd` generates (single `FILE` line, `TRACK`/`INDEX 01` in MM:SS:FF) into per-track byte ranges - not a general-purpose cue parser, just enough for `--verify-chd`'s own extraction output. A pregap (`INDEX 00`) folds into the *preceding* track's range, matching Redump's own split-track convention - see its doc comment. |
| `archive` | zip/7z/rar listing and extraction (replaces the external `7z` dependency). |
| `tools` | Resolves `chdman`/`dolphin-tool`/`extract-xiso`/`skyscraper`: embedded-for-this-platform first, then `PATH`. CHD, RVZ and Xbox XDVDFS are complex, emulator-sensitive codecs not worth reimplementing; Skyscraper is a mature scraper not worth reimplementing either, see Scope boundaries. `AllStatuses` (`status.go`) reports the same embedded/PATH/missing resolution read-only, with no extraction or execution, for `romctl tools`. |
| `datdb` | Embeds and parses No-Intro/Redump DAT snapshots (Logiqx XML: per-game CRC32/MD5/SHA1), staged by `make dats` from packs the maintainer downloads by hand (see "DAT snapshot status" below) — platform-agnostic, unlike `tools`' per-`GOOS`/`GOARCH` embedding, since a DAT file isn't a compiled binary. `systemdb.Dat` maps a romctl system directory name to the provider ("no-intro" or "redump") and that provider's own pack name; a system with no real DAT equivalent (arcade, a few CD/laserdisc niches) is left out, same convention as `SkyscraperPlatform`. `AllStatuses` (`status.go`) reports embedded/missing + each snapshot's own header version/date, read-only, for `romctl tools`'s second table. `commands.Collection` is the thin wrapper that actually hashes a library and matches it against `Load`'s result; see its doc comment for the CHD-unverifiable limitation. `commands.Identify` reuses that same hashing/matching to report every confirmed match - file, game, hash - read-only, same as `Collection`: it never renames anything (a file it can't confirm by hash - no match, a multi-ROM `Game` whose match spans more than one file, a hash shared by two different `Game`s, `ClassArcade`, `.chd` - is simply left off the table rather than guessed at). Actually renaming a confirmed match to its DAT game name is `commands.Rename`'s `verifyDat` path (`--verify-dat`) instead, run through `actions.ApplyScheme` same as `rename`'s own naming-scheme logic; a file `verifyDat` can't confirm still falls through to `rename`'s ordinary scheme-cleanup rather than being left untouched, so every file gets renamed one way or the other - identify answers "what is this file," rename answers "what should it be called." `verifyDat` defaults off, so plain `rename` stays exactly as instant as it's always been; `commands.Identify` is still its own command rather than folded into `Collection`, for the same "an instant command shouldn't grow an occasionally-expensive hashing path" reasoning that already splits `dupes`/`verify` out of `check` - it answers a filesystem-centric question (what is each actual file) `Collection`'s DAT-centric one (what's the status of each known game) doesn't. |
| `executil`, `ttyguard` | Running external processes safely (stdin from the null device for `chdman`/`dolphin-tool`/`extract-xiso`; live-streamed for `rsync`/`ssh`), terminal state save/restore. |
| `discimage` | Unpack-archive-then-build-plain-ISO9660, keyed by system class (a `Spec` table), not hardcoded to one command. PS3 is the only class registered today (RPCS3 doesn't read CHD, unlike every CD/DVD/UMD-class system); a future system needing the same treatment is a new `Spec`, not new logic. `commands.PS3` is a thin wrapper over it. |
| `gamelist` | Keeps ES-DE's `gamelist.xml`/`downloaded_media` pointed at the right files when a rename/convert command moves a ROM that's already been scraped. A silent no-op when `ES-DE/` isn't present locally or the ROM was never scraped. Also used by `commands.Scrape` to correct Skyscraper's own path/media naming for `Title.m3u/` games (see that package's doc comment). |
| `audit`, `commands` | `check`/`bios`/`dupes`/`verify` findings and summary; one file per command. `dupes` (`internal/commands/duplicates.go`) and `verify` (`internal/commands/verify.go`) are their own commands, not `check` flags (an earlier version of `verify` was exactly that, `check --verify`) - both have to read every file's full content (exact-duplicate CRC32 hashing; a full chdman `verify` pass, every compressed hunk's checksum, against every `.chd` - bare files and every disc inside a `Title.m3u/` directory), which can take long enough on a large library that folding either into check's own instant, all-categories-at-once output made a long run indistinguishable from a hang - the same reasoning that already split `bios` out. `verify` additionally resolves `tools.Resolve("chdman")` up front, erroring out before any work starts if it's unavailable. Default mode for both shows a live `cliutil.Progress` done/total counter while hashing/verifying across a `cliutil.Parallel` worker pool, same as any other long batch. `-v`/`--verbose` on a real Fancy terminal (`cliutil.CanAnimate`) instead drives `cliutil.ParallelLive` (`internal/cliutil/spinner.go`): one live spinner row per active worker, finalizing to a permanent colored `✓`/`✗` line the instant that worker moves to its next file - `audit.Report.AddQuiet` records a `verify` failure there instead of `Add`, so its own immediate print doesn't land on the same terminal and corrupt the display's redraw accounting (`dupes` needs no such split: a duplicate is only ever found after every file in its system has already finished hashing, so `Add` never fires while that system's live display is still active). The worker-row count is `GOMAXPROCS` capped by `cliutil.maxLiveSlots` - the terminal's actual current row count (via `term.GetSize`), not raw core count: an earlier version used `GOMAXPROCS` outright, which on a many-core machine is routinely taller than the terminal showing it, and once a redraw's "move cursor up N rows" exceeds what's visible it clamps at row 1 instead of reaching there, so every redraw prints another copy instead of overwriting the last (confirmed against a real run). Capping the row count fixes that while keeping genuine parallelism, rather than dropping to one file at a time. Verbose without a usable terminal (plain/structured output, or redirected to a file) falls back to one plain `[i/total] hash|verify sys/file` line per file, processed one at a time there to keep the printed order sane. |

## Invariants

- **Commands apply by default; `--dry-run`/`-n` previews.** This is the
  opposite of the historical Bash tool's "dry run unless `--apply`" — a
  deliberate choice made when porting to Go, to cut down on retyping
  `--apply`. `plan:` lines print on `--dry-run`, `do:` lines when applying.
- **`clean` is the one exception**: since it's the only step that
  permanently deletes anything, it also asks for confirmation before
  deleting (skipped with `--yes`, or always declined when stdin isn't a
  terminal — see `cliutil.Confirm`).
- **Nothing else is deleted outright.** Replaced originals go to
  `<system>/_to_delete/` via `actions.Retire`; only `clean` removes them. A
  mutating run that retired anything prints a one-line pending-cleanup note
  afterward (`main.go`, via `actions.RetiredCount`).
- **Renames update scraped metadata too.** Every command that renames or
  converts a ROM calls `gamelist.Rename` right after the on-disk change
  succeeds, so `ES-DE/gamelists/<sys>/gamelist.xml` and
  `ES-DE/downloaded_media/<sys>/` (if pulled locally) stay pointed at the
  right file. `internal/discimage` (PS3) is the one exception: an archive
  is never itself a scraped entry in practice, so there's nothing to hook.
- **Atomic outputs.** Write to a temporary name (`*.partial.*`, `*.tmp.*`),
  validate, then rename. `check` reports leftovers of those patterns as FAIL.
- **Case-only renames** must go through `actions.RenamePath` (handles
  case-insensitive filesystems: NTFS, exFAT, APFS by default).
- **Check tool availability before acting.** `tools.Resolve` is called
  before the first mutating action of a `chd`/`rvz`/`scrape` run, never
  mid-run, and its error message says exactly what's missing and how to
  get it. `romctl tools` reports the same resolution read-only, up front.
  `verify`, despite being read-only itself, follows the same "resolve
  before doing any work" rule since it still shells out to chdman.
- **External tool invocations go through `executil`.** `chdman`,
  `dolphin-tool` and `extract-xiso` get stdin from the null device via
  `executil.Run` (buffered output, shown only on failure); `rsync`/`ssh`
  and `skyscraper` (long-running, network-bound - progress matters) get the
  real stdin/stdout/stderr via `executil.RunInherit` (live progress, and an
  interactive password/passphrase prompt still works). Both wrap the call in
  `ttyguard.Save`/`Restore`.
- **Config is parsed, never executed.** `config.Load` decodes TOML
  directly; there is no shell involved, so this was already true, but
  stays true deliberately.
- **Format detection by content, not extension** where a magic number
  exists (CHD header and metadata tags, XISO, N64 byte order, ISO9660 PVD,
  SNES copier header) — see the `formats/` packages.
- **Embedded tools take precedence over `PATH`.** `tools.Resolve` checks
  the platform-specific `go:embed` data first (extracted to a per-version
  cache directory), then falls back to a `PATH` lookup.

## Go style

- `gofmt`-clean and `go vet`-clean (`gofmt -l .`, `make vet`); CI runs `make vet`.
- Standard library first; the only third-party dependencies are
  `spf13/cobra` (the `cmd/romctl/` command tree - built directly on
  `spf13/pflag`, so every flag stays POSIX/GNU-style; also where shell
  completion, including dynamic system-name completion, comes from),
  `golang.org/x/term` (cross-platform terminal state), `BurntSushi/toml`
  (config file parsing and `--format=toml` output - the standard library
  has no TOML support), `gopkg.in/yaml.v3` (`--format=yaml` output - same
  gap for YAML), `jedib0t/go-pretty` (`internal/cliutil`'s rich
  unicode-box-drawing tables - `RenderTable` for a bespoke one like
  audit.Report's Checks/Fixes (per-cell colors, glyphs, aggregation a
  plain struct dump can't express), `RenderList` for a plain "slice of
  `col`-tagged records" one (`list`, `systems`, `tools`, `bios -v`); the
  standard library has no table renderer, and go-pretty measures column
  width correctly around the raw ANSI color codes `cliutil.ColorsFor`
  already produces everywhere else, so cell text can stay colored without
  a second color abstraction), and the pure-Go archive/compression
  libraries in `internal/archive` and `internal/formats/cso`
  (`bodgit/sevenzip`, `nwaples/rardecode`, `pierrec/lz4`).
- One command per file under `internal/commands/`; each takes a
  `*library.Library` and, for mutating commands, an `*actions.Runner`.
- A command that's just "render a slice of records" (`list`, `systems`,
  `tools`, `bios -v`) builds that slice as a struct with a `col:"Header"`
  tag on every field that belongs in a table/CSV column (a field with no
  `col` tag, e.g. a free-length list, is left out of both) and renders it
  with one call to `cliutil.RenderList` - never a hand-rolled `Printf`
  table or a bespoke CSV header/rows pair. Every field, `col`-tagged or
  not, keeps a real json/yaml/toml tag and always encodes there - `col`
  only ever adds or withholds table/CSV visibility on top of that, never
  `json:"-"`, so the same text a table/CSV cell shows is readable straight
  out of `--format=json` too. A field too technical for the table but
  worth keeping in structured output in full (e.g. `bios -v`'s exact MD5/
  size detail) gets its own short, `col`-tagged counterpart instead of
  being hidden from one form or the other - see commands.biosInventoryRow's
  Name/Issue (table+json) alongside Label/Message (json only). A command
  whose rows need per-cell logic a plain struct dump can't express
  (colors, glyphs, aggregation - see `audit.Report`'s Checks/Fixes tables)
  builds `[]table.Row` itself and calls `cliutil.RenderTable` directly
  instead.
- Default to no comments; add one only when the *why* isn't obvious from
  the code (a format-spec quirk, a deliberate behavior change from the
  Bash original, a workaround for a specific tool's behavior).
- Don't reintroduce the external tools `internal/archive`,
  `internal/formats/cso` and `internal/formats/iso9660` replaced (`7z`,
  `maxcso`, `makeps3iso`) — that was the point of the rewrite.

## Documentation style

- Generic and environment-agnostic. No real hostnames, usernames, IPs or paths.
- Placeholders: `example.com`, `host1.example.com`, RFC 5737 addresses
  (`192.0.2.0/24`), `/path/to/...`.

## Testing

- `make vet test` before every commit.
- Exercise commands against a scratch library (`-r /path/to/scratch`) built
  from small synthetic files; never against a real library while developing.
- The pure byte-parsing/format packages (`chd`, `iso9660`, `xiso`, `n64`,
  `cso`) have unit tests with small synthetic fixtures — add to them rather
  than only testing through the CLI when touching format logic.

## Native tool build status

| Tool | Platform | Build verified |
|---|---|---|
| `chdman` (MAME `TOOLS=1 EMULATOR=0`) | linux/amd64, native | yes |
| `chdman` | windows/amd64, cross-built via MinGW-w64 (`make tools-windows-amd64`) | yes |
| `chdman` | linux/arm64, darwin/amd64, darwin/arm64 | **no** — written the same way as the verified builds, untested on real hardware |
| `extract-xiso` (`-DNO_FTP`, see `patches/extract-xiso-no-ftp.patch`) | linux/amd64, native | yes |
| `extract-xiso` | windows/amd64, cross-built via MinGW-w64 | yes — needed its own fixes: the patch replaces upstream's broken `NO_FTP` stub macros and, on MinGW only, prefers MinGW's real `dirent.h` over the bundled MSVC-only `win32/dirent.c` shim, which misbehaves under MinGW's CRT. The libftp dependency (FTP upload mode, which romctl never uses) is dropped entirely rather than cross-built. |
| `extract-xiso` | linux/arm64, darwin/amd64, darwin/arm64 | **no** — written the same way as the verified builds, untested on real hardware |
| `dolphin-tool` (CMake target `dolphin-tool`) | any platform | **no** — submodules not needed for the CLI tool are skipped via `DOLPHIN_SKIP`; imgui, implot, tinygltf and SFML are unconditional in Dolphin's CMake and must not be skipped. Dolphin's own Windows builds use MSVC+vcpkg, not MinGW, so a MinGW cross-build is a bigger lift than chdman's and hasn't been attempted; it needs its own CI job on a real Windows (or native macOS) runner. |
| `skyscraper` (qmake, `QT += core network xml`) | linux/amd64, native | yes — confirmed end-to-end against real ScreenScraper matches (regular single-file and `Title.m3u/` multi-disc games both verified, not just synthetic/unmatched data): resolves via the embedded-tool cache, runs headlessly despite linking QtGui (no display needed), writes correctly-structured relative-path `gamelist.xml` entries and correctly-named media at the real `ES-DE/` paths, including the post-scrape `gamelist.Rename` correction for multi-disc games. |
| `skyscraper` | windows/amd64, any other platform | **no** — upstream only ships an unofficial prebuilt Windows binary with no documented cross-build path; untested elsewhere, same gap `dolphin-tool` already carries. |

Pins live at the top of the Makefile (`*_REF`). Bumping one requires
`make distclean tools`. A platform with no verified build still gets a
working `romctl` for that platform — it just needs `chdman`/`dolphin-tool`/
`extract-xiso`/`skyscraper` on `PATH` instead of embedded (see
`internal/tools`).

## DAT snapshot status

Unlike the native tools above, DAT files aren't fetched or built by any
`make` target that touches the network — see `internal/datdb`'s doc
comment and this repo's earlier discussion for why: No-Intro needs a
Dat-o-Matic session to bulk-download, and Redump's own hosting has shown
signs of instability (a reported inactive-admin/infrastructure issue, with
talk of migrating off redump.org). Writing scraper/login code against
either is a maintenance liability that breaks silently on the next layout
change, for a correctness feature, not a build dependency.

Instead: the maintainer downloads whatever each provider actually offers
and drops it, unzipped and unmodified, under `thirdparty/dats/no-intro/`
and/or `thirdparty/dats/redump/`. The two providers don't offer the same
shape of download: No-Intro's Dat-o-Matic "Daily" aggregate is a single
zip with every system's DAT inside, each carrying its own dynamic
`"<pack name> (<timestamp>).dat"` suffix (confirmed against a real
2026-11-08 "No-Intro Love Pack (DAT)"); Redump has no bulk pack at all —
each system is its own download from `http://redump.org/datfile/<slug>/`
(the slug list is redump.org's own downloads page), unzipping to
`"<pack name> - Datfile (<id>) (<date> <time>).dat"`, a differently-shaped
suffix confirmed against real per-system downloads for every redump
`systemdb.Dat` entry. No picking out or renaming individual files first
either way: the same No-Intro zip bundles hundreds of systems romctl
doesn't track at all, so `make dats` (`go run ./cmd/stage-dats`, thin
wrapper over `internal/datdb.Stage`) is what matches each `systemdb.Dat`
entry by pack name via glob (ignoring either provider's own timestamp
shape - see `datTimestampSuffix`/`redumpSuffix`), cross-checks the
matched file's own `<header><name>` before trusting it, and stages only
those into `internal/datdb/data/<provider>/` with the suffix stripped —
so `datdb.Load`'s own lookup can stay a plain exact match.

No DAT snapshot is committed as of this writing —
`internal/datdb/data/*/MANIFEST` are placeholders, same convention as
`internal/tools/data/*/MANIFEST` for a platform with no native tool build
yet (and the real `.dat` files are gitignored for the same reason the
native tool binaries are — see `.gitignore`). `romctl tools`'s second
table reports, for a given build, which systems actually got a snapshot
embedded and that snapshot's own header version/date — there's no
separate version tracking, since staleness is a property of when `make
dats` was last run, not something to duplicate here.

`systemdb.Dat`'s no-intro entries were cross-checked against that real
pack listing; a few systems publish more than one DAT variant for
different dump container formats (N64 BigEndian/ByteSwapped, NES
Headered/Headerless, Atari Lynx/Jaguar's several formats, etc.) — picked
to match romctl's own target format where one exists, otherwise a
best-effort guess noted inline in `dat.go`. The redump entries have now
also been cross-checked against real per-system downloads, which caught
three wrong pack names - `segacd`/`megacd`/`megacdjp` ("Sega - Mega-CD -
Sega CD" → "Sega - Mega CD & Sega CD"), `neogeocd`/`neogeocdjp` ("SNK -
NeoGeo CD" → "SNK - Neo Geo CD"), and `atarijaguarcd` ("Atari - Jaguar CD
Interactive Multiplayer" → "Atari - Jaguar CD Interactive Multimedia
System") - now corrected in `dat.go`.

Two deliberate, lasting limitations, not TODOs:

- **CHD-converted disc ROMs report as "unverifiable" by default.** No
  CHD metadata stores a per-track hash that lines up with a Redump DAT
  entry (`chdman verify` only checks a CHD against itself). `--verify-chd`
  (on both `collection` and `identify`) opts into a real verdict instead:
  it extracts the `.chd` via `chdman extractcd`/`extractdvd`
  (`internal/commands/chd_verify.go`'s `hashDiscCHD`, into scratch space -
  `chdScratchDir`'s tmpfs when there's room) and hashes the result -
  per-track byte ranges for a CD, parsed from chdman's own generated cue
  sheet (`internal/formats/cuesheet`), the whole file for a DVD/UMD. A
  Redump DAT's own `.cue`/`.ccd`/`.sub`/`.toc` sidecar entries are ignored
  for this path (`stripSidecarROMs`): chdman's regenerated cue sheet can
  never byte-match Redump's distributed one, even with perfectly correct
  track data, so a Game counts as matched once every actual binary track
  is found. This is opt-in, not the default, because it costs about what a
  full `verify` pass already costs (full decompression) and is a new
  runtime dependency on `chdman` neither command otherwise has. Cartridge
  systems (No-Intro) are unaffected either way — they're never
  CHD-converted — so they always get a real have/missing verdict.
- **No parent/clone (1G1R) grouping.** Every DAT entry — including every
  region/revision/translation variant — is its own row in `romctl
  collection`'s output.

## Scope boundaries

- Format conversion, naming, structure validation, BIOS presence/hash checks
  and sync are in scope. So is comparing a library's games against a known-
  games database by hash (`romctl collection`, backed by embedded No-Intro/
  Redump DAT snapshots — see `internal/datdb`); this used to be out of
  scope over concern about the ongoing maintenance of fetching/scraping DAT
  sources at runtime, not a principled objection to the feature — embedding
  a periodically-refreshed snapshot (see "DAT snapshot status" below)
  sidesteps that concern, at the cost of the snapshot going stale between
  romctl releases.
- DRM decryption and sourcing of ROMs or firmware are out of scope.
- Scraping (querying ScreenScraper and writing gamelist.xml/media) is in
  scope, but romctl never talks to a scraper API itself: writing a
  ScreenScraper client was evaluated and declined, since `devid`/
  `devpassword` are issued per-*application* (manual forum approval) and a
  misbehaving client can get that `devid` blacklisted for every user of the
  tool at once. `romctl scrape` instead embeds
  [Skyscraper](https://github.com/muldjord/skyscraper) the same way
  chdman/dolphin-tool/extract-xiso are embedded — its own ScreenScraper
  `devid`/`devpassword` are baked into its source, so only the user's
  personal account (`[scrape] user`) is ever needed. See `internal/commands`'s
  `Scrape` and the "Scraping" section in `README.md`.
