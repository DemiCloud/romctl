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
| `cmd/romctl/` | `main.go`: flag parsing and command dispatch. |
| `internal/` | Everything else, one package per concern — see below. |
| `Makefile` | `go build`/`test`/`vet` wrappers, plus fetching and building the four native tools (`chdman`, `dolphin-tool`, `extract-xiso`, `skyscraper`) into `internal/tools/data/<goos>_<goarch>/` for embedding. |
| `.github/workflows/` | `ci.yml` (test/build matrix), `tools.yml` (native tool builds per platform), `release.yml` (assembles per-platform binaries from both). |
| `README.md` | User documentation. |
| `vendor/`, `build/` | Build output; git-ignored. |

Key `internal/` packages:

| Package | Purpose |
|---|---|
| `actions` | Dry-run/apply framework (`Runner.Act`), `retire`, cross-platform case-only rename, title-cleaning (`CleanName`). |
| `library`, `systemdb`, `config` | Library root resolution, the system/class/BIOS tables (plus `SkyscraperPlatform`, a system→Skyscraper-platform-name map built from Skyscraper's real `--help` output, not guessed - some niche/regional systems fall back to the closest real platform, noted inline), the TOML config file parser (`[sync]`/`[scrape]` tables; `github.com/BurntSushi/toml`). |
| `sync` | rsync/SSH mirroring (`cmd/romctl/main.go`'s `runSync` is a thin wrapper). Device path auto-discovery: unless `[sync] path` overrides it, `sync` reads RetroDECK's own `retrodeck.json` (`paths.rd_home_path`) over SSH, so it's correct whether the install is on internal storage or an SD card with no config needed. `[sync] user` defaults to `deck`. Opens one multiplexed SSH connection (`sshMux`, `ControlMaster`/`ControlPath`) up front, reused by the `retrodeck.json` lookup and every target's rsync, so a password-auth user is prompted once per run instead of once per connection; `sshMux`'s zero value is inactive, so a failure to establish it is a silent, harmless fallback to one prompt per connection. |
| `formats/chd`, `formats/iso9660`, `formats/xiso`, `formats/n64`, `formats/snes`, `formats/cso` | Pure-Go byte-level format detection, plus the ISO9660 writer (replaces `makeps3iso`) and CSO decompressor (replaces `maxcso`). |
| `archive` | zip/7z/rar listing and extraction (replaces the external `7z` dependency). |
| `tools` | Resolves `chdman`/`dolphin-tool`/`extract-xiso`/`skyscraper`: embedded-for-this-platform first, then `PATH`. CHD, RVZ and Xbox XDVDFS are complex, emulator-sensitive codecs not worth reimplementing; Skyscraper is a mature scraper not worth reimplementing either, see Scope boundaries. |
| `executil`, `ttyguard` | Running external processes safely (stdin from the null device for `chdman`/`dolphin-tool`/`extract-xiso`; live-streamed for `rsync`/`ssh`), terminal state save/restore. |
| `discimage` | Unpack-archive-then-build-plain-ISO9660, keyed by system class (a `Spec` table), not hardcoded to one command. PS3 is the only class registered today (RPCS3 doesn't read CHD, unlike every CD/DVD/UMD-class system); a future system needing the same treatment is a new `Spec`, not new logic. `commands.PS3` is a thin wrapper over it. |
| `gamelist` | Keeps ES-DE's `gamelist.xml`/`downloaded_media` pointed at the right files when a rename/convert command moves a ROM that's already been scraped. A silent no-op when `ES-DE/` isn't present locally or the ROM was never scraped. Also used by `commands.Scrape` to correct Skyscraper's own path/media naming for `Title.m3u/` games (see that package's doc comment). |
| `audit`, `commands` | `check`/`bios` findings and summary; one file per command. `check --dupes` (`internal/commands/duplicates.go`) adds an exact-duplicate-file scan (CRC32 of full file content) as its own opt-in `audit.Category`, off by default since it's the one check that has to read every file's full content. |

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
  before the first mutating action of a `chd`/`dvd`/`rvz`/`scrape` run,
  never mid-run, and its error message says exactly what's missing and how
  to get it.
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
- Standard library first; the only third-party dependencies are `pflag`
  (CLI flags, matching this org's other Go CLIs), `golang.org/x/term`
  (cross-platform terminal state), `BurntSushi/toml` (config file parsing
  and `--format=toml` output - the standard library has no TOML support),
  `gopkg.in/yaml.v3` (`--format=yaml` output - same gap for YAML), and the
  pure-Go archive/compression libraries in `internal/archive` and
  `internal/formats/cso` (`bodgit/sevenzip`, `nwaples/rardecode`,
  `pierrec/lz4`).
- One command per file under `internal/commands/`; each takes a
  `*library.Library` and, for mutating commands, an `*actions.Runner`.
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

## Scope boundaries

- Format conversion, naming, structure validation, BIOS presence/hash checks
  and sync are in scope.
- Dump verification against DATs, DRM decryption and sourcing of ROMs or
  firmware are out of scope.
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
