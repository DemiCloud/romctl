package commands

import (
	"os"
	"sort"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/systemdb"
)

// systemRecord is one system's shape for every output form - table
// (Fancy/Plain), csv, json/yaml/toml - via cliutil.RenderList.
type systemRecord struct {
	System string `json:"system" yaml:"system" toml:"system" col:"System"`
	Class  string `json:"class" yaml:"class" toml:"class" col:"Class"`
}

// Systems renders every known system and its class, sorted by name,
// through every format cliutil.RenderList handles.
func Systems(format cliutil.Format) error {
	names := systemdb.Names()
	sort.Strings(names)

	records := make([]systemRecord, 0, len(names))
	for _, s := range names {
		records = append(records, systemRecord{System: s, Class: string(systemdb.ClassOf(s))})
	}
	return cliutil.RenderList(os.Stdout, format, records)
}
