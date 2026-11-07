package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync/atomic"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
)

var discFileRE = regexp.MustCompile(`^(.+) \(Disc (\d+)\)\.chd$`)

// M3U groups loose "Title (Disc N).chd" files into a Title.m3u/ directory
// with a playlist, for every system given.
func M3U(lib *library.Library, runner *actions.Runner, systems []string) error {
	log := cliutil.NewLogger("m3u")
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
		}
		byTitle := map[string][]disc{}
		for _, e := range entries {
			m := discFileRE.FindStringSubmatch(e.Name())
			if m == nil {
				continue
			}
			num, _ := strconv.Atoi(m[2])
			byTitle[m[1]] = append(byTitle[m[1]], disc{path: filepath.Join(sysDir, e.Name()), num: num})
		}

		titles := make([]string, 0, len(byTitle))
		for t := range byTitle {
			titles = append(titles, t)
		}
		sort.Strings(titles)

		for _, title := range titles {
			dir := filepath.Join(sysDir, title+".m3u")
			if exists(dir) {
				log.Errf("exists, skipped: %s/%s.m3u", sys, title)
				continue
			}
			if !runner.Act("group %s/%s (Disc N).chd -> %s.m3u/", sys, title, title) {
				continue
			}
			if err := os.Mkdir(dir, 0o755); err != nil {
				log.Errf("mkdir failed: %s: %v", dir, err)
				failed.Add(1)
				continue
			}

			discs := byTitle[title]
			sort.Slice(discs, func(i, j int) bool { return discs[i].num < discs[j].num })

			playlist := filepath.Join(dir, title+".m3u")
			var lines []string
			for _, d := range discs {
				name := fmt.Sprintf("%s (Disc %02d).chd", title, d.num)
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
	fmt.Printf("m3u: %d grouped, %d failed\n", grouped.Load(), failed.Load())
	return nil
}
