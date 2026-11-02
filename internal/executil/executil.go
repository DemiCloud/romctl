// Package executil runs external processes (chdman, dolphin-tool, rsync,
// ssh) with the terminal-safety and stdin precautions the Bash tool
// applied to every external command: stdin from the null device (some
// tools read stdin or alter terminal state) and the terminal state
// restored afterwards regardless of what the tool did to it.
package executil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/demicloud/romctl/internal/ttyguard"
)

// Run executes path with args, stdin from the null device, and returns
// combined stdout+stderr for error reporting (nothing is shown unless the
// caller prints it). The terminal state is saved before and restored
// after, regardless of outcome. Use this for tools whose output only
// matters on failure (chdman, dolphin-tool).
func Run(path string, args ...string) ([]byte, error) {
	return RunIn("", path, args...)
}

// RunIn is Run with the child's working directory set to dir (ignored if
// empty). Some tools (extract-xiso's -r) write output relative to the
// current directory rather than the input path's directory, so the
// working directory has to be set explicitly to land output correctly.
func RunIn(dir, path string, args ...string) ([]byte, error) {
	guard := ttyguard.Save()
	defer guard.Restore()

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return nil, err
	}
	defer devNull.Close()

	cmd := exec.Command(path, args...)
	cmd.Dir = dir
	cmd.Stdin = devNull
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("%s: %w", path, err)
	}
	return out.Bytes(), nil
}

// RunInherit executes path with args, inheriting the real stdin/stdout/
// stderr: output streams live (progress bars, itemized file lists), and
// stdin stays connected for an interactive password or passphrase prompt.
// Use this for tools whose own I/O is the point (rsync, ssh).
func RunInherit(path string, args ...string) error {
	guard := ttyguard.Save()
	defer guard.Restore()

	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
