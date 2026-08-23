package engine_dispatcher

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
	byID          map[string]*taskmodels.TaskSession
	byIDErr       error
}

func (f *fakeSessions) GetActiveTaskSessionByTaskID(_ context.Context, _ string) (*taskmodels.TaskSession, error) {
	return f.activeSession, f.activeErr
}

func (f *fakeSessions) GetTaskSessionByTaskID(_ context.Context, _ string) (*taskmodels.TaskSession, error) {
	return f.latestSession, f.latestErr
}

func (f *fakeSessions) GetTaskSession(_ context.Context, id string) (*taskmodels.TaskSession, error) {
	if f.byIDErr != nil {
		return nil, f.byIDErr
	}
	if session, ok := f.byID[id]; ok {
		return session, nil
	}
	return nil, taskmodels.ErrTaskSessionNotFound
}

type fakeEngine struct {
	captured engine.HandleInput
	called   bool
	err      error
	result   engine.HandleResult

	decisionCalled  bool
	decisionSession string
	decisionIn      engine.DecisionInfo
	decisionResult  engine.RecordDecisionResult
	decisionErr     error

	quorumCalled  bool
	quorumTaskID  string
	quorumSession string
	quorumResult  engine.QuorumSnapshot
	quorumErr     error

	roleCalled         bool
	roleTaskID         string
	roleStepID         string
	roleAgentProfileID string
	roleResult         string
	roleParticipantID  string
	roleErr            error
}

func (f *fakeEngine) HandleTrigger(_ context.Context, in engine.HandleInput) (engine.HandleResult, error) {
	f.called = true
	f.captured = in
	return f.result, f.err
}

func (f *fakeEngine) RecordParticipantDecision(
	_ context.Context, sessionID string, in engine.DecisionInfo,
) (engine.RecordDecisionResult, error) {
	f.decisionCalled = true
	f.decisionSession = sessionID
	f.decisionIn = in
	return f.decisionResult, f.decisionErr
}

func (f *fakeEngine) EvaluateStepQuorum(
	_ context.Context, taskID, sessionID string,
) (engine.QuorumSnapshot, error) {
	f.quorumCalled = true
	f.quorumTaskID = taskID
	f.quorumSession = sessionID
	return f.quorumResult, f.quorumErr
}

