package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvedDataDir_ExplicitConfig(t *testing.T) {
	cfg := &Config{DataDir: "/custom/data"}
	if got := cfg.ResolvedDataDir(); got != "/custom/data" {
		t.Errorf("ResolvedDataDir() = %q, want %q", got, "/custom/data")
	}
}

func TestResolvedDataDir_FallbackToHome(t *testing.T) {
	cfg := &Config{}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	want := filepath.Join(home, ".kandev", "data")
	if got := cfg.ResolvedDataDir(); got != want {
		t.Errorf("ResolvedDataDir() = %q, want %q", got, want)
	}
}

func TestResolvedDataDir_ConfigTakesPrecedence(t *testing.T) {
	t.Setenv("KANDEV_DATA_DIR", "/env/data")
	// Even with env set, explicit Config.DataDir wins
	cfg := &Config{DataDir: "/explicit/data"}
	if got := cfg.ResolvedDataDir(); got != "/explicit/data" {
		t.Errorf("ResolvedDataDir() = %q, want %q", got, "/explicit/data")
	}
}

func TestResolvedDataDir_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	cfg := &Config{DataDir: "~/kandev"}
	want := filepath.Join(home, "kandev")
	if got := cfg.ResolvedDataDir(); got != want {
		t.Errorf("ResolvedDataDir() = %q, want %q", got, want)
	}
}
