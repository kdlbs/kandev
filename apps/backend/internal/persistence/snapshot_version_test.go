package persistence

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

const historicalSDKVersion = "pluginsdk/v0.1.0-16-gbe9d2485554"

// @covers AC-PLATFORM-SQLITE-SNAPSHOT-001.3, AC-PLATFORM-SQLITE-SNAPSHOT-001.4, AC-PLATFORM-STARTUP-LIFECYCLE-001.7
func TestProvide_UpgradeFromNamespacedVersion(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data directory: %v", err)
	}
	dbPath := filepath.Join(dataDir, "kandev.db")
	seedV0DB(t, dbPath)

	seededDB, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open seeded database: %v", err)
	}
	t.Cleanup(func() { _ = seededDB.Close() })
	if err := ensureMetaTable(seededDB); err != nil {
		t.Fatalf("ensure metadata table: %v", err)
	}
	if err := WriteVersion(seededDB, historicalSDKVersion); err != nil {
		t.Fatalf("write historical version: %v", err)
	}
	if err := seededDB.Close(); err != nil {
		t.Fatalf("close seeded database: %v", err)
	}

	cfg := &config.Config{HomeDir: dir}
	cfg.Database.Path = dbPath
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "warn", Format: "json", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	pool, cleanup, err := Provide(cfg, log, "vCURRENT")
	if err != nil {
		t.Fatalf("Provide: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	backupDir := filepath.Join(dataDir, "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backups directory: %v", err)
	}
	if len(entries) != 1 || entries[0].IsDir() {
		t.Fatalf("backups entries = %#v, want one published file directly in backups", entries)
	}

	backupPath := filepath.Join(backupDir, entries[0].Name())
	if !strings.HasPrefix(entries[0].Name(), "kandev-pluginsdk_v0.1.0-16-gbe9d2485554-") {
		t.Fatalf("snapshot name = %q, want normalized historical version", entries[0].Name())
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("stat snapshot: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot mode = %#o, want 0600", info.Mode().Perm())
	}
	snapshot, err := sqlx.Open("sqlite3", backupPath)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	t.Cleanup(func() { _ = snapshot.Close() })
	var title, snapshotVersion, storedVersion string
	if err := snapshot.Get(&title, `SELECT title FROM tasks WHERE id = 't1'`); err != nil {
		t.Fatalf("read seeded snapshot row: %v", err)
	}
	if title != "hello" {
		t.Fatalf("snapshot task title = %q, want %q", title, "hello")
	}
	if err := snapshot.Get(&snapshotVersion, `SELECT value FROM kandev_meta WHERE key = 'kandev_version'`); err != nil {
		t.Fatalf("read snapshot version: %v", err)
	}
	if snapshotVersion != historicalSDKVersion {
		t.Fatalf("snapshot version = %q, want unchanged %q", snapshotVersion, historicalSDKVersion)
	}
	if err := pool.Writer().Get(&storedVersion, `SELECT value FROM kandev_meta WHERE key = 'kandev_version'`); err != nil {
		t.Fatalf("read source version: %v", err)
	}
	if storedVersion != historicalSDKVersion {
		t.Fatalf("source version = %q, want unchanged %q", storedVersion, historicalSDKVersion)
	}
}

func TestSnapshotPathVersionCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "ordinary version", version: "v1.2.3-alpha+build_7", want: "v1.2.3-alpha+build_7"},
		{name: "empty version fallback", version: "", want: "pre-meta"},
		{name: "reported namespaced SDK version", version: historicalSDKVersion, want: "pluginsdk_v0.1.0-16-gbe9d2485554"},
		{name: "backslash separator", version: `pluginsdk\v0.1.0-16-gbe9d2485554`, want: "pluginsdk_v0.1.0-16-gbe9d2485554"},
		{name: "traversal shaped version", version: "../../outside", want: ".._.._outside"},
		{name: "reserved characters", version: "v1:2?3*build<>|é", want: "v1_2_3_build____"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backupDir := t.TempDir()
			path := snapshotPath(backupDir, tt.version)
			if filepath.Dir(path) != backupDir {
				t.Fatalf("snapshot directory = %q, want %q", filepath.Dir(path), backupDir)
			}
			name := filepath.Base(path)
			if !strings.HasPrefix(name, "kandev-"+tt.want+"-") || !strings.HasSuffix(name, ".db") {
				t.Fatalf("snapshot name = %q, want version component %q", name, tt.want)
			}
			if strings.ContainsAny(name, `/\`) {
				t.Fatalf("snapshot name contains a path separator: %q", name)
			}
		})
	}
}
