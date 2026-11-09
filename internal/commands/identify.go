package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/datdb"
	"github.com/demicloud/romctl/internal/formats/chd"
	"github.com/demicloud/romctl/internal/systemdb"
	"github.com/demicloud/romctl/internal/tools"
)

// identifyRow is one checked path's outcome - one row per (path, match)
// pair, or one row with System/Game blank when nothing matched at all,
// so a clean miss still shows up rather than silently vanishing from a
// multi-path run. Hash is every hash that went into the match attempt,
// comma-joined in disc-track order for a multi-track .chd - same
// multi-value convention collectionRow's own Hash/Matched columns use.
type identifyRow struct {
	Path   string `json:"path" yaml:"path" toml:"path" col:"Path"`
	Size   int64  `json:"size" yaml:"size" toml:"size" col:"Size"`
	Hash   string `json:"hash" yaml:"hash" toml:"hash" col:"Hash"`
	System string `json:"system" yaml:"system" toml:"system" col:"System"`
	Game   string `json:"game" yaml:"game" toml:"game" col:"Game"`
}

// Identify hashes each given path and checks it against every embedded
// (or --dat-dir-overridden) No-Intro/Redump DAT - not just one system's,
// since the point is figuring out what an arbitrary file actually is
// without already knowing. Read-only: it never touches the checked file
// or anything else (see rename --verify-dat for actually renaming a
// confirmed match within a library).
//
// A .chd is extracted automatically - chd.Classify reads the file's own
// header/metadata to pick the right chdman command (see
// hashDiscCHDByClassify), so no --verify-chd flag or --system hint is
// needed; this command only ever handles a handful of files at a time,
// not a whole library, so the extraction cost that makes --verify-chd
// opt-in everywhere else isn't a concern here. Every track's hash goes
// into the same match attempt, so a multi-track CD game is only
// confirmed once every track is accounted for, same as collection's own
// --verify-chd path.
func Identify(paths []string, datDir string, format cliutil.Format) (bool, error) {
	type loadedSource struct {
		src systemdb.DatSource
		df  *datdb.Datafile
	}
	var dats []loadedSource
	for _, src := range datdb.DistinctSources() {
		if df, err := datdb.LoadSource(src, datDir); err == nil {
			dats = append(dats, loadedSource{src, df})
		}
	}

	var chdmanPath string
	ok := true
	var rows []identifyRow
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "identify: %s: %v\n", path, err)
			ok = false
			continue
		}
		if info.IsDir() {
			fmt.Fprintf(os.Stderr, "identify: %s: is a directory\n", path)
			ok = false
			continue
		}

		var hashes []discTrackHash
		size := info.Size()
		if strings.ToLower(filepath.Ext(path)) == ".chd" {
			if chdmanPath == "" {
				chdmanPath, err = tools.Resolve("chdman")
				if err != nil {
					return false, err
				}
			}
			hashes, err = hashDiscCHDByClassify(chdmanPath, path)
			if err == nil {
				if logical, lerr := chd.LogicalSize(path); lerr == nil {
					size = logical
				}
			}
		} else {
			var crc, md5sum, sha1sum string
			crc, md5sum, sha1sum, err = hashROM(path)
			hashes = []discTrackHash{{crc, md5sum, sha1sum}}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "identify: %s: %v\n", path, err)
			ok = false
			continue
		}

		byCRC, byMD5, bySHA1 := map[string]string{}, map[string]string{}, map[string]string{}
		hashParts := make([]string, len(hashes))
		for i, h := range hashes {
			byCRC[h.crc], byMD5[h.md5sum], bySHA1[h.sha1sum] = path, path, path
			hashParts[i] = strings.ToUpper(h.crc)
		}
		hash := strings.Join(hashParts, ", ")

		matched := false
		for _, d := range dats {
			for _, m := range d.df.Match(byCRC, byMD5, bySHA1) {
				if !m.Have {
					continue
				}
				rows = append(rows, identifyRow{Path: path, Size: size, Hash: hash, System: d.src.System, Game: m.Game})
				matched = true
			}
		}
		if !matched {
			rows = append(rows, identifyRow{Path: path, Size: size, Hash: hash})
		}
	}

	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Path < rows[j].Path })
	if format.Structured() || len(rows) > 0 {
		if err := cliutil.RenderList(os.Stdout, format, rows); err != nil {
			return false, err
		}
	}
	return ok, nil
}
