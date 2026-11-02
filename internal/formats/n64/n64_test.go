package n64

import (
	"bytes"
	"os"
	"testing"
)

func TestDetectAndConvert(t *testing.T) {
	z64 := []byte{0x80, 0x37, 0x12, 0x40, 0x01, 0x02, 0x03, 0x04}
	v64 := []byte{0x37, 0x80, 0x40, 0x12, 0x02, 0x01, 0x04, 0x03}
	nn64 := []byte{0x40, 0x12, 0x37, 0x80, 0x04, 0x03, 0x02, 0x01}

	for _, tc := range []struct {
		name string
		data []byte
		want Order
	}{
		{"z64", z64, OrderZ64},
		{"v64", v64, OrderV64},
		{"n64", nn64, OrderN64},
	} {
		path := writeTemp(t, tc.data)
		order, err := Detect(path)
		if err != nil {
			t.Fatalf("%s: Detect: %v", tc.name, err)
		}
		if order != tc.want {
			t.Fatalf("%s: Detect = %v, want %v", tc.name, order, tc.want)
		}
		got, err := ToBigEndian(tc.data, order)
		if err != nil {
			t.Fatalf("%s: ToBigEndian: %v", tc.name, err)
		}
		if !bytes.Equal(got, z64) {
			t.Fatalf("%s: ToBigEndian = %x, want %x", tc.name, got, z64)
		}
	}
}

func writeTemp(t *testing.T, data []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "n64")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}
