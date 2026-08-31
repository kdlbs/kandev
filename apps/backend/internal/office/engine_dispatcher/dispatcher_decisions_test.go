package engine_dispatcher

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// TestDispatcher_RecordDecision_ResolvesActiveSessionAndForwards covers the
// AC-57a happy path: the active session (AC-16) is resolved and threaded
// into Engine.RecordParticipantDecision, and the engine's result maps onto
// RecordDecisionResult including the AC-37 validated StepID.
func TestDispatcher_RecordDecision_ResolvesActiveSessionAndForwards(t *testing.T) {
	decidedAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	eng := &fakeEngine{decisionResult: engine.RecordDecisionResult{
		DecisionID:   "decision-1",
		DecidedAt:    decidedAt,
		Transitioned: true,
		FromStepID:   "review",
		ToStepID:     "approval",
	}}
	sessions := &fakeSessions{activeSession: &taskmodels.TaskSession{ID: "sess-1"}}
	d := New(eng, sessions, logger.Default())

	result, err := d.RecordDecision(context.Background(), RecordDecisionInput{
		TaskID:        "task-1",
		StepID:        "review",
		ParticipantID: "participant-1",
		Decision:      "approved",
		DeciderType:   "agent",
		DeciderID:     "agent-1",
		Role:          "reviewer",
		Comment:       "looks good",
	})
	if err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}
	if !eng.decisionCalled {
		t.Fatal("engine RecordParticipantDecision not invoked")
	}
	if eng.decisionSession != "sess-1" {
		t.Errorf("session id = %q, want sess-1", eng.decisionSession)
	}
	if eng.decisionIn.TaskID != "task-1" || eng.decisionIn.StepID != "review" ||
		eng.decisionIn.ParticipantID != "participant-1" || eng.decisionIn.Decision != "approved" ||
		eng.decisionIn.DeciderType != "agent" || eng.decisionIn.DeciderID != "agent-1" ||
		eng.decisionIn.Role != "reviewer" || eng.decisionIn.Comment != "looks good" {
		t.Errorf("decision input not forwarded verbatim: %+v", eng.decisionIn)
	}
	if result.StepID != "review" {
		t.Errorf("result.StepID = %q, want review (AC-37 validated step)", result.StepID)
	}
	if result.DecisionID != "decision-1" || !result.DecidedAt.Equal(decidedAt) {
		t.Errorf("result decision identity not carried through: %+v", result)
	}
	if !result.Transitioned || result.FromStepID != "review" || result.ToStepID != "approval" {
		t.Errorf("result transition fields not carried through: %+v", result)
	}
}

// TestDispatcher_RecordDecision_SkipsReevaluationWhenNoActiveSession is the
// AC-16a case: no active session is resolvable, so the engine is still
// called (it records the decision) but with a blank session id, which is
// the engine's own signal to skip re-evaluation.
func TestDispatcher_RecordDecision_SkipsReevaluationWhenNoActiveSession(t *testing.T) {
	eng := &fakeEngine{decisionResult: engine.RecordDecisionResult{DecisionID: "decision-1"}}
	sessions := &fakeSessions{activeErr: taskmodels.ErrTaskSessionNotFound}
	d := New(eng, sessions, logger.Default())

	if _, err := d.RecordDecision(context.Background(), RecordDecisionInput{
		TaskID: "task-1", StepID: "review", ParticipantID: "participant-1", Decision: "approved",
	}); err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}
	if !eng.decisionCalled {
		t.Fatal("engine RecordParticipantDecision not invoked")
	}
	if eng.decisionSession != "" {
		t.Errorf("session id = %q, want blank so the engine skips re-evaluation (AC-16a)", eng.decisionSession)
	}
}

