// Package version holds the romctl release version.
package version

// Version is the romctl release version, overridden at build time with
// -ldflags "-X github.com/demicloud/romctl/internal/version.Version=...".
var Version = "dev"
