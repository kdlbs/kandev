package service

// End-to-end coverage of AC-OFFICE-RUN-CAUSATION-001.5: a run queued
// because of a task inherits the task-boundary carrier written on that
// task, through the real queueTaskAssignedRun -> QueueRunFromTaskBoundary
// -> runs/service.QueueRun seam and a real sqlite-backed repo. Internal
// (package service) so queueTaskAssignedRun, unexported, can be called
// directly rather than only reachable through a bus event.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func newRunCausationFromTaskTestService(t *testing.T) (*Service, *officesqlite.Repository) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	repo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	log := logger.Default()
	svc := NewService(ServiceOptions{Repo: repo, Logger: log})
	svc.SetRunsService(runsservice.New(repo.RunsRepository(), nil, log, nil))

	// The office schema (agent_instances, runs, ...) doesn't own the task
	// package's workspaces/tasks/workflow_step_participants tables, so this
	// package's tests hand-roll the minimal shape needed for
	// GetTaskExecutionFields/GetTaskMetadata, mirroring
	// office/repository/sqlite/tasks_test.go's newSearchTestRepo.
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			office_workflow_id TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_id TEXT NOT NULL DEFAULT '',
			workflow_step_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			state TEXT DEFAULT 'TODO',
			project_id TEXT DEFAULT '',
			metadata TEXT DEFAULT '{}',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS workflows (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_template_id TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS workflow_steps (
			id TEXT PRIMARY KEY,
			agent_profile_id TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS workflow_step_participants (
			id TEXT PRIMARY KEY,
			step_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT '',
			agent_profile_id TEXT NOT NULL DEFAULT '',
			decision_required INTEGER NOT NULL DEFAULT 0,
			position INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00',
			provenance TEXT NOT NULL DEFAULT 'manual'
		)`,
	} {
		if _, err := repo.ExecRaw(context.Background(), stmt); err != nil {
			t.Fatalf("create task-schema table: %v", err)
		}
	}
	return svc, repo
}

// seedOfficeTaskWithMetadata inserts a task row directly (bypassing
// task/service) into a workspace whose office_workflow_id matches the
// task's workflow_id, so GetTaskExecutionFields projects IsFromOffice.
func seedOfficeTaskWithMetadata(t *testing.T, repo *officesqlite.Repository, taskID string, metadata map[string]interface{}) {
	t.Helper()
	ctx := context.Background()
	if _, err := repo.ExecRaw(ctx, `
		INSERT OR IGNORE INTO workspaces (id, office_workflow_id) VALUES ('ws-1', 'wf-office')
	`); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if _, err := repo.ExecRaw(ctx,
		`INSERT INTO tasks (id, workspace_id, workflow_id, title, metadata, created_at, updated_at)
		 VALUES (?, 'ws-1', 'wf-office', 'Carrier task', ?, datetime('now'), datetime('now'))`,
		taskID, string(raw),
	); err != nil {
		t.Fatalf("seed task: %v", err)
	}
}

func carrierMetadataLiteral(run *models.Run) map[string]interface{} {
	return carrierMetadataFromRun(run)
}

// runCausationTestAgent builds a minimal AgentInstance for these tests.
// Duplicated from agents_test.go's makeAgent (package service_test,
// unreachable from this internal package test file) rather than exported
// solely for test reuse.
func runCausationTestAgent(name string, role models.AgentRole) *models.AgentInstance {
	return &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        name,
		Role:        role,
		Status:      models.AgentStatusIdle,
	}
}

// TestQueueTaskAssignedRun_InheritsCarrierFromTaskMetadata pins
// AC-OFFICE-RUN-CAUSATION-001.5: a run queued for a task carrying the
// causation carrier inherits that carrier's lineage, actor, and routine
// exactly as though the task creation were the causing run.
func TestQueueTaskAssignedRun_InheritsCarrierFromTaskMetadata(t *testing.T) {
	svc, repo := newRunCausationFromTaskTestService(t)
	ctx := context.Background()

	creator := runCausationTestAgent("creator-agent", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, creator); err != nil {
		t.Fatalf("create creator agent: %v", err)
	}
	assignee := runCausationTestAgent("assignee-agent", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, assignee); err != nil {
		t.Fatalf("create assignee agent: %v", err)
	}

	// The creating run: what the task-boundary carrier will point back to.
	if err := svc.QueueRunWithActor(ctx, creator.ID, "spawn_agent_run", `{}`, "",
		models.ActorKindAgent, creator.ID, ""); err != nil {
		t.Fatalf("queue creating run: %v", err)
	}
	runs, err := repo.ListRuns(ctx, "ws-1")
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs: %v (got %d)", err, len(runs))
	}
	creatingRun := runs[0]

	seedOfficeTaskWithMetadata(t, repo, "task-carrier-1", carrierMetadataLiteral(creatingRun))

	if err := svc.queueTaskAssignedRun(ctx, "task-carrier-1", assignee.ID, false); err != nil {
		t.Fatalf("queueTaskAssignedRun: %v", err)
	}

	all, err := repo.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	var assignedRun *models.Run
	for _, r := range all {
		if r.ID != creatingRun.ID {
			assignedRun = r
		}
	}
	if assignedRun == nil {
		t.Fatal("task-assigned run not found")
	}
	if assignedRun.CausationID != creatingRun.CausationID {
		t.Errorf("causation_id = %q, want carried %q", assignedRun.CausationID, creatingRun.CausationID)
	}
	if assignedRun.ParentRunID != creatingRun.ID {
		t.Errorf("parent_run_id = %q, want the carrier's creating run %q", assignedRun.ParentRunID, creatingRun.ID)
	}
	if assignedRun.CausationDepth != creatingRun.CausationDepth+1 {
		t.Errorf("causation_depth = %d, want %d", assignedRun.CausationDepth, creatingRun.CausationDepth+1)
	}
	if assignedRun.ActorKind != creatingRun.ActorKind || assignedRun.ActorID != creatingRun.ActorID {
		t.Errorf("actor = %s/%s, want carried %s/%s",
			assignedRun.ActorKind, assignedRun.ActorID, creatingRun.ActorKind, creatingRun.ActorID)
	}
	if assignedRun.HumanRooted != creatingRun.HumanRooted {
		t.Errorf("human_rooted = %v, want carried %v", assignedRun.HumanRooted, creatingRun.HumanRooted)
	}
}

// TestQueueTaskAssignedRun_NoCarrierRootsAsSystemActor pins the fallback
// for a task that never went through carrier-writing (an ordinary
// human-created task assigned to an office agent): the resulting run
// roots as a system actor, matching pre-carrier behaviour exactly.
func TestQueueTaskAssignedRun_NoCarrierRootsAsSystemActor(t *testing.T) {
	svc, repo := newRunCausationFromTaskTestService(t)
	ctx := context.Background()

	assignee := runCausationTestAgent("assignee-agent", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, assignee); err != nil {
		t.Fatalf("create assignee agent: %v", err)
	}
	seedOfficeTaskWithMetadata(t, repo, "task-plain-1", map[string]interface{}{
		taskmodels.MetaKeyAutoStartOnCreate: true,
	})

	if err := svc.queueTaskAssignedRun(ctx, "task-plain-1", assignee.ID, false); err != nil {
		t.Fatalf("queueTaskAssignedRun: %v", err)
	}

	runs, err := repo.ListRuns(ctx, "ws-1")
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs: %v (got %d)", err, len(runs))
	}
	run := runs[0]
	if run.ActorKind != models.ActorKindSystem {
		t.Errorf("actor_kind = %q, want %q (no carrier to inherit)", run.ActorKind, models.ActorKindSystem)
	}
	if run.CausationID != run.ID || run.ParentRunID != "" || run.CausationDepth != 0 {
		t.Errorf("lineage = {causation=%q parent=%q depth=%d}, want a fresh root",
			run.CausationID, run.ParentRunID, run.CausationDepth)
	}
	if run.HumanRooted {
		t.Error("human_rooted = true, want false with no carrier")
	}
}
