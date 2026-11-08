package cliutil

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

var spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// RunPhase runs fn, animating a spinner labeled label on stderr while it
// runs. Skipped - fn just runs directly - outside Fancy format (Plain and
// every structured format print no decoration at all), when verbose (fn
// prints its own output instead, which would collide with the spinner),
// or when stderr isn't a terminal (no point animating output nobody sees
// live, and it would otherwise write control characters into a
// piped/redirected stream).
func RunPhase(format Format, verbose bool, label string, fn func()) {
	if !format.Fancy() || verbose || !term.IsTerminal(int(os.Stderr.Fd())) {
		fn()
		return
	}

	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-done:
				fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", len(label)+8))
				return
			case <-ticker.C:
				fmt.Fprintf(os.Stderr, "\r%s  %c", label, spinnerFrames[i%len(spinnerFrames)])
				i++
			}
		}
	}()

	fn()
	close(done)
	<-stopped
}
