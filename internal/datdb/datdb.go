// Package datdb embeds No-Intro/Redump DAT snapshots - per-system lists of
// every known game and its ROM(s)' CRC32/MD5/SHA1 - and parses them for
// commands.Collection's have/missing report. DAT files are staged into
// data/<provider>/ by `make dats` (see Makefile) from packs the maintainer
// downloads by hand; see AGENTS.md for why that step isn't automated.
//
// Unlike internal/tools, embedding is platform-agnostic (one data/ tree,
// not one per GOOS/GOARCH): a DAT file is plain data, not a compiled
// binary.
package datdb

import (
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/demicloud/romctl/internal/systemdb"
)

// ROM is one file a Game expects, by hash - a game with more than one ROM
// (a multi-track disc, a multi-chip cart) needs every one present to count
// as Have.
type ROM struct {
	Name string
	Size int64
	CRC  string // lowercase hex, no "0x"
	MD5  string
	SHA1 string
}

// Game is one entry in a Datafile - usually one released version (a
// region, revision or translation is its own Game, not a variant of one -
// see Collection's doc comment on why clone/parent grouping is out of
// scope for now).
//
// Description is optional: FBNeo-shaped arcade DATs put a terse internal
// set code in Name ("88games") and the actual human title in a sibling
// <description> ("'88 Games") - No-Intro/Redump never populate it, since
// their own Name is already the human title. See Match, which prefers
// Description over Name for MatchResult.Game when it's present.
type Game struct {
	Name        string
	Description string
	ROMs        []ROM
}

// Header is a Datafile's own metadata - Version/Date come straight from
// the DAT's own <header>, so there's nothing else to track separately for
// the "DAT snapshot" table in `romctl tools`.
type Header struct {
	Name    string
	Version string
	Date    string
}

// Datafile is one parsed DAT (Logiqx XML schema, the format No-Intro,
// Redump, TOSEC and every mainstream ROM manager share).
type Datafile struct {
	Header Header
	Games  []Game
}

type xmlROM struct {
	Name string `xml:"name,attr"`
	Size int64  `xml:"size,attr"`
	CRC  string `xml:"crc,attr"`
	MD5  string `xml:"md5,attr"`
	SHA1 string `xml:"sha1,attr"`
}

type xmlGame struct {
	Name        string   `xml:"name,attr"`
	Description string   `xml:"description"`
	ROMs        []xmlROM `xml:"rom"`
}

type xmlHeader struct {
	Name    string `xml:"name"`
	Version string `xml:"version"`
	Date    string `xml:"date"`
}

type xmlDatafile struct {
	XMLName xml.Name  `xml:"datafile"`
	Header  xmlHeader `xml:"header"`
	Games   []xmlGame `xml:"game"`
}

// Parse decodes a Logiqx-schema DAT file's raw XML bytes.
func Parse(data []byte) (*Datafile, error) {
	var raw xmlDatafile
	if err := xml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	df := &Datafile{Header: Header{
		Name:    raw.Header.Name,
		Version: raw.Header.Version,
		Date:    raw.Header.Date,
	}}
	for _, g := range raw.Games {
		game := Game{Name: g.Name, Description: g.Description}
		for _, r := range g.ROMs {
			game.ROMs = append(game.ROMs, ROM{
				Name: r.Name,
				Size: r.Size,
				CRC:  strings.ToLower(r.CRC),
				MD5:  strings.ToLower(r.MD5),
				SHA1: strings.ToLower(r.SHA1),
			})
		}
		df.Games = append(df.Games, game)
	}
	return df, nil
}

// dataPath returns the embedded or override-directory path for src, under
// "data/<provider>/<name>.dat" - the same layout `make dats` stages into.
func dataPath(src systemdb.DatSource) string {
	return src.Provider + "/" + src.System + ".dat"
}

