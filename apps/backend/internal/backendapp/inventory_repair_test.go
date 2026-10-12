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
	"github.com/kandev/kandev/internal/task/inventoryrepair"
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

func TestInventoryRepairBlocksDatabaseAliases(t *testing.T) {
	for _, parentAlias := range []bool{false, true} {
		t.Run(map[bool]string{false: "database symlink", true: "parent symlink"}[parentAlias], func(t *testing.T) {
			database := filepath.Join(t.TempDir(), "kandev.db")
			if err := os.WriteFile(database, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(database+".inventory-repair.json", []byte("pending repair"), 0600); err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(t.TempDir(), "alias")
			target := database
			if parentAlias {
				target = filepath.Dir(database)
			}
			if err := os.Symlink(target, alias); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if parentAlias {
				alias = filepath.Join(alias, filepath.Base(database))
			}
			// A different home has no home fence and must find the database fence.
			cfg := &config.Config{HomeDir: t.TempDir()}
			cfg.Database.Driver, cfg.Database.Path = "sqlite", alias
			owner, err := acquireRuntimeStateOwnership(cfg)
			if owner != nil {
				_ = owner.Close()
			}
			if !errors.Is(err, inventoryrepair.ErrRepairPending) {
				t.Fatalf("database alias bypassed repair fence: %v", err)
			}
			targets, err := ownershiplock.Targets(cfg.HomeDir, "sqlite", database)
			if err != nil {
				t.Fatal(err)
			}
			owner, err = ownershiplock.Acquire(targets)
			if err != nil {
				t.Fatal("failed startup leaked canonical lock: ", err)
			}
			_ = owner.Close()
		})
	}
}
