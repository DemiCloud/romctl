package archive

import (
	"fmt"
	"io"

	"github.com/bodgit/sevenzip"
)

type sevenzipArchive struct {
	rc *sevenzip.ReadCloser
}

func open7z(path string) (Archive, error) {
	rc, err := sevenzip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &sevenzipArchive{rc: rc}, nil
}

func (a *sevenzipArchive) Entries() ([]Entry, error) {
	entries := make([]Entry, 0, len(a.rc.File))
	for _, f := range a.rc.File {
		if f.FileInfo().IsDir() {
			continue
		}
		entries = append(entries, Entry{Name: f.Name, Size: int64(f.UncompressedSize)})
	}
	return entries, nil
}

func (a *sevenzipArchive) Extract(name string, w io.Writer) error {
	for _, f := range a.rc.File {
		if f.Name != name {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		defer r.Close()
		_, err = io.Copy(w, r)
		return err
	}
	return fmt.Errorf("entry not found: %s", name)
}

func (a *sevenzipArchive) Close() error { return a.rc.Close() }
