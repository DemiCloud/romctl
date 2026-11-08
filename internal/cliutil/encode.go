package cliutil

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Encode writes v to w as JSON, YAML or TOML, chosen by format - the one
// place every structured-output command (check's findings, a mutating
// command's result) goes through, instead of each hand-rolling its own
// encoder. v's struct tags need json/yaml/toml names for the three to
// agree on field naming; see audit.jsonFinding and actions.Result.
func Encode(w io.Writer, format Format, v any) error {
	switch format {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	case FormatYAML:
		enc := yaml.NewEncoder(w)
		defer enc.Close()
		return enc.Encode(v)
	case FormatTOML:
		return toml.NewEncoder(w).Encode(v)
	}
	return fmt.Errorf("Encode: not a structured format: %s", format)
}

// EncodeCSV writes header followed by rows as CSV. Callers building rows
// from a map (e.g. a mutating command's counts, which vary by command)
// should sort the keys themselves first - EncodeCSV just writes what it's
// given, in order.
func EncodeCSV(w io.Writer, header []string, rows [][]string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
