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
	"sync"

	"github.com/demicloud/romctl/internal/actions"
)

var pathRE = regexp.MustCompile(`(?s)<path>(.*?)</path>`)

// gameRE matches one <game> or <game ...>...</game> block - (?:\s[^>]*)?
// requires whitespace (or nothing) right after "game", not just "not >",
// so it never also matches <gameList> itself.
var gameRE = regexp.MustCompile(`(?s)[ \t]*<game(?:\s[^>]*)?>.*?</game>\r?\n?`)

var xmlUnescaper = strings.NewReplacer(
	"&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'", "&amp;", "&",
)

var xmlEscaper = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;",
)

// renameLocks serializes Rename calls per (libRoot, sys): conversion
// commands now convert a system's files concurrently, and patchGamelist
// does a read-modify-write of that system's single gamelist.xml, so two
// concurrent Rename calls for the same system could otherwise race (each
// reads the pre-edit file, then the second writer's save silently loses
// the first edit). Keyed per system rather than one global lock so
// concurrent Rename calls for different systems still run unimpeded.
var renameLocks sync.Map

// Rename reflects a ROM move from oldRel to newRel (both paths relative to
// roms/<sys>/) into sys's gamelist.xml and downloaded_media, if either
// exists under libRoot/ES-DE/. A ROM that was never scraped (no matching
// <path>, no matching media file) is not an error: both steps are a silent
// no-op in that case.
func Rename(libRoot, sys, oldRel, newRel string) error {
	if oldRel == newRel {
		return nil
	}
	lockAny, _ := renameLocks.LoadOrStore(libRoot+"\x00"+sys, &sync.Mutex{})
	lock := lockAny.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

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

// Remove deletes the <game> entry (and any downloaded_media files) whose
// <path> matches rel (relative to roms/<sys>/) from sys's gamelist.xml,
// if either exists under libRoot/ES-DE/. Unlike Rename, this is for a ROM
// moving to a *different* system entirely (see commands.Relocate, which
// moves a misfiled cartridge ROM into its correct system's directory):
// there's nothing legitimate to carry over into the destination system's
// own gamelist, since any scrape made under the wrong platform was for
// the wrong game context to begin with - a real-world case, not a
// theoretical one (a Genesis cart dropped into dreamcast/ picks up a
// bogus-or-absent scrape there, and a stale entry would otherwise sit in
// dreamcast's gamelist pointing at a path that no longer exists). A ROM
// that was never scraped is not an error: both steps are a silent no-op,
// same as Rename.
func Remove(libRoot, sys, rel string) error {
	lockAny, _ := renameLocks.LoadOrStore(libRoot+"\x00"+sys, &sync.Mutex{})
	lock := lockAny.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	var errs []string
	if err := removeGamelistEntry(libRoot, sys, rel); err != nil {
		errs = append(errs, err.Error())
	}
	if err := removeMedia(libRoot, sys, rel); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// removeGamelistEntry deletes the whole <game>...</game> block (plus its
// own line's leading indentation and trailing newline, so the file isn't
// left with a blank line) whose <path> unescapes to "./"+rel - unlike
// patchGamelist, which only ever rewrites a <path>'s content in place,
// this removes the entry outright, since Remove's caller has nowhere to
// re-point it to.
func removeGamelistEntry(libRoot, sys, rel string) error {
	path := filepath.Join(libRoot, "ES-DE", "gamelists", sys, "gamelist.xml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	target := "./" + rel
	removed := false
	out := gameRE.ReplaceAllFunc(data, func(m []byte) []byte {
		if removed {
			return m
		}
		sub := pathRE.FindSubmatch(m)
		if sub == nil || xmlUnescaper.Replace(string(sub[1])) != target {
			return m
		}
		removed = true
		return nil
	})
	if !removed {
		return nil
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// removeMedia deletes every downloaded_media file matching rel, across
// whatever media type subdirectories are present - renameMedia's own
// matching rule (bare name or extension-stripped stem), just removing
// instead of moving. A permanent delete, not a Retire into _to_delete/,
// is the deliberate choice here despite romctl's general "nothing is
// deleted outright" rule (see AGENTS.md): that rule exists because a
// replaced ROM original has real recovery value, but art scraped for a
// ROM under the *wrong* platform (Remove's only caller) never correctly
// represents anything to begin with, and clean has no notion of a
// _to_delete/ anywhere under ES-DE/downloaded_media/ to ever collect it
// from - retiring it here would just be permanent clutter with no path
// back to actually being cleaned up, strictly worse than deleting it.
func removeMedia(libRoot, sys, rel string) error {
	mediaRoot := filepath.Join(libRoot, "ES-DE", "downloaded_media", sys)
	types, err := os.ReadDir(mediaRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	stripped := strings.TrimSuffix(rel, filepath.Ext(rel))
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
			stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if stem != rel && stem != stripped {
				continue
			}
			if err := os.Remove(filepath.Join(typeDir, e.Name())); err != nil {
				errs = append(errs, err.Error())
			}
		}
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
