package commands

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/demicloud/romctl/internal/formats/signature"
)

func md5Hex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

// gameFile is one on-disk file worth hashing for a duplicate/DAT-match
// scan, paired with the label a finding or matched-file column should
// show for it.
type gameFile struct {
	path, label string
}

// listGameFiles walks sysDir the same way verify's own CHD-only walk
// already does (verifyJobsFor in verify.go), but for every file filter
// accepts rather than just .chd: bare files and every disc inside a
// Title.m3u/ playlist directory alike, flattened into one list - the
// latter labeled "Title.m3u/Title (Disc N).ext" so a finding or matched-
// file column still shows exactly where it came from. Shared by dupes and
// collection so neither silently skips every multi-disc game the way a
// plain os.ReadDir + skip-dirs loop would. The playlist text file itself
// (same base name as its directory, ".m3u" extension) is never included,
// filter or not - it's not game content.
func listGameFiles(sysDir string, filter func(name string) bool) []gameFile {
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return nil
	}
	var files []gameFile
	for _, e := range entries {
		base := e.Name()
		if e.IsDir() {
			if !strings.HasSuffix(base, ".m3u") {
				continue
			}
			discs, err := os.ReadDir(filepath.Join(sysDir, base))
			if err != nil {
				continue
			}
			for _, d := range discs {
				name := d.Name()
				if d.IsDir() || strings.ToLower(filepath.Ext(name)) == ".m3u" || !filter(name) {
					continue
				}
				files = append(files, gameFile{filepath.Join(sysDir, base, name), filepath.Join(base, name)})
			}
			continue
		}
		if !filter(base) {
			continue
		}
		files = append(files, gameFile{filepath.Join(sysDir, base), base})
	}
	return files
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
