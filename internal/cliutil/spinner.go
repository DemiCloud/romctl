package cliutil

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"golang.org/x/term"
)

var spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// CanAnimate reports whether a live terminal animation (RunPhases,
// Progress, ParallelLive) would actually render usefully: Fancy format
// and stderr connected to a real terminal. Exposed so a caller that has
// to pick its own output shape ahead of time can make exactly the same
// call these already make internally.
func CanAnimate(format Format) bool {
	return format.Fancy() && term.IsTerminal(int(os.Stderr.Fd()))
}

// maxLiveSlots bounds ParallelLive's worker-row count to what the
// terminal can actually show without its cursor-up redraw breaking: a
// block taller than the terminal's current row count can't be moved back
// up to in full - "cursor up N rows" clamps at row 1 instead of actually
// reaching N rows above the cursor once N exceeds what's visible - so
// every redraw ends up printing another copy below the last instead of
// overwriting it (exactly the unscrollable mess a real run surfaced
// before this existed). Leaves a small margin so the block never claims
// the very last row some terminals treat specially, and falls back to a
// conservative fixed count when the size can't be read at all (e.g.
// output captured by something that still reports as a TTY but has no
// real geometry).
func maxLiveSlots() int {
	const fallback = 8
	const margin = 2
	_, h, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || h <= margin {
		return fallback
	}
	return h - margin
}

// Phase is one named unit of work for RunPhases: a label shown on its own
// line and the function that performs it. Fn reports whether the phase
// succeeded, coloring its final marker green (true) or red (false) -
// exactly like ParallelLive's own pass/fail icon - rather than always
// showing success.
type Phase struct {
	Label string
	Fn    func() bool
}

// RunPhases runs every phase in order, each on its own stderr line: all
// labels print up front (right-padded to a shared column so every colon
// lines up), so the full plan is visible before any phase finishes, and
// each line is then updated in place - spinning while its phase runs,
// left reading "label: ✓ done" (or "label: ✗ failed", see Phase.Fn) once
// it finishes - rather than erased, so a completed phase stays visible
// alongside the ones still running. Skipped - every Fn just runs
// directly, in order - outside Fancy format (Plain and every structured
// format print no decoration at all), when verbose (a phase prints its
// own output instead, which would collide with the lines here), when
// dryRun (a preview's "plan:" lines matter more than a spinner, and
// printing them while this animates would corrupt its redraw, the same
// hazard noted on cliutil.ParallelLive's worker-row cap), or when stderr
// isn't a terminal (no point animating output nobody sees live, and it
// would otherwise write control characters into a piped/redirected
// stream).
func RunPhases(format Format, verbose, dryRun bool, phases []Phase) {
	if !format.Fancy() || verbose || dryRun || !term.IsTerminal(int(os.Stderr.Fd())) {
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

		ok := p.Fn()
		close(done)
		<-stopped
		if ok {
			redraw(i, colors.Green+"✓"+colors.Reset+" done")
		} else {
			redraw(i, colors.Red+"✗"+colors.Reset+" failed")
		}
	}
}

// ParallelLive runs fn for every item across a worker pool, driving a
// live terminal display while it runs: one spinning line per active
// worker, naming (via name) the item that worker is currently
// processing, finalized the moment fn returns to a permanent, colored
// "✓ name"/"✗ name" line (fn's own bool return - true for pass, false
// for fail) printed just above the still-spinning block, which then
// keeps scrolling upward as each worker picks up its next item. fn must
// still do its own result handling (counting, recording a finding, &c.)
// - this only renders, the same division of responsibility Progress's
// tick already has.
//
// The pool is sized to GOMAXPROCS, same as Parallel, but capped by
// maxLiveSlots: an earlier version used GOMAXPROCS outright, and on a
// many-core machine that's routinely taller than the terminal actually
// showing it, which breaks the multi-line cursor-up redraw this needs
// (see maxLiveSlots' own doc comment). Capping the slot count keeps every
// visible spinner honest - each one names a file a worker is genuinely
// verifying right now, just bounded to however many rows can actually be
// redrawn safely, rather than either going fully sequential (losing real
// parallelism) or risking the same unscrollable mess again.
//
// Falls back to a plain Parallel call with no animation whenever
// CanAnimate says it wouldn't actually render usefully, or there's
// nothing to show.
func ParallelLive[T any](format Format, items []T, name func(T) string, fn func(T) bool) {
	if !CanAnimate(format) || len(items) == 0 {
		Parallel(items, func(t T) { fn(t) })
		return
	}

	colors := ColorsFor(format)
	workers := runtime.GOMAXPROCS(0)
	if maxSlots := maxLiveSlots(); workers > maxSlots {
		workers = maxSlots
	}
	if workers > len(items) {
		workers = len(items)
	}
	if workers < 1 {
		workers = 1
	}

	// final, when non-empty, is a completed "✓/✗ name" line to print
	// above the live block, after which worker's slot goes idle until its
	// next label event; otherwise label is that worker's new in-progress
	// line.
	type event struct {
		worker       int
		label, final string
	}
	events := make(chan event, workers*2)
	renderDone := make(chan struct{})

	go func() {
		defer close(renderDone)
		slots := make([]string, workers)
		height := 0
		frame := 0

		// erase wipes the live block (if one is currently drawn) back to
		// where it started, so either a completed line can be inserted
		// just above it, or it can be redrawn fresh with the next
		// spinner frame - both need the same "move up height lines, clear
		// to end of screen" first step.
		erase := func() {
			if height > 0 {
				fmt.Fprintf(os.Stderr, "\x1b[%dA\r\x1b[J", height)
			}
		}
		redraw := func() {
			for _, s := range slots {
				if s == "" {
					fmt.Fprintln(os.Stderr)
					continue
				}
				fmt.Fprintf(os.Stderr, "%c %s\n", spinnerFrames[frame%len(spinnerFrames)], s)
			}
			height = len(slots)
		}

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					erase()
					return
				}
				erase()
				if ev.final != "" {
					fmt.Fprintln(os.Stderr, ev.final)
					slots[ev.worker] = ""
				} else {
					slots[ev.worker] = ev.label
				}
				redraw()
			case <-ticker.C:
				frame++
				erase()
				redraw()
			}
		}
	}()

	var wg sync.WaitGroup
	sem := make(chan int, workers)
	for i := 0; i < workers; i++ {
		sem <- i
	}
	for _, item := range items {
		label := name(item)
		w := <-sem
		wg.Add(1)
		go func(item T, label string, w int) {
			defer wg.Done()
			defer func() { sem <- w }()
			events <- event{worker: w, label: label}
			ok := fn(item)
			icon := colors.Red + "✗" + colors.Reset
			if ok {
				icon = colors.Green + "✓" + colors.Reset
			}
			events <- event{worker: w, final: fmt.Sprintf("%s %s", icon, label)}
		}(item, label, w)
	}
	wg.Wait()
	close(events)
	<-renderDone
}
