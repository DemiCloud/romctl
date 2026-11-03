// Package discimage turns archived disc dumps into plain ISO9660 images,
// for whichever system classes need that treatment instead of CHD (CHD
// covers every CD/DVD/UMD-class system; PS3 is currently the only class
// that doesn't, because RPCS3 doesn't read CHD at all). The per-class
// knowledge lives in Spec; UnpackArchives and BuildISOs are the same code
// for any class, so a future system needing the same treatment is a new
// Spec entry, not new bespoke logic.
package discimage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/archive"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/formats/iso9660"
	"github.com/demicloud/romctl/internal/systemdb"
)

// Spec describes how to recognize and package a disc dump for one system
// class.
type Spec struct {
	// FolderSuffix names the staging directory an unpacked archive becomes,
	// e.g. ".ps3".
	FolderSuffix string
	// MarkerDir is the directory name that must appear, at no more than
	// MaxDepth levels deep, in an extracted archive for it to count as a
	// real disc dump, e.g. "PS3_GAME".
	MarkerDir string
	// MarkerFile is the path under MarkerDir that must exist, e.g.
	// "USRDIR/EBOOT.BIN".
	MarkerFile string
	// MaxDepth bounds how deep MarkerDir may appear under the extraction
	// root (0 = directly in the root).
	MaxDepth int
	// CleanupDirs are top-level directory names removed from the disc
	// folder (the directory that is MarkerDir's parent) before packaging,
	// e.g. "PS3_UPDATE".
	CleanupDirs []string
	// VolumeID is the ISO9660 volume identifier for the built image.
	VolumeID string
}

var specs = map[systemdb.Class]Spec{
	systemdb.ClassPS3: {
		FolderSuffix: ".ps3",
		MarkerDir:    "PS3_GAME",
		MarkerFile:   filepath.Join("USRDIR", "EBOOT.BIN"),
		MaxDepth:     2,
		CleanupDirs:  []string{"PS3_UPDATE"},
		VolumeID:     "PS3GAME",
	},
}

// SpecFor returns the disc-image Spec for class, if one is registered.
func SpecFor(class systemdb.Class) (Spec, bool) {
	s, ok := specs[class]
	return s, ok
}

// UnpackArchives extracts every .7z/.zip/.rar archive directly under
// sysDir whose contents match spec into a FolderSuffix-named staging
// directory, retiring the archive on success. scheme (`[naming] scheme`)
// controls how the archive's region/dump tags end up in the staging
// directory's name; see internal/actions.ApplyScheme.
func UnpackArchives(sysDir, sys string, spec Spec, runner *actions.Runner, log *cliutil.Logger, scheme actions.Scheme) error {
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return nil
	}
	failed := false
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".7z" && ext != ".zip" && ext != ".rar" {
			continue
		}
		archivePath := filepath.Join(sysDir, e.Name())
		title := actions.ApplyScheme(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())), scheme)
		dst := filepath.Join(sysDir, title+spec.FolderSuffix)
		if exists(dst) || exists(filepath.Join(sysDir, title+".iso")) {
			continue
		}
		if !runner.Act("unpack %s/%s -> %s%s/", sys, e.Name(), title, spec.FolderSuffix) {
			continue
		}
		if err := unpackOneDisc(sysDir, archivePath, dst, spec); err != nil {
			log.Errf("%v", err)
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("unpack had failures")
	}
	return nil
}

func unpackOneDisc(sysDir, archivePath, dst string, spec Spec) error {
	tmp, err := os.MkdirTemp(sysDir, "_extract.*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	if err := archive.ExtractAll(archivePath, tmp); err != nil {
		return fmt.Errorf("extraction failed: %s: %w", archivePath, err)
	}
	markerDir, err := findMarkerDir(tmp, spec)
	if err != nil {
		return fmt.Errorf("not a disc dump: %s", archivePath)
	}
	root := filepath.Dir(markerDir)
	for _, d := range spec.CleanupDirs {
		os.RemoveAll(filepath.Join(root, d))
	}
	if err := os.Rename(root, dst); err != nil {
		return err
	}
	return actions.Retire(archivePath)
}

// findMarkerDir finds spec.MarkerDir (holding spec.MarkerFile), at no
// more than spec.MaxDepth levels under root.
func findMarkerDir(root string, spec Spec) (string, error) {
	var found string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || found != "" || !info.IsDir() || info.Name() != spec.MarkerDir {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.Count(rel, string(filepath.Separator)) > spec.MaxDepth {
			return nil
		}
		if exists(filepath.Join(path, spec.MarkerFile)) {
			found = path
		}
		return nil
	})
	if found == "" {
		return "", fmt.Errorf("no %s/%s found", spec.MarkerDir, spec.MarkerFile)
	}
	return found, nil
}

// BuildISOs builds a plain ISO9660 image from every FolderSuffix disc
// folder in sysDir that matches spec, retiring the folder on success.
func BuildISOs(sysDir, sys string, spec Spec, runner *actions.Runner, log *cliutil.Logger) error {
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return nil
	}
	failed := false
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), spec.FolderSuffix) {
			continue
		}
		discDir := filepath.Join(sysDir, e.Name())
		if !exists(filepath.Join(discDir, spec.MarkerDir, spec.MarkerFile)) {
			continue
		}
		title := strings.TrimSuffix(e.Name(), spec.FolderSuffix)
		iso := filepath.Join(sysDir, title+".iso")
		part := filepath.Join(sysDir, title+".partial.iso")
		if exists(iso) {
			continue
		}
		if !runner.Act("build %s/%s.iso from %s/", sys, title, e.Name()) {
			continue
		}
		if err := buildOneISO(discDir, iso, part, spec); err != nil {
			log.Errf("failed, folder kept: %s: %v", discDir, err)
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("iso build had failures")
	}
	return nil
}

func buildOneISO(discDir, iso, part string, spec Spec) error {
	defer os.Remove(part)

	f, err := os.Create(part)
	if err != nil {
		return err
	}
	err = iso9660.Build(discDir, f, spec.VolumeID)
	f.Close()
	if err != nil {
		return err
	}
	ok, err := iso9660.Complete(part)
	if err != nil || !ok {
		return fmt.Errorf("built ISO failed validation")
	}
	if err := os.Rename(part, iso); err != nil {
		return err
	}
	return actions.Retire(discDir)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
