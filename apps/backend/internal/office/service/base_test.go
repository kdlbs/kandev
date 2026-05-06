package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/costs"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
)

// initSharedAgentProfilesSchema brings up the merged agent_profiles table.
// Office agent CRUD now reads/writes that table (ADR 0005 Wave C); tests
// must initialise the settings store schema before initialising the office
// repo.
func initSharedAgentProfilesSchema(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if _, _, err := settingsstore.Provide(db, db); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
}

// newTestService creates a Service for testing. Callers may supply a
// ServiceOptions to inject mocks or override defaults; Repo, Logger,
// CfgLoader and CfgWriter are always set from the test harness.
func newTestService(t *testing.T, overrides ...service.ServiceOptions) *service.Service {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Create tasks table so project task counts work. ADR 0005 Wave F:
	// the assignee column was dropped — runner participants live in
	// workflow_step_participants and the office repo's runner
	// projection reads from the stub tables seeded below.
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
		priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('critical','high','medium','low')),
		position INTEGER DEFAULT 0,
		is_ephemeral INTEGER DEFAULT 0,
		parent_id TEXT DEFAULT '',
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
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS workflow_steps (
		id TEXT PRIMARY KEY,
		agent_profile_id TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create workflow_steps: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS workflow_step_participants (
		id TEXT PRIMARY KEY,
		step_id TEXT NOT NULL DEFAULT '',
		task_id TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT '',
		agent_profile_id TEXT NOT NULL DEFAULT '',
		decision_required INTEGER NOT NULL DEFAULT 0,
		position INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		t.Fatalf("create workflow_step_participants: %v", err)
	}

	initSharedAgentProfilesSchema(t, db)

	repo, err := sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()

	// Always set up ConfigLoader + FileWriter so config entity CRUD works.
	base := t.TempDir()
	wsDir := filepath.Join(base, "workspaces", "default")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "kandev.yml"), []byte("name: default\n"), 0o644); err != nil {
		t.Fatalf("write kandev.yml: %v", err)
	}
	loader := configloader.NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	writer := configloader.NewFileWriter(base, loader)

	opts := service.ServiceOptions{
		Repo:      repo,
		Logger:    log,
		CfgLoader: loader,
		CfgWriter: writer,
	}

	// Apply caller-supplied overrides (mocks, extra deps, etc.).
	if len(overrides) > 0 {
		applyServiceOverrides(&opts, overrides[0])
	}

	svc := service.NewService(opts)
	// Wire a costs.CostService so svc.CheckPreExecutionBudget and
	// svc.CheckBudget exercise the canonical budgets implementation.
	// The office service itself satisfies AgentReader+AgentWriter, so
	// pause-agent budget paths run end-to-end against the same repo.
	activity := shared.NewActivityLogger(repo, log)
	svc.SetBudgetChecker(costs.NewCostService(repo, log, activity, svc, svc))
	return svc
}

// applyServiceOverrides merges non-zero fields from o into opts.
func applyServiceOverrides(opts *service.ServiceOptions, o service.ServiceOptions) {
	if o.CfgLoader != nil {
		opts.CfgLoader = o.CfgLoader
	}
	if o.CfgWriter != nil {
		opts.CfgWriter = o.CfgWriter
	}
	if o.TaskStarter != nil {
		opts.TaskStarter = o.TaskStarter
	}
	if o.TaskCanceller != nil {
		opts.TaskCanceller = o.TaskCanceller
	}
	if o.TaskWorkspace != nil {
		opts.TaskWorkspace = o.TaskWorkspace
	}
	if o.TaskCreator != nil {
		opts.TaskCreator = o.TaskCreator
	}
	if o.WorkspaceCreator != nil {
		opts.WorkspaceCreator = o.WorkspaceCreator
	}
	if o.AgentTypeResolver != nil {
		opts.AgentTypeResolver = o.AgentTypeResolver
	}
	if o.ProjectSkillDirResolver != nil {
		opts.ProjectSkillDirResolver = o.ProjectSkillDirResolver
	}
	if o.APIBaseURL != "" {
		opts.APIBaseURL = o.APIBaseURL
	}
	if o.GitManager != nil {
		opts.GitManager = o.GitManager
	}
	if o.EventBus != nil {
		opts.EventBus = o.EventBus
	}
}

