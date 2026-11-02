package archive

import (
	"fmt"
	"io"

	"github.com/nwaples/rardecode/v2"
)

// rarArchive lists via a single List() call, but re-opens and replays the
// archive from the start on every Extract() call: RAR solid archives
// decode sequentially, so an entry's content can depend on every entry
// before it having been decoded first.
type rarArchive struct {
	path string
}

func openRar(path string) (Archive, error) {
	if _, err := rardecode.List(path); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &rarArchive{path: path}, nil
}

func (a *rarArchive) Entries() ([]Entry, error) {
	files, err := rardecode.List(a.path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", a.path, err)
	}
	entries := make([]Entry, 0, len(files))
	for _, f := range files {
		if f.IsDir {
			continue
		}
		entries = append(entries, Entry{Name: f.Name, Size: f.UnPackedSize})
	}
	return entries, nil
}

func (a *rarArchive) Extract(name string, w io.Writer) error {
	rc, err := rardecode.OpenReader(a.path)
	if err != nil {
		return fmt.Errorf("%s: %w", a.path, err)
	}
	defer rc.Close()

	for {
		h, err := rc.Next()
		if err == io.EOF {
			return fmt.Errorf("entry not found: %s", name)
		}
		if err != nil {
			return err
		}
		if h.IsDir || h.Name != name {
			continue
		}
		_, err = io.Copy(w, &rc.Reader)
		return err
	}
}

func (a *rarArchive) Close() error { return nil }
