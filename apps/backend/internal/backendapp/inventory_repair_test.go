package backendapp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/backendapp/ownershiplock"
	"github.com/kandev/kandev/internal/common/config"
)

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
