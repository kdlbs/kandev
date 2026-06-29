package engine_dispatcher

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

type fakeSessions struct {
	activeSession *taskmodels.TaskSession
	latestSession *taskmodels.TaskSession
	err           error
}

func (f *fakeSessions) GetActiveTaskSessionByTaskID(_ context.Context, _ string) (*taskmodels.TaskSession, error) {
	return f.activeSession, f.err
}

func (f *fakeSessions) GetTaskSessionByTaskID(_ context.Context, _ string) (*taskmodels.TaskSession, error) {
	return f.latestSession, f.err
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

type fakeRunQueue struct {
	calls []engine.QueueRunRequest
}

func (f *fakeRunQueue) QueueRun(_ context.Context, req engine.QueueRunRequest) error {
	f.calls = append(f.calls, req)
	return nil
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
	queue := &fakeRunQueue{}
	eng := engine.New(commentWorkflowStore{}, engine.MapRegistry{
		engine.ActionQueueRun: engine.QueueRunCallback{
			Adapter: queue,
			Primary: stubPrimary{id: "agent-primary"},
		},
	})
	sessions := &fakeSessions{
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
	if len(queue.calls) != 1 {
		t.Fatalf("queued runs = %d, want 1", len(queue.calls))
	}
	got := queue.calls[0]
	if got.AgentProfileID != "agent-primary" {
		t.Errorf("agent_profile_id = %q, want agent-primary", got.AgentProfileID)
	}
	if got.TaskID != "task-1" {
		t.Errorf("task_id = %q, want task-1", got.TaskID)
	}
	if got.Reason != "task_comment" {
		t.Errorf("reason = %q, want task_comment", got.Reason)
	}
	if !strings.HasPrefix(got.IdempotencyKey, "task_comment:c-1") {
		t.Errorf("idempotency_key = %q, want task_comment:c-1 prefix", got.IdempotencyKey)
	}
}

func TestDispatcher_DoesNotUseLatestSessionForNonCommentTriggers(t *testing.T) {
	eng := &fakeEngine{}
	sessions := &fakeSessions{
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

func TestDispatcher_ReturnsErrNoSessionOnSessionLookupError(t *testing.T) {
	eng := &fakeEngine{}
	sessions := &fakeSessions{err: errors.New("db down")}
	d := New(eng, sessions, logger.Default())

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnComment,
		engine.OnCommentPayload{}, "op")
	if !errors.Is(err, ErrNoSession) {
		t.Fatalf("err = %v, want ErrNoSession", err)
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
