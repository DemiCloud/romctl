package commands

import (
	"os"
	"sync/atomic"
)

// chdScratchMargin is added on top of the candidate CHD's size estimate
// (the uncompressed input's size - a CHD is never larger than its source)
// when deciding whether a RAM-backed scratch directory has room for it.
const chdScratchMargin = 64 << 20 // 64MiB

// scratchReserved tracks bytes currently claimed by in-flight chdScratchDir
// callers, so concurrent CHD jobs (see CHD/DVD's doc comments on running a
// system's files in parallel) don't all pass an availableSpace check against
// the same stale free-space reading and collectively overrun a RAM-backed
// scratch directory. The check-then-reserve pair below is best-effort, not
// a hard guarantee: a race between two callers' free-space reads is still
// possible, but its failure mode is an ordinary, safely-handled chdman
// error (ENOSPC) on one job, not data loss - see makeCHD.
var scratchReserved atomic.Int64

// chdScratchDir reserves room for a sizeHint-byte candidate CHD on a
// RAM-backed scratch directory and returns that directory plus a release
// func the caller must call exactly once when done with it, win or lose.
// It returns ("", nil) when no such directory exists, or when there isn't
// confidently enough free space - the caller's fallback in that case is to
// stage next to the final destination, as it always has.
//
// This exists because chdman's createcd/createdvd write the candidate CHD,
// then a second pass reads it all back to verify before the caller moves
// it into place - two full passes over the slow medium when the ROM
// library itself is the bottleneck (an SD card), even though the data is
// about to be copied right back onto it anyway. Staging on tmpfs instead
// removes that redundant read. It's deliberately conservative: tmpfs is
// RAM, not just another disk, and PS2/PS3-class CHDs can be multiple GB on
// the same RAM-constrained device (a Steam Deck) this is meant to help, so
// it only engages when there's room to spare.
func chdScratchDir(sizeHint int64) (dir string, release func()) {
	shm := devShmDir()
	if shm == "" {
		return "", nil
	}
	if info, err := os.Stat(shm); err != nil || !info.IsDir() {
		return "", nil
	}
	free, ok := availableSpace(shm)
	if !ok {
		return "", nil
	}
	need := sizeHint + chdScratchMargin
	if free-scratchReserved.Load() < need {
		return "", nil
	}
	scratchReserved.Add(need)
	return shm, func() { scratchReserved.Add(-need) }
}
