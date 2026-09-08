package service_test

// Covers AC-OFFICE-RUN-CAUSATION-001.17/.18: a task created "because of" a
// run must carry that run's causation identity forward so a later run
// queued off the task can inherit causation without re-reading the
// (possibly-gone) creating run row. CreateOfficeTaskAsAgent resolves the
// causingRunID it's given into the carrier metadata set and passes it to
// the TaskCreator; this file proves that resolution end to end through the
// real runs/service seam, and proves the fallback behavior when there is
// nothing to resolve.

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/service"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// newTestServiceWithRunsServiceAndTaskCreator mirrors
// newTestServiceWithRunsService (run_causation_test.go) but also wires a
// TaskCreator, needed here to observe the metadata CreateOfficeTaskAsAgent
// passes through.
func newTestServiceWithRunsServiceAndTaskCreator(
	t *testing.T, creator service.TaskCreator,
) (*service.Service, *officesqlite.Repository) {
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
	svc := service.NewService(service.ServiceOptions{Repo: repo, Logger: log, TaskCreator: creator})
	svc.SetRunsService(runsservice.New(repo.RunsRepository(), nil, log, nil))
	return svc, repo
}

func TestCreateOfficeTaskAsAgent_WithCausingRunID_PersistsCarrier(t *testing.T) {
	mock := &mockTaskCreator{}
	svc, repo := newTestServiceWithRunsServiceAndTaskCreator(t, mock)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRunWithActor(ctx, agent.ID, "heartbeat", `{}`, "",
		models.ActorKindAgent, agent.ID, ""); err != nil {
		t.Fatalf("queue causing run: %v", err)
	}
	runs, err := repo.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("want 1 run, got %d", len(runs))
	}
	causingRun := runs[0]

	taskID, err := svc.CreateOfficeTaskAsAgent(
		ctx, "", "ws-1", "project-1", "agent-assignee", "Build widget", "A description", causingRun.ID,
	)
	if err != nil {
		t.Fatalf("CreateOfficeTaskAsAgent: %v", err)
	}
	if taskID == "" {
		t.Error("expected non-empty task ID")
	}
	if len(mock.agentCalls) != 1 {
		t.Fatalf("expected 1 CreateOfficeTaskAsAgent call, got %d", len(mock.agentCalls))
	}
	metadata := mock.agentCalls[0].Metadata
	if metadata == nil {
		t.Fatal("expected non-nil carrier metadata")
	}
	if metadata[taskmodels.MetaKeyOfficeCarrierCausationID] != causingRun.CausationID {
		t.Errorf("carrier causation_id = %v, want %q",
			metadata[taskmodels.MetaKeyOfficeCarrierCausationID], causingRun.CausationID)
	}
	if metadata[taskmodels.MetaKeyOfficeCarrierCausationDepth] != causingRun.CausationDepth {
		t.Errorf("carrier causation_depth = %v, want %v",
			metadata[taskmodels.MetaKeyOfficeCarrierCausationDepth], causingRun.CausationDepth)
	}
	if metadata[taskmodels.MetaKeyOfficeCarrierCreatingRunID] != causingRun.ID {
		t.Errorf("carrier creating_run_id = %v, want %q",
			metadata[taskmodels.MetaKeyOfficeCarrierCreatingRunID], causingRun.ID)
	}
	if metadata[taskmodels.MetaKeyOfficeCarrierHumanRooted] != causingRun.HumanRooted {
		t.Errorf("carrier human_rooted = %v, want %v",
			metadata[taskmodels.MetaKeyOfficeCarrierHumanRooted], causingRun.HumanRooted)
	}
	if metadata[taskmodels.MetaKeyOfficeCarrierRoutineID] != causingRun.RoutineID {
		t.Errorf("carrier routine_id = %v, want %q",
			metadata[taskmodels.MetaKeyOfficeCarrierRoutineID], causingRun.RoutineID)
	}
	if metadata[taskmodels.MetaKeyOfficeCarrierActorKind] != string(causingRun.ActorKind) {
		t.Errorf("carrier actor_kind = %v, want %q",
			metadata[taskmodels.MetaKeyOfficeCarrierActorKind], causingRun.ActorKind)
	}
	if metadata[taskmodels.MetaKeyOfficeCarrierActorID] != causingRun.ActorID {
		t.Errorf("carrier actor_id = %v, want %q",
			metadata[taskmodels.MetaKeyOfficeCarrierActorID], causingRun.ActorID)
	}
}

func TestCreateOfficeTaskAsAgent_EmptyCausingRunID_NoCarrier(t *testing.T) {
	mock := &mockTaskCreator{}
	svc := newTestService(t, service.ServiceOptions{TaskCreator: mock})
	ctx := context.Background()

	taskID, err := svc.CreateOfficeTaskAsAgent(
		ctx, "", "ws-1", "project-1", "agent-assignee", "Build widget", "A description", "",
	)
	if err != nil {
		t.Fatalf("CreateOfficeTaskAsAgent: %v", err)
	}
	if taskID == "" {
		t.Error("expected non-empty task ID")
	}
	if len(mock.agentCalls) != 1 {
		t.Fatalf("expected 1 CreateOfficeTaskAsAgent call, got %d", len(mock.agentCalls))
	}
	if mock.agentCalls[0].Metadata != nil {
		t.Errorf("expected nil carrier metadata with no causing run, got %v", mock.agentCalls[0].Metadata)
	}
}

func TestCreateOfficeTaskAsAgent_UnreadableCausingRunID_NoCarrierButTaskCreated(t *testing.T) {
	mock := &mockTaskCreator{}
	svc := newTestService(t, service.ServiceOptions{TaskCreator: mock})
	ctx := context.Background()

	taskID, err := svc.CreateOfficeTaskAsAgent(
		ctx, "", "ws-1", "project-1", "agent-assignee", "Build widget", "A description", "does-not-exist",
	)
	if err != nil {
		t.Fatalf("CreateOfficeTaskAsAgent: %v", err)
	}
	if taskID == "" {
		t.Error("expected non-empty task ID; an unreadable causing run must not block task creation")
	}
	if len(mock.agentCalls) != 1 {
		t.Fatalf("expected 1 CreateOfficeTaskAsAgent call, got %d", len(mock.agentCalls))
	}
	if mock.agentCalls[0].Metadata != nil {
		t.Errorf("expected nil carrier metadata for an unreadable causing run, got %v", mock.agentCalls[0].Metadata)
	}
}
