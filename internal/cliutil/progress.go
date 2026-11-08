package cliutil

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

// Progress runs fn, animating "label: done/total" on stderr while it runs,
// then leaves the line reading "label: total/total done" rather than
// erasing it; fn must call the tick function it's given once per
// completed job. Like RunPhases, this is skipped - fn just runs directly,
// tick becoming a no-op - outside Fancy format, when verbose (every job
// already prints its own "do:" line, so a counter on top would be
// redundant), dryRun (a dry run already prints one "plan:" line per
// action, which is the full preview), total is zero, or stderr isn't a
// terminal. This is for long-running, CPU/tool-bound batches
// (chd/dvd/rvz/xiso/n64/snes/unpack) where, without it, a run of hundreds
// of files prints nothing at all between start and the final summary line.
func Progress(format Format, verbose, dryRun bool, label string, total int, fn func(tick func())) {
	if !format.Fancy() || verbose || dryRun || total == 0 || !term.IsTerminal(int(os.Stderr.Fd())) {
		fn(func() {})
		return
	}

	var done atomic.Int64
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				fmt.Fprintf(os.Stderr, "\r%s: %d/%d", label, done.Load(), total)
			}
		}
	}()

	fn(func() { done.Add(1) })
	close(stop)
	<-stopped
	fmt.Fprintf(os.Stderr, "\r%s: %d/%d done\n", label, done.Load(), total)
}
