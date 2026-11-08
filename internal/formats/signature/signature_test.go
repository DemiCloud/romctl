package signature

import (
	"os"
	"path/filepath"
	"testing"
)

func writeAt(t *testing.T, size int, offset int64, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rom.bin")
	buf := make([]byte, size)
	copy(buf[offset:], data)
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIdentifyNES(t *testing.T) {
	path := writeAt(t, 0x20, 0, "NES\x1A")
	fam, ok := Identify(path)
	if !ok || fam != FamilyNES {
		t.Fatalf("Identify = (%v, %v), want (%v, true)", fam, ok, FamilyNES)
	}
}

func TestIdentifyGameBoy(t *testing.T) {
	path := writeAt(t, 0x200, 0x104, "\xCE\xED\x66\x66\xCC\x0D\x00\x0B")
	fam, ok := Identify(path)
	if !ok || fam != FamilyGB {
		t.Fatalf("Identify = (%v, %v), want (%v, true)", fam, ok, FamilyGB)
	}
}

func TestIdentifyGBA(t *testing.T) {
	path := writeAt(t, 0x200, 4, "\x24\xFF\xAE\x51\x69\x9A\xA2\x21")
	fam, ok := Identify(path)
	if !ok || fam != FamilyGBA {
		t.Fatalf("Identify = (%v, %v), want (%v, true)", fam, ok, FamilyGBA)
	}
}

func TestIdentifyGenesisCartridge(t *testing.T) {
	path := writeAt(t, 0x200, 0x100, "SEGA")
	// 0x180 left zeroed - not "BR" - so this reads as a cartridge, not a
	// Sega CD boot sector.
	fam, ok := Identify(path)
	if !ok || fam != FamilyGenesis {
		t.Fatalf("Identify = (%v, %v), want (%v, true)", fam, ok, FamilyGenesis)
	}
	if !ValidFor(fam, "genesis") {
		t.Fatal("ValidFor(genesis, \"genesis\") = false, want true")
	}
	if ValidFor(fam, "segacd") {
		t.Fatal("ValidFor(genesis, \"segacd\") = true, want false")
	}
}

func TestIdentify32X(t *testing.T) {
	path := writeAt(t, 0x400, 0x100, "SEGA")
	data, _ := os.ReadFile(path)
	copy(data[0x3C0:], "MARS CHECK MODE")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	fam, ok := Identify(path)
	if !ok || fam != Family32X {
		t.Fatalf("Identify = (%v, %v), want (%v, true)", fam, ok, Family32X)
	}
}

// TestIdentifySegaCDBootSector guards the exact false-positive this
// package exists to avoid: a legitimately-dumped Sega CD/Mega CD game's
// boot track (IP.BIN) reuses the Genesis cartridge header layout, "SEGA"
// at 0x100 included, for hardware compatibility - it must not be flagged
// as a misfiled Genesis cartridge ROM.
func TestIdentifySegaCDBootSector(t *testing.T) {
	path := writeAt(t, 0x400, 0x100, "SEGA")
	data, _ := os.ReadFile(path)
	copy(data[0x180:], "BR")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	fam, ok := Identify(path)
	if !ok || fam != FamilySegaCDBoot {
		t.Fatalf("Identify = (%v, %v), want (%v, true)", fam, ok, FamilySegaCDBoot)
	}
	if !ValidFor(fam, "segacd") {
		t.Fatal("ValidFor(segacd-boot, \"segacd\") = false, want true")
	}
	if ValidFor(fam, "genesis") {
		t.Fatal("ValidFor(segacd-boot, \"genesis\") = true, want false")
	}
}

func TestIdentifySMS(t *testing.T) {
	path := writeAt(t, 0x8000, 0x7FF0, "TMR SEGA")
	data, _ := os.ReadFile(path)
	data[0x7FF0+0x0F] = 0x30 // machine type nibble: SMS export
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	fam, ok := Identify(path)
	if !ok || fam != FamilySMS {
		t.Fatalf("Identify = (%v, %v), want (%v, true)", fam, ok, FamilySMS)
	}
}

func TestIdentifyGameGear(t *testing.T) {
	path := writeAt(t, 0x8000, 0x7FF0, "TMR SEGA")
	data, _ := os.ReadFile(path)
	data[0x7FF0+0x0F] = 0x50 // machine type nibble: Game Gear
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	fam, ok := Identify(path)
	if !ok || fam != FamilyGameGear {
		t.Fatalf("Identify = (%v, %v), want (%v, true)", fam, ok, FamilyGameGear)
	}
}

func TestIdentifyNothingMatches(t *testing.T) {
	path := writeAt(t, 0x200, 0, "")
	if fam, ok := Identify(path); ok {
		t.Fatalf("Identify = (%v, true), want ok=false", fam)
	}
}

func TestIdentifyShortFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiny.bin")
	if err := os.WriteFile(path, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fam, ok := Identify(path); ok {
		t.Fatalf("Identify = (%v, true), want ok=false", fam)
	}
}
