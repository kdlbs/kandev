package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

// createOfficeIntegrationService spins up a Service against a DB that has
// BOTH task and office migrations applied — mirroring production startup.
// The office migration adds CHECK constraints (notably on tasks.priority)
// and toggles foreign_keys=ON, so any code path that lands a row through
// the task service is exercised against the real schema.
func createOfficeIntegrationService(t *testing.T) *Service {
	t.Helper()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("task repository: %v", err)
	}
	if _, err := worktree.NewSQLiteStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("worktree store: %v", err)
	}
	if _, err := officesqlite.NewWithDB(sqlxDB, sqlxDB, nil); err != nil {
		t.Fatalf("office migrations: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlxDB.Close()
		_ = cleanup()
	})
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level: "error", Format: "json", OutputPath: "stdout",
	})
	return NewService(Repos{
		Workspaces:       repo,
		Tasks:            repo,
		TaskRepos:        repo,
		Workflows:        repo,
		Messages:         repo,
		Turns:            repo,
		Sessions:         repo,
		GitSnapshots:     repo,
		RepoEntities:     repo,
		Executors:        repo,
		Environments:     repo,
		TaskEnvironments: repo,
		Reviews:          repo,
	}, NewMockEventBus(), log, RepositoryDiscoveryConfig{})
}

// TestIntegration_CreateTask_OnboardingShape pins the regression that
// exposed the priority CHECK constraint bug end-to-end: the onboarding
// adapter calls CreateTask with no Priority field set (and an
// "onboarding" Origin that flags it as an office task). Before the
// buildTask default landed, the resulting INSERT failed against the
// CHECK constraint added by the office priority migration. This test
// runs both migrations and asserts the row lands cleanly.
func TestIntegration_CreateTask_OnboardingShape(t *testing.T) {
	svc := createOfficeIntegrationService(t)
	ctx := context.Background()

	// The onboarding adapter (cmd/kandev/adapters.go) sends a request
	// without setting Priority and with Origin = "onboarding". Replicate
	// the shape exactly. Workflow is supplied directly so we don't have
	// to wire the workspace office-workflow mapping for this test.
	_ = svc.workspaces.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-1", Name: "Workspace",
	})
	_ = svc.workflows.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-1", WorkspaceID: "ws-1", Name: "Office",
	})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:            "ws-1",
		WorkflowID:             "wf-1",
		Title:                  "First task from onboarding",
		Description:            "What the CEO would receive",
		ProjectID:              "proj-onboarding",
		AssigneeAgentProfileID: "agent-ceo",
		Origin:                 "onboarding",
	})
	if err != nil {
		t.Fatalf("CreateTask should succeed against the real office schema, got %v", err)
	}
	if task.Priority != "medium" {
		t.Errorf("expected default priority 'medium', got %q", task.Priority)
	}
	// Confirm the row actually landed and round-trips out of the DB.
	got, err := svc.tasks.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("read back task: %v", err)
	}
	if got.Priority != "medium" {
		t.Errorf("persisted priority should be 'medium', got %q", got.Priority)
	}
}
