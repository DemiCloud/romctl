package commands

import (
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

// fileConflict records a file left in place because something already
// occupies its destination - printed as its own section so the user can
// tell at a glance whether it's safe to just delete one copy (identical)
// or needs a manual look (different content, or either side unreadable).
// Shared by every command that relocates files without ever overwriting
// (Saves, Relocate).
type fileConflict struct {
	sys, name        string
	srcDir, dstDir   string
	srcSize, dstSize int64
	srcMod, dstMod   time.Time
	identical        bool
	compareErr       bool
}

// compareFileConflict stats and, size permitting, content-compares src
// and dst (already known to both exist) so printFileConflicts can tell
// the user whether the copy left behind is safe to discard or needs a
// manual look.
func compareFileConflict(sys, name, srcLabel, dstLabel, src, dst string) fileConflict {
	c := fileConflict{sys: sys, name: name, srcDir: srcLabel, dstDir: dstLabel}
	srcInfo, srcErr := os.Stat(src)
	dstInfo, dstErr := os.Stat(dst)
	if srcErr != nil || dstErr != nil {
		c.compareErr = true
		return c
	}
	c.srcSize, c.srcMod = srcInfo.Size(), srcInfo.ModTime()
	c.dstSize, c.dstMod = dstInfo.Size(), dstInfo.ModTime()
	if c.srcSize != c.dstSize {
		return c
	}
	// These files are typically small enough (saves/states, cartridge
	// ROMs) that a CRC32 of each - same check checkDuplicates uses for
	// exact-duplicate detection elsewhere - is cheap here even run per
	// conflict.
	srcSum, err := crc32File(src)
	if err != nil {
		c.compareErr = true
		return c
	}
	dstSum, err := crc32File(dst)
	if err != nil {
		c.compareErr = true
		return c
	}
	c.identical = srcSum == dstSum
	return c
}

// printFileConflicts lists every file left in its original location
// because its destination was already occupied, each with both copies'
// size and modification time so the user can judge which one to keep.
func printFileConflicts(w io.Writer, conflicts []fileConflict) {
	if len(conflicts) == 0 {
		return
	}
	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].sys != conflicts[j].sys {
			return conflicts[i].sys < conflicts[j].sys
		}
		return conflicts[i].name < conflicts[j].name
	})
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Conflicts (destination already exists; left in place untouched):")
	const timeFmt = "2006-01-02 15:04"
	for _, c := range conflicts {
		if c.compareErr {
			fmt.Fprintf(w, "  %-10s %-32s %s vs %s: could not compare\n", c.sys, c.name, c.srcDir, c.dstDir)
			continue
		}
		status := "DIFFERENT - resolve manually"
		if c.identical {
			status = "identical, safe to delete either copy"
		}
		fmt.Fprintf(w, "  %-10s %-32s %s: %s, %s   vs   %s: %s, %s   [%s]\n",
			c.sys, c.name,
			c.srcDir, humanSize(c.srcSize), c.srcMod.Format(timeFmt),
			c.dstDir, humanSize(c.dstSize), c.dstMod.Format(timeFmt),
			status)
	}
}
