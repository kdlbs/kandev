package engine_dispatcher

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

type fakeSessions struct {
	activeSession *taskmodels.TaskSession
	latestSession *taskmodels.TaskSession
	activeErr     error
	latestErr     error
}

func (f *fakeSessions) GetActiveTaskSessionByTaskID(_ context.Context, _ string) (*taskmodels.TaskSession, error) {
	return f.activeSession, f.activeErr
}

func (f *fakeSessions) GetTaskSessionByTaskID(_ context.Context, _ string) (*taskmodels.TaskSession, error) {
	return f.latestSession, f.latestErr
}

type fakeEngine struct {
	captured engine.HandleInput
	called   bool
	err      error
}

func (f *fakeEngine) HandleTrigger(_ context.Context, in engine.HandleInput) (engine.HandleResult, error) {
	f.called = true
	f.captured = in
	return engine.HandleResult{}, f.err
}

type realRunsAdapter struct {
	svc *runsservice.Service
}

func (a realRunsAdapter) QueueRun(ctx context.Context, req engine.QueueRunRequest) error {
	return a.svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: req.AgentProfileID,
		TaskID:         req.TaskID,
		WorkflowStepID: req.WorkflowStepID,
		Reason:         req.Reason,
		IdempotencyKey: req.IdempotencyKey,
		Payload:        req.Payload,
	})
}

type stubPrimary struct {
	id string
}

func (s stubPrimary) PrimaryAgentProfileID(_ context.Context, _ string) (string, error) {
	return s.id, nil
}

type commentWorkflowStore struct{}

func (commentWorkflowStore) LoadState(_ context.Context, taskID, sessionID string) (engine.MachineState, error) {
	return engine.MachineState{
		TaskID:        taskID,
		SessionID:     sessionID,
		WorkflowID:    "workflow-1",
		CurrentStepID: "work",
	}, nil
}

func (commentWorkflowStore) LoadStep(_ context.Context, _, stepID string) (engine.StepSpec, error) {
	return engine.StepSpec{
		ID:         stepID,
		WorkflowID: "workflow-1",
		Events: map[engine.Trigger][]engine.Action{
			engine.TriggerOnComment: {
				{
					Kind: engine.ActionQueueRun,
					QueueRun: &engine.QueueRunAction{
						Target: "primary",
						TaskID: "this",
						Reason: "task_comment",
					},
				},
			},
		},
	}, nil
}

func (commentWorkflowStore) LoadNextStep(context.Context, string, int) (engine.StepSpec, error) {
	return engine.StepSpec{}, errors.New("unexpected next-step lookup")
}

func (commentWorkflowStore) LoadPreviousStep(context.Context, string, int) (engine.StepSpec, error) {
	return engine.StepSpec{}, errors.New("unexpected previous-step lookup")
}

func (commentWorkflowStore) ApplyTransition(context.Context, string, string, string, string, engine.Trigger) error {
	return errors.New("unexpected transition")
}

func (commentWorkflowStore) PersistData(context.Context, string, map[string]any) error {
	return nil
}

func (commentWorkflowStore) IsOperationApplied(context.Context, string) (bool, error) {
	return false, nil
}

func (commentWorkflowStore) MarkOperationApplied(context.Context, string) error {
	return nil
}

func newDispatcherRunsService(t *testing.T) (*runsservice.Service, *runssqlite.Repository) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	officeRepo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}
	log := logger.Default()
	runsRepo := officeRepo.RunsRepository()
	svc := runsservice.New(runsRepo, bus.NewMemoryEventBus(log), log, nil)
	return svc, runsRepo
}

func TestDispatcher_ResolvesSessionAndForwards(t *testing.T) {
	eng := &fakeEngine{}
	sessions := &fakeSessions{activeSession: &taskmodels.TaskSession{ID: "sess-1"}}
	d := New(eng, sessions, logger.Default())

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnComment,
		engine.OnCommentPayload{CommentID: "c-1"}, "task_comment:c-1")
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !eng.called {
		t.Fatal("engine not invoked")
	}
	if eng.captured.SessionID != "sess-1" {
		t.Errorf("session id = %q, want sess-1", eng.captured.SessionID)
	}
	if eng.captured.OperationID != "task_comment:c-1" {
		t.Errorf("operation id mismatch")
	}
	if eng.captured.Trigger != engine.TriggerOnComment {
		t.Errorf("trigger = %q", eng.captured.Trigger)
	}
}

func TestDispatcher_UsesLatestSessionForCommentWhenActiveSessionMissing(t *testing.T) {
	eng := &fakeEngine{}
	sessions := &fakeSessions{
		activeErr: taskmodels.ErrTaskSessionNotFound,
		latestSession: &taskmodels.TaskSession{
			ID:    "sess-completed",
			State: taskmodels.TaskSessionStateCompleted,
		},
	}
	d := New(eng, sessions, logger.Default())

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnComment,
		engine.OnCommentPayload{CommentID: "c-1"}, "task_comment:c-1")
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !eng.called {
		t.Fatal("engine not invoked")
	}
	if eng.captured.SessionID != "sess-completed" {
		t.Errorf("session id = %q, want sess-completed", eng.captured.SessionID)
	}
}