// TestDispatcher_RecordDecision_PropagatesActiveSessionLookupError proves a
// genuine session-store failure is not silently treated as AC-16a's
// "no session resolvable" case.
func TestDispatcher_RecordDecision_PropagatesActiveSessionLookupError(t *testing.T) {
	dbErr := errors.New("db down")
	eng := &fakeEngine{}
	sessions := &fakeSessions{activeErr: dbErr}
	d := New(eng, sessions, logger.Default())

	_, err := d.RecordDecision(context.Background(), RecordDecisionInput{
		TaskID: "task-1", StepID: "review", ParticipantID: "participant-1", Decision: "approved",
	})
	if !errors.Is(err, dbErr) {
		t.Fatalf("err = %v, want wrapped db error", err)
	}
	if eng.decisionCalled {
		t.Error("engine should not be called when active session lookup fails")
	}
}

// TestDispatcher_RecordDecision_PropagatesEngineError proves a decision
// write/re-evaluation failure surfaces to the caller unwrapped of context.
func TestDispatcher_RecordDecision_PropagatesEngineError(t *testing.T) {
	engineErr := errors.New("decision store not wired")
	eng := &fakeEngine{decisionErr: engineErr}
	sessions := &fakeSessions{activeSession: &taskmodels.TaskSession{ID: "sess-1"}}
	d := New(eng, sessions, logger.Default())

	_, err := d.RecordDecision(context.Background(), RecordDecisionInput{
		TaskID: "task-1", StepID: "review", ParticipantID: "participant-1", Decision: "approved",
	})
	if !errors.Is(err, engineErr) {
		t.Fatalf("err = %v, want wrapped engine error", err)
	}
}

// TestDispatcher_RecordDecision_KeepsResultWhenReevaluationFails is AC-15:
// when the engine's write succeeded but its post-write re-evaluation
// failed, RecordParticipantDecision returns a *populated* result alongside
// a non-nil error (see quorum.go's RecordParticipantDecision, which does
// `return result, err` rather than `return RecordDecisionResult{}, err`).
// The dispatcher — the single funnel both recordTaskDecision and
// RecordAgentDecision call through — must keep that result and report
// success rather than collapsing a re-evaluation failure into a write
// failure and losing decision_id/decided_at.
func TestDispatcher_RecordDecision_KeepsResultWhenReevaluationFails(t *testing.T) {
	decidedAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	reevalErr := errors.New("load step: db down")
	eng := &fakeEngine{
		decisionResult: engine.RecordDecisionResult{DecisionID: "decision-1", DecidedAt: decidedAt},
		decisionErr:    reevalErr,
	}
	sessions := &fakeSessions{activeSession: &taskmodels.TaskSession{ID: "sess-1"}}
	d := New(eng, sessions, logger.Default())

	result, err := d.RecordDecision(context.Background(), RecordDecisionInput{
		TaskID: "task-1", StepID: "review", ParticipantID: "participant-1", Decision: "approved",
	})
	if err != nil {
		t.Fatalf("RecordDecision: %v, want success per AC-15 (write succeeded, re-eval failed)", err)
	}
	if result.DecisionID != "decision-1" || !result.DecidedAt.Equal(decidedAt) {
		t.Errorf("result decision identity lost: %+v", result)
	}
	if result.StepID != "review" {
		t.Errorf("result.StepID = %q, want review", result.StepID)
	}
	if result.Transitioned {
		t.Error("Transitioned = true, want false: re-evaluation errored, no transition could have applied")
	}
}

