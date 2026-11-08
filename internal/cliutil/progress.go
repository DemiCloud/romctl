package cliutil

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

// Progress runs fn, animating "label: done/total" on stderr while it runs;
// fn must call the tick function it's given once per completed job. Like
// RunPhase, this is skipped - fn just runs directly, tick becoming a
// no-op - outside Fancy format, when verbose (every job already prints
// its own "do:" line, so a counter on top would be redundant), dryRun (a
// dry run already prints one "plan:" line per action, which is the full
// preview), total is zero, or stderr isn't a terminal. This is for
// long-running, CPU/tool-bound batches (chd/dvd/rvz/xiso/n64/snes/unpack)
// where, without it, a run of hundreds of files prints nothing at all
// between start and the final summary line.
func Progress(format Format, verbose, dryRun bool, label string, total int, fn func(tick func())) {
	if !format.Fancy() || verbose || dryRun || total == 0 || !term.IsTerminal(int(os.Stderr.Fd())) {
		fn(func() {})
		return
	}

	var done atomic.Int64
	stop := make(chan struct{})
	stopped := make(chan struct{})
	render := func() {
		fmt.Fprintf(os.Stderr, "\r%s: %d/%d", label, done.Load(), total)
	}
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", len(label)+2+2*len(fmt.Sprint(total))))
				return
			case <-ticker.C:
				render()
			}
		}
	}()

	fn(func() { done.Add(1) })
	close(stop)
	<-stopped
}