func TestDispatcher_CompletedSessionCommentQueuesRun(t *testing.T) {
	runsSvc, runsRepo := newDispatcherRunsService(t)
	eng := engine.New(commentWorkflowStore{}, engine.MapRegistry{
		engine.ActionQueueRun: engine.QueueRunCallback{
			Adapter: realRunsAdapter{svc: runsSvc},
			Primary: stubPrimary{id: "agent-primary"},
		},
	})
	sessions := &fakeSessions{
		activeErr: taskmodels.ErrTaskSessionNotFound,
		latestSession: &taskmodels.TaskSession{
			ID:    "sess-completed",
			State: taskmodels.TaskSessionStateCompleted,
		},
	}
	d := New(eng, sessions, logger.Default())

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnComment,
		engine.OnCommentPayload{CommentID: "c-1", AuthorID: "user-1"}, "task_comment:c-1")
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	statuses, err := runsRepo.GetRunsByCommentIDs(context.Background(), []string{"c-1"})
	if err != nil {
		t.Fatalf("get comment runs: %v", err)
	}
	status, ok := statuses["c-1"]
	if !ok {
		t.Fatalf("missing comment run for c-1: %+v", statuses)
	}
	got, err := runsRepo.GetRunByID(context.Background(), status.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.AgentProfileID != "agent-primary" {
		t.Errorf("agent_profile_id = %q, want agent-primary", got.AgentProfileID)
	}
	if got.Reason != "task_comment" {
		t.Errorf("reason = %q, want task_comment", got.Reason)
	}
	if got.IdempotencyKey == nil || *got.IdempotencyKey != "task_comment:c-1" {
		t.Errorf("idempotency_key = %v, want task_comment:c-1", got.IdempotencyKey)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(got.Payload), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	for k, want := range map[string]string{
		"agent_profile_id": "agent-primary",
		"task_id":          "task-1",
		"workflow_step_id": "work",
		"comment_id":       "c-1",
		"author_id":        "user-1",
	} {
		if got, _ := payload[k].(string); got != want {
			t.Errorf("payload[%s] = %q, want %q (payload=%v)", k, got, want, payload)
		}
	}
}

func TestDispatcher_DoesNotUseLatestSessionForNonCommentTriggers(t *testing.T) {
	eng := &fakeEngine{}
	sessions := &fakeSessions{
		activeErr: taskmodels.ErrTaskSessionNotFound,
		latestSession: &taskmodels.TaskSession{
			ID:    "sess-completed",
			State: taskmodels.TaskSessionStateCompleted,
		},
	}
	d := New(eng, sessions, logger.Default())

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnBlockerResolved,
		engine.OnBlockerResolvedPayload{ResolvedBlockerIDs: []string{"blocker-1"}}, "blocker:1")
	if !errors.Is(err, ErrNoSession) {
		t.Fatalf("err = %v, want ErrNoSession", err)
	}
	if eng.called {
		t.Fatal("engine should not be invoked for non-comment triggers without an active session")
	}
}

func TestDispatcher_ReturnsErrNoSessionWhenSessionMissing(t *testing.T) {
	eng := &fakeEngine{}
	sessions := &fakeSessions{} // session nil
	d := New(eng, sessions, logger.Default())

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnComment,
		engine.OnCommentPayload{}, "op")
	if !errors.Is(err, ErrNoSession) {
		t.Fatalf("err = %v, want ErrNoSession", err)
	}
	if eng.called {
		t.Error("engine should not be called when session is missing")
	}
}

func TestDispatcher_PropagatesActiveSessionLookupError(t *testing.T) {
	eng := &fakeEngine{}
	dbErr := errors.New("db down")
	sessions := &fakeSessions{
		activeErr: dbErr,
		latestSession: &taskmodels.TaskSession{
			ID:    "sess-completed",
			State: taskmodels.TaskSessionStateCompleted,
		},
	}
	d := New(eng, sessions, logger.Default())

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnComment,
		engine.OnCommentPayload{}, "op")
	if !errors.Is(err, dbErr) {
		t.Fatalf("err = %v, want wrapped db error", err)
	}
	if errors.Is(err, ErrNoSession) {
		t.Fatalf("err must not masquerade as ErrNoSession: %v", err)
	}
	if eng.called {
		t.Error("engine should not be called when active session lookup fails")
	}
}

func TestDispatcher_PropagatesLatestSessionLookupError(t *testing.T) {
	eng := &fakeEngine{}
	dbErr := errors.New("db down")
	sessions := &fakeSessions{
		activeErr: taskmodels.ErrTaskSessionNotFound,
		latestErr: dbErr,
	}
	d := New(eng, sessions, logger.Default())

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnComment,
		engine.OnCommentPayload{}, "op")
	if !errors.Is(err, dbErr) {
		t.Fatalf("err = %v, want wrapped db error", err)
	}
	if errors.Is(err, ErrNoSession) {
		t.Fatalf("err must not masquerade as ErrNoSession: %v", err)
	}
	if eng.called {
		t.Error("engine should not be called when latest session lookup fails")
	}
}

func TestDispatcher_PropagatesEngineError(t *testing.T) {
	eng := &fakeEngine{err: errors.New("boom")}
	sessions := &fakeSessions{activeSession: &taskmodels.TaskSession{ID: "sess-1"}}
	d := New(eng, sessions, logger.Default())

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnComment,
		engine.OnCommentPayload{}, "op")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrNoSession) {
		t.Fatalf("err must not masquerade as ErrNoSession: %v", err)
	}
}

func TestDispatcher_RejectsEmptyTaskID(t *testing.T) {
	d := New(&fakeEngine{}, &fakeSessions{}, logger.Default())
	if err := d.HandleTrigger(context.Background(), "", engine.TriggerOnComment, nil, ""); err == nil {
		t.Fatal("expected error for empty task id")
	}
}
