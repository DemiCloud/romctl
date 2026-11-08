package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/demicloud/romctl/internal/audit"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/executil"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
	"github.com/demicloud/romctl/internal/tools"
)

// verifyJob is one .chd file to verify, flattened across every CD/DVD/UMD-
// class system given - same rationale as CHD's own chdJob: chdman verify
// is CPU/IO-bound and independent per file, so it runs through one shared
// worker pool sized to GOMAXPROCS instead of bottlenecking on however many
// systems were asked for (often just one). label is what the finding
// names: the bare filename, or "Title.m3u/Title (Disc N).chd" for a disc
// inside a multi-disc playlist directory.
type verifyJob struct {
	sys, path, label string
}

// verifyJobsFor walks every given CD/DVD/UMD-class system's directory and
// flattens every .chd it finds - bare files and every disc inside a
// Title.m3u/ playlist directory alike - into one job list. Pure
// filesystem walk, no chdman involved, so it's testable on its own.
func verifyJobsFor(lib *library.Library, systems []string) []verifyJob {
	var jobs []verifyJob
	for _, sys := range systems {
		class := systemdb.ClassOf(sys)
		if class != systemdb.ClassCD && class != systemdb.ClassDVD && class != systemdb.ClassUMD {
			continue
		}
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			base := e.Name()
			path := filepath.Join(sysDir, base)
			if e.IsDir() {
				if !strings.HasSuffix(base, ".m3u") {
					continue
				}
				discs, err := os.ReadDir(path)
				if err != nil {
					continue
				}
				for _, d := range discs {
					if d.IsDir() || strings.ToLower(filepath.Ext(d.Name())) != ".chd" {
						continue
					}
					jobs = append(jobs, verifyJob{sys, filepath.Join(path, d.Name()), filepath.Join(base, d.Name())})
				}
				continue
			}
			if strings.ToLower(filepath.Ext(base)) != ".chd" {
				continue
			}
			jobs = append(jobs, verifyJob{sys, path, base})
		}
	}
	return jobs
}

// Verify hands every .chd across the given systems to chdman's own
// `verify` subcommand - a full pass over every compressed hunk's
// checksum, not just the header/metadata read `check`'s containers
// category already does for free (see checkCHD). This catches corruption
// the header-only check can't: a .chd whose header is intact but whose
// data got corrupted afterward (bitrot, a transfer interrupted partway
// through).
//
// Its own command, rather than a check flag: unlike every other check,
// this means reading every CHD's full content, which can take long
// enough on a library with large CHDs that folding it into check's own
// instant, all-categories-at-once output made a long run indistinguishable
// from a hang - the same reasoning that already split `bios` out as its
// own command. It needs chdman itself available (embedded or on PATH; see
// README's Requirements), failing up front with the same actionable
// message any other chdman-backed command would if it isn't - see
// runVerify for the rest, kept separate so it's testable with a stand-in
// chdman instead of needing a real one resolved.
func Verify(lib *library.Library, systems []string, verbose bool, format cliutil.Format) (bool, error) {
	chdmanPath, err := tools.Resolve("chdman")
	if err != nil {
		return false, err
	}
	return runVerify(verifyJobsFor(lib, systems), chdmanPath, verbose, format)
}

// runVerify is Verify's testable core: given an already-resolved chdman
// and job list, it runs every verify and renders the result.
//
// Default shows a live "verify: done/total" counter while it runs
// (cliutil.Progress), verifying across a worker pool. -v/--verbose, on a
// real Fancy terminal (cliutil.CanAnimate), instead drives
// cliutil.ParallelLive: one spinning line per active worker naming the
// file it's verifying right now, finalized to a permanent colored ✓/✗
// the moment that worker moves to its next file - so a long run actually
// shows what's happening, including genuine concurrency, not just a
// counter or a single file name sitting still. report.AddQuiet records
// a failure there instead of Add, since Add's own immediate print would
// otherwise land on the same terminal ParallelLive is actively managing
// and corrupt its redraw accounting. Verbose without a usable terminal
// (plain/structured output, or redirected to a file) falls back to one
// plain "[i/total] verify sys/file" line per file instead, processed one
// at a time to keep the printed order sane.
func runVerify(jobs []verifyJob, chdmanPath string, verbose bool, format cliutil.Format) (bool, error) {
	report := audit.New(os.Stdout, verbose, format, "")
	report.Scan(audit.CategoryVerify, len(jobs))

	live := verbose && cliutil.CanAnimate(format)
	addFail := report.Add
	if live {
		addFail = report.AddQuiet
	}
	verifyOne := func(j verifyJob) bool {
		out, err := executil.Run(chdmanPath, "verify", "-i", j.path)
		if err != nil {
			addFail(audit.Fail, audit.CategoryVerify, j.sys, j.label,
				fmt.Sprintf("failed data verify: %v: %s", err, strings.TrimSpace(string(out))), "")
			return false
		}
		return true
	}

	switch {
	case live:
		cliutil.ParallelLive(format, jobs, func(j verifyJob) string { return j.sys + "/" + j.label }, verifyOne)
	case verbose:
		for i, j := range jobs {
			fmt.Printf("[%d/%d] verify %s/%s\n", i+1, len(jobs), j.sys, j.label)
			verifyOne(j)
		}
	default:
		cliutil.Progress(format, false, false, "verify", len(jobs), func(tick func()) {
			cliutil.Parallel(jobs, func(j verifyJob) {
				verifyOne(j)
				tick()
			})
		})
	}

	if format.Structured() {
		return !report.Failed(), report.Encode(os.Stdout, format)
	}
	report.Table(os.Stdout)
	report.Fixes(os.Stdout)
	return !report.Failed(), nil
}
