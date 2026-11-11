package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
)

// discFileRE matches a loose disc-container file already in its system's
// final, emulator-native format: .chd for CD/DVD/UMD-class systems, .rvz
// for ClassDolphin (GameCube/Wii - they never go through CHD at all, see
// rvz.go). A file still in its pre-conversion format (.bin, .iso, ...)
// isn't grouped yet; check's "belongs in a .m3u directory" warning expects
// chd/rvz to have already run first (see fixSteps' ordering in fix.go).
var discFileRE = regexp.MustCompile(`^(.+) \(Disc (\d+)\)\.(chd|rvz)$`)

// M3U groups loose "Title (Disc N).chd"/"Title (Disc N).rvz" files into a
// Title.m3u/ directory with a playlist, for every system given. A title
// with only one such file - no sibling disc number present - is left
// alone rather than wrapped in a pointless one-file .m3u/ directory: see
// the loop's own comment for why.
func M3U(lib *library.Library, runner *actions.Runner, systems []string) error {
	start := time.Now()
	log := cliutil.NewLogger("m3u", runner.Format)
	var grouped, failed atomic.Int64
	for _, sys := range systems {
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			continue
		}

		type disc struct {
			path string
			num  int
			ext  string
		}
		byTitle := map[string][]disc{}
		for _, e := range entries {
			m := discFileRE.FindStringSubmatch(e.Name())
			if m == nil {
				continue
			}
			num, _ := strconv.Atoi(m[2])
			byTitle[m[1]] = append(byTitle[m[1]], disc{path: filepath.Join(sysDir, e.Name()), num: num, ext: m[3]})
		}

		titles := make([]string, 0, len(byTitle))
		for t := range byTitle {
			titles = append(titles, t)
		}
		sort.Strings(titles)

		for _, title := range titles {
			discs := byTitle[title]
			if len(discs) < 2 {
				// A lone "(Disc N)" file with no sibling disc number is
				// just as likely to be a single-disc game whose name
				// happens to carry a disc tag as it is to be genuinely
				// missing the rest of a set (see check's own
				// "incomplete disc set" finding, which tells the two
				// apart where it can) - grouping it alone into a
				// one-file .m3u/ directory would be pure clutter either
				// way, so it's left loose instead.
				continue
			}

			dir := filepath.Join(sysDir, title+".m3u")
			if exists(dir) {
				log.Errf("exists, skipped: %s/%s.m3u", sys, title)
				continue
			}

			sort.Slice(discs, func(i, j int) bool { return discs[i].num < discs[j].num })

			if !runner.Act("group %s/%s (Disc N).%s -> %s.m3u/", sys, title, discs[0].ext, title) {
				continue
			}
			if err := os.Mkdir(dir, 0o755); err != nil {
				log.Errf("mkdir failed: %s: %v", dir, err)
				failed.Add(1)
				continue
			}

			playlist := filepath.Join(dir, title+".m3u")
			var lines []string
			for _, d := range discs {
				name := fmt.Sprintf("%s (Disc %02d).%s", title, d.num, d.ext)
				if err := os.Rename(d.path, filepath.Join(dir, name)); err != nil {
					log.Errf("move failed: %s: %v", d.path, err)
					continue
				}
				lines = append(lines, name)
			}
			data := ""
			for _, l := range lines {
				data += l + "\n"
			}
			if err := os.WriteFile(playlist, []byte(data), 0o644); err != nil {
				log.Errf("writing playlist failed: %s: %v", playlist, err)
				failed.Add(1)
				continue
			}
			origFirst := filepath.Base(discs[0].path)
			if err := gamelist.Rename(lib.Root, sys, origFirst, title+".m3u"); err != nil {
				log.Errf("gamelist update failed: %s/%s: %v", sys, title, err)
			}
			grouped.Add(1)
		}
	}
	if runner.Format.Structured() {
		runner.SetCount("grouped", grouped.Load())
		runner.SetCount("failed", failed.Load())
		return nil
	}
	color, reset := cliutil.StatusColor(runner.Format, failed.Load() > 0)
	line := fmt.Sprintf("%sm3u: %d grouped, %d failed in %s%s\n", color, grouped.Load(), failed.Load(), time.Since(start).Round(time.Second), reset)
	if runner.Quiet {
		runner.Summarize(line)
	} else {
		fmt.Print(line)
	}
	return nil
}
