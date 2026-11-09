package commands

import (
	"os"
	"sort"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/systemdb"
)

// systemRecord is one system's shape for every output form - table
// (Fancy/Plain), csv, json/yaml/toml - via cliutil.RenderList. System is
// the nice display name (systemdb.DisplayName); Directory is the actual
// roms/<Directory>/ name systemRecord's own System field used to be -
// kept under its own column so both are visible at once instead of one
// replacing the other.
type systemRecord struct {
	System    string `json:"system" yaml:"system" toml:"system" col:"System"`
	Directory string `json:"directory" yaml:"directory" toml:"directory" col:"Directory"`
	Class     string `json:"class" yaml:"class" toml:"class" col:"Class"`
}

// Systems renders every known system - its nice display name, its
// roms/ directory name, and its class - sorted by directory name, through
// every format cliutil.RenderList handles.
func Systems(format cliutil.Format) error {
	names := systemdb.Names()
	sort.Strings(names)

	records := make([]systemRecord, 0, len(names))
	for _, s := range names {
		records = append(records, systemRecord{System: systemdb.DisplayName(s), Directory: s, Class: string(systemdb.ClassOf(s))})
	}
	return cliutil.RenderList(os.Stdout, format, records)
}
