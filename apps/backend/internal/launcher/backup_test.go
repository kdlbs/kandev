package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsProductionDb(t *testing.T) {
	devCases := []string{
		"/repo/.kandev-dev/data/kandev.db",
		"/.kandev-dev/kandev.db",
		filepath.Join("repo", ".kandev-dev", "data", "kandev.db"),
	}
	for _, dbPath := range devCases {
		if isProductionDb(dbPath) {
			t.Fatalf("isProductionDb(%q) = true, want false (dev-isolated)", dbPath)
		}
	}

	prodCases := []string{
		"/home/user/.kandev/data/kandev.db",
		"/tmp/custom.db",
		"/data/kandev.db",
		"/repo/.kandev/data/kandev.db",
	}
	for _, dbPath := range prodCases {
		if !isProductionDb(dbPath) {
			t.Fatalf("isProductionDb(%q) = false, want true", dbPath)
		}
	}
}

func TestBackupProductionDbSkipsMissingSource(t *testing.T) {
	path, err := backupProductionDb(filepath.Join(t.TempDir(), "missing.db"), t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		t.Fatalf("backupProductionDb(missing) = %q, want empty path", path)
	}
}

func TestBackupProductionDbCreatesStampedSnapshot(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, ".kandev", "data", "kandev.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("original-db-content"), 0o600); err != nil {
		t.Fatal(err)
	}

	stamp := time.Date(2026, 8, 7, 12, 30, 45, 123_000_000, time.UTC)
	created, err := backupProductionDb(dbPath, home, stamp)
	if err != nil {
		t.Fatal(err)
	}

	wantName := backupPrefix + "2026-08-07T123045123Z" + backupSuffix
	wantPath := filepath.Join(home, ".kandev", "data", "backups", wantName)
	if created != wantPath {
		t.Fatalf("backup path = %q, want %q", created, wantPath)
	}
	data, err := os.ReadFile(created)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original-db-content" {
		t.Fatalf("backup content = %q", data)
	}
	info, err := os.Stat(created)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(stamp) {
		t.Fatalf("backup mtime = %v, want %v", info.ModTime(), stamp)
	}
}

func TestBackupProductionDbPrunesToFiveNewest(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "kandev.db")
	if err := os.WriteFile(dbPath, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		if _, err := backupProductionDb(dbPath, home, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}

	backupDir := filepath.Join(home, ".kandev", "data", "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), backupPrefix) && strings.HasSuffix(entry.Name(), backupSuffix) {
			kept = append(kept, entry.Name())
		}
	}
	if len(kept) != maxBackups {
		t.Fatalf("kept %d backups, want %d: %v", len(kept), maxBackups, kept)
	}
	// The newest five survive; the first (oldest) is pruned.
	for _, name := range kept {
		if strings.Contains(name, "000000Z") {
			t.Fatalf("oldest backup %q was not pruned", name)
		}
	}
}

func TestBackupProductionDbPruneLeavesOtherFamilies(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "kandev.db")
	if err := os.WriteFile(dbPath, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}

	backupDir := filepath.Join(home, ".kandev", "data", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := []string{
		"kandev-20260807T000000Z.db",
		"manual-20260807T000000Z.db",
		"dev-prod-db-notes.txt",
		"readme.md",
	}
	for _, name := range foreign {
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	base := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		if _, err := backupProductionDb(dbPath, home, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}

	for _, name := range foreign {
		if _, err := os.Stat(filepath.Join(backupDir, name)); err != nil {
			t.Fatalf("pruning removed foreign file %q", name)
		}
	}
}
