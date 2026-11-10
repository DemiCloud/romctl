package cliutil

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"golang.org/x/term"
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

// terminalWidth returns w's current column count, or 0 if w isn't a real
// terminal (redirected to a file/pipe, or - in tests - a plain
// bytes.Buffer): the same "only matters when someone's actually looking at
// it live" reasoning CanAnimate/maxLiveSlots already apply to stderr,
// applied here to wherever RenderTable's own output is going.
func terminalWidth(w io.Writer) int {
	f, ok := w.(interface{ Fd() uintptr })
	if !ok {
		return 0
	}
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return width
}

// squeezeColumnConfigs returns the per-column WidthMax/WidthMaxEnforcer
// settings that bring a headers/rows table's rendered width down to
// termWidth, or nil if it already fits. Rather than hardcoding the
// border/padding overhead a given table.Style adds (which this package
// doesn't otherwise need to know), it probes by rendering a disposable copy
// of the same table and measuring its actual line width - self-correcting
// if NewTable's styling ever changes, instead of a width formula that
// could silently drift out of sync with it.
//
// Only ever the widest column(s) shrink, down to minSqueezeWidth, one
// character at a time, until the total fits - so a column that's already
// narrower than its fair share (Status, Hash, ...) is never touched, and
// what does shrink is always whichever free-text field (a game/system
// name, a filename) actually ran long, never a fixed set of column names.
// This is the one way RenderTable avoids wrapping a row across multiple
// display lines on a narrow terminal: a squeezed cell's text is ellipsized
// (text.Snip) instead, however unreadable that gets at the extreme - the
// user's own call to switch to a format a narrow console can actually show
// (e.g. --format=json), not something this package tries to solve for.
func squeezeColumnConfigs(format Format, headers []string, rows []table.Row, termWidth int) []table.ColumnConfig {
	hr := make(table.Row, len(headers))
	for i, h := range headers {
		hr[i] = h
	}

	var probe bytes.Buffer
	pt := NewTable(&probe, format)
	pt.AppendHeader(hr)
	for _, r := range rows {
		pt.AppendRow(r)
	}
	pt.Render()
	lines := strings.SplitN(probe.String(), "\n", 2)
	if len(lines) == 0 {
		return nil
	}
	naturalWidth := text.StringWidthWithoutEscSequences(lines[0])
	if naturalWidth <= termWidth {
		return nil
	}

	colWidths := make([]int, len(headers))
	for i, h := range hr {
		colWidths[i] = text.StringWidthWithoutEscSequences(fmt.Sprint(h))
	}
	for _, r := range rows {
		for i := 0; i < len(colWidths) && i < len(r); i++ {
			if w := text.StringWidthWithoutEscSequences(fmt.Sprint(r[i])); w > colWidths[i] {
				colWidths[i] = w
			}
		}
	}

	const minSqueezeWidth = 3
	overhead := naturalWidth - sumInts(colWidths)
	shrunk := squeezeWidths(colWidths, termWidth-overhead, minSqueezeWidth)

	var cfgs []table.ColumnConfig
	for i, w := range shrunk {
		if w < colWidths[i] {
			cfgs = append(cfgs, table.ColumnConfig{
				Number:           i + 1,
				WidthMax:         w,
				WidthMaxEnforcer: snipEllipsis,
			})
		}
	}
	return cfgs
}

// snipEllipsis is squeezeColumnConfigs' table.WidthEnforcer: text.Snip
// trims to maxLen and appends an ellipsis, correctly around any ANSI color
// codes a cell (e.g. a colored Status) already carries.
func snipEllipsis(col string, maxLen int) string {
	return text.Snip(col, maxLen, "…")
}

// squeezeWidths returns widths reduced, one character at a time from
// whichever entry is currently largest (and still above min), until the
// total is at most target - or everything's already at min and it can't
// shrink any further.
func squeezeWidths(widths []int, target, min int) []int {
	out := append([]int(nil), widths...)
	for sumInts(out) > target {
		maxIdx := -1
		for i, w := range out {
			if w > min && (maxIdx == -1 || w > out[maxIdx]) {
				maxIdx = i
			}
		}
		if maxIdx == -1 {
			break
		}
		out[maxIdx]--
	}
	return out
}

func sumInts(nums []int) int {
	total := 0
	for _, n := range nums {
		total += n
	}
	return total
}
