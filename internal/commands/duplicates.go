package commands

import (
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/demicloud/romctl/internal/audit"
	"github.com/demicloud/romctl/internal/cliutil"
)

// checkDuplicates finds files in sysDir with identical content (exact
// byte-for-byte matches, via CRC32) and flags all but the first in each
// group as a WARN finding. It's a separate, opt-in pass (see --dupes)
// rather than part of the default check: unlike every other check, it has
// to read every file's full content, which is a meaningfully different
// cost on a library with large CHD/ISO files.
//
// This only catches exact duplicates. A game present under two different
// region dumps (different hashes, same underlying game) isn't detected -
// doing that precisely needs No-Intro/Redump-style DAT clone-group data,
// which is a deliberately separate, bigger scope question.
func checkDuplicates(report *audit.Report, sys, sysDir string) {
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || !hashable(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}

	// Hashing full file contents is the expensive part of --dupes (the
	// whole reason it's opt-in) - do it across a worker pool instead of
	// one file at a time.
	var mu sync.Mutex
	byHash := map[uint32][]string{}
	cliutil.Parallel(names, func(name string) {
		sum, err := crc32File(filepath.Join(sysDir, name))
		if err != nil {
			return
		}
		mu.Lock()
		byHash[sum] = append(byHash[sum], name)
		mu.Unlock()
	})

	hashes := make([]uint32, 0, len(byHash))
	for h := range byHash {
		hashes = append(hashes, h)
	}
	sort.Slice(hashes, func(i, j int) bool { return hashes[i] < hashes[j] })

	for _, h := range hashes {
		names := byHash[h]
		if len(names) < 2 {
			continue
		}
		sort.Strings(names)
		for _, name := range names[1:] {
			report.Add(audit.Warn, audit.CategoryDuplicates, sys, name,
				fmt.Sprintf("identical to %s (CRC32 %08X)", names[0], h), "")
		}
	}
}

// hashable reports whether a filename is worth hashing for duplicate
// detection - the same non-ROM extensions and leftover markers the rest
// of check already ignores.
func hashable(name string) bool {
	if name == "systeminfo.txt" || name == ".directory" {
		return false
	}
	if strings.Contains(name, ".partial.") || strings.Contains(name, ".tmp.") || strings.Contains(name, ".romctl-rename.") {
		return false
	}
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")) {
	case "txt", "ini", "cheats", "ips", "bps", "ups", "sbi", "dat", "directory",
		"sav", "srm", "state", "sh", "part", "tmp":
		return false
	}
	return true
}

func crc32File(path string) (uint32, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	h := crc32.NewIEEE()
	if _, err := io.Copy(h, f); err != nil {
		return 0, err
	}
	return h.Sum32(), nil
}
