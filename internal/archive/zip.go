package archive

import (
	"archive/zip"
	"fmt"
	"io"
)

type zipArchive struct {
	rc *zip.ReadCloser
}

func openZip(path string) (Archive, error) {
	rc, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &zipArchive{rc: rc}, nil
}

func (a *zipArchive) Entries() ([]Entry, error) {
	entries := make([]Entry, 0, len(a.rc.File))
	for _, f := range a.rc.File {
		entries = append(entries, Entry{Name: f.Name, Size: int64(f.UncompressedSize64)})
	}
	return entries, nil
}

func (a *zipArchive) Extract(name string, w io.Writer) error {
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

func (a *zipArchive) Close() error { return a.rc.Close() }
