package commands

import (
	"os"

	"github.com/demicloud/romctl/internal/cliutil"
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

// Tools reports, for each native binary romctl shells out to
// (chdman/dolphin-tool/extract-xiso/skyscraper), whether this specific
// binary has one embedded for this platform, found one on PATH, or has
// neither - the same resolution tools.Resolve performs lazily, the first
// time a command actually needs one, surfaced up front instead of as a
// mid-run failure. Not every tool is guaranteed to be embedded on every
// platform (see AGENTS.md's native tool build status table) - this is
// how to find out which ones this particular binary actually got.
func Tools(format cliutil.Format) error {
	colors := cliutil.ColorsFor(format)
	records := make([]toolRecord, 0, 4)
	for _, st := range tools.AllStatuses() {
		records = append(records, toolRecordFor(st, colors))
	}
	return cliutil.RenderList(os.Stdout, format, records)
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