// Load reads and parses the DAT for a romctl system directory name. ok is
// false (and err nil) when the system has no DatSource entry at all -
// distinct from a real error, which means a mapping exists but the file
// itself couldn't be read (not embedded yet, or an overrideDir missing
// it) or didn't parse. overrideDir, if non-empty, is read instead of the
// embedded set (romctl collection's --dat-dir).
func Load(system, overrideDir string) (df *Datafile, ok bool, err error) {
	src, has := systemdb.Dat[system]
	if !has {
		return nil, false, nil
	}
	df, err = LoadSource(src, overrideDir)
	if err != nil {
		return nil, true, fmt.Errorf("%s: %w", system, err)
	}
	return df, true, nil
}

// LoadSource reads and parses src directly, skipping Load's romctl
// system-name lookup - for a caller that already has a DatSource in
// hand, or wants every distinct one regardless of which romctl system
// key(s) happen to share it (see DistinctSources; romctl identify's
// by-path lookup checks each actual DAT exactly once rather than once
// per alias - segacd/megacd/megacdjp all point at the same pack).
func LoadSource(src systemdb.DatSource, overrideDir string) (*Datafile, error) {
	var data []byte
	var err error
	if overrideDir != "" {
		data, err = os.ReadFile(filepath.Join(overrideDir, filepath.FromSlash(dataPath(src))))
	} else {
		data, err = fs.ReadFile(embedded, "data/"+dataPath(src))
	}
	if err != nil {
		return nil, fmt.Errorf("no DAT for %s: %w", src.System, err)
	}
	df, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dataPath(src), err)
	}
	return df, nil
}

// DistinctSources returns every DatSource systemdb.Dat maps to, with
// duplicates (several romctl system keys sharing one pack, e.g. saturn/
// saturnjp) collapsed to one entry each.
func DistinctSources() []systemdb.DatSource {
	seen := make(map[systemdb.DatSource]bool, len(systemdb.Dat))
	out := make([]systemdb.DatSource, 0, len(systemdb.Dat))
	for _, src := range systemdb.Dat {
		if seen[src] {
			continue
		}
		seen[src] = true
		out = append(out, src)
	}
	return out
}

// have reports whether every ROM in g has a matching hash in have (CRC32
// first, since No-Intro keys on it; MD5/SHA1 for Redump entries that carry
// no CRC) - and, if so, which local filename(s) satisfied it.
func (g Game) have(byCRC, byMD5, bySHA1 map[string]string) (matched []string, ok bool) {
	for _, r := range g.ROMs {
		name, found := "", false
		if r.CRC != "" {
			if n, hit := byCRC[r.CRC]; hit {
				name, found = n, true
			}
		}
		if !found && r.MD5 != "" {
			if n, hit := byMD5[r.MD5]; hit {
				name, found = n, true
			}
		}
		if !found && r.SHA1 != "" {
			if n, hit := bySHA1[r.SHA1]; hit {
				name, found = n, true
			}
		}
		if !found {
			return nil, false
		}
		matched = append(matched, name)
	}
	return matched, true
}

// Match reports, for every Game in df, whether it's present among files
// already hashed into byCRC/byMD5/bySHA1 (lowercase hex hash -> matched
// filename). unverifiable names files that were never hashed at all (e.g.
// a .chd - see Collection's doc comment) so a caller can report them as a
// distinct status rather than silently treating them as present or absent.
type MatchResult struct {
	Game    string
	Have    bool
	Matched []string // local filename(s) that satisfied this game's ROM(s), if Have
}

// Match compares df's games against already-hashed local files.
func (df *Datafile) Match(byCRC, byMD5, bySHA1 map[string]string) []MatchResult {
	out := make([]MatchResult, 0, len(df.Games))
	for _, g := range df.Games {
		matched, ok := g.have(byCRC, byMD5, bySHA1)
		name := g.Name
		if g.Description != "" {
			name = g.Description
		}
		out = append(out, MatchResult{Game: name, Have: ok, Matched: matched})
	}
	return out
}

// pathDataPath is a test/status helper so callers outside this package
// (commands.Tools) never need to know the embed layout themselves.
func pathDataPath(src systemdb.DatSource) string {
	return path.Join("data", dataPath(src))
}
