// Package config loads the romctl TOML configuration file, parsed into
// structured data and never executed as code.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds the settings read from the romctl config file.
type Config struct {
	Sync   SyncConfig   `toml:"sync"`
	Scrape ScrapeConfig `toml:"scrape"`
}

// SyncConfig holds the `sync` table.
type SyncConfig struct {
	Host    string   `toml:"host"`
	User    string   `toml:"user"` // defaults to "deck", the standard Steam Deck username, if unset
	Path    string   `toml:"path"` // optional override; if unset, resolved from RetroDECK's own config over SSH (see internal/sync)
	Exclude []string `toml:"exclude"`
}

// ScrapeConfig holds the `scrape` table.
type ScrapeConfig struct {
	User    string `toml:"user"`    // ScreenScraper personal account, "ssid:sspassword" (see internal/commands's Scrape)
	Threads int    `toml:"threads"` // 0 means let Skyscraper auto-detect the account's real allowance (see internal/commands's Scrape)
}

// DefaultPath returns the config file path: $ROMCTL_CONFIG, or
// $XDG_CONFIG_HOME/romctl/config.toml, or ~/.config/romctl/config.toml.
func DefaultPath() (string, error) {
	if p := os.Getenv("ROMCTL_CONFIG"); p != "" {
		return p, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = home + "/.config"
	}
	return base + "/romctl/config.toml", nil
}

// Load reads and parses the TOML config file at path. A missing file is
// not an error; it returns a zero-value Config.
func Load(path string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	meta, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return cfg, fmt.Errorf("%s: unknown key(s): %s", path, strings.Join(keys, ", "))
	}
	cfg.Sync.Path = strings.TrimSuffix(cfg.Sync.Path, "/")
	return cfg, nil
}
