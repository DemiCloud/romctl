package commands

import (
	"fmt"
	"os"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/datdb"
	"github.com/demicloud/romctl/internal/tools"
)

// toolRecord is one tool's shape for every output form - table (Fancy/
// Plain, Status colored by Available - see toolRecordFor), csv,
// json/yaml/toml - via cliutil.RenderList.
type toolRecord struct {
	Tool      string `json:"tool" yaml:"tool" toml:"tool" col:"Tool"`
	Status    string `json:"status" yaml:"status" toml:"status" col:"Status"` // "embedded", "PATH", "missing"
	Version   string `json:"version" yaml:"version" toml:"version" col:"Version"`
	Available bool   `json:"available" yaml:"available" toml:"available" col:"Available"`
}

// datRecord is one system's DAT-coverage shape for every output form, same
// cliutil.RenderList convention as toolRecord.
type datRecord struct {
	Provider string `json:"provider" yaml:"provider" toml:"provider" col:"Provider"`
	System   string `json:"system" yaml:"system" toml:"system" col:"System"`
	Name     string `json:"name" yaml:"name" toml:"name" col:"Name"`
	Version  string `json:"version" yaml:"version" toml:"version" col:"Version"`
	Date     string `json:"date" yaml:"date" toml:"date" col:"Date"`
	Embedded bool   `json:"embedded" yaml:"embedded" toml:"embedded" col:"Embedded"`
}

// toolsOutput wraps both tables as one document for json/yaml/toml, so a
// structured `romctl tools` is still a single Encode call rather than two
// concatenated ones - cliutil.RenderList has no multi-table primitive of
// its own (see its doc comment); this is the one place that matters, so
// the wrapping happens here instead of in cliutil.
type toolsOutput struct {
	Tools []toolRecord `json:"tools" yaml:"tools" toml:"tools"`
	Dats  []datRecord  `json:"dats" yaml:"dats" toml:"dats"`
}

// Tools reports, for each native binary romctl shells out to
// (chdman/dolphin-tool/extract-xiso/skyscraper), whether this specific
// binary has one embedded for this platform, found one on PATH, or has
// neither - the same resolution tools.Resolve performs lazily, the first
// time a command actually needs one, surfaced up front instead of as a
// mid-run failure. Not every tool is guaranteed to be embedded on every
// platform (see AGENTS.md's native tool build status table) - this is
// how to find out which ones this particular binary actually got.
//
// Also reports, as a second table, which systems have a No-Intro/Redump
// DAT snapshot embedded (see internal/datdb) and that snapshot's own
// version/date - the same "what did this specific build actually ship
// with" question, for romctl collection's DAT coverage instead of native
// tools.
func Tools(format cliutil.Format) error {
	colors := cliutil.ColorsFor(format)
	toolRecords := make([]toolRecord, 0, 4)
	for _, st := range tools.AllStatuses() {
		toolRecords = append(toolRecords, toolRecordFor(st, colors))
	}
	datRecords := make([]datRecord, 0, len(datdb.AllStatuses()))
	for _, st := range datdb.AllStatuses() {
		datRecords = append(datRecords, datRecordFor(st))
	}

	if format.Structured() && format != cliutil.FormatCSV {
		return cliutil.Encode(os.Stdout, format, toolsOutput{Tools: toolRecords, Dats: datRecords})
	}
	if format == cliutil.FormatCSV {
		if err := cliutil.EncodeListCSV(os.Stdout, toolRecords); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout)
		return cliutil.EncodeListCSV(os.Stdout, datRecords)
	}
	if err := cliutil.RenderList(os.Stdout, format, toolRecords); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout)
	return cliutil.RenderList(os.Stdout, format, datRecords)
}

func datRecordFor(st datdb.Status) datRecord {
	dash := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	return datRecord{
		Provider: st.Provider, System: st.System, Name: st.Name,
		Version: dash(st.Version), Date: dash(st.Date), Embedded: st.Embedded,
	}
}

func toolRecordFor(st tools.Status, colors cliutil.Colors) toolRecord {
	color := func(available bool) string {
		if available {
			return colors.Green
		}
		return colors.Red
	}
	version := func(v string) string {
		if v == "" {
			return "-"
		}
		return v
	}
	switch {
	case st.Embedded:
		return toolRecord{Tool: st.Tool, Status: color(true) + "embedded" + colors.Reset, Version: version(tools.Version(st.Tool)), Available: true}
	case st.OnPath != "":
		return toolRecord{Tool: st.Tool, Status: color(true) + "PATH" + colors.Reset, Version: version(tools.Version(st.Tool)), Available: true}
	default:
		return toolRecord{Tool: st.Tool, Status: color(false) + "missing" + colors.Reset, Version: version(""), Available: false}
	}
}
