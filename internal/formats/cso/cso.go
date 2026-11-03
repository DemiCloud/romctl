// Package cso decompresses PSP CISO/ZSO disc images (the ".cso" files
// produced by tools like ciso and maxcso), replacing the Bash tool's
// dependency on maxcso --decompress. It supports the common CISO v1/v2
// layout (raw-deflate blocks) and the ZSO variant (LZ4 blocks); DAX, a
// different and much rarer PSP compression format, is out of scope.
package cso

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/pierrec/lz4/v4"
)

const (
	notCompressedBit = 0x80000000
	offsetMask       = 0x7fffffff
)

type header struct {
	magic      [4]byte
	headerSize uint32
	totalBytes uint64
	blockSize  uint32
	version    uint8
	align      uint8
}

// Decompress reads a CISO/ZSO image at path and writes the decompressed
// disc image to out.
func Decompress(path string, out *os.File) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var h header
	raw := make([]byte, 24)
	if _, err := io.ReadFull(f, raw); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	copy(h.magic[:], raw[0:4])
	h.headerSize = binary.LittleEndian.Uint32(raw[4:8])
	h.totalBytes = binary.LittleEndian.Uint64(raw[8:16])
	h.blockSize = binary.LittleEndian.Uint32(raw[16:20])
	h.version = raw[20]
	h.align = raw[21]

	magic := string(h.magic[:])
	if magic != "CISO" && magic != "ZISO" {
		return fmt.Errorf("%s: not a recognized CSO/ZSO image (magic %q)", path, magic)
	}
	if h.blockSize == 0 {
		return fmt.Errorf("%s: invalid block size 0", path)
	}

	numBlocks := int(h.totalBytes / uint64(h.blockSize))
	if h.totalBytes%uint64(h.blockSize) != 0 {
		numBlocks++
	}
	index := make([]uint32, numBlocks+1)
	indexRaw := make([]byte, 4*len(index))
	if _, err := f.ReadAt(indexRaw, int64(h.headerSize)); err != nil {
		return fmt.Errorf("%s: reading block index: %w", path, err)
	}
	for i := range index {
		index[i] = binary.LittleEndian.Uint32(indexRaw[i*4 : i*4+4])
	}

	if err := out.Truncate(int64(h.totalBytes)); err != nil {
		return err
	}

	remaining := int64(h.totalBytes)
	for i := 0; i < numBlocks; i++ {
		entry := index[i]
		next := index[i+1]
		notCompressed := entry&notCompressedBit != 0
		start := int64(entry&offsetMask) << h.align
		end := int64(next&offsetMask) << h.align
		if end < start {
			return fmt.Errorf("%s: block %d: corrupt index (end %d before start %d)", path, i, end, start)
		}

		outLen := int64(h.blockSize)
		if remaining < outLen {
			outLen = remaining
		}

		block := make([]byte, end-start)
		if _, err := f.ReadAt(block, start); err != nil {
			return fmt.Errorf("%s: block %d: %w", path, i, err)
		}

		var plain []byte
		switch {
		case notCompressed:
			plain = block
		case magic == "ZISO":
			plain = make([]byte, outLen)
			n, err := lz4.UncompressBlock(block, plain)
			if err != nil {
				return fmt.Errorf("%s: block %d: lz4: %w", path, i, err)
			}
			plain = plain[:n]
		default:
			r := flate.NewReader(bytes.NewReader(block))
			buf := make([]byte, outLen)
			if _, err := io.ReadFull(r, buf); err != nil && err != io.ErrUnexpectedEOF {
				return fmt.Errorf("%s: block %d: deflate: %w", path, i, err)
			}
			r.Close()
			plain = buf
		}

		if int64(len(plain)) > outLen {
			plain = plain[:outLen]
		}
		if _, err := out.WriteAt(plain, int64(i)*int64(h.blockSize)); err != nil {
			return fmt.Errorf("%s: writing block %d: %w", path, i, err)
		}
		remaining -= outLen
	}
	return nil
}
