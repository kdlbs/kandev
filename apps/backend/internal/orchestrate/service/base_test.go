package service_test

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrate/repository/sqlite"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func newTestService(t *testing.T) *service.Service {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Create tasks table so project task counts work.
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		project_id TEXT DEFAULT '',
		state TEXT NOT NULL DEFAULT 'TODO',
		title TEXT DEFAULT '',
		description TEXT DEFAULT '',
		identifier TEXT DEFAULT '',
		workflow_id TEXT DEFAULT '',
		workflow_step_id TEXT DEFAULT '',
		priority INTEGER DEFAULT 0,
		position INTEGER DEFAULT 0,
		is_ephemeral INTEGER DEFAULT 0,
		parent_id TEXT DEFAULT '',
		assignee_agent_instance_id TEXT DEFAULT '',
		execution_policy TEXT DEFAULT '',
		execution_state TEXT DEFAULT '',
		checkout_agent_id TEXT,
		checkout_at DATETIME,
		archived_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("create tasks table: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	log := logger.Default()
	return service.NewService(repo, log)
}
