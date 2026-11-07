// Package retrodeck reads RetroDECK's own resolved-paths config
// (retrodeck.json) - the source of truth for where a RetroDECK install
// keeps its library root, correct whether that's internal storage or an
// SD card, without romctl needing to know which. Used both remotely (see
// internal/sync, over SSH) and locally (see cmd/romctl/main.go, for
// romctl running directly on the device itself).
package retrodeck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RemotePath is where RetroDECK's Flatpak keeps retrodeck.json, relative
// to the user's home directory. For use as an ssh command argument,
// where `~` is expanded by the remote login shell - not for local reads,
// which have no shell to do that (see LocalPath).
const RemotePath = "~/.var/app/net.retrodeck.retrodeck/config/retrodeck/retrodeck.json"

// localSuffix is RemotePath with the leading "~/" stripped, joined onto
// a resolved home directory for local reads.
const localSuffix = ".var/app/net.retrodeck.retrodeck/config/retrodeck/retrodeck.json"

// LocalPath returns retrodeck.json's path under the current user's home
// directory.
func LocalPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, localSuffix), nil
}

// ParseRDHomePath extracts paths.rd_home_path - the library root - from
// a retrodeck.json config file's contents.
func ParseRDHomePath(data []byte) (string, error) {
	var parsed struct {
		Paths struct {
			RDHomePath string `json:"rd_home_path"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if parsed.Paths.RDHomePath == "" {
		return "", fmt.Errorf("no paths.rd_home_path in config")
	}
	return parsed.Paths.RDHomePath, nil
}

// ResolveLocal reads retrodeck.json from the local filesystem - romctl
// running directly on the RetroDECK device, not over SSH - and returns
// its rd_home_path. Its error is expected and silently ignored by a
// caller that just wants to try autodiscovery before falling back to
// something else (there's no RetroDECK install here, or none at the
// default location).
func ResolveLocal() (string, error) {
	path, err := LocalPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return ParseRDHomePath(data)
}
