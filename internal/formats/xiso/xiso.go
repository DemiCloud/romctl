// Package xiso detects the Xbox XISO filesystem magic.
package xiso

import (
	"bytes"
	"os"
)

var magic = []byte("MICROSOFT*XBOX*MEDIA")

const magicOffset = 65536

// IsXISO reports whether path has the XISO magic at the expected offset.
func IsXISO(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, len(magic))
	if _, err := f.ReadAt(buf, magicOffset); err != nil {
		return false, nil
	}
	return bytes.Equal(buf, magic), nil
}