// insertTestCostEvent inserts a cost event directly into the DB for
// budget rollup tests. costSubcents is hundredths of a cent.
func insertTestCostEvent(t *testing.T, svc interface {
	ExecSQL(t *testing.T, q string, args ...interface{})
}, agentID, taskID string, costSubcents int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	svc.ExecSQL(t,
		`INSERT INTO office_cost_events (id, agent_profile_id, task_id, cost_subcents, occurred_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), agentID, taskID, costSubcents, now, now,
	)
}

// createTestAgent creates an agent instance for test setup.
func createTestAgent(t *testing.T, svc *service.Service, wsID, agentID string) {
	t.Helper()
	agent := &models.AgentInstance{
		ID:          agentID,
		WorkspaceID: wsID,
		Name:        "test-" + agentID,
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create test agent: %v", err)
	}
}

// insertTestTask inserts a task row into the tasks table.
func insertTestTask(t *testing.T, svc *service.Service, taskID, workspaceID string) {
	t.Helper()
	svc.ExecSQL(t,
		`INSERT OR IGNORE INTO tasks (id, workspace_id) VALUES (?, ?)`,
		taskID, workspaceID)
}

// setTestTaskAssignee writes a 'runner' participant row for the task,
// the post-Wave-F replacement for `UPDATE tasks SET assignee_agent_profile_id`.
// The participant key is (task.workflow_step_id, task_id, role='runner');
// when the task has no workflow_step_id the row is keyed against an
// empty step_id, which the runner projection still resolves because
// the task's workflow_step_id is also empty in that case.
func setTestTaskAssignee(t *testing.T, svc *service.Service, taskID, agentID string) {
	t.Helper()
	if agentID == "" {
		svc.ExecSQL(t,
			`DELETE FROM workflow_step_participants WHERE task_id = ? AND role = 'runner'`,
			taskID)
		return
	}
	// Inline the upsert: probe + insert/update by natural key.
	svc.ExecSQL(t, `INSERT OR REPLACE INTO workflow_step_participants
		(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES (
			COALESCE((SELECT id FROM workflow_step_participants WHERE task_id = ? AND role = 'runner'),
			         'p-runner-' || ?),
			COALESCE((SELECT workflow_step_id FROM tasks WHERE id = ?), ''),
			?, 'runner', ?, 0, 0
		)`, taskID, taskID, taskID, taskID, agentID)
}

// newTestServiceWithConfig returns a service backed by in-memory SQLite with a
// real filesystem ConfigLoader+FileWriter (the same setup newTestService uses).
// The second return value is the tmpDir used for the config root; callers that
// don't need it may discard it with _.
func newTestServiceWithConfig(t *testing.T) (*service.Service, string) {
	t.Helper()
	// Re-create the same wiring as newTestService so callers that need the
	// tmpDir path can use it.
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	initSharedAgentProfilesSchema(t, db)

	repo, err := sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()

	base := t.TempDir()
	wsDir := filepath.Join(base, "workspaces", "default")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "kandev.yml"), []byte("name: default\nslug: default\n"), 0o644); err != nil {
		t.Fatalf("write kandev.yml: %v", err)
	}
	loader := configloader.NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	writer := configloader.NewFileWriter(base, loader)

	svc := service.NewService(service.ServiceOptions{
		Repo:      repo,
		Logger:    log,
		CfgLoader: loader,
		CfgWriter: writer,
	})
	activity := shared.NewActivityLogger(repo, log)
	svc.SetBudgetChecker(costs.NewCostService(repo, log, activity, svc, svc))
	return svc, base
}
