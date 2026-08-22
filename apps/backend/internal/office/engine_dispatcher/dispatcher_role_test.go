package engine_dispatcher

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// TestDispatcher_ResolveParticipantRole_ForwardsVerbatim proves the
// dispatcher passes taskID/stepID/agentProfileID straight through to the
// engine and returns its role/participantID/error verbatim — this entry
// point needs no session resolution, unlike RecordDecision and
// EvaluateStepQuorum.
func TestDispatcher_ResolveParticipantRole_ForwardsVerbatim(t *testing.T) {
	eng := &fakeEngine{roleResult: "approver", roleParticipantID: "seat-1"}
	d := New(eng, &fakeSessions{}, logger.Default())

	role, participantID, err := d.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a")
	if err != nil {
		t.Fatalf("ResolveParticipantRole: %v", err)
	}
	if !eng.roleCalled {
		t.Fatal("engine ResolveParticipantRole not invoked")
	}
	if eng.roleTaskID != "task-1" || eng.roleStepID != "review" || eng.roleAgentProfileID != "agent-a" {
		t.Errorf("engine called with (%q, %q, %q), want (task-1, review, agent-a)",
			eng.roleTaskID, eng.roleStepID, eng.roleAgentProfileID)
	}
	if role != "approver" || participantID != "seat-1" {
		t.Errorf("role/participantID = %q/%q, want approver/seat-1", role, participantID)
	}
}

// TestDispatcher_ResolveParticipantRole_PropagatesEngineError proves a
// permission/store error from the engine surfaces to the caller unwrapped
// of context, including sentinel errors such as engine.ErrParticipantNotFound.
func TestDispatcher_ResolveParticipantRole_PropagatesEngineError(t *testing.T) {
	notFound := errors.New("not a participant")
	eng := &fakeEngine{roleErr: notFound}
	d := New(eng, &fakeSessions{}, logger.Default())

	_, _, err := d.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a")
	if !errors.Is(err, notFound) {
		t.Fatalf("err = %v, want %v", err, notFound)
	}
}

// TestDispatcher_ResolveParticipantRole_NoSessionLookup proves this entry
// point never touches the session resolver — a nil-prone SessionResolver
// still succeeds, since role resolution has no session dependency.
func TestDispatcher_ResolveParticipantRole_NoSessionLookup(t *testing.T) {
	eng := &fakeEngine{roleResult: "reviewer", roleParticipantID: "seat-2"}
	sessions := &fakeSessions{activeErr: errors.New("session store must not be consulted")}
	d := New(eng, sessions, logger.Default())

	if _, _, err := d.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a"); err != nil {
		t.Fatalf("ResolveParticipantRole: %v", err)
	}
}
