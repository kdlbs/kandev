package engine_dispatcher

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// TestDispatcher_ResolveParticipantRoleReadOnly_ForwardsVerbatim mirrors
// TestDispatcher_ResolveParticipantRole_ForwardsVerbatim for the read-only
// entry point: same pass-through contract, distinct engine method.
func TestDispatcher_ResolveParticipantRoleReadOnly_ForwardsVerbatim(t *testing.T) {
	eng := &fakeEngine{roleReadOnlyResult: "approver", roleReadOnlyParticipantID: "seat-1"}
	d := New(eng, &fakeSessions{}, logger.Default())

	role, participantID, err := d.ResolveParticipantRoleReadOnly(context.Background(), "task-1", "review", "agent-a")
	if err != nil {
		t.Fatalf("ResolveParticipantRoleReadOnly: %v", err)
	}
	if !eng.roleReadOnlyCalled {
		t.Fatal("engine ResolveParticipantRoleReadOnly not invoked")
	}
	if eng.roleCalled {
		t.Error("recording ResolveParticipantRole must not be invoked by the read-only entry point")
	}
	if eng.roleReadOnlyTaskID != "task-1" || eng.roleReadOnlyStepID != "review" || eng.roleReadOnlyAgentProfileID != "agent-a" {
		t.Errorf("engine called with (%q, %q, %q), want (task-1, review, agent-a)",
			eng.roleReadOnlyTaskID, eng.roleReadOnlyStepID, eng.roleReadOnlyAgentProfileID)
	}
	if role != "approver" || participantID != "seat-1" {
		t.Errorf("role/participantID = %q/%q, want approver/seat-1", role, participantID)
	}
}

// TestDispatcher_ResolveParticipantRoleReadOnly_PropagatesEngineError proves
// errors (including engine.ErrParticipantNotFound) surface unwrapped.
func TestDispatcher_ResolveParticipantRoleReadOnly_PropagatesEngineError(t *testing.T) {
	notFound := errors.New("not a participant")
	eng := &fakeEngine{roleReadOnlyErr: notFound}
	d := New(eng, &fakeSessions{}, logger.Default())

	_, _, err := d.ResolveParticipantRoleReadOnly(context.Background(), "task-1", "review", "agent-a")
	if !errors.Is(err, notFound) {
		t.Fatalf("err = %v, want %v", err, notFound)
	}
}

// TestDispatcher_ResolveParticipantRoleReadOnly_NoSessionLookup proves this
// entry point never touches the session resolver, matching the recording
// variant's contract.
func TestDispatcher_ResolveParticipantRoleReadOnly_NoSessionLookup(t *testing.T) {
	eng := &fakeEngine{roleReadOnlyResult: "reviewer", roleReadOnlyParticipantID: "seat-2"}
	sessions := &fakeSessions{activeErr: errors.New("session store must not be consulted")}
	d := New(eng, sessions, logger.Default())

	if _, _, err := d.ResolveParticipantRoleReadOnly(context.Background(), "task-1", "review", "agent-a"); err != nil {
		t.Fatalf("ResolveParticipantRoleReadOnly: %v", err)
	}
}