// TestDispatcher_RecordDecision_UsesSuppliedSessionOverActiveSibling is the
// regression proof for the wrong-binding defect: once a task can carry more
// than one concurrent active session (WO-72), a task-scoped
// resolveActiveSessionID pick (started_at DESC) can return an unrelated
// sibling instead of the session that actually made the decision. When the
// caller supplies its own session id and that session belongs to the task
// and is still active, RecordDecision must bind to that exact session, not
// whichever session resolveActiveSessionID would have picked.
func TestDispatcher_RecordDecision_UsesSuppliedSessionOverActiveSibling(t *testing.T) {
	eng := &fakeEngine{decisionResult: engine.RecordDecisionResult{DecisionID: "decision-1"}}
	sessions := &fakeSessions{
		// The started_at DESC "active" winner is a sibling agent's session.
		activeSession: &taskmodels.TaskSession{
			ID: "sess-assignee", TaskID: "task-1", State: taskmodels.TaskSessionStateRunning,
		},
		byID: map[string]*taskmodels.TaskSession{
			"sess-reviewer": {ID: "sess-reviewer", TaskID: "task-1", State: taskmodels.TaskSessionStateRunning},
		},
	}
	d := New(eng, sessions, logger.Default())

	if _, err := d.RecordDecision(context.Background(), RecordDecisionInput{
		TaskID: "task-1", StepID: "review", ParticipantID: "participant-1",
		Decision: "approved", SessionID: "sess-reviewer",
	}); err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}
	if eng.decisionSession != "sess-reviewer" {
		t.Errorf("session id = %q, want sess-reviewer (the deciding agent's own session, not the active sibling sess-assignee)", eng.decisionSession)
	}
}

// TestDispatcher_RecordDecision_SuppliedSessionForDifferentTaskSkipsReevaluation
// covers a supplied session that exists but belongs to a different task: it
// must not be trusted (a cross-task session id is not this decider's own
// session), and RecordDecision must not silently fall back to the
// task-scoped active-session lookup either — the caller explicitly named a
// session, and that session doesn't check out, so it is treated exactly
// like AC-16a's "no session resolvable": the decision is recorded (the
// write succeeds either way, so this only proves binding) but
// re-evaluation is skipped with a blank session id.
func TestDispatcher_RecordDecision_SuppliedSessionForDifferentTaskSkipsReevaluation(t *testing.T) {
	eng := &fakeEngine{decisionResult: engine.RecordDecisionResult{DecisionID: "decision-1"}}
	sessions := &fakeSessions{
		// If RecordDecision wrongly fell back to the active-session lookup
		// when the supplied session fails validation, this would surface as
		// eng.decisionSession == "sess-active-fallback" instead of "".
		activeSession: &taskmodels.TaskSession{
			ID: "sess-active-fallback", TaskID: "task-1", State: taskmodels.TaskSessionStateRunning,
		},
		byID: map[string]*taskmodels.TaskSession{
			"sess-foreign": {ID: "sess-foreign", TaskID: "task-2", State: taskmodels.TaskSessionStateRunning},
		},
	}
	d := New(eng, sessions, logger.Default())

	if _, err := d.RecordDecision(context.Background(), RecordDecisionInput{
		TaskID: "task-1", StepID: "review", ParticipantID: "participant-1",
		Decision: "approved", SessionID: "sess-foreign",
	}); err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}
	if eng.decisionSession != "" {
		t.Errorf("session id = %q, want blank: a foreign-task session must not bind, and must not fall back to the active-session lookup", eng.decisionSession)
	}
}

// TestDispatcher_RecordDecision_SuppliedSessionTerminalStateSkipsReevaluation
// covers a supplied session that belongs to the task but has since moved to
// a terminal state (COMPLETED/FAILED/CANCELLED/IDLE) — a stale session id
// from a decision that took a while to record. Same AC-16a-style skip as
// the foreign-task case: recorded, re-evaluation skipped, no fallback.
func TestDispatcher_RecordDecision_SuppliedSessionTerminalStateSkipsReevaluation(t *testing.T) {
	eng := &fakeEngine{decisionResult: engine.RecordDecisionResult{DecisionID: "decision-1"}}
	sessions := &fakeSessions{
		activeSession: &taskmodels.TaskSession{
			ID: "sess-active-fallback", TaskID: "task-1", State: taskmodels.TaskSessionStateRunning,
		},
		byID: map[string]*taskmodels.TaskSession{
			"sess-done": {ID: "sess-done", TaskID: "task-1", State: taskmodels.TaskSessionStateCompleted},
		},
	}
	d := New(eng, sessions, logger.Default())

	if _, err := d.RecordDecision(context.Background(), RecordDecisionInput{
		TaskID: "task-1", StepID: "review", ParticipantID: "participant-1",
		Decision: "approved", SessionID: "sess-done",
	}); err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}
	if eng.decisionSession != "" {
		t.Errorf("session id = %q, want blank: a terminal-state session must not bind, and must not fall back to the active-session lookup", eng.decisionSession)
	}
}

