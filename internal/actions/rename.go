package actions

import (
	"fmt"
	"os"
)

// RenamePath renames src to dst, handling case-only renames on
// case-insensitive filesystems (NTFS, exFAT, APFS by default) where src and
// dst resolve to the same file: a direct rename would otherwise be a no-op
// on some of those filesystems, so it goes through a temporary name first.
func RenamePath(src, dst string) error {
	if src == dst {
		return nil
	}
	if same, _ := sameFile(src, dst); same {
		tmp := fmt.Sprintf("%s.romctl-rename.%d", src, os.Getpid())
		if err := os.Rename(src, tmp); err != nil {
			return err
		}
		if err := os.Rename(tmp, dst); err != nil {
			_ = os.Rename(tmp, src)
			return err
		}
		return nil
	}
	return os.Rename(src, dst)
}

func sameFile(a, b string) (bool, error) {
	fa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(fa, fb), nil
}
