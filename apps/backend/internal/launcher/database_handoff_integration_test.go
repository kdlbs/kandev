package launcher

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/persistence"
)

func TestLauncherDefaultDatabaseHandoffPreservesLegacyTask(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	unsetEnvForTest(t, "KANDEV_DATABASE_PATH")
	unsetEnvForTest(t, "KANDEV_DATABASE_DRIVER")
	t.Chdir(t.TempDir())

	homeDir := t.TempDir()
	t.Setenv("KANDEV_HOME_DIR", homeDir)
	legacyPath := filepath.Join(homeDir, "kandev.db")
	seedLauncherLegacyTaskDatabase(t, legacyPath)

	startupConfig, err := config.Load()
	if err != nil {
		t.Fatalf("load startup configuration: %v", err)
	}
	if got := startupConfig.SourceFor("database.path"); got != config.SourceDefault {
		t.Fatalf("database.path source = %q, want %q", got, config.SourceDefault)
	}

	childEnv := backendEnvForConfig(
		portConfig{BackendPort: 4321, AgentctlPort: 4322},
		"info",
		"warn",
		false,
		"health-token",
		nil,
		startupConfig,
	)
	for _, item := range childEnv {
		if strings.HasPrefix(item, "KANDEV_DATABASE_PATH=") {
			t.Fatalf("default database path was handed to child as an explicit override: %q", item)
		}
	}

	childConfig, err := config.Load()
	if err != nil {
		t.Fatalf("load child configuration: %v", err)
	}
	if childConfig.Database.Path != "" {
		t.Fatalf("child database.path = %q, want a default-derived path", childConfig.Database.Path)
	}

	pool, cleanup, err := persistence.Provide(childConfig, nil, "")
	if err != nil {
		t.Fatalf("provide child database: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	var title string
	if err := pool.Reader().Get(&title, `SELECT title FROM tasks WHERE id = 'handoff-task'`); err != nil {
		t.Fatalf("read handed-off legacy task: %v", err)
	}
	if title != "handoff task" {
		t.Fatalf("handed-off task title = %q, want %q", title, "handoff task")
	}
}

func seedLauncherLegacyTaskDatabase(t *testing.T, path string) {
	t.Helper()
	database, err := sqlx.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, title TEXT NOT NULL)`); err != nil {
		_ = database.Close()
		t.Fatalf("create legacy tasks: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO tasks (id, title) VALUES ('handoff-task', 'handoff task')`); err != nil {
		_ = database.Close()
		t.Fatalf("insert legacy task: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}
}
