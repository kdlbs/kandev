package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadPopulatesServerIdentity pins that every launch gets a non-empty,
// unique opaque identity value -- the "opaque per-launch value echoed on an
// identity endpoint" design 01 describes -- regardless of whether bootstrap
// auth is configured.
func TestLoadPopulatesServerIdentity(t *testing.T) {
	first := load(nil)
	second := load(nil)

	if first.ServerIdentity == "" {
		t.Fatal("ServerIdentity is empty, want a generated value")
	}
	if first.ServerIdentity == second.ServerIdentity {
		t.Fatal("ServerIdentity is identical across two loads, want distinct per-launch values")
	}
}

// TestLoadResolvesHomeDirFromKandevHomeDirEnv pins that HomeDir honors
// KANDEV_HOME_DIR directly (already the Kandev root), matching
// adapter/transport/shared.resolveACPLogDir's precedence so agentctl reports
// the same installation identity everywhere it resolves the Kandev home.
func TestLoadResolvesHomeDirFromKandevHomeDirEnv(t *testing.T) {
	t.Setenv("KANDEV_HOME_DIR", "/custom/kandev/home")

	cfg := load(nil)

	if cfg.HomeDir != "/custom/kandev/home" {
		t.Fatalf("HomeDir = %q, want %q", cfg.HomeDir, "/custom/kandev/home")
	}
}

// TestLoadResolvesHomeDirFallsBackToUserHomeDir pins the fallback when
// KANDEV_HOME_DIR is unset: the user's home directory plus ".kandev".
func TestLoadResolvesHomeDirFallsBackToUserHomeDir(t *testing.T) {
	t.Setenv("KANDEV_HOME_DIR", "")

	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no user home dir available in this environment: %v", err)
	}

	cfg := load(nil)

	want := filepath.Join(userHome, ".kandev")
	if cfg.HomeDir != want {
		t.Fatalf("HomeDir = %q, want %q", cfg.HomeDir, want)
	}
}
