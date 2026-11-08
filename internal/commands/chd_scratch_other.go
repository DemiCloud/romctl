//go:build !linux

package commands

// No RAM-backed scratch directory on these platforms: Windows and macOS
// have no universal tmpfs-like equivalent to detect without guessing, so
// chdScratchDir's callers always fall back to staging next to the final
// destination, same as before this existed.
func devShmDir() string {
	return ""
}

func availableSpace(dir string) (int64, bool) {
	return 0, false
}
