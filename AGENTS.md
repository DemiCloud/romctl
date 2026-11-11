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
| `datdb` | Embeds and parses No-Intro/Redump/FBNeo DAT snapshots (Logiqx XML: per-game CRC32/MD5/SHA1), staged by `make dats` from packs the maintainer downloads by hand (see "DAT snapshot status" below) — platform-agnostic, unlike `tools`' per-`GOOS`/`GOARCH` embedding, since a DAT file isn't a compiled binary. `systemdb.Dat` maps a romctl system directory name to the provider ("no-intro", "redump", or "fbneo") and that provider's own pack name; a system with no real DAT equivalent (every `ClassArcade` system except `fbneo` — MAME proper, Naomi, CPS, etc. — plus a few CD/laserdisc niches) is left out, same convention as `SkyscraperPlatform`. `fbneo` is the one arcade exception: FinalBurn Neo's own repo ships a genuine Logiqx DAT generated from its driver source (version-matched to itself, the same idea as `mame -verifyroms` against the installed core's own compiled-in tables — which romctl doesn't wrap, but FBNeo's DAT achieves the same end a different way). `Game` gained an optional `Description` field (`<description>`, FBNeo-shaped DATs only — No-Intro/Redump never populate it) that `Match` prefers over the terse `Name` attribute when present, so arcade rows show `'88 Games` instead of `88games`. `commands.Collection` still lists a left-out system's files as `no dat` rows (`listOnlyRows`) instead of returning nothing with no explanation, with a stderr note (`printNoDatSystems`) distinct from `printDatLoadIssues`' "mapped but not staged" case - neither counts as a run failure. `AllStatuses` (`status.go`) reports embedded/missing + each snapshot's own header version/date, read-only, for `romctl tools`'s second table. `commands.Collection` is the thin wrapper that actually hashes a library and matches it against `Load`'s result, reporting one row per known game (`have`/`missing`/`unverifiable`, plus the matched filename(s) and their CRC32(s) for a `have` row — `-` for an arcade row, since one outer zip maps to dozens of different per-chip CRCs, not one meaningful value); see its doc comment for the CHD-unverifiable limitation and for how arcade's pervasive parent/clone ROM sharing (most FBNeo games declare a `romof`/`cloneof` parent whose zip holds some of their ROMs) resolves for free through the same system-wide hash pool every other system already builds (`hashArcadeEntries` hashes every entry inside a multi-ROM zip into it) — no `romof`/`cloneof`/`merge` parsing needed at all, as long as the parent's zip is also present in the directory being scanned. `commands.Identify` answers the opposite question: given one or more arbitrary file paths (not a system - `collection`/`check`/etc.'s own `[SYSTEM...|all]` argument shape doesn't apply here at all), hash each one and check it against *every* `datdb.DistinctSources()` DAT at once, not just one system's, since the point is figuring out what an unsorted file actually is without already knowing. A `.chd` is extracted automatically - `formats/chd.Classify` reads the file's own header to pick the right `chdman` command (`commands.hashDiscCHDByClassify`), so no `--verify-chd` flag or system hint is needed; this command only ever handles a handful of files at a time; `--dat-dir` still applies. Read-only like `Collection`: it never renames or otherwise touches the checked file. Actually renaming a confirmed match within a library to its DAT game name is `commands.Rename`'s `verifyDat` path (`--verify-dat`) instead, run through `actions.ApplyScheme` same as `rename`'s own naming-scheme logic; a file `verifyDat` can't confirm still falls through to `rename`'s ordinary scheme-cleanup rather than being left untouched, so every file gets renamed one way or the other - identify answers "what is this file," rename answers "what should it be called." `verifyDat` defaults off, so plain `rename` stays exactly as instant as it's always been. |
| `executil`, `ttyguard` | Running external processes safely (stdin from the null device for `chdman`/`dolphin-tool`/`extract-xiso`; live-streamed for `rsync`/`ssh`), terminal state save/restore. |
| `discimage` | Unpack-archive-then-build-plain-ISO9660, keyed by system class (a `Spec` table), not hardcoded to one command. PS3 is the only class registered today (RPCS3 doesn't read CHD, unlike every CD/DVD/UMD-class system); a future system needing the same treatment is a new `Spec`, not new logic. `commands.PS3` is a thin wrapper over it. |
| `gamelist` | Keeps ES-DE's `gamelist.xml`/`downloaded_media` pointed at the right files when a rename/convert command moves a ROM that's already been scraped. A silent no-op when `ES-DE/` isn't present locally or the ROM was never scraped. Also used by `commands.Scrape` to correct Skyscraper's own path/media naming for `Title.m3u/` games (see that package's doc comment). `StaleEntries` is the one read-only, non-reactive function in the package: it finds entries whose ROM is simply gone from disk (not moved/renamed by romctl, which `Rename`/`Remove` already handle the instant it happens) - `commands.Prune` is its only caller. |
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
  right file. `internal/discimage`'s `UnpackArchives` (archive ->
  `Title.ps3/` folder) is the one step that doesn't: an archive is never
  itself a scraped entry in practice, so there's nothing to hook. Its own
  `BuildISOs` (`Title.ps3/` folder -> `Title.iso`) is not an exception -
  the folder is a real ES-DE game entry in its own right (the same "one
  directory is one game" shape as `Title.m3u/`), so it calls
  `gamelist.Rename` same as any other conversion. `relocate` (moving a
  misfiled cartridge ROM into a *different* system's directory) calls
  `gamelist.Remove` instead of `Rename`: there's nothing legitimate to
  carry over into the destination system's own gamelist (any scrape made
  under the wrong platform was for the wrong game context to begin with),
  but a stale entry left behind in the source system's gamelist - pointing
  at a path that no longer exists there - is a real case (a cartridge
  dropped into the wrong system's directory and left unnoticed until its
  gamelist has a dead entry for it), not a theoretical one. All of this
  only covers a ROM romctl itself moved/retired - a file deleted, merged,
  or deduped by some other means leaves its gamelist entry behind with no
  hook to catch it; `prune` (`gamelist.StaleEntries` + `gamelist.Remove`)
  is the separate, explicit sweep for that case - confirmed necessary
  against a real Skyscraper run: its own gamelist generation reloads and
  rewrites every field of an existing `gamelist.xml`, but still leaves an
  entry for a now-missing file in place rather than dropping it, since it
  always merges with whatever's already there instead of trusting only
  what it can see on disk.
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
- **`check`'s only real cost is gated behind an anomaly it already found
  for free - but when that anomaly needs extraction to confirm, `check`
  now actually does it.** `checkStructureFile`'s "incomplete disc set"
  finding is the one place `check` touches a DAT (`datdb.Load`) or
  hashes a file at all - and only for a "(Disc N)"-tagged file whose
  cheap, directory-listing-only sibling check (`buildDiscGroups`, and
  its `Title.m3u/`-directory counterpart `m3uLoneDiscGroup` -
  `checkM3uDiscSet`) already found has no other disc number present.
  Everything else about `check` stays exactly as cheap as it's always
  been: this only ever runs on the rare file/directory that's actually
  ambiguous, never a whole system's worth. For a raw file that's still
  cheap (`hashROM`); for an already-converted `.chd`/`.rvz`,
  `discFileHashes` now actually extracts it (`hashDiscCHD`/
  `hashDiscRVZ`, the exact same functions `collection --verify-chd`
  uses) via a lazily-resolved `discToolResolver` - chdman/dolphin-tool
  are only ever resolved, and only ever invoked, the first time an
  ambiguous disc of that format is actually found, so a library with
  none never needs either tool at all. This is a deliberate exception to
  the "resolve a tool before any work starts" invariant above: `check`
  must stay usable with neither tool installed on the common path,
  unlike `chd`/`rvz`/`verify`, which always need their tool the moment
  they're asked to run on a relevant system. (`.cso` has no equivalent
  "just extract this one" path today and stays unconfirmed.) Confirmed
  against a real library that this really can get slow per disc (a real
  dolphin-tool extraction of a ~1GB RVZ took several seconds over a slow
  mount) - every ambiguous disc found across every system's cheap scan
  is collected into a `discJobCollector` first and only *then* run
  through one shared `cliutil.Parallel` worker pool
  (`discCompletenessJob`), rather than resolved inline as each is found:
  without this, two ambiguous discs in the very same system would
  confirm one after another despite each being a real extraction, not a
  filename check - confirmed against that same real library, where
  exactly that case went from ~31s to ~13s once parallelized.
  `romctl discs [SYSTEM...|all]` is the dedicated, focused report for
  exactly these findings (`commands.Discs`, reusing
  `buildDiscGroups`/`m3uLoneDiscGroup`/`checkIncompleteDiscSet` directly,
  with its own `discsJob`/`cliutil.Parallel` pass built the same way, not
  duplicating the scan) - `check`'s own Fixes table points its
  "Incomplete disc set" bucket there (`audit.labelDetailCommands`)
  instead of the generic, unscoped `check -v`. `m3u` itself won't create
  a new one-disc `Title.m3u/` directory (see its own entry below), which
  is what made this whole check possible to extend into one in the first
  place.
- **`m3u` never groups a lone disc with no sibling disc number.** A
  "(Disc N)"-tagged file with nothing else sharing its title is just as
  likely to be a single-disc game whose name happens to carry a tag as a
  genuinely incomplete set, and wrapping it alone in a `Title.m3u/`
  directory either way would be pure clutter. Confirmed as a real, not
  theoretical, bug: an earlier `m3u` run against a real library wrapped
  a single `.rvz` with no sibling into its own one-file `Title.m3u/`
  directory - at the time, that made it genuinely invisible to `check`/
  `discs` (neither looked inside an existing `.m3u/` directory yet), so
  the only way to ever find out it was actually missing its second disc
  was `collection --verify-chd --missing`. `check`/`discs` now cover this
  case directly (see above), but `m3u` still refuses to create a new one
  like it going forward regardless - there was never a good reason to.
- **A loose `.bin`/`.img` with no matching `.cue` sheet is its own
  containers finding, not a silent pass.** `chdman createcd` always
  needs a cue sheet to know a disc's track layout - a bare `.bin` alone
  is permanently unconvertible by `chd`, not just not-yet-converted, and
  check's own `checkContainerFile` used to return out of that case with
  no finding at all (same branch as the ordinary, correctly-paired
  `.bin`/`.img` chd already handles via its own `.cue`). Confirmed
  against a real library: 8 loose `.bin` files with zero accompanying
  `.cue` at all, most with no `(Disc N)` tag to make them stand out any
  other way. `loadCueReferences` reads every `.cue` already found in the
  system's directory listing (cheap - tiny text files, not an
  extraction) and collects the filename(s) each one's own `FILE` line(s)
  reference; a `.bin`/`.img` is only flagged when nothing references it.
  Matching by the cue's own `FILE` directive, not by filename stem, is
  deliberate: chdman resolves a cue's referenced file by the name
  written inside it, not by the cue and bin sharing a basename, and a
  real multi-track dump routinely names its bin files
  `"Title (Track N).bin"` while the cue itself is just `"Title.cue"` -
  stem-matching alone would falsely flag that legitimate pairing.
- **`cull` is the explicit "what's actually dead vs. what we just can't
  check" sweep, meant to run after `fix` (and `scrape`, if used) have
  already had their chance.** It keeps two things carefully apart,
  never conflating "confirmed broken" with "romctl doesn't understand
  this." `commands.cullMessages` is the narrow, curated set of check
  findings (structure/containers, read back via the new
  `audit.Report.Findings()` rather than a second implementation of the
  same judgment calls) that mean a file is positively confirmed dead - a
  corrupt CHD/XISO, a truncated PS3 ISO, a PS3 dump with no EBOOT.BIN, a
  leftover extraction/incomplete-transfer artifact, a loose `.bin` with
  no `.cue` anywhere. Those get `actions.Retire`d, same `_to_delete/`
  convention every other mutating command uses - never deleted outright,
  `clean` still owns that. Deliberately excluded even though check also
  flags them: an old-version CHD (works fine, just outdated), a
  DVD-type CHD in a CD-class system (misfiled, not necessarily broken -
  already its own plain `check` finding), a PS3 installed/PSN-style
  folder (may run fine as-is), and an incomplete disc set (the present
  disc is itself confirmed good - see `discs`). Separately,
  `discClassKnownExts` lists, per disc-based class (cd/dvd/umd/dolphin/
  xbox/ps3 - cartridge-shaped classes have no equivalent closed
  extension set, so they're not covered at all), every extension
  `checkContainerFile` already recognizes; anything else found there -
  confirmed against a real library: `.PBP` (PS1-on-PSP packaging) under
  `psx/`, `.cdi` (DiscJuggler) under `dreamcast/` - is only ever
  *reported*, never retired: romctl has no conversion/recognition path
  for either, but both routinely play fine as-is in the right emulator
  core without ever being converted, so there's no evidence they're
  actually broken. A disc already grouped inside a `Title.m3u/`
  directory is reported the same way even when it matches a
  cull-worthy message, rather than retired: removing just one disc
  would leave the playlist referencing a file that's gone, and this
  doesn't attempt to fix that up.
- **`check`'s Fixes table names whether romctl can actually fix a given
  issue itself.** The `Auto-Fix` column is `yes` only for a row backed by
  a real romctl command (`knownFixes`, e.g. `m3u`/`chd`/`rename`); every
  command-less finding (`fix==""`, e.g. a missing BIOS file or an
  incomplete disc set) is `no`, regardless of what instructional text or
  detail command (`bios -v`/`discs`/`check -v`) its own `Fix` column
  shows. Acquiring a replacement BIOS file or the rest of an incomplete
  disc set is always `no` and always will be - romctl does not source or
  provide ROM/BIOS content; see "Scope boundaries" below.
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

A third provider, `fbneo`, follows a simpler version of the same
convention: FBNeo's own repo
(https://github.com/libretro/FBNeo/tree/master/dats/) ships one single,
statically-named file per platform it emulates (no per-day timestamp
suffix to strip) — `"FinalBurn Neo (ClrMame Pro XML, Arcade only).dat"`
is the only one romctl cares about (the others are FBNeo's own DATs for
NES/SNES/Mega Drive/etc., irrelevant here since those systems already
have their own proper No-Intro DAT under their own romctl system
directory). Since there's nothing dynamic to pattern-match, the
maintainer just renames that one file to
`"FinalBurn Neo - Arcade Games.dat"` (matching its own `<header><name>`
and `systemdb.Dat["fbneo"]`'s `System` value exactly) before dropping it
under `thirdparty/dats/fbneo/` — `findDatFiles`'s existing plain
`system+".dat"` exact-match fallback picks it up with **no stage.go
changes**, confirmed by reading `Stage`/`findDatFiles`/the `Makefile`/
`cmd/stage-dats` in full: none of that machinery hardcodes a provider
list anywhere: it's entirely driven by `systemdb.DatSource.Provider`/
`.System` strings, and the `//go:embed data` directive embeds the whole
`data/` tree recursively, so a new `data/fbneo/` subdirectory needs no
Go source change either.

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

- **CHD-converted disc ROMs, and RVZ-converted GameCube/Wii ROMs, report
  as "unverifiable" by default in `collection`.** Neither format's own
  metadata stores a hash that lines up with a Redump DAT entry (`chdman
  verify` only checks a CHD against itself; a compressed `.rvz`'s raw
  bytes never equal the DAT's raw-ISO hash no matter what). Before RVZ
  got this same unverifiable-by-default treatment, `collection` silently
  hashed an `.rvz`'s raw container bytes anyway and reported every one as
  outright `missing` - a real, confirmed bug (every owned RVZ game in a
  real library showed as not owned at all), fixed alongside adding RVZ's
  own verification path rather than left as a second, narrower
  unverifiable case. `--verify-chd` opts into a real verdict for either
  format: it extracts the `.chd` via `chdman extractcd`/`extractdvd`
  (`internal/commands/chd_verify.go`'s `hashDiscCHD`, into scratch space -
  `chdScratchDir`'s tmpfs when there's room) and hashes the result -
  per-track byte ranges for a CD, parsed from chdman's own generated cue
  sheet (`internal/formats/cuesheet`), the whole file for a DVD/UMD - or
  the `.rvz` via `dolphin-tool convert -f iso`
  (`internal/commands/rvz_verify.go`'s `hashDiscRVZ`, a plain OS temp
  dir, no tmpfs-sizing attempt - there's no `chd.LogicalSize` equivalent
  to size the request from without decompressing first) and hashes the
  resulting whole-file ISO - confirmed byte-exact even from an NKit-
  shrunk source (dolphin-tool's own junk-data regeneration reproduces the
  original disc image regardless, verified against a real file: its
  extracted ISO's CRC32 matched Redump's listed hash exactly). A Redump
  DAT's own `.cue`/`.ccd`/`.sub`/`.toc` sidecar entries are ignored for
  the CHD path (`stripSidecarROMs`): chdman's regenerated cue sheet can
  never byte-match Redump's distributed one, even with perfectly correct
  track data, so a Game counts as matched once every actual binary track
  is found (RVZ has no sidecar-file equivalent to strip). This is opt-in,
  not the default, because it costs about what a full `verify` pass
  already costs (full decompression) and is a new runtime dependency on
  `chdman`/`dolphin-tool` `collection` otherwise doesn't have at all.
  `identify` (the by-path lookup) doesn't have this limitation or the
  flag at all for either format - see its own entry above for why a
  `.chd`/`.rvz` there is always extracted
  (`hashDiscCHDByClassify`/`hashDiscRVZ`), unconditionally, since it's
  never checking a whole library. Cartridge systems (No-Intro) are
  unaffected either way — they're never CHD/RVZ-converted — so they
  always get a real have/missing verdict. Since Redump gives a
  multi-disc game's own discs separate Game entries (one row each, not
  one Game with several ROMs), `--verify-chd` plus `--missing` surfaces
  an incomplete disc set for free this same way - confirmed against a
  real library: a GameCube `Title.m3u/` directory holding only disc 1 of
  a genuine 2-disc Redump release shows disc 1 as `have` and disc 2 as
  `missing`, no separate "incomplete" status needed anywhere.
- **GD-ROM (Dreamcast) CHDs stay unverifiable even when extraction is
  attempted** — `hashDiscCHD` rejects them before ever invoking chdman,
  so this applies to `identify` too, not just `collection --verify-chd`.
  chdman's own `extractcd` auto-enables `--splitbin` for this disc type
  (confirmed against a real chdman 0.289: it errors outright without a
  `%t` track variable in the output bin filename once it does), which
  produces one bin file per track instead of the single combined file
  `cuesheet.Parse`'s byte-range convention assumes. Reproducing Redump's
  own split-track layout from that per-track output isn't just a flag
  away either — real-world reports describe needing a separate
  binmerge-style reconstruction pass to get back to the original track
  boundaries, which this package doesn't implement. Every other
  CD/DVD/UMD-class system (ordinary multi-track CDs, DVDs, UMDs) is
  unaffected.
- **No parent/clone (1G1R) grouping.** Every DAT entry — including every
  region/revision/translation variant — is its own row in `romctl
  collection`'s output.

## Scope boundaries

- Format conversion, naming, structure validation, BIOS presence/hash checks
  and sync are in scope. So is comparing a library's games against a known-
  games database by hash (`romctl collection`, backed by embedded No-Intro/
  Redump/FBNeo DAT snapshots — see `internal/datdb`); this used to be out of
  scope over concern about the ongoing maintenance of fetching/scraping DAT
  sources at runtime, not a principled objection to the feature — embedding
  a periodically-refreshed snapshot (see "DAT snapshot status" below)
  sidesteps that concern, at the cost of the snapshot going stale between
  romctl releases. `fbneo` is covered this way too, sourced from FBNeo's
  own repo rather than No-Intro/Redump; other arcade cores (MAME proper,
  Naomi, CPS, ...) remain out of scope below — not an architecture
  limitation (the parent/clone ROM-sharing problem that made arcade look
  hard turned out to resolve for free through `collection`'s existing
  per-system shared hash pool, no new matching logic needed), just no
  equivalent trustworthy, version-matched DAT source lined up for them yet.
- DRM decryption and sourcing of ROMs or firmware are out of scope. This
  isn't a gap to close later: romctl's official position is that every
  user has legally acquired their own ROM and BIOS files, and the project
  does not engage in or facilitate piracy. A finding that boils down to
  "acquire this file" (a missing/bad BIOS, an incomplete multi-disc set -
  see `discs`/the "incomplete disc set" check) is always reported as
  `Auto-Fix: no` in `check`'s Fixes table for exactly this reason, and
  always will be - romctl tells you what's missing, never where to get
  it.
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
