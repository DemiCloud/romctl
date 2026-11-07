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
	return buildCISOHeaderSize(t, plain, blockSize, 24)
}

// buildCISOHeaderSize is buildCISO with the header_size field set to an
// arbitrary value, independent of the real (always 24-byte) on-disk
// header layout - for exercising encoders that write 0 there instead of
// the struct's actual size (see TestDecompressZeroHeaderSizeField).
func buildCISOHeaderSize(t *testing.T, plain []byte, blockSize uint32, headerSizeField uint32) string {
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
	binary.LittleEndian.PutUint32(header[4:8], headerSizeField)
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

// TestDecompressZeroHeaderSizeField guards against a regression: some
// real-world CISO encoders write 0 in the header_size field instead of
// the struct's actual 24-byte size. Trusting that literal value made the
// block index get read starting at the magic bytes themselves, producing
// a bogus first entry ("CISO" read back as a little-endian uint32) and a
// "corrupt index" error on every block.
func TestDecompressZeroHeaderSizeField(t *testing.T) {
	plain := bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog "), 100)
	path := buildCISOHeaderSize(t, plain, 2048, 0)

	out := mustCreateTemp(t)
	if err := Decompress(path, out); err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	out.Close()

	got, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("decompressed %d bytes, want %d bytes to match", len(got), len(plain))
	}
}

// writeHeaderOnly writes a minimal CISO header (no index, no block data) so
// a malformed-field test can control exactly one value in isolation.
func writeHeaderOnly(t *testing.T, totalBytes uint64, blockSize uint32) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "malformed.cso")
	header := make([]byte, 24)
	copy(header[0:4], "CISO")
	binary.LittleEndian.PutUint32(header[4:8], 24)
	binary.LittleEndian.PutUint64(header[8:16], totalBytes)
	binary.LittleEndian.PutUint32(header[16:20], blockSize)
	header[20] = 1
	header[21] = 0
	if err := os.WriteFile(path, header, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestDecompressMalformedInputsReturnErrors confirms that corrupt/malicious
// header and index fields produce a plain error rather than a panic
// (division by zero on a zero block size, or a negative-length slice from
// an index entry that runs backwards).
func TestDecompressMalformedInputsReturnErrors(t *testing.T) {
	t.Run("zero block size", func(t *testing.T) {
		path := writeHeaderOnly(t, 100, 0)
		out := mustCreateTemp(t)
		if err := Decompress(path, out); err == nil {
			t.Fatal("Decompress with blockSize=0: expected an error, got nil")
		}
	})

	t.Run("not CISO/ZISO", func(t *testing.T) {
		path := writeHeaderOnly(t, 100, 2048)
		data, _ := os.ReadFile(path)
		copy(data[0:4], "JUNK")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		out := mustCreateTemp(t)
		if err := Decompress(path, out); err == nil {
			t.Fatal("Decompress with bad magic: expected an error, got nil")
		}
	})

	t.Run("index entry runs backwards", func(t *testing.T) {
		// One block (totalBytes == blockSize) whose index has entry > next,
		// so end-start underflows to a negative slice length.
		path := writeHeaderOnly(t, 2048, 2048)
		f, err := os.OpenFile(path, os.O_RDWR, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		index := make([]byte, 8)
		binary.LittleEndian.PutUint32(index[0:4], 100) // entry
		binary.LittleEndian.PutUint32(index[4:8], 50)  // next < entry
		if _, err := f.WriteAt(index, 24); err != nil {
			t.Fatal(err)
		}
		f.Close()

		out := mustCreateTemp(t)
		if err := Decompress(path, out); err == nil {
			t.Fatal("Decompress with a backwards index entry: expected an error, got nil")
		}
	})
}

func mustCreateTemp(t *testing.T) *os.File {
	t.Helper()
	out, err := os.Create(filepath.Join(t.TempDir(), "out.iso"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { out.Close() })
	return out
}
