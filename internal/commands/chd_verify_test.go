package commands

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/datdb"
	"github.com/demicloud/romctl/internal/systemdb"
)

// writeFakeCHDHeader writes just enough of a real v5 CHD header (magic,
// version, logical size) for formats/chd.LogicalSize to succeed, at
// path. hashDiscCHD's own extraction step goes through the stand-in
// chdman, which never actually reads this file's contents - only its
// LogicalSize-readable header needs to be real.
func writeFakeCHDHeader(path string, logicalSize int64) {
	buf := make([]byte, 124)
	copy(buf[0:8], "MComprHD")
	binary.BigEndian.PutUint32(buf[12:16], 5)
	binary.BigEndian.PutUint64(buf[32:40], uint64(logicalSize))
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		panic(err)
	}
}

// writeFakeGDHeader writes a minimal real v5 CHD header carrying a
// "CHGD" metadata tag - the same shape formats/chd.Classify reads to
// return KindGD, confirmed against the chd package's own equivalent
// test fixture (writeFakeCHD in formats/chd's tests, not exported).
func writeFakeGDHeader(path string) {
	buf := make([]byte, 124+16)
	copy(buf[0:8], "MComprHD")
	binary.BigEndian.PutUint32(buf[12:16], 5)
	binary.BigEndian.PutUint64(buf[48:56], 124) // metaOffset
	copy(buf[124:128], "CHGD")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		panic(err)
	}
}

// fakeChdmanExtract stands in for chdman's extractcd/extractdvd: it
// writes cueContent (extractcd only) and binContent to whatever paths
// follow "-o"/"-ob" on its command line, regardless of the input CHD
// (which never needs to be real for this stand-in). Content is
// round-tripped through base64 in the generated script so arbitrary
// bytes survive shell quoting intact.
func fakeChdmanExtract(t *testing.T, cueContent string, binContent []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "chdman")
	script := fmt.Sprintf(`#!/bin/sh
mode="$1"; shift
prev=""
out=""
ob=""
for arg in "$@"; do
  case "$prev" in
    -o) out="$arg" ;;
    -ob) ob="$arg" ;;
  esac
  prev="$arg"
done
case "$mode" in
  extractcd)
    echo %s | base64 -d > "$out"
    echo %s | base64 -d > "$ob"
    ;;
  extractdvd)
    echo %s | base64 -d > "$out"
    ;;
esac
exit 0
`, base64.StdEncoding.EncodeToString([]byte(cueContent)),
		base64.StdEncoding.EncodeToString(binContent),
		base64.StdEncoding.EncodeToString(binContent))
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHashDiscCHDSingleTrackCD(t *testing.T) {
	trackData := []byte("single-track-cd-data-bytes-padded-out")
	cue := `FILE "x.bin" BINARY
  TRACK 01 MODE1/2048
    INDEX 01 00:00:00
`
	chdman := fakeChdmanExtract(t, cue, trackData)
	chdPath := filepath.Join(t.TempDir(), "game.chd")
	if err := os.WriteFile(chdPath, []byte("not a real chd, fake chdman ignores it"), 0o644); err != nil {
		t.Fatal(err)
	}

	hashes, err := hashDiscCHDWithFakeSize(chdman, chdPath, systemdb.ClassCD, int64(len(trackData)))
	if err != nil {
		t.Fatalf("hashDiscCHD: %v", err)
	}
	if len(hashes) != 1 {
		t.Fatalf("len(hashes) = %d, want 1", len(hashes))
	}
	want := mustHashReader(t, trackData)
	if hashes[0] != want {
		t.Errorf("hashes[0] = %+v, want %+v", hashes[0], want)
	}
}

