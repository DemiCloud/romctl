package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRootPrecedence(t *testing.T) {
	t.Run("explicit flag wins outright", func(t *testing.T) {
		got := resolveRoot("/explicit", true, "/configured")
		if got != "/explicit" {
			t.Errorf("resolveRoot() = %q, want /explicit", got)
		}
	})

	t.Run("config wins over a local roms/ and autodiscovery", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "roms"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HOME", t.TempDir()) // no local RetroDECK install
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(cwd)

		got := resolveRoot(".", false, "/configured")
		if got != "/configured" {
			t.Errorf("resolveRoot() = %q, want /configured", got)
		}
	})

	t.Run("a local roms/ wins over RetroDECK autodiscovery", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "roms"), 0o755); err != nil {
			t.Fatal(err)
		}
		home := t.TempDir()
		cfgDir := filepath.Join(home, ".var/app/net.retrodeck.retrodeck/config/retrodeck")
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfgDir, "retrodeck.json"),
			[]byte(`{"paths": {"rd_home_path": "/some/other/retrodeck"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HOME", home)
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(cwd)

		got := resolveRoot(".", false, "")
		if got != "." {
			t.Errorf("resolveRoot() = %q, want . (the local roms/ dir), not the RetroDECK install's path", got)
		}
	})

	t.Run("falls back to RetroDECK autodiscovery when there's no local roms/", func(t *testing.T) {
		dir := t.TempDir() // no roms/ here
		home := t.TempDir()
		cfgDir := filepath.Join(home, ".var/app/net.retrodeck.retrodeck/config/retrodeck")
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, "retrodeck")
		if err := os.WriteFile(filepath.Join(cfgDir, "retrodeck.json"),
			[]byte(`{"paths": {"rd_home_path": "`+want+`"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HOME", home)
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(cwd)

		got := resolveRoot(".", false, "")
		if got != want {
			t.Errorf("resolveRoot() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to the flag default when nothing else applies", func(t *testing.T) {
		dir := t.TempDir() // no roms/ here
		t.Setenv("HOME", t.TempDir())
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(cwd)

		got := resolveRoot(".", false, "")
		if got != "." {
			t.Errorf("resolveRoot() = %q, want .", got)
		}
	})
}
