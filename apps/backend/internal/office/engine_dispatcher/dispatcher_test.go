package engine_dispatcher

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

type fakeSessions struct {
	session *taskmodels.TaskSession
	err     error
}

func (f *fakeSessions) GetActiveTaskSessionByTaskID(_ context.Context, _ string) (*taskmodels.TaskSession, error) {
	return f.session, f.err
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

func TestDispatcher_ResolvesSessionAndForwards(t *testing.T) {
	eng := &fakeEngine{}
	sessions := &fakeSessions{session: &taskmodels.TaskSession{ID: "sess-1"}}
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
	sessions := &fakeSessions{session: &taskmodels.TaskSession{ID: "sess-1"}}
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
