package backendapp

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/backendapp/ownershiplock"
	"github.com/kandev/kandev/internal/common/config"
)

func TestInventoryRepairStartupExplainsRecovery(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".kandev-inventory-repair.json"), []byte("pending repair"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run", "^TestBackendStartupConflictHelper$")
	cmd.Env = append(os.Environ(), "KANDEV_BACKEND_OWNERSHIP_HELPER=1", "KANDEV_HOME_DIR="+home)
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("startup error=%v; output=%s", err, output)
	}
	text := string(output)
	if !strings.Contains(text, "--apply or --rollback") || strings.Contains(text, "second instance") {
		t.Fatalf("wrong pending-repair guidance: %s", text)
	}
	assertNoStartupConflictMarker(t, text)
	if _, err := os.Stat(filepath.Join(home, "data", "kandev.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("startup initialized database before repair: %v", err)
	}
}

func TestInventoryRepairBlocksStartupAndReleasesOwnership(t *testing.T) {
	for _, external := range []bool{false, true} {
		t.Run(map[bool]string{false: "home", true: "external database"}[external], func(t *testing.T) {
			home := t.TempDir()
			cfg := &config.Config{HomeDir: home}
			cfg.Database.Driver = "sqlite"
			cfg.Database.Path = filepath.Join(t.TempDir(), "kandev.db")
			fence := filepath.Join(home, ".kandev-inventory-repair.json")
			if external {
				fence = cfg.Database.Path + ".inventory-repair.json"
			}
			if err := os.WriteFile(fence, []byte("pending repair"), 0600); err != nil {
				t.Fatal(err)
			}
			owner, err := acquireRuntimeStateOwnership(cfg)
			if err == nil {
				_ = owner.Close()
				t.Fatal("startup admitted unresolved inventory repair")
			}
			targets, err := ownershiplock.Targets(home, cfg.Database.Driver, cfg.Database.Path)
			if err != nil {
				t.Fatal(err)
			}
			owner, err = ownershiplock.Acquire(targets)
			if err != nil {
				t.Fatal("failed startup leaked lock: ", err)
			}
			_ = owner.Close()
		})
	}
}
