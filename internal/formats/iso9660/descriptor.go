package iso9660

import (
	"bytes"
	"sort"
	"time"
)

// bothOrder32 encodes a 32-bit value in ISO9660's "both-byte-order" form:
// little-endian followed by big-endian (8 bytes total).
func bothOrder32(v uint32) []byte {
	b := make([]byte, 8)
	b[0], b[1], b[2], b[3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
	b[4], b[5], b[6], b[7] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
	return b
}

// bothOrder16 is bothOrder32's 16-bit counterpart (4 bytes total).
func bothOrder16(v uint16) []byte {
	b := make([]byte, 4)
	b[0], b[1] = byte(v), byte(v>>8)
	b[2], b[3] = byte(v>>8), byte(v)
	return b
}

func padded(s string, n int) []byte {
	b := bytes.Repeat([]byte{' '}, n)
	copy(b, s)
	if len(s) > n {
		copy(b, s[:n])
	}
	return b
}

// dirRecord builds one ISO9660 directory record for a child named name,
// occupying extent lba for extentBytes bytes (the full padded extent size
// for a directory, or the exact file size for a file).
func dirRecord(name string, lba, extentBytes uint32, isDir bool, when time.Time) []byte {
	idLen := len(name)
	recLen := 33 + idLen
	if recLen%2 == 1 {
		recLen++
	}
	rec := make([]byte, recLen)
	rec[0] = byte(recLen)
	rec[1] = 0 // extended attribute record length
	copy(rec[2:10], bothOrder32(lba))
	copy(rec[10:18], bothOrder32(extentBytes))
	rec[18] = byte(when.Year() - 1900)
	rec[19] = byte(when.Month())
	rec[20] = byte(when.Day())
	rec[21] = byte(when.Hour())
	rec[22] = byte(when.Minute())
	rec[23] = byte(when.Second())
	rec[24] = 0 // GMT offset
	if isDir {
		rec[25] = flagDirectory
	}
	rec[26] = 0 // file unit size
	rec[27] = 0 // interleave gap size
	copy(rec[28:32], bothOrder16(1))
	rec[32] = byte(idLen)
	copy(rec[33:33+idLen], name)
	return rec
}

// sectorsForDir computes how many sectors d's own directory extent needs,
// without requiring any LBA to have been assigned yet (record length only
// depends on name lengths).
func sectorsForDir(d *dirEntry) uint32 {
	return sectorsFor(len(renderDirRecords(d)))
}

// renderDirRecords builds d's full directory extent: "." and ".." first,
// then every child (directories and files together, sorted by name), never
// letting a record straddle a sector boundary.
func renderDirRecords(d *dirEntry) []byte {
	now := time.Now()
	selfSize := d.sectors * sectorSize
	parent := d.parentRef
	if parent == nil {
		parent = d
	}
	parentSize := parent.sectors * sectorSize

	type child struct {
		name string
		lba  uint32
		size uint32
		dir  bool
	}
	children := make([]child, 0, len(d.children)+len(d.files))
	for _, c := range d.children {
		children = append(children, child{name: c.name, lba: c.lba, size: c.sectors * sectorSize, dir: true})
	}
	for _, f := range d.files {
		children = append(children, child{name: f.name + ";1", lba: f.lba, size: uint32(f.size), dir: false})
	}
	sort.Slice(children, func(i, j int) bool { return children[i].name < children[j].name })

	var buf bytes.Buffer
	buf.Write(dirRecord("\x00", d.lba, selfSize, true, now))
	buf.Write(dirRecord("\x01", parent.lba, parentSize, true, now))
	for _, c := range children {
		rec := dirRecord(c.name, c.lba, c.size, c.dir, now)
		pos := buf.Len() % sectorSize
		if pos+len(rec) > sectorSize {
			buf.Write(make([]byte, sectorSize-pos))
		}
		buf.Write(rec)
	}
	if pad := sectorSize - buf.Len()%sectorSize; pad != sectorSize {
		buf.Write(make([]byte, pad))
	}
	return buf.Bytes()
}

// pathTableEntry builds one L-type (little-endian) or M-type (big-endian)
// path table entry for d.
func pathTableEntry(d *dirEntry, bigEndian bool) []byte {
	name := d.name
	idLen := 1
	if d.num != 1 {
		idLen = len(name)
	}
	entryLen := 8 + idLen
	rec := make([]byte, entryLen+entryLen%2)
	rec[0] = byte(idLen)
	rec[1] = 0
	if bigEndian {
		rec[2], rec[3], rec[4], rec[5] = byte(d.lba>>24), byte(d.lba>>16), byte(d.lba>>8), byte(d.lba)
		rec[6], rec[7] = byte(d.parentNum>>8), byte(d.parentNum)
	} else {
		rec[2], rec[3], rec[4], rec[5] = byte(d.lba), byte(d.lba>>8), byte(d.lba>>16), byte(d.lba>>24)
		rec[6], rec[7] = byte(d.parentNum), byte(d.parentNum>>8)
	}
	if d.num == 1 {
		rec[8] = 0
	} else {
		copy(rec[8:8+idLen], name)
	}
	return rec
}

// buildPathTables builds the L-type and M-type path tables for every
// directory in order (which must list parents before their children).
func buildPathTables(order []*dirEntry) (lBytes, mBytes []byte) {
	var l, m bytes.Buffer
	for _, d := range order {
		l.Write(pathTableEntry(d, false))
		m.Write(pathTableEntry(d, true))
	}
	return padToSector(l.Bytes()), padToSector(m.Bytes())
}

func padToSector(b []byte) []byte {
	if pad := sectorSize - len(b)%sectorSize; pad != sectorSize {
		b = append(b, make([]byte, pad)...)
	}
	return b
}

func dtField(t time.Time) []byte {
	return []byte(t.Format("20060102150405") + "00\x00")
}

// buildPVD builds the 2048-byte Primary Volume Descriptor.
func buildPVD(volumeID string, totalSectors uint32, root *dirEntry, lptLBA, mptLBA, pathTableSize uint32) []byte {
	b := make([]byte, sectorSize)
	b[0] = 1
	copy(b[1:6], "CD001")
	b[6] = 1
	copy(b[8:40], padded("", 32))
	copy(b[40:72], padded(volumeID, 32))
	copy(b[80:88], bothOrder32(totalSectors))
	copy(b[120:124], bothOrder16(1))
	copy(b[124:128], bothOrder16(1))
	copy(b[128:132], bothOrder16(sectorSize))
	copy(b[132:140], bothOrder32(pathTableSize))
	b[140], b[141], b[142], b[143] = byte(lptLBA), byte(lptLBA>>8), byte(lptLBA>>16), byte(lptLBA>>24)
	b[148], b[149], b[150], b[151] = byte(mptLBA>>24), byte(mptLBA>>16), byte(mptLBA>>8), byte(mptLBA)
	copy(b[156:190], dirRecord("\x00", root.lba, root.sectors*sectorSize, true, time.Now()))
	copy(b[190:318], padded("", 128))
	copy(b[318:446], padded("", 128))
	copy(b[446:574], padded("", 128))
	copy(b[574:702], padded("", 128))
	copy(b[702:739], padded("", 37))
	copy(b[739:776], padded("", 37))
	copy(b[776:813], padded("", 37))
	now := dtField(time.Now())
	copy(b[813:830], now)
	copy(b[830:847], now)
	copy(b[847:864], []byte("0000000000000000\x00"))
	copy(b[864:881], now)
	b[881] = 1
	return b
}

// buildTerminator builds the Volume Descriptor Set Terminator.
func buildTerminator() []byte {
	b := make([]byte, sectorSize)
	b[0] = 255
	copy(b[1:6], "CD001")
	b[6] = 1
	return b
}
