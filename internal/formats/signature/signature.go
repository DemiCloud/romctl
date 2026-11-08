// Package signature identifies a cartridge/handheld ROM's actual format by
// its header bytes, so a command can catch a ROM that's been misfiled into
// the wrong system's directory (e.g. a Genesis cartridge dump sitting in
// roms/segacd/) before acting on it.
//
// Offsets and magic values are taken from the file(1)/libmagic project's
// own magic source (magic/Magdir/console, BSD-licensed,
// https://github.com/file/file) rather than guessed, the same way
// systemdb.SkyscraperPlatform was built from Skyscraper's real --help
// output instead of guessed.
//
// Coverage is deliberately partial: only formats with a strong,
// unambiguous header signature are included. SNES's own header check is a
// weak heuristic even in libmagic itself (no fixed offset - LoROM/HiROM
// headers land at different places, and copier headers shift it further),
// and most pre-16-bit cartridge systems (Atari 2600/5200/7800,
// ColecoVision, Intellivision, Vectrex, Odyssey2, SG-1000, PC Engine,
// WonderSwan, ...) have no reliable magic bytes at all, so none of those
// are included rather than guessed at.
package signature

import (
	"bytes"
	"os"

	"github.com/demicloud/romctl/internal/formats/n64"
)

// Family identifies a detected ROM format.
type Family string

const (
	FamilyNES        Family = "nes"
	FamilyFDS        Family = "fds"
	FamilyGB         Family = "gb"
	FamilyGBA        Family = "gba"
	FamilyN64        Family = "n64"
	FamilyGenesis    Family = "genesis"
	Family32X        Family = "32x"
	FamilySegaCDBoot Family = "segacd-boot"
	FamilySMS        Family = "sms"
	FamilyGameGear   Family = "gamegear"
	FamilyLynx       Family = "lynx"
	FamilyNGP        Family = "ngp"
)

// Name is a short human-readable label for a Family, for use in messages.
var Name = map[Family]string{
	FamilyNES:        "NES/Famicom",
	FamilyFDS:        "Famicom Disk System",
	FamilyGB:         "Game Boy/Color",
	FamilyGBA:        "Game Boy Advance",
	FamilyN64:        "Nintendo 64",
	FamilyGenesis:    "Genesis/Mega Drive",
	Family32X:        "Sega 32X",
	FamilySegaCDBoot: "Sega CD/Mega CD boot",
	FamilySMS:        "Sega Master System",
	FamilyGameGear:   "Sega Game Gear",
	FamilyLynx:       "Atari Lynx",
	FamilyNGP:        "Neo Geo Pocket",
}

// Systems maps each detectable Family to the systemdb directory names its
// content is valid for - i.e. not a misfiled-ROM finding.
var Systems = map[Family][]string{
	FamilyNES:        {"nes", "famicom"},
	FamilyFDS:        {"fds"},
	FamilyGB:         {"gb", "gbc", "sgb"},
	FamilyGBA:        {"gba"},
	FamilyN64:        {"n64"},
	FamilyGenesis:    {"genesis", "megadrive", "megadrivejp"},
	Family32X:        {"sega32x", "sega32xjp", "sega32xna"},
	FamilySegaCDBoot: {"segacd", "megacd", "megacdjp"},
	FamilySMS:        {"mastersystem", "mark3"},
	FamilyGameGear:   {"gamegear"},
	FamilyLynx:       {"atarilynx"},
	FamilyNGP:        {"ngp", "ngpc"},
}

// ValidFor reports whether fam's content is expected in system directory
// sys.
func ValidFor(fam Family, sys string) bool {
	for _, s := range Systems[fam] {
		if s == sys {
			return true
		}
	}
	return false
}

// readAt returns exactly n bytes read from f at offset, or nil on a short
// read or error - every check below treats "can't read that far" the same
// as "doesn't match", never as a reportable error, since a non-matching
// file (the overwhelming majority of calls) is the expected case, not a
// failure.
func readAt(f *os.File, offset int64, n int) []byte {
	buf := make([]byte, n)
	if read, _ := f.ReadAt(buf, offset); read < n {
		return nil
	}
	return buf
}

func stringAt(f *os.File, offset int64, want string) bool {
	b := readAt(f, offset, len(want))
	return b != nil && string(b) == want
}

var (
	gbLogo  = []byte{0xCE, 0xED, 0x66, 0x66, 0xCC, 0x0D, 0x00, 0x0B}
	gbaLogo = []byte{0x24, 0xFF, 0xAE, 0x51, 0x69, 0x9A, 0xA2, 0x21}
)

// Identify reads just enough of path's header to classify it, returning
// ("", false) when nothing matches - the common case for the large
// majority of files checked, not an error.
func Identify(path string) (Family, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	if stringAt(f, 0, "NES\x1A") {
		return FamilyNES, true
	}
	if stringAt(f, 0x11, "*NINTENDO-HVC*") || stringAt(f, 1, "*NINTENDO-HVC*") {
		return FamilyFDS, true
	}
	if b := readAt(f, 0x104, 8); b != nil && bytes.Equal(b, gbLogo) {
		return FamilyGB, true
	}
	if b := readAt(f, 4, 8); b != nil && bytes.Equal(b, gbaLogo) {
		return FamilyGBA, true
	}
	if order, err := n64.Detect(path); err == nil && order != n64.OrderUnknown {
		return FamilyN64, true
	}
	if stringAt(f, 0x0A, "BY SNK CORPORATION") {
		return FamilyNGP, true
	}
	if stringAt(f, 0, "LYNX") {
		return FamilyLynx, true
	}

	// Sega Mega Drive/Genesis header: "SEGA" at 0x100 (or, on some dumps,
	// 0x101). A Sega CD/Mega CD boot track's IP.BIN deliberately reuses
	// this same header layout for hardware compatibility, so "SEGA" alone
	// doesn't distinguish a cartridge from a CD boot sector - the
	// "BR" domestic-serial marker at 0x180 does (file(1) draws the same
	// distinction, for the same reason).
	if stringAt(f, 0x100, "SEGA") || stringAt(f, 0x101, "SEGA") {
		if stringAt(f, 0x180, "BR") {
			return FamilySegaCDBoot, true
		}
		if stringAt(f, 0x3C0, "MARS CHECK MODE") {
			return Family32X, true
		}
		return FamilyGenesis, true
	}

	// SMS/Game Gear: "TMR SEGA" at one of three fixed offsets depending on
	// cartridge size; the machine-type nibble right after it tells the two
	// apart.
	for _, off := range []int64{0x7FF0, 0x3FF0, 0x1FF0} {
		if stringAt(f, off, "TMR SEGA") {
			if b := readAt(f, off+0x0F, 1); b != nil {
				switch b[0] & 0xF0 {
				case 0x50, 0x60, 0x70:
					return FamilyGameGear, true
				}
			}
			return FamilySMS, true
		}
	}

	return "", false
}
