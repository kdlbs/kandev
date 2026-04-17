package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
)

func TestMigrateLegacyDBPath_MovesLegacyDB(t *testing.T) {
	homeDir := t.TempDir()
	legacyPath := filepath.Join(homeDir, "kandev.db")
	newPath := filepath.Join(homeDir, "data", "kandev.db")

	if err := os.WriteFile(legacyPath, []byte("legacy-db-content"), 0o600); err != nil {
		t.Fatalf("seed legacy db: %v", err)
	}

	cfg := &config.Config{HomeDir: homeDir}
	if err := migrateLegacyDBPath(cfg, newPath, nil); err != nil {
		t.Fatalf("migrateLegacyDBPath: %v", err)
	}

	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Errorf("legacy file still exists: %v", err)
	}
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read migrated db: %v", err)
	}
	if string(data) != "legacy-db-content" {
		t.Errorf("migrated content = %q, want %q", data, "legacy-db-content")
	}
}

func TestMigrateLegacyDBPath_MovesWalAndShm(t *testing.T) {
	homeDir := t.TempDir()
	legacyPath := filepath.Join(homeDir, "kandev.db")
	newPath := filepath.Join(homeDir, "data", "kandev.db")

	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.WriteFile(legacyPath+suffix, []byte("x"+suffix), 0o600); err != nil {
			t.Fatalf("seed %s: %v", legacyPath+suffix, err)
		}
	}

	cfg := &config.Config{HomeDir: homeDir}
	if err := migrateLegacyDBPath(cfg, newPath, nil); err != nil {
		t.Fatalf("migrateLegacyDBPath: %v", err)
	}

	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(newPath + suffix); err != nil {
			t.Errorf("missing new file %s: %v", newPath+suffix, err)
		}
		if _, err := os.Stat(legacyPath + suffix); !os.IsNotExist(err) {
			t.Errorf("legacy file %s still exists", legacyPath+suffix)
		}
	}
}

func TestMigrateLegacyDBPath_NoLegacyFile(t *testing.T) {
	homeDir := t.TempDir()
	newPath := filepath.Join(homeDir, "data", "kandev.db")

	cfg := &config.Config{HomeDir: homeDir}
	if err := migrateLegacyDBPath(cfg, newPath, nil); err != nil {
		t.Fatalf("migrateLegacyDBPath: %v", err)
	}

	// Should be a no-op; the data dir should NOT be created if nothing to migrate.
	if _, err := os.Stat(filepath.Dir(newPath)); !os.IsNotExist(err) {
		t.Errorf("data dir was created unexpectedly")
	}
}

func TestMigrateLegacyDBPath_NewPathExists_SkipsMigration(t *testing.T) {
	homeDir := t.TempDir()
	legacyPath := filepath.Join(homeDir, "kandev.db")
	newPath := filepath.Join(homeDir, "data", "kandev.db")

	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("legacy"), 0o600); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o600); err != nil {
		t.Fatalf("seed new: %v", err)
	}

	cfg := &config.Config{HomeDir: homeDir}
	if err := migrateLegacyDBPath(cfg, newPath, nil); err != nil {
		t.Fatalf("migrateLegacyDBPath: %v", err)
	}

	// New path untouched.
	got, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("new path was overwritten: got %q", got)
	}
	// Legacy untouched.
	if _, err := os.Stat(legacyPath); err != nil {
		t.Errorf("legacy file removed despite new existing: %v", err)
	}
}

func TestMigrateLegacyDBPath_LegacyEqualsNew_NoOp(t *testing.T) {
	// If HomeDir somehow resolves so that legacy == new (shouldn't happen with
	// the current resolver, but guard against it), we must not rename a file
	// onto itself.
	tmp := t.TempDir()
	sameFile := filepath.Join(tmp, "kandev.db")
	if err := os.WriteFile(sameFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cfg := &config.Config{HomeDir: tmp}
	// Pretend the derived path is the same as the legacy path.
	if err := migrateLegacyDBPath(cfg, sameFile, nil); err != nil {
		t.Fatalf("migrateLegacyDBPath: %v", err)
	}
	if _, err := os.Stat(sameFile); err != nil {
		t.Errorf("file disappeared: %v", err)
	}
}
