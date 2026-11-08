package systemdb

import "testing"

func TestDisplayNameWithPrefersOverride(t *testing.T) {
	overrides := map[string]string{"psx": "PlayStation (RetroDECK)"}
	if got := DisplayNameWith("psx", overrides); got != "PlayStation (RetroDECK)" {
		t.Errorf("DisplayNameWith = %q, want override value", got)
	}
}

func TestDisplayNameWithFallsBackToFullName(t *testing.T) {
	if got := DisplayNameWith("psx", nil); got != FullName["psx"] {
		t.Errorf("DisplayNameWith = %q, want %q", got, FullName["psx"])
	}
}

func TestDisplayNameWithFallsBackToDirName(t *testing.T) {
	if got := DisplayNameWith("totally-unknown-system", map[string]string{"other": "x"}); got != "totally-unknown-system" {
		t.Errorf("DisplayNameWith = %q, want the bare directory name", got)
	}
}
