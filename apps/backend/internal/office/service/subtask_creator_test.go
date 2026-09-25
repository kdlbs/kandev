package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// createSubtaskCall records a CreateOfficeSubtask call.
type createSubtaskCall struct {
	ParentTaskID    string
	AssigneeAgentID string
	Title           string
	Description     string
	Metadata        map[string]interface{}
}

// mockSubtaskCreator implements service.TaskCreator (so it can be passed as
// ServiceOptions.TaskCreator) and optionally service.SubtaskCreator, mirroring
// production's single taskCreatorAdapter implementing both interfaces.
type mockSubtaskCreator struct {
	mockTaskCreator
	subtaskCalls []createSubtaskCall
	subtaskID    string
	subtaskErr   error
}

func (m *mockSubtaskCreator) CreateOfficeSubtask(
	_ context.Context, parentTaskID, assigneeAgentID, title, description string,
	metadata map[string]interface{},
) (string, error) {
	m.subtaskCalls = append(m.subtaskCalls, createSubtaskCall{
		ParentTaskID:    parentTaskID,
		AssigneeAgentID: assigneeAgentID,
		Title:           title,
		Description:     description,
		Metadata:        metadata,
	})
	id := m.subtaskID
	if id == "" {
		id = "new-subtask-id"
	}
	return id, m.subtaskErr
}

// newTestServiceWithRunsServiceAndSubtaskCreator mirrors
// newTestServiceWithRunsServiceAndTaskCreator (task_creator_carrier_test.go)
// but wires a mockSubtaskCreator so tests can observe the metadata
// CreateOfficeSubtaskAsAgent passes through.
func newTestServiceWithRunsServiceAndSubtaskCreator(
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

func TestCreateOfficeSubtaskAsAgent_WithCausingRunID_PersistsCarrier(t *testing.T) {
	mock := &mockSubtaskCreator{}
	svc, repo := newTestServiceWithRunsServiceAndSubtaskCreator(t, mock)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRunWithActor(ctx, agent.ID, "heartbeat", `{}`, "",
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

	taskID, err := svc.CreateOfficeSubtaskAsAgent(
		ctx, "", "parent-task-1", "agent-assignee", "Build widget", "A description", causingRun.ID,
	)
	if err != nil {
		t.Fatalf("CreateOfficeSubtaskAsAgent: %v", err)
	}
	if taskID == "" {
		t.Error("expected non-empty task ID")
	}
	if len(mock.subtaskCalls) != 1 {
		t.Fatalf("expected 1 CreateOfficeSubtask call, got %d", len(mock.subtaskCalls))
	}
	call := mock.subtaskCalls[0]
	if call.ParentTaskID != "parent-task-1" {
		t.Errorf("parentTaskID = %q, want parent-task-1", call.ParentTaskID)
	}
	metadata := call.Metadata
	if metadata == nil {
		t.Fatal("expected non-nil carrier metadata")
	}
	if metadata[taskmodels.MetaKeyOfficeCarrierCausationID] != causingRun.ChainCausationID {
		t.Errorf("carrier causation_id = %v, want %q",
			metadata[taskmodels.MetaKeyOfficeCarrierCausationID], causingRun.ChainCausationID)
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

func TestCreateOfficeSubtaskAsAgent_EmptyCausingRunID_NoCarrier(t *testing.T) {
	mock := &mockSubtaskCreator{}
	svc := newTestService(t, service.ServiceOptions{TaskCreator: mock})
	ctx := context.Background()

	taskID, err := svc.CreateOfficeSubtaskAsAgent(
		ctx, "", "parent-task-1", "agent-assignee", "Build widget", "A description", "",
	)
	if err != nil {
		t.Fatalf("CreateOfficeSubtaskAsAgent: %v", err)
	}
	if taskID == "" {
		t.Error("expected non-empty task ID")
	}
	if len(mock.subtaskCalls) != 1 {
		t.Fatalf("expected 1 CreateOfficeSubtask call, got %d", len(mock.subtaskCalls))
	}
	if mock.subtaskCalls[0].Metadata != nil {
		t.Errorf("expected nil carrier metadata with no causing run, got %v", mock.subtaskCalls[0].Metadata)
	}
}

func TestCreateOfficeSubtaskAsAgent_UnreadableCausingRunID_NoCarrierButTaskCreated(t *testing.T) {
	mock := &mockSubtaskCreator{}
	svc := newTestService(t, service.ServiceOptions{TaskCreator: mock})
	ctx := context.Background()

	taskID, err := svc.CreateOfficeSubtaskAsAgent(
		ctx, "", "parent-task-1", "agent-assignee", "Build widget", "A description", "does-not-exist",
	)
	if err != nil {
		t.Fatalf("CreateOfficeSubtaskAsAgent: %v", err)
	}
	if taskID == "" {
		t.Error("expected non-empty task ID; an unreadable causing run must not block subtask creation")
	}
	if len(mock.subtaskCalls) != 1 {
		t.Fatalf("expected 1 CreateOfficeSubtask call, got %d", len(mock.subtaskCalls))
	}
	if mock.subtaskCalls[0].Metadata != nil {
		t.Errorf("expected nil carrier metadata for an unreadable causing run, got %v", mock.subtaskCalls[0].Metadata)
	}
}

func TestCreateOfficeSubtaskAsAgent_NilCreatorReturnsError(t *testing.T) {
	svc := newTestService(t) // no TaskCreator
	ctx := context.Background()

	_, err := svc.CreateOfficeSubtaskAsAgent(ctx, "", "parent-task-1", "", "Task", "", "")
	if err == nil {
		t.Fatal("expected error when task creator is nil")
	}
}

func TestCreateOfficeSubtaskAsAgent_CreatorWithoutSubtaskSupportReturnsError(t *testing.T) {
	// mockTaskCreator implements service.TaskCreator but not
	// service.SubtaskCreator, mirroring an older test double that hasn't
	// been widened.
	mock := &mockTaskCreator{}
	svc := newTestService(t, service.ServiceOptions{TaskCreator: mock})
	ctx := context.Background()

	_, err := svc.CreateOfficeSubtaskAsAgent(ctx, "", "parent-task-1", "", "Task", "", "")
	if err == nil {
		t.Fatal("expected error when task creator does not implement SubtaskCreator")
	}
}

func TestCreateOfficeSubtaskAsAgent_ForbiddenWhenPermissionMissing(t *testing.T) {
	mock := &mockSubtaskCreator{}
	svc := newTestService(t, service.ServiceOptions{TaskCreator: mock})
	ctx := context.Background()

	caller := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "no-create-specialist",
		Role:        models.AgentRoleSpecialist,
		Permissions: `{"can_create_tasks": false}`,
	}
	if err := svc.CreateAgentInstance(ctx, caller); err != nil {
		t.Fatalf("create caller: %v", err)
	}

	_, err := svc.CreateOfficeSubtaskAsAgent(ctx, caller.ID, "parent-task-1", "", "Blocked task", "", "")
	if err == nil {
		t.Fatal("expected ErrForbidden, got nil")
	}
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got: %v", err)
	}
	if len(mock.subtaskCalls) != 0 {
		t.Error("subtask creator should not be called when forbidden")
	}
}
