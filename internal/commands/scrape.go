package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/config"
	"github.com/demicloud/romctl/internal/executil"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
	"github.com/demicloud/romctl/internal/tools"
)

// skyscraperArtworkXML is a minimal artwork.xml: one <output> per type
// Skyscraper's compositor supports, none with nested <layer> nodes, which
// (per its own docs) makes it export each type as raw, unmanipulated
// artwork instead of compositing a stylized render. Skyscraper requires a
// working artwork.xml to exist even just to scrape, not only to generate a
// gamelist, and romctl doesn't ship Skyscraper's full example install
// (fonts, resource images, etc.) - just enough to make it run.
const skyscraperArtworkXML = `<?xml version="1.0" encoding="UTF-8"?>
<artwork>
  <output type="screenshot"/>
  <output type="cover"/>
  <output type="marquee"/>
  <output type="wheel"/>
</artwork>
`

// skyscraperArtworkPath writes skyscraperArtworkXML to a stable cache
// location (reused across runs) and returns its path.
func skyscraperArtworkPath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "romctl", "skyscraper")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "artwork.xml")
	if err := os.WriteFile(path, []byte(skyscraperArtworkXML), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Scrape fetches game metadata and media for every populated system that
// has a Skyscraper platform equivalent (systemdb.SkyscraperPlatform),
// writing directly into ES-DE/gamelists/<sys>/ and
// ES-DE/downloaded_media/<sys>/ via the embedded/PATH Skyscraper binary.
// romctl never talks to a scraper API itself: Skyscraper's own
// ScreenScraper developer credentials are baked in, so only the user's
// personal account ([scrape] user) is needed.
func Scrape(lib *library.Library, runner *actions.Runner, systems []string, cfg config.Config) (bool, error) {
	log := cliutil.NewLogger("scrape")
	if cfg.Scrape.User == "" {
		return false, fmt.Errorf(`set [scrape] user = "ssid:sspassword" in the config file, or pass --scrape-user (see --help)`)
	}
	path, err := tools.Resolve("skyscraper")
	if err != nil {
		return false, err
	}
	artwork, err := skyscraperArtworkPath()
	if err != nil {
		return false, fmt.Errorf("writing skyscraper artwork.xml: %w", err)
	}
	scraped, skipped, failed := 0, 0, 0
	for _, sys := range systems {
		platform, ok := systemdb.SkyscraperPlatform[sys]
		if !ok {
			log.Errf("no Skyscraper platform for %s, skipped", sys)
			skipped++
			continue
		}
		sysDir := filepath.Join(lib.Roms, sys)
		if !library.HasGames(sysDir) {
			continue
		}
		if !runner.Act("scrape %s (platform %s)", sys, platform) {
			continue
		}
		if err := scrapeOne(path, artwork, lib.Root, sys, sysDir, platform, cfg.Scrape.User, cfg.Scrape.Threads); err != nil {
			log.Errf("%s: %v", sys, err)
			failed++
			continue
		}
		scraped++
	}

	if scraped+skipped+failed > 0 {
		fmt.Printf("scrape: %d scraped, %d skipped, %d failed\n", scraped, skipped, failed)
	}
	return failed == 0, nil
}

// scrapeOne scrapes one system. Multi-disc games (romctl's Title.m3u/
// directories) are handled separately from everything else: Skyscraper has
// no concept of "a directory is the game" beyond its own ScummVM-specific
// marker-file convention, so its normal directory scan would otherwise
// recurse into Title.m3u/ and scrape every individual disc file inside as
// its own unrelated game (duplicate entries, wasted ScreenScraper
// lookups). Instead:
//  1. The normal pass scrapes everything else, with the "nosubdirs" flag
//     so Skyscraper never recurses into Title.m3u/ (or _to_delete/,
//     _extract.*/) in the first place.
//  2. A second pass scrapes each Title.m3u/ game explicitly by its
//     playlist file path alone - never the disc files - passed as a
//     positional argument, which also skips Skyscraper's normal directory
//     scan entirely for that invocation.
//  3. Skyscraper's own naming for that second pass doesn't know about
//     romctl's Title.m3u/ convention either: it writes paths/media keyed
//     off the playlist file's basename with one extension stripped
//     ("Title", not "Title.m3u"). gamelist.Rename (the same helper every
//     other rename-tracking command already uses) corrects both the
//     <path> and the media filenames to the convention ES-DE actually
//     expects afterward.
func scrapeOne(skyscraper, artwork, libRoot, sys, sysDir, platform, user string, threads int) error {
	gamelistDir := filepath.Join(libRoot, "ES-DE", "gamelists", sys)
	mediaDir := filepath.Join(libRoot, "ES-DE", "downloaded_media", sys)
	if err := os.MkdirAll(gamelistDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return err
	}
	var m3uTitles []string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".m3u") {
			m3uTitles = append(m3uTitles, strings.TrimSuffix(e.Name(), ".m3u"))
		}
	}

	if err := scrapePass(skyscraper, artwork, sysDir, platform, user, threads, "nosubdirs", nil); err != nil {
		return fmt.Errorf("flat files: %w", err)
	}
	if err := generatePass(skyscraper, artwork, sysDir, gamelistDir, mediaDir, platform, "nosubdirs", nil); err != nil {
		return fmt.Errorf("flat files: %w", err)
	}

	if len(m3uTitles) == 0 {
		return nil
	}

	playlists := make([]string, len(m3uTitles))
	for i, title := range m3uTitles {
		playlists[i] = filepath.Join(sysDir, title+".m3u", title+".m3u")
	}
	if err := scrapePass(skyscraper, artwork, sysDir, platform, user, threads, "", playlists); err != nil {
		return fmt.Errorf("multi-disc: %w", err)
	}
	if err := generatePass(skyscraper, artwork, sysDir, gamelistDir, mediaDir, platform, "", playlists); err != nil {
		return fmt.Errorf("multi-disc: %w", err)
	}
	for _, title := range m3uTitles {
		playlistRel := filepath.Join(title+".m3u", title+".m3u")
		m3uRel := title + ".m3u"
		if err := gamelist.Rename(libRoot, sys, playlistRel, m3uRel); err != nil {
			return fmt.Errorf("fixing up %s: %w", title, err)
		}
		if err := gamelist.Rename(libRoot, sys, title, m3uRel); err != nil {
			return fmt.Errorf("fixing up %s: %w", title, err)
		}
	}
	return nil
}

