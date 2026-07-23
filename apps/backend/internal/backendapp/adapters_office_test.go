package backendapp

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

type adapterStartStepResolver struct {
	repo *tasksqlite.Repository
}

func (r *adapterStartStepResolver) ResolveStartStep(ctx context.Context, workflowID string) (string, error) {
	var stepID string
	err := r.repo.DB().QueryRowContext(ctx,
		`SELECT id FROM workflow_steps WHERE workflow_id = ? AND is_start_step = 1 LIMIT 1`,
		workflowID,
	).Scan(&stepID)
	if err == sql.ErrNoRows {
		return r.ResolveFirstStep(ctx, workflowID)
	}
	return stepID, err
}

func (r *adapterStartStepResolver) ResolveFirstStep(ctx context.Context, workflowID string) (string, error) {
	var stepID string
	err := r.repo.DB().QueryRowContext(ctx,
		`SELECT id FROM workflow_steps WHERE workflow_id = ? ORDER BY position LIMIT 1`,
		workflowID,
	).Scan(&stepID)
	return stepID, err
}

func TestTaskCreatorAdapterPersistsOriginByCreationPath(t *testing.T) {
	adapter, taskSvc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()

	agentTaskID, err := adapter.CreateOfficeTaskAsAgent(
		ctx, "ws-1", "project-1", "agent-worker", "Agent task", "Created at runtime",
	)
	if err != nil {
		t.Fatalf("CreateOfficeTaskAsAgent: %v", err)
	}
	agentTask, err := taskSvc.GetTask(ctx, agentTaskID)
	if err != nil {
		t.Fatalf("GetTask(agent): %v", err)
	}
	if agentTask.Origin != models.TaskOriginAgentCreated {
		t.Errorf("agent task origin = %q, want %q", agentTask.Origin, models.TaskOriginAgentCreated)
	}
	if agentTask.ProjectID != "project-1" {
		t.Errorf("agent task project_id = %q, want project-1", agentTask.ProjectID)
	}
	if agentTask.AssigneeAgentProfileID != "agent-worker" {
		t.Errorf("agent task assignee_agent_profile_id = %q, want agent-worker", agentTask.AssigneeAgentProfileID)
	}

	onboardingTaskID, err := adapter.CreateOfficeTask(
		ctx, "ws-1", "project-setup", "agent-ceo", "Onboarding task", "Created during setup",
	)
	if err != nil {
		t.Fatalf("CreateOfficeTask: %v", err)
	}
	onboardingTask, err := taskSvc.GetTask(ctx, onboardingTaskID)
	if err != nil {
		t.Fatalf("GetTask(onboarding): %v", err)
	}
	if onboardingTask.Origin != models.TaskOriginOnboarding {
		t.Errorf("onboarding task origin = %q, want %q", onboardingTask.Origin, models.TaskOriginOnboarding)
	}
}

func newOfficeTaskAdapterHarness(t *testing.T) (*taskCreatorAdapter, *taskservice.Service) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "office-adapter.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	repo, cleanup, err := repository.Provide(database, database, nil)
	if err != nil {
		t.Fatalf("task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	if _, err := worktree.NewSQLiteStore(database, database); err != nil {
		t.Fatalf("worktree store: %v", err)
	}
	if _, err := officesqlite.NewWithDB(database, database, nil); err != nil {
		t.Fatalf("office migrations: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	taskSvc := taskservice.NewService(taskservice.Repos{
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
		ResourceCleanups: repo,
	}, bus.NewMemoryEventBus(log), log, taskservice.RepositoryDiscoveryConfig{})

	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := repo.EnsureOfficeWorkflow(ctx, "ws-1"); err != nil {
		t.Fatalf("ensure office workflow: %v", err)
	}
	taskSvc.SetStartStepResolver(&adapterStartStepResolver{repo: repo})
	return &taskCreatorAdapter{taskSvc: taskSvc}, taskSvc
}
