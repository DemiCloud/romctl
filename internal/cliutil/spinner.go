package cliutil

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

var spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// Phase is one named unit of work for RunPhases: a label shown on its own
// line and the function that performs it.
type Phase struct {
	Label string
	Fn    func()
}

// RunPhases runs every phase in order, each on its own stderr line: all
// labels print up front (right-padded to a shared column so every colon
// lines up), so the full plan is visible before any phase finishes, and
// each line is then updated in place - spinning while its phase runs,
// left reading "label: ✓ done" once it finishes - rather than erased, so
// a completed phase stays visible alongside the ones still running.
// Skipped - every Fn just runs directly, in order - outside Fancy format
// (Plain and every structured format print no decoration at all), when
// verbose (a phase prints its own output instead, which would collide
// with the lines here), or when stderr isn't a terminal (no point
// animating output nobody sees live, and it would otherwise write control
// characters into a piped/redirected stream).
func RunPhases(format Format, verbose bool, phases []Phase) {
	if !format.Fancy() || verbose || !term.IsTerminal(int(os.Stderr.Fd())) {
		for _, p := range phases {
			p.Fn()
		}
		return
	}

	colors := ColorsFor(format)
	width := 0
	for _, p := range phases {
		if w := len(p.Label) + 1; w > width {
			width = w
		}
	}
	label := func(i int) string {
		return fmt.Sprintf("%-*s", width, phases[i].Label+":")
	}

	n := len(phases)
	for i := range phases {
		fmt.Fprintf(os.Stderr, "%s\n", label(i))
	}

	// redraw rewrites phase i's line in place, then returns the cursor to
	// column 0 just past the last line, where the next redraw (or the
	// next phase's own output) expects to find it.
	redraw := func(i int, suffix string) {
		up := n - i
		fmt.Fprintf(os.Stderr, "\x1b[%dA\r%s %s\x1b[K\x1b[%dB\r", up, label(i), suffix, up)
	}

	for i, p := range phases {
		done := make(chan struct{})
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			f := 0
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					redraw(i, string(spinnerFrames[f%len(spinnerFrames)]))
					f++
				}
			}
		}()

		p.Fn()
		close(done)
		<-stopped
		redraw(i, colors.Green+"✓"+colors.Reset+" done")
	}
}
