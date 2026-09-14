package scheduler

import (
	"context"
	"testing"
)

// TestApplyTaskMutation_SameAgentRepeatAssignment_QueuesWithoutInterrupt
// pins the reactivity gate relaxation this feature made: ApplyTaskMutation
// now calls reactToAssigneeChange for EVERY non-nil NewAssigneeID, including
// a repeat assignment to the agent that already holds the seat, because that
// is a real occurrence (the operator asking for the work again) rather than
// a no-op. Before this feature the caller-side equality gate suppressed it
// entirely. The relocated interrupt-comparison guard must still hold: a
// same-agent repeat must NOT hard-cancel the agent's own in-flight session.
// Before this test, ApplyTaskMutation/reactToStatusChange/
// reactToAssigneeChange were at 0.0% coverage.
func TestApplyTaskMutation_SameAgentRepeatAssignment_QueuesWithoutInterrupt(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-repeat")

	task := &TaskSnapshot{
		ID:                     "task-repeat-assign",
		WorkspaceID:            "ws-1",
		AssigneeAgentProfileID: "agent-repeat", // already holds the seat
	}
	newAssignee := "agent-repeat"
	gen := int64(1)
	change := TaskMutation{
		NewAssigneeID:        &newAssignee,
		AssignmentGeneration: &gen,
		ActorID:              "user-1",
		ActorType:            "user",
	}

	res, err := ss.ApplyTaskMutation(context.Background(), task, change)
	if err != nil {
		t.Fatalf("ApplyTaskMutation: %v", err)
	}

	if res.InterruptSessionID != "" {
		t.Fatalf("same-agent repeat assignment set InterruptSessionID = %q, want empty (must not cancel its own run)", res.InterruptSessionID)
	}
	found := false
	for _, r := range res.Runs {
		if r.AgentID == "agent-repeat" && r.Reason == RunReasonTaskAssigned {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a task_assigned run queued for agent-repeat, got %+v", res.Runs)
	}
}

// TestApplyTaskMutation_DifferentAgentReassignment_InterruptsPreviousAssignee
// is the positive counterpart: reassigning to a DIFFERENT agent must still
// hard-cancel the previous assignee's session — the interrupt guard's
// comparison (moved inside reactToAssigneeChange by this feature) must not
// have accidentally suppressed the case it was already handling.
func TestApplyTaskMutation_DifferentAgentReassignment_InterruptsPreviousAssignee(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-old")
	createChildrenCompletedAgent(t, repo, "agent-new")

	task := &TaskSnapshot{
		ID:                     "task-reassign",
		WorkspaceID:            "ws-1",
		AssigneeAgentProfileID: "agent-old",
	}
	newAssignee := "agent-new"
	gen := int64(1)
	change := TaskMutation{
		NewAssigneeID:        &newAssignee,
		AssignmentGeneration: &gen,
		ActorID:              "user-1",
		ActorType:            "user",
	}

	res, err := ss.ApplyTaskMutation(context.Background(), task, change)
	if err != nil {
		t.Fatalf("ApplyTaskMutation: %v", err)
	}

	if res.InterruptSessionID != task.ID {
		t.Fatalf("InterruptSessionID = %q, want %q (previous assignee's session must be cancelled)", res.InterruptSessionID, task.ID)
	}
	found := false
	for _, r := range res.Runs {
		if r.AgentID == "agent-new" && r.Reason == RunReasonTaskAssigned {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a task_assigned run queued for agent-new, got %+v", res.Runs)
	}
}

// TestApplyTaskMutation_StatusChangeAndCommentDispatch drives
// ApplyTaskMutation's NewStatus and Comment branches together (an unblock
// plus an attached comment), covering the dispatcher itself rather than only
// the reactToStatusChange/reactToComment helpers it delegates to.
func TestApplyTaskMutation_StatusChangeAndCommentDispatch(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-dispatch")

	task := &TaskSnapshot{
		ID:                     "task-dispatch",
		WorkspaceID:            "ws-1",
		State:                  "BLOCKED",
		AssigneeAgentProfileID: "agent-dispatch",
	}
	newStatus := "todo"
	change := TaskMutation{
		NewStatus: &newStatus,
		Comment:   &MutationComment{ID: "comment-dispatch", AuthorType: "user", AuthorID: "user-1"},
		ActorID:   "user-1",
		ActorType: "user",
	}

	res, err := ss.ApplyTaskMutation(context.Background(), task, change)
	if err != nil {
		t.Fatalf("ApplyTaskMutation: %v", err)
	}

	reasons := map[string]bool{}
	for _, r := range res.Runs {
		reasons[r.Reason] = true
	}
	if !reasons[RunReasonTaskUnblocked] {
		t.Fatalf("expected %s among dispatched runs, got %+v", RunReasonTaskUnblocked, res.Runs)
	}
	if !reasons[RunReasonTaskComment] {
		t.Fatalf("expected %s among dispatched runs, got %+v", RunReasonTaskComment, res.Runs)
	}
}
