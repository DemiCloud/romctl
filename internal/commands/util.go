package commands

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/demicloud/romctl/internal/formats/signature"
)

func md5Hex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

// signatureMismatch reports, as a ready-to-use error message, whether
// path's content identifies as a cartridge/handheld ROM family that
// doesn't belong in sys - the same misfiled-ROM determination check's own
// containers category makes (see checkContainerFile in check.go), run
// again here by every mutating command that converts/unpacks a file by
// its extension (chd, snes, xiso, rvz, unpack) so none of them ever
// blindly hands an external tool, or its own byte-shuffling, a file it
// can already tell isn't actually what the command assumes it is -
// instead of surfacing that tool's own opaque failure for it, or worse,
// silently mis-processing it. "" means path is fine to act on, or (the
// overwhelming majority of real ROMs/disc images) simply has no
// signature.Identify match at all - a .cue/.gdi text playlist always
// falls into this case too, since neither is the raw image a tool like
// chdman actually reads.
func signatureMismatch(sys, path string) string {
	fam, ok := signature.Identify(path)
	if !ok || signature.ValidFor(fam, sys) {
		return ""
	}
	return fmt.Sprintf("%s identifies as a %s ROM, not what %s expects - move it to the correct system instead",
		filepath.Base(path), signature.Name[fam], sys)
}
