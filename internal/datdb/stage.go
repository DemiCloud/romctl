package datdb

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/demicloud/romctl/internal/systemdb"
)

// datTimestampSuffix matches a real pack filename's own dynamic
// "<base> (<timestamp>).dat" suffix - group 1 is everything before it.
// A plain glob ("<System> (*).dat") isn't precise enough here: `*` also
// matches across a *different* trailing parenthetical group, e.g.
// "Nintendo - Game Boy Advance (Multiboot) (20261012-143535).dat" would
// wrongly glob-match the plain "Nintendo - Game Boy Advance" pattern too.
// Anchoring the timestamp itself to its actual digit-dash-digit shape -
// confirmed against a real 2026-11-08 No-Intro pack - makes group 1
// resolve to the *entire* name before that one real timestamp group,
// however many other parenthetical qualifiers (e.g. "(A78)", "(BIN)")
// appear earlier in the name as part of the base name itself.
var datTimestampSuffix = regexp.MustCompile(`^(.+) \(\d{8}-\d{6}\)\.dat$`)

// findDatFiles returns every file under dir whose name resolves to
// exactly system, either already timestamp-stripped ("<system>.dat") or
// in a real pack's own dynamic form (matched via datTimestampSuffix).
// More than one result means an unresolvable ambiguity - the caller
// reports that rather than guessing.
func findDatFiles(dir, system string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var matches []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == system+".dat" {
			matches = append(matches, filepath.Join(dir, name))
			continue
		}
		if m := datTimestampSuffix.FindStringSubmatch(name); m != nil && m[1] == system {
			matches = append(matches, filepath.Join(dir, name))
		}
	}
	return matches, nil
}

// StageResult is one systemdb.Dat entry's outcome from Stage.
type StageResult struct {
	Provider, System string // System is romctl's directory name, e.g. "nes"
	Name             string // the provider's pack name, e.g. "Nintendo - ..."
	Status           string // "staged", "missing", "ambiguous", "mismatch", "error"
	Detail           string
}

// Stage matches every systemdb.Dat entry against whatever the maintainer
// has dropped, unmodified, under srcRoot/<provider>/ (typically an entire
// unzipped upstream pack - hundreds of files, most irrelevant, each
// carrying its own dynamic "(<timestamp>)" suffix that changes release to
// release) and copies just the ones that are actually needed into
// dstRoot/<provider>/<pack name>.dat for embedding, with that suffix
// stripped - see findDatFiles - driven by `make dats`. Every match is
// parsed (see Parse) and its own <header><name> cross-checked against the
// expected pack name before staging, so a wrong file - or a pack whose
// DAT accidentally matched the wrong system - is reported as "mismatch,"
// not silently staged.
func Stage(srcRoot, dstRoot string) ([]StageResult, error) {
	results := make([]StageResult, 0, len(systemdb.Dat))
	for sys, src := range systemdb.Dat {
		results = append(results, stageOne(srcRoot, dstRoot, sys, src))
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Provider != results[j].Provider {
			return results[i].Provider < results[j].Provider
		}
		return results[i].System < results[j].System
	})
	return results, nil
}

func stageOne(srcRoot, dstRoot, sys string, src systemdb.DatSource) StageResult {
	result := StageResult{Provider: src.Provider, System: sys, Name: src.System}

	srcDir := filepath.Join(srcRoot, src.Provider)
	matches, err := findDatFiles(srcDir, src.System)
	if err != nil {
		result.Status, result.Detail = "error", err.Error()
		return result
	}
	if len(matches) == 0 {
		result.Status = "missing"
		result.Detail = fmt.Sprintf("no %q (with or without a timestamp suffix) under %s", src.System+".dat", srcDir)
		return result
	}
	if len(matches) > 1 {
		result.Status = "ambiguous"
		result.Detail = fmt.Sprintf("%d files matched: %v", len(matches), matches)
		return result
	}

	data, err := os.ReadFile(matches[0])
	if err != nil {
		result.Status, result.Detail = "error", err.Error()
		return result
	}
	df, err := Parse(data)
	if err != nil {
		result.Status = "error"
		result.Detail = fmt.Sprintf("%s: not a valid DAT: %v", matches[0], err)
		return result
	}
	if df.Header.Name != src.System {
		result.Status = "mismatch"
		result.Detail = fmt.Sprintf("%s: header name %q != expected %q", matches[0], df.Header.Name, src.System)
		return result
	}

	dstDir := filepath.Join(dstRoot, src.Provider)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		result.Status, result.Detail = "error", err.Error()
		return result
	}
	dst := filepath.Join(dstDir, src.System+".dat")
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		result.Status, result.Detail = "error", err.Error()
		return result
	}
	result.Status = "staged"
	result.Detail = dst
	return result
}