func (f *fakeEngine) ResolveParticipantRole(
	_ context.Context, taskID, stepID, agentProfileID string,
) (string, string, error) {
	f.roleCalled = true
	f.roleTaskID = taskID
	f.roleStepID = stepID
	f.roleAgentProfileID = agentProfileID
	return f.roleResult, f.roleParticipantID, f.roleErr
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

func (s stubPrimary) PrimaryAgentProfileID(_ context.Context, _, _ string) (string, error) {
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

func (commentWorkflowStore) ApplyTransitionIfAtStep(
	context.Context, string, string, string, string, engine.Trigger,
) (bool, error) {
	return false, errors.New("unexpected transition")
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

type transitionWorkflowStore struct {
	appliedTaskID    string
	appliedSessionID string
	appliedFrom      string
	appliedTo        string
	appliedTrigger   engine.Trigger
}

func (s *transitionWorkflowStore) LoadState(_ context.Context, taskID, sessionID string) (engine.MachineState, error) {
	return engine.MachineState{
		TaskID:        taskID,
		SessionID:     sessionID,
		WorkflowID:    "workflow-1",
		CurrentStepID: "review",
	}, nil
}

func (s *transitionWorkflowStore) LoadStep(_ context.Context, _, stepID string) (engine.StepSpec, error) {
	return engine.StepSpec{
		ID:         stepID,
		WorkflowID: "workflow-1",
		Events: map[engine.Trigger][]engine.Action{
			engine.TriggerOnComment: {
				{
					Kind:       engine.ActionMoveToStep,
					MoveToStep: &engine.MoveToStepAction{StepID: "follow-up"},
				},
			},
		},
	}, nil
}

func (s *transitionWorkflowStore) LoadNextStep(context.Context, string, int) (engine.StepSpec, error) {
	return engine.StepSpec{}, errors.New("unexpected next-step lookup")
}

func (s *transitionWorkflowStore) LoadPreviousStep(context.Context, string, int) (engine.StepSpec, error) {
	return engine.StepSpec{}, errors.New("unexpected previous-step lookup")
}

func (s *transitionWorkflowStore) ApplyTransition(
	_ context.Context, taskID, sessionID, fromStepID, toStepID string, trigger engine.Trigger,
) error {
	s.appliedTaskID = taskID
	s.appliedSessionID = sessionID
	s.appliedFrom = fromStepID
	s.appliedTo = toStepID
	s.appliedTrigger = trigger
	return nil
}

func (s *transitionWorkflowStore) ApplyTransitionIfAtStep(
	_ context.Context, taskID, sessionID, expectedStepID, toStepID string, trigger engine.Trigger,
) (bool, error) {
	s.appliedTaskID = taskID
	s.appliedSessionID = sessionID
	s.appliedFrom = expectedStepID
	s.appliedTo = toStepID
	s.appliedTrigger = trigger
	return true, nil
}

func (s *transitionWorkflowStore) PersistData(context.Context, string, map[string]any) error {
	return nil
}

func (s *transitionWorkflowStore) IsOperationApplied(context.Context, string) (bool, error) {
	return false, nil
}

func (s *transitionWorkflowStore) MarkOperationApplied(context.Context, string) error {
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

func TestDispatcher_HandleTriggerHandledReportsNoop(t *testing.T) {
	eng := &fakeEngine{result: engine.HandleResult{}}
	sessions := &fakeSessions{activeSession: &taskmodels.TaskSession{ID: "sess-1"}}
	d := New(eng, sessions, logger.Default())

	handled, err := d.HandleTriggerHandled(context.Background(), "task-1", engine.TriggerOnComment,
		engine.OnCommentPayload{CommentID: "c-1"}, "task_comment:c-1")
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if handled {
		t.Fatal("handled = true, want false for no-action engine result")
	}
}

func TestDispatcher_UsesLatestSessionForCommentWhenActiveSessionMissing(t *testing.T) {
	for _, state := range []taskmodels.TaskSessionState{
		taskmodels.TaskSessionStateCompleted,
		taskmodels.TaskSessionStateIdle,
	} {
		t.Run(string(state), func(t *testing.T) {
			eng := &fakeEngine{}
			sessions := &fakeSessions{
				activeErr: taskmodels.ErrTaskSessionNotFound,
				latestSession: &taskmodels.TaskSession{
					ID:    "sess-reusable",
					State: state,
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
			if eng.captured.SessionID != "sess-reusable" {
				t.Errorf("session id = %q, want sess-reusable", eng.captured.SessionID)
			}
		})
	}
}

func TestDispatcher_SkipsLatestSessionForCommentWhenNotReusable(t *testing.T) {
	for _, state := range []taskmodels.TaskSessionState{
		taskmodels.TaskSessionStateFailed,
		taskmodels.TaskSessionStateCancelled,
	} {
		t.Run(string(state), func(t *testing.T) {
			eng := &fakeEngine{}
			sessions := &fakeSessions{
				activeErr: taskmodels.ErrTaskSessionNotFound,
				latestSession: &taskmodels.TaskSession{
					ID:    "sess-latest",
					State: state,
				},
			}
			d := New(eng, sessions, logger.Default())

			err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnComment,
				engine.OnCommentPayload{CommentID: "c-1"}, "task_comment:c-1")
			if !errors.Is(err, ErrNoSession) {
				t.Fatalf("err = %v, want ErrNoSession", err)
			}
			if eng.called {
				t.Fatal("engine should not be invoked for non-completed latest session")
			}
		})
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
	if got.IdempotencyKey == nil ||
		!strings.HasPrefix(*got.IdempotencyKey, "task_comment:c-1:work:task-1:agent-primary:") {
		t.Errorf("idempotency_key = %v, want salted task_comment key", got.IdempotencyKey)
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

func TestDispatcher_CompletedSessionCommentCanApplyTransition(t *testing.T) {
	store := &transitionWorkflowStore{}
	eng := engine.New(store, engine.MapRegistry{})
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
	if store.appliedTaskID != "task-1" {
		t.Fatalf("transition task_id = %q, want task-1", store.appliedTaskID)
	}
	if store.appliedSessionID != "sess-completed" {
		t.Fatalf("transition session_id = %q, want sess-completed", store.appliedSessionID)
	}
	if store.appliedFrom != "review" || store.appliedTo != "follow-up" {
		t.Fatalf("transition = %q -> %q, want review -> follow-up",
			store.appliedFrom, store.appliedTo)
	}
	if store.appliedTrigger != engine.TriggerOnComment {
		t.Fatalf("transition trigger = %q, want on_comment", store.appliedTrigger)
	}
}

// TestDispatcher_UsesLatestFailedSessionForAgentError pins WO-05's E5
// fix: TriggerOnAgentError must resolve the task's latest session when
// it is FAILED, since GetActiveTaskSessionByTaskID's state filter
// ('CREATED','STARTING','RUNNING','WAITING_FOR_INPUT') never includes
// FAILED. Without this, the on_agent_error dispatch added in
// event_subscribers.go would be nondeterministically inert whenever the
// orchestrator's AgentFailed subscriber wins the race and flips the
// session to FAILED before the office subscriber's engine dispatch
// runs.
func TestDispatcher_UsesLatestFailedSessionForAgentError(t *testing.T) {
	eng := &fakeEngine{}
	sessions := &fakeSessions{
		activeErr: taskmodels.ErrTaskSessionNotFound,
		latestSession: &taskmodels.TaskSession{
			ID:    "sess-failed",
			State: taskmodels.TaskSessionStateFailed,
		},
	}
	d := New(eng, sessions, logger.Default())

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnAgentError,
		engine.OnAgentErrorPayload{FailedAgentID: "agent-1"}, "agent_error:run-1")
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !eng.called {
		t.Fatal("engine not invoked")
	}
	if eng.captured.SessionID != "sess-failed" {
		t.Errorf("session id = %q, want sess-failed", eng.captured.SessionID)
	}
}

// TestDispatcher_SkipsLatestSessionForAgentErrorWhenNotFailed pins that
// on_agent_error only reuses a latest session that is actually FAILED —
// an unrelated non-terminal-failure session state must not be treated
// as the failed session's stand-in.
func TestDispatcher_SkipsLatestSessionForAgentErrorWhenNotFailed(t *testing.T) {
	for _, state := range []taskmodels.TaskSessionState{
		taskmodels.TaskSessionStateCompleted,
		taskmodels.TaskSessionStateIdle,
		taskmodels.TaskSessionStateCancelled,
	} {
		t.Run(string(state), func(t *testing.T) {
			eng := &fakeEngine{}
			sessions := &fakeSessions{
				activeErr: taskmodels.ErrTaskSessionNotFound,
				latestSession: &taskmodels.TaskSession{
					ID:    "sess-latest",
					State: state,
				},
			}
			d := New(eng, sessions, logger.Default())

			err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnAgentError,
				engine.OnAgentErrorPayload{FailedAgentID: "agent-1"}, "agent_error:run-1")
			if !errors.Is(err, ErrNoSession) {
				t.Fatalf("err = %v, want ErrNoSession", err)
			}
			if eng.called {
				t.Fatal("engine should not be invoked for a non-failed latest session")
			}
		})
	}
}

// TestDispatcher_AgentErrorUsesFailedSessionIDNotLatestSibling pins the
// Review round-1 F1 fix: an office task has one session per (task, agent)
// (executor_office.go's GetTaskSessionByTaskAndAgent find-or-create), so
// office-default.yml's multi-agent workflow routinely has more than one
// session on a task. The latest-session-if-FAILED heuristic picks whichever
// session started last, not the one that actually failed — here the latest
// session is an unrelated sibling sitting IDLE (a normal post-turn state)
// while the failed session is older. The dispatcher must resolve
// FailedSessionID directly instead of falling through to that heuristic.
func TestDispatcher_AgentErrorUsesFailedSessionIDNotLatestSibling(t *testing.T) {
	eng := &fakeEngine{}
	sessions := &fakeSessions{
		activeErr: taskmodels.ErrTaskSessionNotFound,
		latestSession: &taskmodels.TaskSession{
			ID:    "sess-sibling-idle",
			State: taskmodels.TaskSessionStateIdle,
		},
		byID: map[string]*taskmodels.TaskSession{
			"sess-failed": {
				ID: "sess-failed", TaskID: "task-1", State: taskmodels.TaskSessionStateFailed,
			},
		},
	}
	d := New(eng, sessions, logger.Default())

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnAgentError,
		engine.OnAgentErrorPayload{FailedAgentID: "agent-1", FailedSessionID: "sess-failed"},
		"agent_error:run-1")
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !eng.called {
		t.Fatal("engine not invoked")
	}
	if eng.captured.SessionID != "sess-failed" {
		t.Errorf("session id = %q, want sess-failed (latest-sibling heuristic used instead)",
			eng.captured.SessionID)
	}
}

// TestDispatcher_AgentErrorUsesFailedSessionIDOverActiveSibling pins F1's
// second failure mode: a sibling session sitting WAITING_FOR_INPUT is
// returned by the *active*-session lookup before the latest-session
// fallback is ever reached, so the dispatcher would target the wrong
// session's engine state (and, in production, its workflow step) even
// though the trigger never gets ErrNoSession. FailedSessionID must win
// over the active lookup whenever it is present.
func TestDispatcher_AgentErrorUsesFailedSessionIDOverActiveSibling(t *testing.T) {
	eng := &fakeEngine{}
	sessions := &fakeSessions{
		activeSession: &taskmodels.TaskSession{
			ID:    "sess-sibling-waiting",
			State: taskmodels.TaskSessionStateWaitingForInput,
		},
		byID: map[string]*taskmodels.TaskSession{
			"sess-failed": {
				ID: "sess-failed", TaskID: "task-1", State: taskmodels.TaskSessionStateFailed,
			},
		},
	}
	d := New(eng, sessions, logger.Default())

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnAgentError,
		engine.OnAgentErrorPayload{FailedAgentID: "agent-1", FailedSessionID: "sess-failed"},
		"agent_error:run-1")
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if eng.captured.SessionID != "sess-failed" {
		t.Errorf("session id = %q, want sess-failed (active sibling used instead)",
			eng.captured.SessionID)
	}
}

// TestDispatcher_AgentErrorFailedSessionIDNotFoundYieldsNoSession covers the
// direct-lookup miss: FailedSessionID names a session that no longer
// resolves (e.g. deleted). This must surface as a normal "no session"
// no-op, not an engine call against some other session and not a hard
// error.
func TestDispatcher_AgentErrorFailedSessionIDNotFoundYieldsNoSession(t *testing.T) {
	eng := &fakeEngine{}
	sessions := &fakeSessions{
		activeSession: &taskmodels.TaskSession{
			ID:    "sess-unrelated-active",
			State: taskmodels.TaskSessionStateWaitingForInput,
		},
		// byID intentionally empty — "sess-missing" resolves to not-found
	}
	d := New(eng, sessions, logger.Default())

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnAgentError,
		engine.OnAgentErrorPayload{FailedAgentID: "agent-1", FailedSessionID: "sess-missing"},
		"agent_error:run-1")
	if !errors.Is(err, ErrNoSession) {
		t.Fatalf("err = %v, want ErrNoSession", err)
	}
	if eng.called {
		t.Fatal("engine should not be invoked when the failed session id can't be resolved")
	}
}

// TestDispatcher_AgentErrorRejectsFailedSessionFromAnotherTask pins the
// task/session ownership boundary for the direct FailedSessionID lookup.
func TestDispatcher_AgentErrorRejectsFailedSessionFromAnotherTask(t *testing.T) {
	eng := &fakeEngine{}
	sessions := &fakeSessions{
		activeSession: &taskmodels.TaskSession{
			ID:     "sess-task-1-active",
			TaskID: "task-1",
			State:  taskmodels.TaskSessionStateWaitingForInput,
		},
		byID: map[string]*taskmodels.TaskSession{
			"sess-task-2-failed": {
				ID: "sess-task-2-failed", TaskID: "task-2", State: taskmodels.TaskSessionStateFailed,
			},
		},
	}
	d := New(eng, sessions, logger.Default())

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnAgentError,
		engine.OnAgentErrorPayload{FailedAgentID: "agent-1", FailedSessionID: "sess-task-2-failed"},
		"agent_error:run-1")
	if !errors.Is(err, ErrNoSession) {
		t.Fatalf("err = %v, want ErrNoSession", err)
	}
	if eng.called {
		t.Fatal("engine should not be invoked for a session owned by another task")
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

func TestDispatcher_PropagatesActiveSessionLookupErrorForNonCommentTrigger(t *testing.T) {
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

	err := d.HandleTrigger(context.Background(), "task-1", engine.TriggerOnBlockerResolved,
		engine.OnBlockerResolvedPayload{ResolvedBlockerIDs: []string{"blocker-1"}}, "op")
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
