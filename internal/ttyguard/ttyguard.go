// Package ttyguard saves the calling terminal's state before running an
// external tool and restores it afterwards. Some external tools (notably
// chdman and dolphin-tool) can leave the terminal in a raw/no-echo state
// if they touch it; this works cross-platform via golang.org/x/term, which
// wraps termios on Unix and the console mode on Windows.
package ttyguard

import (
	"os"

	"golang.org/x/term"
)

// Guard restores the terminal's state when it goes out of scope. Restore
// is a no-op if state was never captured (stdin isn't a terminal).
type Guard struct {
	state *term.State
}

// Save captures the current terminal state of stdin, if it is a terminal.
func Save() *Guard {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return &Guard{}
	}
	state, err := term.GetState(fd)
	if err != nil {
		return &Guard{}
	}
	return &Guard{state: state}
}

// Restore puts the terminal back the way Save found it.
func (g *Guard) Restore() {
	if g.state == nil {
		return
	}
	_ = term.Restore(int(os.Stdin.Fd()), g.state)
}
