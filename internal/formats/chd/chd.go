// Package chd parses just enough of the MAME CHD container format to
// classify an existing file (chd_kind in the Bash tool): CD vs DVD vs GD,
// a CD image stored with a single MODE1/2048 track, or an old/invalid file.
// Actually creating or verifying CHDs still goes through the embedded
// chdman binary (internal/tools); this package only reads headers.
package chd

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"regexp"
)

// Kind is the result of classifying a CHD file.
type Kind string

const (
	KindBad         Kind = "bad"         // not a CHD, or unreadable.
	KindDVD         Kind = "dvd"         // DVD-type CHD.
	KindGD          Kind = "gd"          // GD-ROM (Dreamcast) CHD.
	KindCD          Kind = "cd"          // ordinary multi-track CD CHD.
	KindCDAsDVDMAGE Kind = "cd-dvdimage" // single MODE1/2048 track: a DVD image stored as a CD-type CHD.
)

// OldVersion returns a Kind like "v4" for a CHD whose header version isn't 5.
func OldVersion(v uint32) Kind { return Kind(fmt.Sprintf("v%d", v)) }

var (
	magic   = []byte("MComprHD")
	mode1RE = regexp.MustCompile(`(^|\s)TYPE:MODE1\s`)
)

const (
	metaTagDVD  = "DVD "
	metaTagGD   = "CHGD"
	metaTagCHT2 = "CHT2"
	metaTagCHTR = "CHTR"
	metaTagCHCD = "CHCD"
)

// Classify reads path's CHD header and metadata chain and returns its Kind.
func Classify(path string) (Kind, error) {
	f, err := os.Open(path)
	if err != nil {
		return KindBad, err
	}
	defer f.Close()

	header := make([]byte, 124)
	if _, err := f.ReadAt(header, 0); err != nil {
		return KindBad, nil
	}
	if !bytes.Equal(header[0:8], magic) {
		return KindBad, nil
	}
	version := binary.BigEndian.Uint32(header[12:16])
	if version != 5 {
		return OldVersion(version), nil
	}

	metaOffset := binary.BigEndian.Uint64(header[48:56])
	tracks := 0
	mode1 := false
	for guard := 0; metaOffset > 0 && guard < 256; guard++ {
		entry := make([]byte, 16)
		if _, err := f.ReadAt(entry, int64(metaOffset)); err != nil {
			break
		}
		tag := string(entry[0:4])
		length := uint32(entry[5])<<16 | uint32(entry[6])<<8 | uint32(entry[7])
		next := binary.BigEndian.Uint64(entry[8:16])

		switch tag {
		case metaTagDVD:
			return KindDVD, nil
		case metaTagGD:
			return KindGD, nil
		case metaTagCHT2, metaTagCHTR, metaTagCHCD:
			tracks++
			if tracks == 1 {
				data := make([]byte, length)
				if _, err := f.ReadAt(data, int64(metaOffset)+16); err == nil {
					data = bytes.ReplaceAll(data, []byte{0}, nil)
					if mode1RE.MatchString(string(data)) {
						mode1 = true
					}
				}
			}
		}
		metaOffset = next
	}

	if tracks == 1 && mode1 {
		return KindCDAsDVDMAGE, nil
	}
	return KindCD, nil
}

// LogicalSize returns a v5 CHD's logical (uncompressed) byte count,
// straight out of its header - used to size a scratch-space request
// before extracting the file (see commands.hashDiscCHD). Errors for
// anything Classify would call KindBad or an old (non-v5) version; a
// caller only ever calls this after Classify has already confirmed the
// file is a real, current-version CHD.
func LogicalSize(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	header := make([]byte, 124)
	if _, err := f.ReadAt(header, 0); err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	if !bytes.Equal(header[0:8], magic) {
		return 0, fmt.Errorf("%s: not a CHD", path)
	}
	version := binary.BigEndian.Uint32(header[12:16])
	if version != 5 {
		return 0, fmt.Errorf("%s: CHD version %d, want 5", path, version)
	}
	return int64(binary.BigEndian.Uint64(header[32:40])), nil
}
