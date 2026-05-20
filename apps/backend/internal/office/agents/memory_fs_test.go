package agents_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newTestAgentServiceWithConfig creates an AgentService wired with a real filesystem
// ConfigLoader+FileWriter for testing memory operations.
func newTestAgentServiceWithConfig(t *testing.T) (*agents.AgentService, string) {
	t.Helper()
	tmpDir := t.TempDir()
	loader := configloader.NewConfigLoader(tmpDir)

	wsDir := filepath.Join(tmpDir, "workspaces", "default")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("create workspace dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "kandev.yml"), []byte("name: default\nslug: default\n"), 0o644); err != nil {
		t.Fatalf("write kandev.yml: %v", err)
	}
	if err := loader.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	writer := configloader.NewFileWriter(tmpDir, loader)

	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()
	svc := agents.NewAgentService(repo, log, nil)
	svc.SetConfigWriter(writer)

	return svc, tmpDir
}

func TestMemoryFilesystem(t *testing.T) {
	svc, tmpDir := newTestAgentServiceWithConfig(t)

	if err := svc.UpsertMemoryFromConfig("test-agent", "knowledge", "greeting", "Hello"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	memPath := filepath.Join(tmpDir, "workspaces", "default", "agents", "test-agent", "memory", "knowledge", "greeting.md")
	if _, err := os.Stat(memPath); err != nil {
		t.Fatalf("file not created: %v", err)
	}
	content, err := svc.GetMemoryFromConfig("test-agent", "knowledge", "greeting")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if content != "Hello" {
		t.Errorf("content = %q, want Hello", content)
	}
	entries, err := svc.ListMemoryFromConfig("test-agent", "knowledge")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if err := svc.DeleteMemoryFromConfig("test-agent", "knowledge", "greeting"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(memPath); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}