// TestDispatcher_RecordDecision_SuppliedSessionStoreLookupErrorPropagated
// proves a genuine session-store failure on the supplied-SessionID path is
// not silently treated as resolveDeciderSessionID's "not found" case
// (dispatcher.go's errors.Is(err, ErrTaskSessionNotFound) branch) — mirrors
// TestDispatcher_RecordDecision_PropagatesActiveSessionLookupError for the
// blank-SessionID path above.
func TestDispatcher_RecordDecision_SuppliedSessionStoreLookupErrorPropagated(t *testing.T) {
	dbErr := errors.New("db down")
	eng := &fakeEngine{decisionResult: engine.RecordDecisionResult{DecisionID: "decision-1"}}
	sessions := &fakeSessions{byIDErr: dbErr}
	d := New(eng, sessions, logger.Default())

	_, err := d.RecordDecision(context.Background(), RecordDecisionInput{
		TaskID: "task-1", StepID: "review", ParticipantID: "participant-1",
		Decision: "approved", SessionID: "sess-any",
	})
	if !errors.Is(err, dbErr) {
		t.Fatalf("err = %v, want wrapped db error", err)
	}
	if eng.decisionCalled {
		t.Error("engine should not be called when the supplied-session lookup fails")
	}
}

// TestDispatcher_RecordDecision_RejectsEmptyTaskID mirrors HandleTrigger's
// existing input validation for the new write-side entry point.
func TestDispatcher_RecordDecision_RejectsEmptyTaskID(t *testing.T) {
	d := New(&fakeEngine{}, &fakeSessions{}, logger.Default())
	if _, err := d.RecordDecision(context.Background(), RecordDecisionInput{Decision: "approved"}); err == nil {
		t.Fatal("expected error for empty task id")
	}
}

// TestDispatcher_EvaluateStepQuorum_UsesLatestSessionForStateRead is F38:
// the state read uses GetTaskSessionByTaskID (latest, any state) even when
// that session is not "reusable" in the comment-trigger sense, because
// EvaluateStepQuorum only needs a session id to satisfy LoadState's
// signature, not a session eligible to run a turn.
func TestDispatcher_EvaluateStepQuorum_UsesLatestSessionForStateRead(t *testing.T) {
	eng := &fakeEngine{quorumResult: engine.QuorumSnapshot{StepID: "review"}}
	sessions := &fakeSessions{
		activeErr: taskmodels.ErrTaskSessionNotFound,
		latestSession: &taskmodels.TaskSession{
			ID:    "sess-failed",
			State: taskmodels.TaskSessionStateFailed,
		},
	}
	d := New(eng, sessions, logger.Default())

	if _, err := d.EvaluateStepQuorum(context.Background(), "task-1"); err != nil {
		t.Fatalf("EvaluateStepQuorum: %v", err)
	}
	if !eng.quorumCalled {
		t.Fatal("engine EvaluateStepQuorum not invoked")
	}
	if eng.quorumTaskID != "task-1" {
		t.Errorf("task id = %q, want task-1", eng.quorumTaskID)
	}
	if eng.quorumSession != "sess-failed" {
		t.Errorf("session id = %q, want sess-failed (F38: any session, any state)", eng.quorumSession)
	}
}

// TestDispatcher_EvaluateStepQuorum_NoSessionAtAllStillCallsEngineWithBlankID
// covers F38's boundary: a task that has never had any session returns the
// engine's own successful empty snapshot rather than erroring.
func TestDispatcher_EvaluateStepQuorum_NoSessionAtAllStillCallsEngineWithBlankID(t *testing.T) {
	eng := &fakeEngine{}
	sessions := &fakeSessions{
		activeErr: taskmodels.ErrTaskSessionNotFound,
		latestErr: taskmodels.ErrTaskSessionNotFound,
	}
	d := New(eng, sessions, logger.Default())

	snapshot, err := d.EvaluateStepQuorum(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("EvaluateStepQuorum: %v", err)
	}
	if eng.quorumSession != "" {
		t.Errorf("session id = %q, want blank", eng.quorumSession)
	}
	if len(snapshot.Guards) != 0 || snapshot.ReevaluationBlocked {
		t.Errorf("snapshot = %+v, want empty/unblocked", snapshot)
	}
}

