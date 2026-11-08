package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// listRecord is one system's shape in List's structured output.
type listRecord struct {
	System string   `json:"system" yaml:"system" toml:"system"`
	Name   string   `json:"name" yaml:"name" toml:"name"`
	Games  int      `json:"games" yaml:"games" toml:"games"`
	Titles []string `json:"titles,omitempty" yaml:"titles,omitempty" toml:"titles,omitempty"`
}

// populated is one system with at least one game, shared by List and listCSV.
type populated struct {
	system string
	games  []string
}

// List prints every given system that has at least one game, its full
// name, and how many games it has; showGames additionally lists each
// game's title. Under a structured format, the same data is encoded
// instead - CSV emits one row per game when showGames (system, name,
// title), otherwise one row per system (system, name, game count); titles
// are left out of a system-per-row CSV, same reasoning as every other
// command's CSV output (a free-length list doesn't fit a tabular column).
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

	if format.Structured() {
		if format == cliutil.FormatCSV {
			return listCSV(results, esdeNames, showGames)
		}
		records := make([]listRecord, 0, len(results))
		for _, r := range results {
			names := append([]string(nil), r.games...)
			sort.Strings(names)
			rec := listRecord{System: r.system, Name: systemdb.DisplayNameWith(r.system, esdeNames), Games: len(r.games)}
			if showGames {
				rec.Titles = names
			}
			records = append(records, rec)
		}
		return cliutil.Encode(os.Stdout, format, records)
	}

	for _, r := range results {
		fmt.Printf("%-40s %-18s %4d game(s)\n", systemdb.DisplayNameWith(r.system, esdeNames), "("+r.system+")", len(r.games))
		if !showGames {
			continue
		}
		names := append([]string(nil), r.games...)
		sort.Strings(names)
		for _, name := range names {
			fmt.Printf("    %s\n", name)
		}
	}
	return nil
}

func listCSV(results []populated, esdeNames map[string]string, showGames bool) error {
	if showGames {
		header := []string{"system", "name", "title"}
		var rows [][]string
		for _, r := range results {
			names := append([]string(nil), r.games...)
			sort.Strings(names)
			name := systemdb.DisplayNameWith(r.system, esdeNames)
			for _, title := range names {
				rows = append(rows, []string{r.system, name, title})
			}
		}
		return cliutil.EncodeCSV(os.Stdout, header, rows)
	}
	header := []string{"system", "name", "games"}
	rows := make([][]string, 0, len(results))
	for _, r := range results {
		rows = append(rows, []string{r.system, systemdb.DisplayNameWith(r.system, esdeNames), strconv.Itoa(len(r.games))})
	}
	return cliutil.EncodeCSV(os.Stdout, header, rows)
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
