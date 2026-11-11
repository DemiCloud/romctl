package commands

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
)

// Prune removes gamelist.xml entries (and their downloaded_media files)
// for ROMs that no longer exist under roms/<sys>/ - see
// gamelist.StaleEntries's doc comment for why scrape's own Skyscraper
// invocation never cleans these up itself. The usual case is a ROM
// deleted, merged away, or deduped by some means other than romctl's own
// tracked rename/convert/relocate (which already keep the gamelist
// correct the moment they touch a file - see AGENTS.md's "renames update
// scraped metadata too" invariant); the natural time to run this is
// right before scrape, so a rescrape doesn't sit next to dangling
// entries for games that are no longer actually in the library.
func Prune(lib *library.Library, runner *actions.Runner, systems []string) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("prune", runner.Format)
	var pruned, failed atomic.Int64

	cliutil.Parallel(systems, func(sys string) {
		stale, err := gamelist.StaleEntries(lib.Root, sys)
		if err != nil {
			log.Errf("%s: %v", sys, err)
			failed.Add(1)
			return
		}
		for _, rel := range stale {
			if !runner.Act("remove dangling gamelist entry: %s/%s", sys, rel) {
				continue
			}
			if err := gamelist.Remove(lib.Root, sys, rel); err != nil {
				log.Errf("%s/%s: %v", sys, rel, err)
				failed.Add(1)
				continue
			}
			pruned.Add(1)
		}
	})

	if runner.Format.Structured() {
		runner.SetCount("pruned", pruned.Load())
		runner.SetCount("failed", failed.Load())
		return failed.Load() == 0, nil
	}
	color, reset := cliutil.StatusColor(runner.Format, failed.Load() > 0)
	line := fmt.Sprintf("%sprune: %d removed, %d failed in %s%s\n", color, pruned.Load(), failed.Load(), time.Since(start).Round(time.Second), reset)
	if runner.Quiet {
		runner.Summarize(line)
	} else {
		fmt.Print(line)
	}
	return failed.Load() == 0, nil
}
