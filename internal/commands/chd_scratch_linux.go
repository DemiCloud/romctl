//go:build linux

package commands

import "syscall"

// devShmDir is tmpfs on every mainline Linux distribution, including
// SteamOS - no config or detection needed.
func devShmDir() string {
	return "/dev/shm"
}

func availableSpace(dir string) (int64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, false
	}
	return int64(st.Bavail) * st.Bsize, true
}
