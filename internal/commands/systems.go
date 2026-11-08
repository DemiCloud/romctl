package commands

import (
	"fmt"
	"os"
	"sort"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/systemdb"
)

// systemRecord is one system's shape in Systems' structured output.
type systemRecord struct {
	System string `json:"system" yaml:"system" toml:"system"`
	Class  string `json:"class" yaml:"class" toml:"class"`
}

// Systems prints every known system and its class, sorted by name, or
// encodes the same list under a structured format.
func Systems(format cliutil.Format) error {
	names := systemdb.Names()
	sort.Strings(names)

	if format.Structured() {
		if format == cliutil.FormatCSV {
			rows := make([][]string, 0, len(names))
			for _, s := range names {
				rows = append(rows, []string{s, string(systemdb.ClassOf(s))})
			}
			return cliutil.EncodeCSV(os.Stdout, []string{"system", "class"}, rows)
		}
		records := make([]systemRecord, 0, len(names))
		for _, s := range names {
			records = append(records, systemRecord{System: s, Class: string(systemdb.ClassOf(s))})
		}
		return cliutil.Encode(os.Stdout, format, records)
	}

	for _, s := range names {
		fmt.Printf("%-16s %s\n", s, systemdb.ClassOf(s))
	}
	return nil
}
