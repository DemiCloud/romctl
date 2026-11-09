package commands

import (
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"

	"strings"

	"github.com/demicloud/romctl/internal/datdb"
	"github.com/demicloud/romctl/internal/executil"
	"github.com/demicloud/romctl/internal/formats/chd"
	"github.com/demicloud/romctl/internal/formats/cuesheet"
	"github.com/demicloud/romctl/internal/systemdb"
	"github.com/demicloud/romctl/internal/tools"
)

// isDiscClass reports whether class is one hashDiscCHD knows how to
// extract - the three CHD-targeting disc classes (see systemdb.Class's
// doc comments). Arcade, cart, N64, SNES, Dolphin, Xbox and PS3 CHDs/
// images don't go through this path at all.
func isDiscClass(class systemdb.Class) bool {
	return class == systemdb.ClassCD || class == systemdb.ClassDVD || class == systemdb.ClassUMD
}

// resolveCHDManIfNeeded resolves chdman once, up front, only if at least
// one of the given systems is disc-based - same "resolve before any
// work, actionable error" invariant Verify already follows. Returns ""
// with no error if no requested system needs it, so --verify-chd on an
// all-cartridge selection never requires chdman at all.
func resolveCHDManIfNeeded(systems []string) (string, error) {
	needed := false
	for _, sys := range systems {
		if isDiscClass(systemdb.ClassOf(sys)) {
			needed = true
			break
		}
	}
	if !needed {
		return "", nil
	}
	return tools.Resolve("chdman")
}

// sidecarExts are files Redump DATs commonly hash alongside a disc's
// binary tracks (the cue/toc sheet itself, subchannel/control data) that
// a CHD-extraction round-trip can never reproduce byte-for-byte: chdman
// regenerates its own cue sheet, which won't match Redump's distributed
// one regardless of how correct the actual track data is.
var sidecarExts = []string{".cue", ".ccd", ".sub", ".toc"}

// stripSidecarROMs returns a copy of df with any ROM whose name has a
// sidecarExts extension removed from every Game - so a Game counts as
// matched once every actual binary track is found, without an
// unreproducible sidecar file permanently blocking it. Only applied on
// the --verify-chd path (see Collection/Identify); normal file-based
// matching is untouched, since a real file on disk could in principle
// still carry a byte-identical cue sheet.
func stripSidecarROMs(df *datdb.Datafile) *datdb.Datafile {
	out := &datdb.Datafile{Header: df.Header, Games: make([]datdb.Game, len(df.Games))}
	for i, g := range df.Games {
		roms := make([]datdb.ROM, 0, len(g.ROMs))
		for _, r := range g.ROMs {
			if isSidecar(r.Name) {
				continue
			}
			roms = append(roms, r)
		}
		// A Game stripped down to zero ROMs would make have() vacuously
		// true (an empty loop never fails) - nothing would actually have
		// been verified. Keep the original, unstrippable ROM list for
		// that one Game instead, so it just never matches via extraction
		// rather than falsely matching everything.
		if len(roms) == 0 {
			out.Games[i] = g
			continue
		}
		out.Games[i] = datdb.Game{Name: g.Name, ROMs: roms}
	}
	return out
}

func isSidecar(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range sidecarExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// discTrackHash is one track's CRC32/MD5/SHA1 (lowercase hex), same
// shape hashROM/hashReader already produce for a regular file.
type discTrackHash struct{ crc, md5sum, sha1sum string }

// hashDiscCHD extracts path (a CD/DVD/UMD-class .chd) via chdman and
// returns one hash triple per track, in disc order: ClassDVD/ClassUMD
// always have exactly one (the whole extracted .iso); ClassCD can have
// more than one, parsed from the cue sheet chdman's own extractcd
// generates (see internal/formats/cuesheet). Only called from the
// --verify-chd path - see Collection/Identify's doc comments.
//
// Extraction is staged in scratch space - chdScratchDir's tmpfs when
// there's room, a plain OS temp directory otherwise. Unlike makeCHD's
// own scratch use, nothing here needs to land on any particular
// filesystem afterward: the extracted file is read, hashed, and
// discarded, never written back into the library. Always cleaned up,
// win or lose.
func hashDiscCHD(chdmanPath, path string, class systemdb.Class) ([]discTrackHash, error) {
	size, err := chd.LogicalSize(path)
	if err != nil {
		return nil, err
	}

	shm, releaseShm := chdScratchDir(size)
	defer releaseShm()
	base := shm
	if base == "" {
		base = os.TempDir()
	}
	work, err := os.MkdirTemp(base, "romctl-verify-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)

	switch class {
	case systemdb.ClassDVD, systemdb.ClassUMD:
		iso := filepath.Join(work, "x.iso")
		if out, err := executil.Run(chdmanPath, "extractdvd", "-i", path, "-o", iso); err != nil {
			return nil, fmt.Errorf("extractdvd: %w: %s", err, out)
		}
		h, err := hashRange(iso, 0, -1)
		if err != nil {
			return nil, err
		}
		return []discTrackHash{h}, nil

	case systemdb.ClassCD:
		cueFile := filepath.Join(work, "x.cue")
		binFile := filepath.Join(work, "x.bin")
		if out, err := executil.Run(chdmanPath, "extractcd", "-i", path, "-o", cueFile, "-ob", binFile); err != nil {
			return nil, fmt.Errorf("extractcd: %w: %s", err, out)
		}
		cueData, err := os.ReadFile(cueFile)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(binFile)
		if err != nil {
			return nil, err
		}
		_, tracks, err := cuesheet.Parse(cueData, info.Size())
		if err != nil {
			return nil, fmt.Errorf("%s: %w", cueFile, err)
		}
		hashes := make([]discTrackHash, 0, len(tracks))
		for _, tr := range tracks {
			h, err := hashRange(binFile, tr.Start, tr.End)
			if err != nil {
				return nil, fmt.Errorf("track %d: %w", tr.Number, err)
			}
			hashes = append(hashes, h)
		}
		return hashes, nil

	default:
		return nil, fmt.Errorf("%s: not a disc-based CHD class", path)
	}
}

// hashRange hashes path's bytes [start, end) - end < 0 means "to EOF".
func hashRange(path string, start, end int64) (discTrackHash, error) {
	f, err := os.Open(path)
	if err != nil {
		return discTrackHash{}, err
	}
	defer f.Close()
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return discTrackHash{}, err
	}
	var r io.Reader = f
	if end >= 0 {
		r = io.LimitReader(f, end-start)
	}
	hc, hm, hs := crc32.NewIEEE(), md5.New(), sha1.New()
	if _, err := io.Copy(io.MultiWriter(hc, hm, hs), r); err != nil {
		return discTrackHash{}, err
	}
	return discTrackHash{
		crc:     fmt.Sprintf("%08x", hc.Sum32()),
		md5sum:  fmt.Sprintf("%x", hm.Sum(nil)),
		sha1sum: fmt.Sprintf("%x", hs.Sum(nil)),
	}, nil
}
