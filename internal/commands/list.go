package commands

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// listRecord is one system's shape for every output form when -g isn't
// given - table (Fancy/Plain), csv, json/yaml/toml - via
// cliutil.RenderList.
type listRecord struct {
	System string `json:"system" yaml:"system" toml:"system" col:"System"`
	Name   string `json:"name" yaml:"name" toml:"name" col:"Name"`
	Games  int    `json:"games" yaml:"games" toml:"games" col:"Games"`
}

// listGameRecord is one game's shape in List's output when -g/--games is
// given - one row per game instead of one row per system - again the
// same shape rendering through every format via cliutil.RenderList.
type listGameRecord struct {
	System string `json:"system" yaml:"system" toml:"system" col:"System"`
	Name   string `json:"name" yaml:"name" toml:"name" col:"Name"`
	Title  string `json:"title" yaml:"title" toml:"title" col:"Title"`
}

// populated is one system with at least one game.
type populated struct {
	system string
	games  []string
}

// List renders every given system that has at least one game, its full
// name, and how many games it has; showGames instead renders one row per
// game (system, name, title) rather than one row per system, through
// every format cliutil.RenderList handles.
func List(lib *library.Library, systems []string, showGames bool, format cliutil.Format) error {
	esdeNames, _ := lib.ESDESystems()

	var results []populated
	for _, sys := range systems {
		games, err := gameTitles(filepath.Join(lib.Roms, sys))
		if err != nil || len(games) == 0 {
			continue
		}
		results = append(results, populated{system: sys, games: games})
	}
	sort.Slice(results, func(i, j int) bool {
		return systemdb.DisplayNameWith(results[i].system, esdeNames) < systemdb.DisplayNameWith(results[j].system, esdeNames)
	})

	if showGames {
		var records []listGameRecord
		for _, r := range results {
			names := append([]string(nil), r.games...)
			sort.Strings(names)
			name := systemdb.DisplayNameWith(r.system, esdeNames)
			for _, title := range names {
				records = append(records, listGameRecord{System: r.system, Name: name, Title: title})
			}
		}
		return cliutil.RenderList(os.Stdout, format, records)
	}

	records := make([]listRecord, 0, len(results))
	for _, r := range results {
		records = append(records, listRecord{System: r.system, Name: systemdb.DisplayNameWith(r.system, esdeNames), Games: len(r.games)})
	}
	return cliutil.RenderList(os.Stdout, format, records)
}

// gameTitles lists the title of every game entry directly under sysDir,
// using the same "one entry = one game" rule as library.HasGames (a
// directory with a recognized extension, such as a .m3u playlist or a
// .ps3 disc folder, counts as a single game, not its contents).
func gameTitles(sysDir string) ([]string, error) {
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return nil, err
	}
	var titles []string
	for _, e := range entries {
		name := e.Name()
		if name == "systeminfo.txt" || name == ".directory" || name == "_to_delete" ||
			strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_extract.") ||
			strings.HasSuffix(name, ".txt") || isLeftover(name) {
			continue
		}
		titles = append(titles, gameTitle(name))
	}
	return titles, nil
}

// isLeftover matches the incomplete-conversion leftovers "check" flags as
// FAIL: not a game, just debris from an interrupted run.
func isLeftover(name string) bool {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	return strings.Contains(name, ".partial.") || strings.Contains(name, ".tmp.") ||
		strings.Contains(name, ".romctl-rename.") || ext == "part" || ext == "tmp"
}

func gameTitle(name string) string {
	switch {
	case strings.HasSuffix(name, ".m3u"):
		return strings.TrimSuffix(name, ".m3u")
	case strings.HasSuffix(name, ".ps3"):
		return strings.TrimSuffix(name, ".ps3")
	default:
		return strings.TrimSuffix(name, filepath.Ext(name))
	}
}
