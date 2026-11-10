package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadLibraryConfig(t *testing.T) {
	path := writeConfig(t, `[library]
root = "/path/to/library"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Library.Root != "/path/to/library" {
		t.Errorf("Library.Root = %q", cfg.Library.Root)
	}
}

func TestLoadMinimalConfig(t *testing.T) {
	path := writeConfig(t, `[sync]
host = "host1.example.com"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sync.Host != "host1.example.com" {
		t.Errorf("Sync.Host = %q", cfg.Sync.Host)
	}
	if cfg.Sync.User != "" {
		t.Errorf("Sync.User should be empty (defaulted at sync time), got %q", cfg.Sync.User)
	}
	if cfg.Sync.Path != "" {
		t.Errorf("Sync.Path should be empty (resolved at sync time), got %q", cfg.Sync.Path)
	}
}

func TestLoadExplicitPath(t *testing.T) {
	path := writeConfig(t, `[sync]
user = "deck"
host = "host1.example.com"
path = "/path/on/device/retrodeck"
exclude = ["*.keep"]
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sync.Path != "/path/on/device/retrodeck" {
		t.Errorf("Sync.Path = %q", cfg.Sync.Path)
	}
	if len(cfg.Sync.Exclude) != 1 || cfg.Sync.Exclude[0] != "*.keep" {
		t.Errorf("Sync.Exclude = %v", cfg.Sync.Exclude)
	}
}

func TestLoadScrapeConfig(t *testing.T) {
	path := writeConfig(t, `[scrape]
user = "ssid:sspassword"
threads = 6
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Scrape.User != "ssid:sspassword" {
		t.Errorf("Scrape.User = %q", cfg.Scrape.User)
	}
	if cfg.Scrape.Threads != 6 {
		t.Errorf("Scrape.Threads = %d", cfg.Scrape.Threads)
	}
}

func TestLoadNamingConfig(t *testing.T) {
	path := writeConfig(t, `[naming]
scheme = "redump"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Naming.Scheme != "redump" {
		t.Errorf("Naming.Scheme = %q, want %q", cfg.Naming.Scheme, "redump")
	}
}

func TestLoadUnknownKey(t *testing.T) {
	path := writeConfig(t, `[sync]
nonsense = 1
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for an unknown key")
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sync.Host != "" || cfg.Scrape.User != "" {
		t.Errorf("expected a zero-value Config, got %+v", cfg)
	}
}

func TestDefaultPathUsesExplicitOverride(t *testing.T) {
	t.Setenv("ROMCTL_CONFIG", "/explicit/path/config.toml")
	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/explicit/path/config.toml" {
		t.Errorf("DefaultPath() = %q, want %q", got, "/explicit/path/config.toml")
	}
}

func TestDefaultPathUsesXDGConfigHome(t *testing.T) {
	t.Setenv("ROMCTL_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg/config")
	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/xdg/config", "romctl", "config.toml")
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

// TestDefaultPathFallsBackToHomeConfig guards against raw string
// concatenation (home + "/.config") producing a mixed-separator path on
// a platform where os.UserHomeDir() doesn't return a "/"-separated
// value (Windows, where it reads %USERPROFILE%) - filepath.Join builds
// the path components cleanly instead.
func TestDefaultPathFallsBackToHomeConfig(t *testing.T) {
	t.Setenv("ROMCTL_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "romctl", "config.toml")
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}
