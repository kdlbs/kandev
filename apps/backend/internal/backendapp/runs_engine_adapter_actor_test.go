package backendapp

// Covers AC-OFFICE-RUN-CAUSATION-001.23's "wake caused by a task" row: the
// workflow engine's queue_run action (internal/workflow/engine's
// RunQueueAdapter, bridged here by runsServiceEngineAdapter) always
// resolves a TaskID before calling QueueRun, but until this change never
// read the task-boundary carrier — every engine-queued run reached
// runs/service.QueueRun with a zero-value ActorKind, silently falling back
// to office_launch_actor_missing_total rather than carrying the task's
// real actor. runsServiceEngineAdapter.QueueRun now sources the actor (and
// the rest of the carrier) off officeSvc.TaskBoundaryCarrier, matching
// office/service.QueueRunFromTaskBoundary's existing behaviour for the
// same declared source.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeservice "github.com/kandev/kandev/internal/office/service"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowengine "github.com/kandev/kandev/internal/workflow/engine"
	"github.com/kandev/kandev/internal/worktree"
)

func newRunsEngineAdapterActorTestHarness(t *testing.T) (
	*runsServiceEngineAdapter, *taskservice.Service, *officesqlite.Repository,
) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "engine-adapter.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	taskRepo, cleanup, err := repository.Provide(database, database, nil)
	if err != nil {
		t.Fatalf("task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	if _, err := worktree.NewSQLiteStore(database, database); err != nil {
		t.Fatalf("worktree store: %v", err)
	}
	if _, _, err := settingsstore.Provide(database, database, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	// agent_instances.agent_id is a FK onto this CLI-tool registration row;
	// CreateAgentInstance's inheritance fallback needs at least one to
	// attach to under FK enforcement (db.OpenSQLite always enables it).
	if _, err := database.Exec(
		`INSERT INTO agents (id, name, created_at, updated_at) VALUES ('cli-test', 'Test CLI', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("seed agents row: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("office repository: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces:       taskRepo,
		Tasks:            taskRepo,
		TaskRepos:        taskRepo,
		Workflows:        taskRepo,
		Messages:         taskRepo,
		Turns:            taskRepo,
		Sessions:         taskRepo,
		GitSnapshots:     taskRepo,
		RepoEntities:     taskRepo,
		Executors:        taskRepo,
		Environments:     taskRepo,
		TaskEnvironments: taskRepo,
		Reviews:          taskRepo,
		ResourceCleanups: taskRepo,
	}, bus.NewMemoryEventBus(log), log, taskservice.RepositoryDiscoveryConfig{})

	ctx := context.Background()
	if err := taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := taskRepo.EnsureOfficeWorkflow(ctx, "ws-1"); err != nil {
		t.Fatalf("ensure office workflow: %v", err)
	}
	taskSvc.SetStartStepResolver(&adapterStartStepResolver{repo: taskRepo})

	officeSvc := officeservice.NewService(officeservice.ServiceOptions{Repo: officeRepo, Logger: log})
	runsSvc := runsservice.New(officeRepo.RunsRepository(), nil, log, nil)
	officeSvc.SetRunsService(runsSvc)

	adapter := &runsServiceEngineAdapter{svc: runsSvc, officeSvc: officeSvc}
	return adapter, taskSvc, officeRepo
}

func seedTaskWithCarrier(t *testing.T, taskSvc *taskservice.Service, carrierMetadata map[string]interface{}) string {
	t.Helper()
	ctx := context.Background()
	workflows, err := taskSvc.ListWorkflows(ctx, "ws-1", true)
	if err != nil || len(workflows) == 0 {
		t.Fatalf("ListWorkflows: %v (len=%d)", err, len(workflows))
	}
	// OfficeCarrierMetadata, not Metadata: only the server-trusted field
	// survives create-time carrier stripping (AC-OFFICE-RUN-CAUSATION-001.17).
	result, err := taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{ //nolint:exhaustruct
		WorkspaceID:           "ws-1",
		WorkflowID:            workflows[0].ID,
		Title:                 "Carrier task",
		OfficeCarrierMetadata: carrierMetadata,
		Origin:                models.TaskOriginOnboarding,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return result.Task.ID
}

// TestRunsServiceEngineAdapter_QueueRunInheritsTaskCarrier pins
// AC-OFFICE-RUN-CAUSATION-001.23: a queue_run action (always carrying a
// TaskID) sources its actor from the task-boundary carrier rather than
// reaching the queue with none.
func TestRunsServiceEngineAdapter_QueueRunInheritsTaskCarrier(t *testing.T) {
	adapter, taskSvc, officeRepo := newRunsEngineAdapterActorTestHarness(t)
	ctx := context.Background()

	agent := &officemodels.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "assignee-agent",
		Role:        officemodels.AgentRoleWorker,
		Status:      officemodels.AgentStatusIdle,
	}
	if err := officeRepo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent instance: %v", err)
	}

	taskID := seedTaskWithCarrier(t, taskSvc, map[string]interface{}{
		models.MetaKeyOfficeCarrierCausationID:    "causation-engine-1",
		models.MetaKeyOfficeCarrierCausationDepth: 1,
		models.MetaKeyOfficeCarrierCreatingRunID:  "run-engine-1",
		models.MetaKeyOfficeCarrierHumanRooted:    true,
		models.MetaKeyOfficeCarrierRoutineID:      "routine-engine-1",
		models.MetaKeyOfficeCarrierActorKind:      string(officemodels.ActorKindAgent),
		models.MetaKeyOfficeCarrierActorID:        "creator-agent-1",
	})

	outcome, err := adapter.QueueRun(ctx, workflowengine.QueueRunRequest{
		AgentProfileID: agent.ID,
		TaskID:         taskID,
		Reason:         "on_enter",
	})
	if err != nil {
		t.Fatalf("QueueRun: %v", err)
	}
	if outcome != workflowengine.QueueOutcomeQueued {
		t.Fatalf("outcome = %q, want queued", outcome)
	}

	runs, err := officeRepo.ListRuns(ctx, "ws-1")
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs: %v (got %d)", err, len(runs))
	}
	run := runs[0]
	if run.ActorKind != officemodels.ActorKindAgent || run.ActorID != "creator-agent-1" {
		t.Errorf("actor = %s/%s, want carried agent/creator-agent-1", run.ActorKind, run.ActorID)
	}
	if run.RoutineID != "routine-engine-1" {
		t.Errorf("routine_id = %q, want carried routine-engine-1", run.RoutineID)
	}
	if run.ParentRunID != "run-engine-1" || run.CausationID != "causation-engine-1" || run.CausationDepth != 2 {
		t.Errorf("lineage = {parent=%q causation=%q depth=%d}, want carried run-engine-1/causation-engine-1/2",
			run.ParentRunID, run.CausationID, run.CausationDepth)
	}
	if !run.HumanRooted {
		t.Error("human_rooted = false, want true (carried)")
	}
}

// TestRunsServiceEngineAdapter_QueueRunNoCarrierRootsAsSystemActor pins the
// fallback for a task that never carried a carrier at all: the resulting
// run roots as an unattributed system actor, exactly as before this fix.
func TestRunsServiceEngineAdapter_QueueRunNoCarrierRootsAsSystemActor(t *testing.T) {
	adapter, taskSvc, officeRepo := newRunsEngineAdapterActorTestHarness(t)
	ctx := context.Background()

	agent := &officemodels.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "assignee-agent",
		Role:        officemodels.AgentRoleWorker,
		Status:      officemodels.AgentStatusIdle,
	}
	if err := officeRepo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent instance: %v", err)
	}
	taskID := seedTaskWithCarrier(t, taskSvc, nil)

	if _, err := adapter.QueueRun(ctx, workflowengine.QueueRunRequest{
		AgentProfileID: agent.ID,
		TaskID:         taskID,
		Reason:         "on_enter",
	}); err != nil {
		t.Fatalf("QueueRun: %v", err)
	}

	runs, err := officeRepo.ListRuns(ctx, "ws-1")
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs: %v (got %d)", err, len(runs))
	}
	run := runs[0]
	if run.CausationID != run.ID || run.ParentRunID != "" || run.CausationDepth != 0 {
		t.Errorf("lineage = {causation=%q parent=%q depth=%d}, want a fresh root",
			run.CausationID, run.ParentRunID, run.CausationDepth)
	}
}
