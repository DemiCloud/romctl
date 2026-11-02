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
