# AGENTS.md

Guidance for coding agents working in this repository.

## Project

`romctl` is a single Bash script that audits and normalizes a ROM library laid
out for RetroDECK/ES-DE (`roms/<system>/`, `bios/`), and syncs it to a device
over rsync/SSH. `README.md` is the user-facing reference; keep it in step with
behavior changes.

| Path | Purpose |
|---|---|
| `romctl` | The tool. Single file, no runtime dependencies beyond the tools it shells out to. |
| `Makefile` | Fetches pinned upstream sources into `vendor/` and builds external tools into `bin/`. |
| `README.md` | User documentation. |
| `vendor/`, `bin/` | Build output; git-ignored. |

## Invariants

- **Dry run by default.** Every command that changes files prints `plan:` lines
  and does nothing unless `--apply` is given. With `--apply` it prints `do:`.
- **Nothing is deleted outright.** Replaced originals go to
  `<system>/_to_delete/` via `retire`; only `romctl clean` removes them.
- **Atomic outputs.** Write to a temporary name (`*.partial.*`, `*.tmp.*`),
  validate, then rename. `check` reports leftovers of those patterns as FAIL.
- **Case-only renames** must go through `rename_path` (case-insensitive
  filesystems such as NTFS/exFAT are expected targets).
- **Check tool availability before acting.** Call `need` before the first
  `do:` line, never mid-run.
- **External tools get `</dev/null`.** Some (maxcso, makeps3iso) read stdin or
  alter terminal state; `save_tty` restores the TTY on exit.
- **Config is parsed, never sourced.** `load_config` reads `KEY=value` lines.
- **Format detection by content, not extension** where a magic number exists
  (CHD header and metadata tags, XISO, N64 byte order, ISO9660 PVD, SNES copier
  header).
- Tools built by the Makefile take precedence: `main` prepends the script's own
  `bin/` to `PATH`, resolved with `readlink -f` so symlinked installs work.

## Bash style

- `#!/bin/bash`; must pass `shellcheck` and `shfmt -i 2 -ci` (`make lint`).
- Google Shell Style Guide. No `set -eo pipefail`; handle errors explicitly
  with `err` / `die`.
- Prefer keywords over builtins over external commands. `[[ ]]` for tests,
  `(( ))` for arithmetic, `$()` for substitution.
- Arrays for lists of arguments; quote every expansion.
- Every function has a header comment (description, globals, arguments,
  outputs, returns as applicable). No top-level code between functions.
- Declare `local` separately from command-substitution assignment.
- Heredocs for multi-line output.
- `main` is invoked only under a `BASH_SOURCE` guard that `exit`s inside the
  guard, so editing the script while it runs cannot execute partial content.
- Associative-array keys containing `.` or `-` must be quoted.

## Documentation style

- Generic and environment-agnostic. No real hostnames, usernames, IPs or paths.
- Placeholders: `example.com`, `host1.example.com`, RFC 5737 addresses
  (`192.0.2.0/24`), `/path/to/...`.

## Testing

- `make lint` before every commit.
- Exercise commands against a scratch library (`-r /path/to/scratch`) built
  from small synthetic files; never against a real library while developing.
- Never run `makeps3iso` without arguments; it prompts interactively and hangs.

## Makefile status

| Tool | Build verified |
|---|---|
| `7z` (7-Zip, `Alone2/makefile.gcc` → `7zz`) | yes |
| `maxcso` (links system libuv, lz4, zlib) | yes |
| `makeps3iso` | yes |
| `chdman` (MAME `TOOLS=1 EMULATOR=0`) | **no** — project generation needs SDL2 headers; optional OSD backends disabled via `MAME_FLAGS` |
| `dolphin-tool` (CMake target `dolphin-tool`) | **no** — submodules not needed for the CLI tool are skipped via `DOLPHIN_SKIP`; imgui, implot, tinygltf and SFML are unconditional in Dolphin's CMake and must not be skipped |

Pins live at the top of the Makefile (`*_REF`). Bumping one requires
`make distclean all`.

## Scope boundaries

- Format conversion, naming, structure validation, BIOS presence/hash checks
  and sync are in scope.
- Dump verification against DATs, DRM decryption and sourcing of ROMs or
  firmware are out of scope.
