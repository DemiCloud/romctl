// Package cliutil holds small helpers shared by every romctl command:
// program-name formatting, timestamped error logging, NO_COLOR-aware
// colorizing, and a confirmation prompt.
package cliutil

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// ProgName returns the program name exactly as invoked (os.Args[0]), so
// printed command examples can be copy-pasted verbatim (e.g. "./romctl",
// not a hardcoded "romctl").
func ProgName() string {
	return os.Args[0]
}

// Logger writes timestamped diagnostics to stderr, prefixed with the
// program and command name so output is easy to grep. In Fancy format,
// the message itself prints in red - every Logger call is either an
// error or a "skipped" notice, never routine progress.
type Logger struct {
	Command string
	colors  Colors
}

// NewLogger returns a Logger prefixing messages with the given command
// name, colored (or not) per format.
func NewLogger(command string, format Format) *Logger {
	return &Logger{Command: command, colors: ColorsFor(format)}
}

// Errf logs a formatted error to stderr.
func (l *Logger) Errf(format string, args ...any) {
	l.print(fmt.Sprintf(format, args...))
}

// Err logs an error to stderr.
func (l *Logger) Err(err error) {
	l.print(err.Error())
}

func (l *Logger) print(msg string) {
	// Local time-only, not a full RFC3339 timestamp: long enough to tell
	// how far apart two lines are during an unattended multi-hour run,
	// short enough not to dominate a line that's otherwise as terse as
	// every other romctl error.
	fmt.Fprintf(os.Stderr, "[%s] %s: %s: %s%s%s\n",
		time.Now().Format("15:04:05"), ProgName(), l.Command, l.colors.Red, msg, l.colors.Reset)
}

// Colors holds ANSI escape codes, blank when color should be suppressed.
type Colors struct {
	Green, Yellow, Red, Reset string
}

// ColorsFor returns Colors for format: populated for Fancy (unless
// NO_COLOR is set: https://no-color.org/ - that always wins, even over an
// explicit --format=fancy), blank for everything else. Unlike the old
// per-terminal check this replaces, it's driven by the already-resolved
// Format rather than re-testing os.Stdout itself, so an explicit
// --format=fancy colors output even when redirected, same as an explicit
// --format=plain suppresses it even on a terminal.
func ColorsFor(format Format) Colors {
	if !format.Fancy() {
		return Colors{}
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return Colors{}
	}
	return Colors{Green: "\x1b[32m", Yellow: "\x1b[33m", Red: "\x1b[31m", Reset: "\x1b[0m"}
}

// StatusColor returns the color/reset pair for a command's one-line
// summary: red if it reports any failure, green otherwise, blank outside
// Fancy format. Every mutating command's final print line uses this so
// "something went wrong" is visible at a glance without reading the
// numbers.
func StatusColor(format Format, failed bool) (color, reset string) {
	colors := ColorsFor(format)
	if failed {
		return colors.Red, colors.Reset
	}
	return colors.Green, colors.Reset
}

// Interactive reports whether stdin is a terminal - the same check Confirm
// makes before prompting, exposed separately for a caller (e.g. a one-time
// notice) that needs to skip asking entirely rather than have Confirm's
// non-interactive "false" read as a declined answer.
func Interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// Confirm asks a yes/no question on stderr and reads an answer from stdin.
// It defaults to false when stdin is not a terminal, so scripted/CI runs
// never hang or silently proceed with a destructive action.
func Confirm(question string) bool {
	if !Interactive() {
		return false
	}
	fmt.Fprintf(os.Stderr, "%s [y/N] ", question)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
