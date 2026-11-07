// Package ttyguard saves the calling terminal's state before running an
// external tool and restores it afterwards. Some external tools (notably
// chdman and dolphin-tool) can leave the terminal in a raw/no-echo state
// if they touch it; this works cross-platform via golang.org/x/term, which
// wraps termios on Unix and the console mode on Windows.
package ttyguard

import (
	"os"
	"sync"

	"golang.org/x/term"
)

// mu, depth and saved coordinate concurrent Save/Restore pairs (romctl's
// conversion commands now run per-system work in parallel, each wrapping
// its own external-tool call in one). Only the first concurrent Save
// actually reads the terminal state - a later one could otherwise capture
// a sibling call's already-altered raw-mode state instead of the real
// original - and only the last concurrent Restore to release puts it back.
var (
	mu    sync.Mutex
	depth int
	saved *term.State
)

// Guard restores the terminal's state when it goes out of scope. Restore
// is a no-op if state was never captured (stdin isn't a terminal).
type Guard struct {
	active bool
}

// Save captures the current terminal state of stdin, if it is a terminal.
func Save() *Guard {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return &Guard{}
	}
	mu.Lock()
	defer mu.Unlock()
	if depth == 0 {
		state, err := term.GetState(fd)
		if err != nil {
			return &Guard{}
		}
		saved = state
	}
	depth++
	return &Guard{active: true}
}

// Restore puts the terminal back the way Save found it, once every
// concurrent Save/Restore pair has released.
func (g *Guard) Restore() {
	if !g.active {
		return
	}
	mu.Lock()
	depth--
	last := depth == 0
	mu.Unlock()
	if last {
		_ = term.Restore(int(os.Stdin.Fd()), saved)
	}
}
