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
// program and command name so output is easy to grep.
type Logger struct {
	Command string
}

// NewLogger returns a Logger prefixing messages with the given command name.
func NewLogger(command string) *Logger {
	return &Logger{Command: command}
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
	fmt.Fprintf(os.Stderr, "[%s] %s: %s: %s\n",
		time.Now().Format("2006-01-02T15:04:05Z07:00"), ProgName(), l.Command, msg)
}

// Colors holds ANSI escape codes, blank when color should be suppressed
// (not a terminal, or NO_COLOR is set: https://no-color.org/).
type Colors struct {
	Green, Yellow, Red, Reset string
}

// NewColors returns Colors for the given output file, honoring NO_COLOR.
func NewColors(f *os.File) Colors {
	if _, ok := os.LookupEnv("NO_COLOR"); ok || !term.IsTerminal(int(f.Fd())) {
		return Colors{}
	}
	return Colors{Green: "\x1b[32m", Yellow: "\x1b[33m", Red: "\x1b[31m", Reset: "\x1b[0m"}
}

// Confirm asks a yes/no question on stderr and reads an answer from stdin.
// It defaults to false when stdin is not a terminal, so scripted/CI runs
// never hang or silently proceed with a destructive action.
func Confirm(question string) bool {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
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
