package cliutil

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Format is romctl's output mode: the fancy/plain split (colors, unicode
// symbols, spinners and a live progress counter vs. bare ASCII text) and
// four machine-readable encodings (json/yaml/toml/csv) that every command
// renders its result through instead of a one-line text summary.
type Format string

const (
	// FormatAuto is the zero value - "not explicitly set", resolved to
	// Fancy or Plain by Resolve once we know whether stdout is a terminal.
	FormatAuto  Format = ""
	FormatFancy Format = "fancy"
	FormatPlain Format = "plain"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
	FormatTOML  Format = "toml"
	FormatCSV   Format = "csv"
)

// formatNames is every value ParseFormat accepts, in the order they're
// listed in error messages and --help.
var formatNames = []string{"fancy", "plain", "json", "yaml", "toml", "csv"}

// ParseFormat validates value against formatNames; an empty string is
// FormatAuto (not an error - it means "let Resolve decide").
func ParseFormat(value string) (Format, error) {
	if value == "" {
		return FormatAuto, nil
	}
	for _, n := range formatNames {
		if value == n {
			return Format(value), nil
		}
	}
	return "", fmt.Errorf("invalid --format %q (want one of: %s)", value, strings.Join(formatNames, ", "))
}

// Resolve turns FormatAuto into Fancy or Plain by checking whether out is
// a terminal; every other value (including an explicit "plain" or
// "fancy") passes through unchanged, since an explicit choice always
// wins over auto-detection.
func (f Format) Resolve(out *os.File) Format {
	if f != FormatAuto {
		return f
	}
	if term.IsTerminal(int(out.Fd())) {
		return FormatFancy
	}
	return FormatPlain
}

// Structured reports whether f is one of the machine-readable encodings -
// the point past which a command stops printing text (plan:/do: lines,
// spinners, one-line summaries) and instead renders a single encoded
// result at the end.
func (f Format) Structured() bool {
	switch f {
	case FormatJSON, FormatYAML, FormatTOML, FormatCSV:
		return true
	}
	return false
}

// Fancy reports whether f calls for colors/unicode/spinners - false for
// Plain and for every structured format, which have no visual styling at
// all.
func (f Format) Fancy() bool {
	return f == FormatFancy
}
