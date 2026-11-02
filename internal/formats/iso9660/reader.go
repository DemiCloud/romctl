// Package iso9660 reads and writes plain ISO9660 (ECMA-119) disc images:
// enough to validate an existing image (Complete, replacing the Bash
// tool's iso_complete) and to build one from a PS3 disc folder (Writer,
// replacing makeps3iso). No Joliet, Rock Ridge or UDF bridging — PS3 disc
// dumps are plain ISO9660, which is also all this package's own reader
// validates.
package iso9660

import (
	"encoding/binary"
	"os"
)

const (
	sectorSize = 2048
	pvdSector  = 16
	pvdOffset  = pvdSector * sectorSize
	volSizeOff = 80 // offset within the PVD of the volume space size field
)

// Complete reports whether path looks like a complete ISO9660 image: a
// primary volume descriptor is present at sector 16, and its declared
// volume size matches the file's actual size.
func Complete(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	magic := make([]byte, 5)
	if _, err := f.ReadAt(magic, pvdOffset+1); err != nil {
		return false, nil
	}
	if string(magic) != "CD001" {
		return false, nil
	}

	volSize := make([]byte, 4)
	if _, err := f.ReadAt(volSize, pvdOffset+volSizeOff); err != nil {
		return false, nil
	}
	sectors := binary.LittleEndian.Uint32(volSize) // both-endian field; LE half is first

	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	return int64(sectors)*sectorSize == info.Size(), nil
}
