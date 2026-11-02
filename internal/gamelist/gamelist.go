// Package gamelist keeps ES-DE's scraped metadata pointed at the right
// files as romctl renames and converts ROMs. ES-DE stores this data
// outside roms/: a per-system ES-DE/gamelists/<system>/gamelist.xml (each
// <game> has a <path> to the ROM) and ES-DE/downloaded_media/<system>/
// <type>/ (covers, screenshots, videos, ...), where each media file's name
// mirrors the ROM's filename. romctl never creates this data itself (that's
// ES-DE's job, on-device); it only exists locally if pulled down via
// `romctl sync --pull media`.
package gamelist

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/demicloud/romctl/internal/actions"
)

var pathRE = regexp.MustCompile(`(?s)<path>(.*?)</path>`)

var xmlUnescaper = strings.NewReplacer(
	"&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'", "&amp;", "&",
)

var xmlEscaper = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;",
)

// Rename reflects a ROM move from oldRel to newRel (both paths relative to
// roms/<sys>/) into sys's gamelist.xml and downloaded_media, if either
// exists under libRoot/ES-DE/. A ROM that was never scraped (no matching
// <path>, no matching media file) is not an error: both steps are a silent
// no-op in that case.
func Rename(libRoot, sys, oldRel, newRel string) error {
	if oldRel == newRel {
		return nil
	}
	var errs []string
	if err := patchGamelist(libRoot, sys, oldRel, newRel); err != nil {
		errs = append(errs, err.Error())
	}
	if err := renameMedia(libRoot, sys, oldRel, newRel); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// patchGamelist rewrites the one <path> element (if any) whose unescaped
// content is "./"+oldRel to "./"+newRel, leaving every other byte of the
// file - formatting, other fields, other games - untouched.
func patchGamelist(libRoot, sys, oldRel, newRel string) error {
	path := filepath.Join(libRoot, "ES-DE", "gamelists", sys, "gamelist.xml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	old := "./" + oldRel
	replaced := false
	out := pathRE.ReplaceAllFunc(data, func(m []byte) []byte {
		if replaced {
			return m
		}
		sub := pathRE.FindSubmatch(m)
		if xmlUnescaper.Replace(string(sub[1])) != old {
			return m
		}
		replaced = true
		return []byte("<path>" + xmlEscaper.Replace("./"+newRel) + "</path>")
	})
	if !replaced {
		return nil
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// renameMedia moves every downloaded_media file matching oldRel to the
// equivalent name for newRel, across whatever media type subdirectories
// are present.
func renameMedia(libRoot, sys, oldRel, newRel string) error {
	mediaRoot := filepath.Join(libRoot, "ES-DE", "downloaded_media", sys)
	types, err := os.ReadDir(mediaRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	oldStripped := strings.TrimSuffix(oldRel, filepath.Ext(oldRel))
	newStem := mediaStem(libRoot, sys, newRel)

	var errs []string
	for _, t := range types {
		if !t.IsDir() {
			continue
		}
		typeDir := filepath.Join(mediaRoot, t.Name())
		entries, err := os.ReadDir(typeDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := filepath.Ext(e.Name())
			stem := strings.TrimSuffix(e.Name(), ext)
			if stem != oldRel && stem != oldStripped {
				continue
			}
			src := filepath.Join(typeDir, e.Name())
			dst := filepath.Join(typeDir, newStem+ext)
			if err := actions.RenamePath(src, dst); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// mediaStem is the media filename (minus the media type's own extension)
// for newRel: newRel itself when it's a directory (ES-DE's convention for
// "directory is the game" entries, e.g. ScummVM or romctl's own Title.m3u/
// groups), otherwise newRel with its extension stripped.
func mediaStem(libRoot, sys, newRel string) string {
	abs := filepath.Join(libRoot, "roms", sys, newRel)
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		return strings.TrimSuffix(newRel, filepath.Ext(newRel))
	}
	return newRel
}
