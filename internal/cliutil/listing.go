package cliutil

import (
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
)

// RenderTable renders headers/rows as a single rich table (unicode
// box-drawing in Fancy format, bare ASCII in Plain - see NewTable) to w.
// This is the one place every command builds a table through, so every
// listing (the structured "slice of records" ones RenderList renders, and
// a bespoke one like audit.Report's Checks/Fixes tables, whose rows need
// per-cell logic - colors, glyphs, aggregation - a plain struct dump
// can't express) ends up styled identically.
func RenderTable(w io.Writer, format Format, headers []string, rows []table.Row) {
	t := NewTable(w, format)
	hr := make(table.Row, len(headers))
	for i, h := range headers {
		hr[i] = h
	}
	t.AppendHeader(hr)
	for _, r := range rows {
		t.AppendRow(r)
	}
	if width := terminalWidth(w); width > 0 {
		if cfgs := squeezeColumnConfigs(format, headers, rows, width); cfgs != nil {
			t.SetColumnConfigs(cfgs)
		}
	}
	t.Render()
}

// col is the struct tag RenderList's reflection reads: `col:"Header"` on
// an exported field marks it as a table/CSV column (in field declaration
// order) with that header text. A field with no `col` tag is left out of
// the table and CSV forms - too free-form for either (e.g. list's Titles,
// a variable-length game list; or, like commands.biosInventoryRow's
// Label/Message, a fuller version of a field already shown in short form
// elsewhere in the same struct) - but still appears under json/yaml/toml,
// which encode the struct directly via its own tags, independent of this
// one. This is the one rule, with no exceptions: a struct's field should
// always keep a real json/yaml/toml tag (so the same text a table/CSV
// shows is readable straight out of structured output too, not hidden
// behind `json:"-"`), and `col` is only ever what adds or withholds
// table/CSV visibility on top of that.
const colTag = "col"

// elemType returns v's element type: v must be a slice (or pointer to
// one) of structs, the only shape RenderList/Columns/Rows understand.
func elemType(v any) reflect.Type {
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	return t
}

// colFields returns the index, in declaration order, of every field of t
// carrying a `col` tag.
func colFields(t reflect.Type) []int {
	var out []int
	for i := 0; i < t.NumField(); i++ {
		if _, ok := t.Field(i).Tag.Lookup(colTag); ok {
			out = append(out, i)
		}
	}
	return out
}

// Columns returns v's `col`-tagged column headers, in field order.
func Columns(v any) []string {
	t := elemType(v)
	var out []string
	for _, i := range colFields(t) {
		out = append(out, t.Field(i).Tag.Get(colTag))
	}
	return out
}

// Rows returns each element of v (a slice of structs) as a table.Row of
// its `col`-tagged field values, in the same order Columns reports -
// used both for RenderTable's rich table and (stringified) for CSV, so
// the two can never drift out of sync with each other.
func Rows(v any) []table.Row {
	rv := reflect.ValueOf(v)
	fields := colFields(elemType(v))
	rows := make([]table.Row, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i)
		row := make(table.Row, len(fields))
		for j, f := range fields {
			row[j] = cellValue(item.Field(f))
		}
		rows = append(rows, row)
	}
	return rows
}

// cellValue returns v as a value fit for one table/CSV cell: a slice
// (e.g. a short list that still fits one column, unlike Titles above)
// joins with ", " instead of printing as Go syntax or panicking; anything
// else prints as its default string form, which already renders int/bool/
// string sensibly.
func cellValue(v reflect.Value) any {
	if v.Kind() != reflect.Slice {
		return v.Interface()
	}
	parts := make([]string, v.Len())
	for i := range parts {
		parts[i] = fmt.Sprint(v.Index(i).Interface())
	}
	return strings.Join(parts, ", ")
}

// RowsToStrings stringifies Rows' output for EncodeListCSV/EncodeCSV,
// which only deal in strings - exported so a caller building its own
// `col`-tagged struct slice (e.g. audit.Report's findings) can reuse
// Columns/Rows for its own CSV branch too, instead of hand-rolling one.
func RowsToStrings(rows []table.Row) [][]string {
	out := make([][]string, len(rows))
	for i, r := range rows {
		row := make([]string, len(r))
		for j, c := range r {
			row[j] = fmt.Sprint(c)
		}
		out[i] = row
	}
	return out
}

// EncodeListCSV writes v (a slice of `col`-tagged structs) as CSV, from
// the very same Columns/Rows RenderList's table uses - the one place a
// `col`-tagged CSV is actually produced, whether v is rendered as a full
// RenderList (list/systems/tools/bios -v) or, like audit.Report's
// findings, has its own bespoke non-CSV forms (the Checks/Fixes tables,
// full json/yaml/toml) built around the same struct for everything else.
func EncodeListCSV(w io.Writer, v any) error {
	return EncodeCSV(w, Columns(v), RowsToStrings(Rows(v)))
}

// RenderList renders v (a slice of `col`-tagged structs) to w: a rich
// table via RenderTable in Fancy/Plain format, or - under a structured
// format - CSV via EncodeListCSV, or json/yaml/toml encoding v directly
// via Encode (using its own json/yaml/toml tags, independent of `col` -
// so a field RenderTable/CSV can't usefully show, like a free-length
// list, still appears there). This is the one place every simple "here's
// a slice of records" command (list, systems, tools, bios -v) renders
// through, instead of each hand-rolling its own Printf table and CSV rows.
func RenderList(w io.Writer, format Format, v any) error {
	if format.Structured() {
		if format == FormatCSV {
			return EncodeListCSV(w, v)
		}
		return Encode(w, format, v)
	}
	RenderTable(w, format, Columns(v), Rows(v))
	return nil
}