// TestDispatcher_EvaluateStepQuorum_BlockedOnlyWhenNoActiveSession proves
// the AC-62 conjunction: the engine's ReevaluationBlocked (decisions
// non-empty at current step) is ANDed with "no active session" here, since
// the engine cannot compute that second conjunct itself.
func TestDispatcher_EvaluateStepQuorum_BlockedOnlyWhenNoActiveSession(t *testing.T) {
	tests := []struct {
		name          string
		activeSession *taskmodels.TaskSession
		activeErr     error
		wantBlocked   bool
	}{
		{
			name:        "no active session: blocked",
			activeErr:   taskmodels.ErrTaskSessionNotFound,
			wantBlocked: true,
		},
		{
			name:          "active session present: not blocked",
			activeSession: &taskmodels.TaskSession{ID: "sess-active"},
			wantBlocked:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := &fakeEngine{quorumResult: engine.QuorumSnapshot{
				StepID:              "review",
				ReevaluationBlocked: true, // engine's own decisions-non-empty conjunct
			}}
			sessions := &fakeSessions{
				activeSession: tt.activeSession,
				activeErr:     tt.activeErr,
				latestSession: &taskmodels.TaskSession{ID: "sess-latest"},
			}
			d := New(eng, sessions, logger.Default())

			snapshot, err := d.EvaluateStepQuorum(context.Background(), "task-1")
			if err != nil {
				t.Fatalf("EvaluateStepQuorum: %v", err)
			}
			if snapshot.ReevaluationBlocked != tt.wantBlocked {
				t.Errorf("ReevaluationBlocked = %v, want %v", snapshot.ReevaluationBlocked, tt.wantBlocked)
			}
		})
	}
}

// TestDispatcher_EvaluateStepQuorum_SkipsActiveSessionLookupWhenNotBlocked
// proves the AC-62 conjunction short-circuits: when the engine's own
// conjunct is already false, the second conjunct's active-session lookup
// never runs, so a failing session store cannot spuriously error a
// not-blocked read.
func TestDispatcher_EvaluateStepQuorum_SkipsActiveSessionLookupWhenNotBlocked(t *testing.T) {
	eng := &fakeEngine{quorumResult: engine.QuorumSnapshot{StepID: "review", ReevaluationBlocked: false}}
	sessions := &fakeSessions{
		activeErr:     errors.New("db down"),
		latestSession: &taskmodels.TaskSession{ID: "sess-latest"},
	}
	d := New(eng, sessions, logger.Default())

	snapshot, err := d.EvaluateStepQuorum(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("EvaluateStepQuorum: %v", err)
	}
	if snapshot.ReevaluationBlocked {
		t.Error("ReevaluationBlocked = true, want false")
	}
}

// TestDispatcher_EvaluateStepQuorum_PropagatesEngineError proves a store
// error from the engine's slate read surfaces to the caller (AC-57d
// reserves method-level errors for the not-wired/unresolvable-task cases,
// both handled above this method).
func TestDispatcher_EvaluateStepQuorum_PropagatesEngineError(t *testing.T) {
	engineErr := errors.New("boom")
	eng := &fakeEngine{quorumErr: engineErr}
	sessions := &fakeSessions{activeSession: &taskmodels.TaskSession{ID: "sess-1"}}
	d := New(eng, sessions, logger.Default())

	_, err := d.EvaluateStepQuorum(context.Background(), "task-1")
	if !errors.Is(err, engineErr) {
		t.Fatalf("err = %v, want wrapped engine error", err)
	}
}
