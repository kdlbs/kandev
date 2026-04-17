package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvedHomeDir_NoDataDir(t *testing.T) {
	cfg := &Config{}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	want := filepath.Join(home, ".kandev")
	if got := cfg.ResolvedHomeDir(); got != want {
		t.Errorf("ResolvedHomeDir() = %q, want %q", got, want)
	}
}

func TestResolvedHomeDir_WithDataDir(t *testing.T) {
	cfg := &Config{DataDir: "/data"}
	if got := cfg.ResolvedHomeDir(); got != "/data" {
		t.Errorf("ResolvedHomeDir() = %q, want %q", got, "/data")
	}
}

func TestResolvedHomeDir_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	cfg := &Config{DataDir: "~/mykandev"}
	want := filepath.Join(home, "mykandev")
	if got := cfg.ResolvedHomeDir(); got != want {
		t.Errorf("ResolvedHomeDir() = %q, want %q", got, want)
	}
}

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

func TestResolvedHomeDir_WithHomeDir(t *testing.T) {
	cfg := &Config{HomeDir: "/custom/kandev"}
	if got := cfg.ResolvedHomeDir(); got != "/custom/kandev" {
		t.Errorf("ResolvedHomeDir() = %q, want %q", got, "/custom/kandev")
	}
}

func TestResolvedHomeDir_HomeDirBeatsDataDir(t *testing.T) {
	// HomeDir takes precedence over legacy DataDir override.
	cfg := &Config{HomeDir: "/home/root", DataDir: "/data/flat"}
	if got := cfg.ResolvedHomeDir(); got != "/home/root" {
		t.Errorf("ResolvedHomeDir() = %q, want %q", got, "/home/root")
	}
}

func TestResolvedHomeDir_TildeExpansion_HomeDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	cfg := &Config{HomeDir: "~/.kandev-dev"}
	want := filepath.Join(home, ".kandev-dev")
	if got := cfg.ResolvedHomeDir(); got != want {
		t.Errorf("ResolvedHomeDir() = %q, want %q", got, want)
	}
}

func TestResolvedDataDir_DerivedFromHomeDir(t *testing.T) {
	// When HomeDir is set and DataDir is empty, data lives under <HomeDir>/data.
	cfg := &Config{HomeDir: "/custom/kandev"}
	want := filepath.Join("/custom/kandev", "data")
	if got := cfg.ResolvedDataDir(); got != want {
		t.Errorf("ResolvedDataDir() = %q, want %q", got, want)
	}
}

func TestResolvedDataDir_DataDirBeatsHomeDir(t *testing.T) {
	// Explicit DataDir still wins for flat-layout (Docker) use cases.
	cfg := &Config{HomeDir: "/home/root", DataDir: "/flat/data"}
	if got := cfg.ResolvedDataDir(); got != "/flat/data" {
		t.Errorf("ResolvedDataDir() = %q, want %q", got, "/flat/data")
	}
}