// scrapePass runs Skyscraper's scrape phase against ScreenScraper,
// populating its local resource cache. extraFlag, if non-empty, is added
// to --flags (alongside the ones always used); files, if non-empty, are
// passed as explicit positional arguments instead of letting Skyscraper
// scan -i itself.
func scrapePass(skyscraper, artwork, sysDir, platform, user string, threads int, extraFlag string, files []string) error {
	flags := "unattendskip,onlymissing"
	if extraFlag != "" {
		flags += "," + extraFlag
	}
	args := []string{
		"-p", platform, "-s", "screenscraper", "-u", user,
		"-i", sysDir, "-a", artwork, "--flags", flags,
	}
	if threads > 0 {
		args = append(args, "-t", strconv.Itoa(threads))
	}
	args = append(args, files...)
	if err := executil.RunInherit(skyscraper, args...); err != nil {
		return fmt.Errorf("skyscraper scrape pass: %w", err)
	}
	return nil
}

// generatePass runs Skyscraper's gamelist generation phase, composing its
// local resource cache into gamelist.xml and media files. extraFlag and
// files behave the same as in scrapePass.
func generatePass(skyscraper, artwork, sysDir, gamelistDir, mediaDir, platform, extraFlag string, files []string) error {
	flags := "unattendskip,relative"
	if extraFlag != "" {
		flags += "," + extraFlag
	}
	args := []string{
		"-p", platform, "-i", sysDir,
		"-g", gamelistDir, "-o", mediaDir, "-f", "emulationstation",
		"-a", artwork, "--flags", flags,
	}
	args = append(args, files...)
	if err := executil.RunInherit(skyscraper, args...); err != nil {
		return fmt.Errorf("skyscraper generate pass: %w", err)
	}
	return nil
}
