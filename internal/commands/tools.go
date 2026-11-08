package commands

import (
	"fmt"
	"os"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/tools"
)

// toolRecord is one tool's shape in Tools' structured output.
type toolRecord struct {
	Tool      string `json:"tool" yaml:"tool" toml:"tool"`
	Status    string `json:"status" yaml:"status" toml:"status"` // "embedded", "PATH", "missing"
	Version   string `json:"version,omitempty" yaml:"version,omitempty" toml:"version,omitempty"`
	Available bool   `json:"available" yaml:"available" toml:"available"`
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
	records := make([]toolRecord, 0, 4)
	for _, st := range tools.AllStatuses() {
		records = append(records, toolRecordFor(st))
	}

	if format.Structured() {
		if format == cliutil.FormatCSV {
			header := []string{"tool", "status", "version", "available"}
			rows := make([][]string, 0, len(records))
			for _, r := range records {
				rows = append(rows, []string{r.Tool, r.Status, r.Version, fmt.Sprintf("%t", r.Available)})
			}
			return cliutil.EncodeCSV(os.Stdout, header, rows)
		}
		return cliutil.Encode(os.Stdout, format, records)
	}

	colors := cliutil.ColorsFor(format)
	fmt.Printf("%-14s %-9s %s\n", "TOOL", "STATUS", "VERSION")
	for _, r := range records {
		color := colors.Green
		if !r.Available {
			color = colors.Red
		}
		version := r.Version
		if version == "" {
			version = "-"
		}
		fmt.Printf("%-14s %s%-9s%s %s\n", r.Tool, color, r.Status, colors.Reset, version)
	}
	return nil
}

func toolRecordFor(st tools.Status) toolRecord {
	switch {
	case st.Embedded:
		return toolRecord{Tool: st.Tool, Status: "embedded", Version: tools.Version(st.Tool), Available: true}
	case st.OnPath != "":
		return toolRecord{Tool: st.Tool, Status: "PATH", Version: tools.Version(st.Tool), Available: true}
	default:
		return toolRecord{Tool: st.Tool, Status: "missing"}
	}
}
