package tools

import (
	"io/fs"
	"os/exec"
	"regexp"

	"github.com/demicloud/romctl/internal/executil"
)

// knownTools is every binary Resolve knows how to find, in the order
// Status reports them.
var knownTools = []string{"chdman", "dolphin-tool", "extract-xiso", "skyscraper"}

// Status is one tool's availability for this specific binary and
// platform: whether a build was embedded at compile time, and/or found
// on PATH. Checking status never extracts or runs anything - unlike
// Resolve, it has no side effects, even for a tool that is embedded.
type Status struct {
	Tool     string
	Embedded bool
	OnPath   string // the resolved PATH location, "" if not found there
}

// Available reports whether this tool is usable at all - embedded for
// this platform, or found on PATH.
func (s Status) Available() bool {
	return s.Embedded || s.OnPath != ""
}

// AllStatuses reports Status for every tool Resolve can be asked for, in
// the same preference order Resolve itself checks (embedded, then PATH).
func AllStatuses() []Status {
	out := make([]Status, 0, len(knownTools))
	for _, tool := range knownTools {
		st := Status{Tool: tool}
		if _, err := fs.Stat(embedded, "data/"+platformDir()+"/"+binaryName(tool)); err == nil {
			st.Embedded = true
		}
		if path, err := exec.LookPath(tool); err == nil {
			st.OnPath = path
		}
		out = append(out, st)
	}
	return out
}

// versionArgs is, per tool, whatever argument makes it print its own
// version - chdman prints a version banner on any invocation (including
// none at all), extract-xiso and skyscraper each have a dedicated flag.
// dolphin-tool has no version output to parse at all, under any flag
// tried; omitted here rather than guessed at.
var versionArgs = map[string][]string{
	"chdman":       nil,
	"extract-xiso": {"-v"},
	"skyscraper":   {"--version"},
}

var versionPattern = regexp.MustCompile(`\d+(?:\.\d+){1,3}`)

// Version best-effort queries tool for its own version string. Unlike
// AllStatuses, this resolves (and, for an embedded tool, extracts to
// cache) the actual binary - the real side effect Resolve always has -
// and runs it, so it only makes sense to call for a tool Status already
// reports as Available. Returns "" if the tool has no known way to
// report its version (dolphin-tool), isn't resolvable, or its output
// didn't contain anything version-shaped.
func Version(tool string) string {
	args, ok := versionArgs[tool]
	if !ok {
		return ""
	}
	path, err := Resolve(tool)
	if err != nil {
		return ""
	}
	out, _ := executil.Run(path, args...)
	m := versionPattern.FindString(string(out))
	if m == "" {
		return ""
	}
	return "v" + m
}
