package cliutil

import (
	"io"

	"github.com/jedib0t/go-pretty/v6/table"
)

// NewTable returns a table.Writer rendering to w: unicode light box-drawing
// (table.StyleLight - "┌─┬─┐" corners, not go-pretty's rounded default) in
// Fancy format, bare ASCII (table.StyleDefault) everywhere else that still
// builds one (Plain only - a structured format never builds a table at
// all, see Format.Structured). Cell text colored via ColorsFor's raw ANSI
// codes renders correctly either way: go-pretty measures column width by
// stripping escape sequences, the same as every other color-aware string
// this package already prints.
func NewTable(w io.Writer, format Format) table.Writer {
	t := table.NewWriter()
	t.SetOutputMirror(w)
	if format.Fancy() {
		t.SetStyle(table.StyleLight)
	} else {
		t.SetStyle(table.StyleDefault)
	}
	return t
}
