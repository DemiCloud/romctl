package iso9660

import (
	"io"
	"os"
	"path/filepath"
	"sort"
)

// flagDirectory is the ISO9660 directory-record file-flag bit marking an
// entry as a directory.
const flagDirectory = 1 << 1

type dirEntry struct {
	name      string
	children  []*dirEntry // sorted, directories only
	files     []*fileEntry
	num       int // 1-based path table number, root is 1
	parentNum int
	parentRef *dirEntry
	lba       uint32
	sectors   uint32
}

type fileEntry struct {
	name string
	path string // source path on disk
	size int64
	lba  uint32
}

// Build creates a plain ISO9660 (ECMA-119, Level 2, ISO 646/relaxed names)
// image of srcDir's contents and writes it to out, replacing the Bash
// tool's dependency on makeps3iso. File and directory names are copied
// through as-is (any byte but NUL and '/'); PS3 disc folders are already
// ISO9660-safe names extracted from a real disc, so strict d-character
// validation would only get in the way.
func Build(srcDir string, out *os.File, volumeID string) error {
	root := &dirEntry{name: "", num: 1, parentNum: 1}
	if err := scanDir(srcDir, root); err != nil {
		return err
	}

	order := bfsOrder(root)
	for _, d := range order {
		d.sectors = sectorsForDir(d)
	}

	// Layout: system area (16) + PVD (1) + terminator (1) + path tables,
	// then every directory's own extent (in BFS order), then file data.
	lba := uint32(18)
	lPT, mPT := buildPathTables(order)
	lptLBA := lba
	lba += sectorsFor(len(lPT))
	mptLBA := lba
	lba += sectorsFor(len(mPT))

	for _, d := range order {
		d.lba = lba
		lba += d.sectors
	}
	for _, d := range order {
		for _, f := range d.files {
			f.lba = lba
			lba += sectorsFor64(f.size)
		}
	}
	totalSectors := lba

	if err := out.Truncate(int64(totalSectors) * sectorSize); err != nil {
		return err
	}

	for _, d := range order {
		rec := renderDirRecords(d)
		if _, err := out.WriteAt(rec, int64(d.lba)*sectorSize); err != nil {
			return err
		}
	}
	for _, d := range order {
		for _, f := range d.files {
			if err := copyFileData(f, out); err != nil {
				return err
			}
		}
	}

	if _, err := out.WriteAt(lPT, int64(lptLBA)*sectorSize); err != nil {
		return err
	}
	if _, err := out.WriteAt(mPT, int64(mptLBA)*sectorSize); err != nil {
		return err
	}

	pvd := buildPVD(volumeID, totalSectors, root, lptLBA, mptLBA, uint32(len(lPT)))
	if _, err := out.WriteAt(pvd, pvdSector*sectorSize); err != nil {
		return err
	}
	term := buildTerminator()
	if _, err := out.WriteAt(term, (pvdSector+1)*sectorSize); err != nil {
		return err
	}
	return nil
}

func scanDir(path string, d *dirEntry) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		full := filepath.Join(path, e.Name())
		if e.IsDir() {
			child := &dirEntry{name: e.Name()}
			if err := scanDir(full, child); err != nil {
				return err
			}
			d.children = append(d.children, child)
			continue
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		d.files = append(d.files, &fileEntry{name: e.Name(), path: full, size: info.Size()})
	}
	return nil
}

// bfsOrder numbers directories breadth-first (root=1), the order ECMA-119
// requires for the path table: every parent precedes its children.
func bfsOrder(root *dirEntry) []*dirEntry {
	order := []*dirEntry{root}
	queue := []*dirEntry{root}
	next := 2
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, c := range cur.children {
			c.num = next
			next++
			c.parentNum = cur.num
			c.parentRef = cur
			order = append(order, c)
			queue = append(queue, c)
		}
	}
	return order
}

func sectorsFor(n int) uint32 { return sectorsFor64(int64(n)) }
func sectorsFor64(n int64) uint32 {
	if n <= 0 {
		return 1
	}
	return uint32((n + sectorSize - 1) / sectorSize)
}

func copyFileData(f *fileEntry, out *os.File) error {
	src, err := os.Open(f.path)
	if err != nil {
		return err
	}
	defer src.Close()

	w := io.NewOffsetWriter(out, int64(f.lba)*sectorSize)
	if _, err := io.Copy(w, src); err != nil {
		return err
	}
	// Zero-pad to a full sector so the next extent starts cleanly.
	if pad := int64(sectorsFor64(f.size))*sectorSize - f.size; pad > 0 {
		if _, err := w.Write(make([]byte, pad)); err != nil {
			return err
		}
	}
	return nil
}