func TestHashDiscCHDMultiTrackCD(t *testing.T) {
	track1 := repeatByte(0xAA, 4*2352)
	track2 := repeatByte(0xBB, 3*2352)
	combined := append(append([]byte{}, track1...), track2...)
	cue := `FILE "x.bin" BINARY
  TRACK 01 MODE1/2352
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    INDEX 01 00:00:04
`
	chdman := fakeChdmanExtract(t, cue, combined)
	chdPath := filepath.Join(t.TempDir(), "game.chd")
	if err := os.WriteFile(chdPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	hashes, err := hashDiscCHDWithFakeSize(chdman, chdPath, systemdb.ClassCD, int64(len(combined)))
	if err != nil {
		t.Fatalf("hashDiscCHD: %v", err)
	}
	if len(hashes) != 2 {
		t.Fatalf("len(hashes) = %d, want 2", len(hashes))
	}
	if want := mustHashReader(t, track1); hashes[0] != want {
		t.Errorf("hashes[0] = %+v, want %+v (track 1)", hashes[0], want)
	}
	if want := mustHashReader(t, track2); hashes[1] != want {
		t.Errorf("hashes[1] = %+v, want %+v (track 2)", hashes[1], want)
	}
}

func TestHashDiscCHDDVD(t *testing.T) {
	isoData := []byte("a whole dvd iso's worth of bytes, pretend")
	chdman := fakeChdmanExtract(t, "", isoData)
	chdPath := filepath.Join(t.TempDir(), "game.chd")
	if err := os.WriteFile(chdPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	hashes, err := hashDiscCHDWithFakeSize(chdman, chdPath, systemdb.ClassDVD, int64(len(isoData)))
	if err != nil {
		t.Fatalf("hashDiscCHD: %v", err)
	}
	if len(hashes) != 1 {
		t.Fatalf("len(hashes) = %d, want 1", len(hashes))
	}
	if want := mustHashReader(t, isoData); hashes[0] != want {
		t.Errorf("hashes[0] = %+v, want %+v", hashes[0], want)
	}
}

// TestHashDiscCHDRejectsGDWithoutInvokingChdman guards hashDiscCHD's
// GD-ROM short-circuit: a GD-ROM CHD must fail cleanly before chdman is
// ever invoked - passing a chdmanPath that would itself error loudly if
// actually run confirms that.
func TestHashDiscCHDRejectsGDWithoutInvokingChdman(t *testing.T) {
	chdPath := filepath.Join(t.TempDir(), "game.chd")
	writeFakeGDHeader(chdPath)

	_, err := hashDiscCHD("/nonexistent/chdman", chdPath, systemdb.ClassCD)
	if err == nil {
		t.Fatal("expected an error for a GD-ROM CHD")
	}
	if strings.Contains(err.Error(), "no such file or directory") {
		t.Errorf("expected a clean GD-ROM rejection, not a failed chdman invocation: %v", err)
	}
}

func TestIsDiscClass(t *testing.T) {
	for class, want := range map[systemdb.Class]bool{
		systemdb.ClassCD:      true,
		systemdb.ClassDVD:     true,
		systemdb.ClassUMD:     true,
		systemdb.ClassCart:    false,
		systemdb.ClassArcade:  false,
		systemdb.ClassN64:     false,
		systemdb.ClassDolphin: false,
	} {
		if got := isDiscClass(class); got != want {
			t.Errorf("isDiscClass(%v) = %v, want %v", class, got, want)
		}
	}
}

func TestStripSidecarROMs(t *testing.T) {
	df := &datdb.Datafile{Games: []datdb.Game{
		{Name: "Normal Game", ROMs: []datdb.ROM{
			{Name: "Track01.bin", CRC: "aaaaaaaa"},
			{Name: "Game.cue", CRC: "bbbbbbbb"},
		}},
		{Name: "Cue Only (degenerate)", ROMs: []datdb.ROM{
			{Name: "Game.cue", CRC: "cccccccc"},
		}},
	}}
	stripped := stripSidecarROMs(df)

	if len(stripped.Games[0].ROMs) != 1 || stripped.Games[0].ROMs[0].Name != "Track01.bin" {
		t.Errorf("Games[0].ROMs = %+v, want only Track01.bin", stripped.Games[0].ROMs)
	}
	// A Game stripped to zero ROMs must keep its original (unstrippable)
	// list, not end up vacuously matchable.
	if len(stripped.Games[1].ROMs) != 1 || stripped.Games[1].ROMs[0].Name != "Game.cue" {
		t.Errorf("Games[1].ROMs = %+v, want the original Game.cue kept (would otherwise be vacuously true)", stripped.Games[1].ROMs)
	}
}

func TestDedupStrings(t *testing.T) {
	got := dedupStrings([]string{"a.chd", "a.chd", "b.chd", "a.chd"})
	want := []string{"a.chd", "b.chd"}
	if len(got) != len(want) {
		t.Fatalf("dedupStrings = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dedupStrings = %v, want %v", got, want)
		}
	}
}

// hashDiscCHDWithFakeSize calls hashDiscCHD, but first points
// chd.LogicalSize's target CHD read past - since the stand-in chdman
// ignores the input CHD's contents entirely, the real file just needs
// to exist; this helper writes a minimal real v5 CHD header with the
// given logical size so hashDiscCHD's own chd.LogicalSize call succeeds
// rather than erroring on a fake, headerless file.
func hashDiscCHDWithFakeSize(chdmanPath, chdPath string, class systemdb.Class, logicalSize int64) ([]discTrackHash, error) {
	writeFakeCHDHeader(chdPath, logicalSize)
	return hashDiscCHD(chdmanPath, chdPath, class)
}

func repeatByte(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func mustHashReader(t *testing.T, data []byte) discTrackHash {
	t.Helper()
	crc, md5sum, sha1sum, err := hashReader(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return discTrackHash{crc: crc, md5sum: md5sum, sha1sum: sha1sum}
}
