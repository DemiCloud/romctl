package cso

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildCISO writes a minimal synthetic CISO file wrapping plain, using
// raw-deflate blocks, so Decompress can be tested without a real tool.
func buildCISO(t *testing.T, plain []byte, blockSize uint32) string {
	t.Helper()
	numBlocks := (len(plain) + int(blockSize) - 1) / int(blockSize)

	var blocks [][]byte
	for i := 0; i < numBlocks; i++ {
		start := i * int(blockSize)
		end := min(start+int(blockSize), len(plain))
		var buf bytes.Buffer
		w, err := flate.NewWriter(&buf, flate.BestSpeed)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(plain[start:end]); err != nil {
			t.Fatal(err)
		}
		w.Close()
		blocks = append(blocks, buf.Bytes())
	}

	headerSize := 24
	index := make([]uint32, numBlocks+1)
	offset := uint32(headerSize + 4*len(index))
	for i, b := range blocks {
		index[i] = offset
		offset += uint32(len(b))
	}
	index[numBlocks] = offset

	path := filepath.Join(t.TempDir(), "test.cso")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	header := make([]byte, headerSize)
	copy(header[0:4], "CISO")
	binary.LittleEndian.PutUint32(header[4:8], uint32(headerSize))
	binary.LittleEndian.PutUint64(header[8:16], uint64(len(plain)))
	binary.LittleEndian.PutUint32(header[16:20], blockSize)
	header[20] = 1 // version
	header[21] = 0 // align shift (offsets are exact bytes)
	f.Write(header)

	indexRaw := make([]byte, 4*len(index))
	for i, v := range index {
		binary.LittleEndian.PutUint32(indexRaw[i*4:i*4+4], v)
	}
	f.Write(indexRaw)

	for _, b := range blocks {
		f.Write(b)
	}
	return path
}

func TestDecompress(t *testing.T) {
	plain := bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog "), 100)
	path := buildCISO(t, plain, 2048)

	outPath := filepath.Join(t.TempDir(), "out.iso")
	out, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Decompress(path, out); err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	out.Close()

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("decompressed %d bytes, want %d bytes to match", len(got), len(plain))
	}
}
