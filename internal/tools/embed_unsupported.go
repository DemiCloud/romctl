//go:build !((linux && amd64) || (linux && arm64) || (windows && amd64) || (darwin && amd64) || (darwin && arm64))

// This build has no embedded chdman/dolphin-tool for its GOOS/GOARCH;
// Resolve falls back to PATH.
package tools

import "embed"

//go:embed data/unsupported
var embedded embed.FS
