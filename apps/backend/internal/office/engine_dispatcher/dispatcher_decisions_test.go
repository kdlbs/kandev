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

// TestDispatcher_RecordDecision_UsesSuppliedSessionOverActiveSession is the
// office-session-identity happy path: when RecordDecisionInput.SessionID is
// populated, the dispatcher resolves and forwards THAT session rather than
// the task's most-recently-started ("active") session — even when the two
// differ, proving the decider's own session wins.
func TestDispatcher_RecordDecision_UsesSuppliedSessionOverActiveSession(t *testing.T) {
	eng := &fakeEngine{decisionResult: engine.RecordDecisionResult{DecisionID: "decision-1"}}
	sessions := &fakeSessions{
		// The "active" (most-recently-started) session is a different one
		// than the decider's own — proves SessionID, not activeSession, wins.
		activeSession: &taskmodels.TaskSession{ID: "sess-runner"},
		byID: map[string]*taskmodels.TaskSession{
			"sess-reviewer": {ID: "sess-reviewer", TaskID: "task-1", State: taskmodels.TaskSessionStateRunning},
		},
	}
	d := New(eng, sessions, logger.Default())

	_, err := d.RecordDecision(context.Background(), RecordDecisionInput{
		TaskID: "task-1", StepID: "review", ParticipantID: "participant-1",
		Decision: "approved", SessionID: "sess-reviewer",
	})
	if err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}
	if eng.decisionSession != "sess-reviewer" {
		t.Errorf("session id = %q, want sess-reviewer (decider's own session, not the active one)", eng.decisionSession)
	}
}

// TestDispatcher_RecordDecision_ForeignSessionFallsBackToUnresolvable
// covers the "session_unresolvable, not an error" contract: a supplied
// session that belongs to a different task must not leak cross-task state
// into re-evaluation, but must also not fail the decision outright — it
// degrades to the same blank-session-id "skip re-evaluation" behavior
// AC-16a already gives an unresolvable active session.
func TestDispatcher_RecordDecision_ForeignSessionFallsBackToUnresolvable(t *testing.T) {
	eng := &fakeEngine{decisionResult: engine.RecordDecisionResult{DecisionID: "decision-1"}}
	sessions := &fakeSessions{
		byID: map[string]*taskmodels.TaskSession{
			"sess-foreign": {ID: "sess-foreign", TaskID: "other-task", State: taskmodels.TaskSessionStateRunning},
		},
	}
	d := New(eng, sessions, logger.Default())

	_, err := d.RecordDecision(context.Background(), RecordDecisionInput{
		TaskID: "task-1", StepID: "review", ParticipantID: "participant-1",
		Decision: "approved", SessionID: "sess-foreign",
	})
	if err != nil {
		t.Fatalf("RecordDecision: %v, want success (session_unresolvable, not an error)", err)
	}
	if !eng.decisionCalled {
		t.Fatal("engine RecordParticipantDecision not invoked")
	}
	if eng.decisionSession != "" {
		t.Errorf("session id = %q, want blank: foreign session must not be forwarded", eng.decisionSession)
	}
}

// TestDispatcher_RecordDecision_TerminalSessionFallsBackToUnresolvable
// covers the other session_unresolvable case: a supplied session that
// belongs to the right task but is in a terminal (non-active) state.
func TestDispatcher_RecordDecision_TerminalSessionFallsBackToUnresolvable(t *testing.T) {
	eng := &fakeEngine{decisionResult: engine.RecordDecisionResult{DecisionID: "decision-1"}}
	sessions := &fakeSessions{
		byID: map[string]*taskmodels.TaskSession{
			"sess-done": {ID: "sess-done", TaskID: "task-1", State: taskmodels.TaskSessionStateCompleted},
		},
	}
	d := New(eng, sessions, logger.Default())

	_, err := d.RecordDecision(context.Background(), RecordDecisionInput{
		TaskID: "task-1", StepID: "review", ParticipantID: "participant-1",
		Decision: "approved", SessionID: "sess-done",
	})
	if err != nil {
		t.Fatalf("RecordDecision: %v, want success (session_unresolvable, not an error)", err)
	}
	if eng.decisionSession != "" {
		t.Errorf("session id = %q, want blank: terminal session must not be forwarded", eng.decisionSession)
	}
}

// TestDispatcher_RecordDecision_MissingSessionFallsBackToUnresolvable
// covers the third session_unresolvable case: a supplied session id that
// does not exist at all (e.g. a stale/deleted session).
func TestDispatcher_RecordDecision_MissingSessionFallsBackToUnresolvable(t *testing.T) {
	eng := &fakeEngine{decisionResult: engine.RecordDecisionResult{DecisionID: "decision-1"}}
	sessions := &fakeSessions{} // byID is nil: GetTaskSession returns ErrTaskSessionNotFound
	d := New(eng, sessions, logger.Default())

	_, err := d.RecordDecision(context.Background(), RecordDecisionInput{
		TaskID: "task-1", StepID: "review", ParticipantID: "participant-1",
		Decision: "approved", SessionID: "sess-missing",
	})
	if err != nil {
		t.Fatalf("RecordDecision: %v, want success (session_unresolvable, not an error)", err)
	}
	if eng.decisionSession != "" {
		t.Errorf("session id = %q, want blank: missing session must not be forwarded", eng.decisionSession)
	}
}

// TestDispatcher_RecordDecision_SessionLookupErrorPropagates proves a
// genuine session-store failure while resolving the supplied SessionID is
// not silently swallowed as session_unresolvable — mirroring
// TestDispatcher_RecordDecision_PropagatesActiveSessionLookupError for the
// resolveActiveSessionID path.
func TestDispatcher_RecordDecision_SessionLookupErrorPropagates(t *testing.T) {
	dbErr := errors.New("db down")
	eng := &fakeEngine{}
	sessions := &fakeSessions{byIDErr: dbErr}
	d := New(eng, sessions, logger.Default())

	_, err := d.RecordDecision(context.Background(), RecordDecisionInput{
		TaskID: "task-1", StepID: "review", ParticipantID: "participant-1",
		Decision: "approved", SessionID: "sess-1",
	})
	if !errors.Is(err, dbErr) {
		t.Fatalf("err = %v, want wrapped db error", err)
	}
	if eng.decisionCalled {
		t.Error("engine should not be called when session lookup fails")
	}
}
