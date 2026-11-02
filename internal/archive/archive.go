// Package archive lists and extracts .zip, .7z and .rar files with pure
// Go decoders, replacing the Bash tool's dependency on an external 7z
// binary for "unpack", "chd" (archived disc images) and "ps3" (archived
// dumps).
package archive

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Entry is one file inside an archive.
type Entry struct {
	Name string // path within the archive, forward-slash separated
	Size int64
}

// Archive lists and extracts entries from a single archive file.
type Archive interface {
	Entries() ([]Entry, error)
	Extract(name string, w io.Writer) error
	Close() error
}

// Open opens path (.zip, .7z or .rar, by extension) for listing/extraction.
func Open(path string) (Archive, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip":
		return openZip(path)
	case ".7z":
		return open7z(path)
	case ".rar":
		return openRar(path)
	default:
		return nil, fmt.Errorf("%s: unsupported archive type", path)
	}
}

// ExtractAll extracts every entry of path into destDir, preserving the
// archive's relative paths.
func ExtractAll(path, destDir string) error {
	a, err := Open(path)
	if err != nil {
		return err
	}
	defer a.Close()

	entries, err := a.Entries()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name, "/") || e.Name == "" {
			continue
		}
		dest := filepath.Join(destDir, filepath.FromSlash(e.Name))
		if !strings.HasPrefix(dest, filepath.Clean(destDir)+string(filepath.Separator)) {
			return fmt.Errorf("%s: entry escapes destination: %s", path, e.Name)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		f, err := os.Create(dest)
		if err != nil {
			return err
		}
		err = a.Extract(e.Name, f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %s: %w", path, e.Name, err)
		}
	}
	return nil
}

// IsSingleFile reports whether path contains exactly one non-directory
// entry (the "unpack" command refuses multi-file archives).
func IsSingleFile(path string) (bool, error) {
	a, err := Open(path)
	if err != nil {
		return false, err
	}
	defer a.Close()
	entries, err := a.Entries()
	if err != nil {
		return false, err
	}
	n := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name, "/") {
			n++
		}
	}
	return n == 1, nil
}
