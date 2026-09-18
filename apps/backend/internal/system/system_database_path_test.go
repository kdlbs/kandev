package system

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	"github.com/kandev/kandev/internal/system/jobs"
)

func TestConfiguredDatabasePath_ProvideListsSiblingBackups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	databasePath := filepath.Join(root, "custom", "named.db")
	pool := openSystemDatabasePathPool(t, databasePath)
	t.Cleanup(func() { _ = pool.Close() })

	backupDir := filepath.Join(filepath.Dir(databasePath), "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir custom backups: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "manual-1.db"), []byte("custom"), 0o644); err != nil {
		t.Fatalf("write custom backup: %v", err)
	}
	oldBackupDir := filepath.Join(homeDir, "data", "backups")
	if err := os.MkdirAll(oldBackupDir, 0o755); err != nil {
		t.Fatalf("mkdir old backups: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldBackupDir, "manual-old.db"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write old backup: %v", err)
	}

	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	cfg := &config.Config{
		HomeDir: homeDir,
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			Path:   databasePath,
		},
	}
	service := Provide(cfg, log, pool, nil, BuildInfo{}, Wiring{})
	router := gin.New()
	service.RegisterRoutes(router, log)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/backups", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	var payload struct {
		Snapshots []struct {
			Name string `json:"name"`
		} `json:"snapshots"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Snapshots) != 1 || payload.Snapshots[0].Name != "manual-1.db" {
		t.Fatalf("snapshots = %+v, want only custom backup", payload.Snapshots)
	}
}

func TestProvideWiresRestoreQuiesce(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "custom", "named.db")
	pool := openSystemDatabasePathPool(t, databasePath)
	t.Cleanup(func() { _ = pool.Close() })
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	calls := 0
	cfg := &config.Config{
		HomeDir: filepath.Join(root, "home"),
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			Path:   databasePath,
		},
	}
	service := Provide(cfg, log, pool, nil, BuildInfo{}, Wiring{
		RestoreQuiesce: func() error {
			calls++
			return nil
		},
	})
	if service.Backups.RestoreQuiesce == nil {
		t.Fatal("Backups.RestoreQuiesce is nil")
	}
	if err := service.Backups.RestoreQuiesce(); err != nil {
		t.Fatalf("RestoreQuiesce: %v", err)
	}
	if calls != 1 {
		t.Fatalf("RestoreQuiesce calls = %d, want 1", calls)
	}
}

func TestProvideMarksPersistenceUnavailableForFactoryReset(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "custom", "named.db")
	pool := openSystemDatabasePathPool(t, databasePath)
	t.Cleanup(func() { _ = pool.Close() })
	if _, err := pool.Writer().Exec(`CREATE TABLE kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create kandev_meta: %v", err)
	}

	tracker, err := requiredstores.NewTracker([]requiredstores.Descriptor{{
		ID: "settings", OwnerPackage: "internal/system/settings", RequiredTables: []string{"settings"},
	}})
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	if err := tracker.RecordSuccess("settings"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	health := requiredstores.NewHealth(tracker, pool, nil)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	service := Provide(&config.Config{
		HomeDir:  root,
		Database: config.DatabaseConfig{Driver: "sqlite", Path: databasePath},
	}, log, pool, nil, BuildInfo{}, Wiring{
		RequiredStores:    tracker,
		PersistenceHealth: health,
	})
	if err := health.Check(context.Background()); err != nil {
		t.Fatalf("initial health check: %v", err)
	}
	jobID, err := service.Database.FactoryReset(context.Background(), "RESET")
	if err != nil {
		t.Fatalf("FactoryReset: %v", err)
	}
	job := waitForSystemJob(t, service.Jobs, jobID)
	if job.State != jobs.StateSucceeded {
		t.Fatalf("factory reset state = %s, want succeeded: %s", job.State, job.Message)
	}
	if health.Healthy() {
		t.Fatal("required persistence recovered before the frontend restart")
	}
}

func TestProvideMarksPersistenceUnavailableForRestore(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "custom", "named.db")
	pool := openSystemDatabasePathPool(t, databasePath)
	t.Cleanup(func() { _ = pool.Close() })

	tracker, err := requiredstores.NewTracker([]requiredstores.Descriptor{{
		ID: "settings", OwnerPackage: "internal/system/settings", RequiredTables: []string{"settings"},
	}})
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	if err := tracker.RecordSuccess("settings"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	health := requiredstores.NewHealth(tracker, pool, nil)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	service := Provide(&config.Config{
		HomeDir:  root,
		Database: config.DatabaseConfig{Driver: "sqlite", Path: databasePath},
	}, log, pool, nil, BuildInfo{}, Wiring{
		RequiredStores:    tracker,
		PersistenceHealth: health,
	})
	if err := health.Check(context.Background()); err != nil {
		t.Fatalf("initial health check: %v", err)
	}
	snapshotJobID := service.Backups.Create(context.Background())
	snapshotJob := waitForSystemJob(t, service.Jobs, snapshotJobID)
	if snapshotJob.State != jobs.StateSucceeded {
		t.Fatalf("snapshot state = %s, want succeeded: %s", snapshotJob.State, snapshotJob.Message)
	}
	snapshots, err := service.Backups.List()
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("List() = %#v, %v; want one snapshot", snapshots, err)
	}

	restoreJobID, err := service.Backups.Restore(context.Background(), snapshots[0].Name, "RESTORE")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	restoreJob := waitForSystemJob(t, service.Jobs, restoreJobID)
	if restoreJob.State != jobs.StateSucceeded {
		t.Fatalf("restore state = %s, want succeeded: %s", restoreJob.State, restoreJob.Message)
	}
	if health.Healthy() {
		t.Fatal("required persistence recovered before the frontend restart")
	}
}

func waitForSystemJob(t *testing.T, tracker *jobs.Tracker, id string) *jobs.Job {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		job := tracker.Get(id)
		if job != nil && (job.State == jobs.StateSucceeded || job.State == jobs.StateFailed) {
			return job
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("job %q did not finish", id)
		}
	}
}

func openSystemDatabasePathPool(t *testing.T, databasePath string) *db.Pool {
	t.Helper()
	writerRaw, err := db.OpenSQLite(databasePath)
	if err != nil {
		t.Fatalf("open sqlite writer: %v", err)
	}
	readerRaw, err := db.OpenSQLiteReader(databasePath)
	if err != nil {
		_ = writerRaw.Close()
		t.Fatalf("open sqlite reader: %v", err)
	}
	return db.NewPool(sqlx.NewDb(writerRaw, "sqlite3"), sqlx.NewDb(readerRaw, "sqlite3"))
}
