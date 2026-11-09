package chd

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func writeFakeCHD(t *testing.T, version uint32, metaTag string) string {
	t.Helper()
	buf := make([]byte, 124+16)
	copy(buf[0:8], "MComprHD")
	binary.BigEndian.PutUint32(buf[12:16], version)
	if version == 5 && metaTag != "" {
		binary.BigEndian.PutUint64(buf[48:56], 124)
		copy(buf[124:128], metaTag)
		// flags(1) + length(3) = 0, next(8) = 0
	}
	path := filepath.Join(t.TempDir(), "test.chd")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFakeCHDWithSize(t *testing.T, version uint32, logicalSize uint64) string {
	t.Helper()
	buf := make([]byte, 124)
	copy(buf[0:8], "MComprHD")
	binary.BigEndian.PutUint32(buf[12:16], version)
	binary.BigEndian.PutUint64(buf[32:40], logicalSize)
	path := filepath.Join(t.TempDir(), "sized.chd")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLogicalSize(t *testing.T) {
	path := writeFakeCHDWithSize(t, 5, 123456789)
	got, err := LogicalSize(path)
	if err != nil {
		t.Fatalf("LogicalSize: %v", err)
	}
	if got != 123456789 {
		t.Errorf("LogicalSize = %d, want 123456789", got)
	}
}

func TestLogicalSizeRejectsOldVersion(t *testing.T) {
	path := writeFakeCHDWithSize(t, 4, 1000)
	if _, err := LogicalSize(path); err == nil {
		t.Fatal("expected an error for a non-v5 CHD")
	}
}

func TestLogicalSizeRejectsBadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.chd")
	if err := os.WriteFile(path, []byte("not a chd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LogicalSize(path); err == nil {
		t.Fatal("expected an error for a non-CHD file")
	}
}

func TestClassify(t *testing.T) {
	if k, err := Classify(writeFakeCHD(t, 5, "DVD ")); err != nil || k != KindDVD {
		t.Fatalf("DVD: got %v, %v", k, err)
	}
	if k, err := Classify(writeFakeCHD(t, 5, "CHGD")); err != nil || k != KindGD {
		t.Fatalf("GD: got %v, %v", k, err)
	}
	if k, err := Classify(writeFakeCHD(t, 4, "")); err != nil || k != OldVersion(4) {
		t.Fatalf("v4: got %v, %v", k, err)
	}

	bad := filepath.Join(t.TempDir(), "bad.chd")
	if err := os.WriteFile(bad, []byte("not a chd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if k, err := Classify(bad); err != nil || k != KindBad {
		t.Fatalf("bad: got %v, %v", k, err)
	}
}
