package cliutil

import (
	"strings"
	"testing"

	"github.com/jedib0t/go-pretty/v6/table"
)

func TestSqueezeWidths(t *testing.T) {
	// The widest column gives up characters first, one at a time, so the
	// two big columns (10, 20) level off equal to each other once they
	// meet - a water-filling result, not "drain the widest to the floor
	// first." The column already below min (index 1) is never touched.
	got := squeezeWidths([]int{10, 2, 20}, 12, 3)
	want := []int{5, 2, 5}
	if len(got) != len(want) {
		t.Fatalf("squeezeWidths length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("squeezeWidths()[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestSqueezeWidthsCannotReachTarget(t *testing.T) {
	// Every column already at min: nothing left to give up, target missed.
	got := squeezeWidths([]int{3, 3, 3}, 1, 3)
	for i, w := range got {
		if w != 3 {
			t.Errorf("squeezeWidths()[%d] = %d, want 3 (floor)", i, w)
		}
	}
}

func TestSqueezeColumnConfigsFitsAlready(t *testing.T) {
	headers := []string{"System", "Status"}
	rows := []table.Row{{"PS3", "have"}}
	if cfgs := squeezeColumnConfigs(FormatPlain, headers, rows, 200); cfgs != nil {
		t.Errorf("squeezeColumnConfigs() = %v, want nil when it already fits", cfgs)
	}
}

func TestSqueezeColumnConfigsShrinksOnlyTheLongColumn(t *testing.T) {
	headers := []string{"System", "Game", "Status"}
	rows := []table.Row{
		{"Sony PlayStation 3", "Metal Gear Solid 4: Guns of the Patriots (USA)", "have"},
		{"Nintendo 64", "Super Mario 64 (USA)", "have"},
	}
	cfgs := squeezeColumnConfigs(FormatPlain, headers, rows, 60)
	if cfgs == nil {
		t.Fatal("squeezeColumnConfigs() = nil, want configs for an overflowing table")
	}
	for _, c := range cfgs {
		if c.Number == 3 {
			t.Errorf("Status (short, unambiguous column) was squeezed: %+v", c)
		}
	}

	var buf strings.Builder
	rt := NewTable(&buf, FormatPlain)
	hr := make(table.Row, len(headers))
	for i, h := range headers {
		hr[i] = h
	}
	rt.AppendHeader(hr)
	for _, r := range rows {
		rt.AppendRow(r)
	}
	rt.SetColumnConfigs(cfgs)
	rt.Render()

	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if n := len([]rune(line)); n > 60 {
			t.Errorf("rendered line is %d runes wide, want <= 60: %q", n, line)
		}
		if strings.Contains(line, "\n") {
			t.Errorf("rendered line wrapped instead of being squeezed: %q", line)
		}
	}
}
