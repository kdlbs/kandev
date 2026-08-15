package system

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
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
