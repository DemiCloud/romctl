# Third-party licenses

A romctl release binary embeds four native tools, built from upstream
source at the pinned refs in the Makefile's `*_REF` variables (`make
tools`/`make tools-windows-amd64`; see `AGENTS.md`'s "Native tool build
status" table for which platforms that's actually been verified on). Each
one keeps its own upstream license; this file records what those are and
why embedding them is compatible with romctl's own GPL-3.0-or-later
license (see `LICENSE`).

| Tool | Upstream | License | Compatibility |
|---|---|---|---|
| `chdman` | [MAME](https://github.com/mamedev/mame) | GPL-2.0-or-later (the project as a whole; individual files are mostly BSD-3-Clause, but MAME's own README states the whole is GPL-2.0+ because it links GPL-licensed components) | "or-later" GPL-2.0 code may be combined into a GPL-3.0 work. |
| `dolphin-tool` | [Dolphin](https://github.com/dolphin-emu/dolphin) | GPL-2.0-or-later (per-file, via SPDX tags; Dolphin's own `COPYING` states the aggregate repository is GPL-3.0-compatible) | Same as above; upstream states this explicitly. |
| `extract-xiso` | [extract-xiso](https://github.com/XboxDev/extract-xiso) | A modified BSD license (see its `LICENSE.TXT`): permissive, requires keeping the original copyright notice and disclaimer in redistributions | BSD is GPL-compatible; the copyright notice is retained by distributing the tool's own unmodified source (and the patch in `patches/`) alongside the pinned ref. |
| `skyscraper` | [Skyscraper](https://github.com/muldjord/skyscraper) | GPL-3.0 | Exact match with romctl's own license. |

Each upstream project's full license text lives in its own repository at
the ref pinned in the Makefile; romctl doesn't vendor copies of them here.
Building `make tools` fetches that exact source, so the complete
corresponding source for every embedded binary in a given romctl release is
always the tagged upstream commit named in that release's Makefile.
