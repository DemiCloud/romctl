package tools

import (
	"io/fs"
	"os/exec"
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
