package scheduler

import (
	"context"
	"testing"
	"time"
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

// TestApplyTaskMutation_DifferentAgentReassignment_RateLimitedStillInterrupts
// pins AC-OFFICE-ASSIGN-RATE-001.7 through the real dispatcher, not just
// through checkAssignmentWakeAllowance or QueueRun directly: when the
// task's agent-initiated assignment allowance is already exhausted, the
// reassignment wake itself is refused (absent from res.Runs), but the
// mutation is not aborted — the previous assignee's session interrupt
// (res.InterruptSessionID, set in reactToAssigneeChange before queue() is
// ever called) still fires, and ApplyTaskMutation returns no error.
func TestApplyTaskMutation_DifferentAgentReassignment_RateLimitedStillInterrupts(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-old")
	createChildrenCompletedAgent(t, repo, "agent-new")

	taskID := "task-reassign-rate-limited"
	for i := 0; i < AssignmentWakeAllowanceN; i++ {
		createAssignmentWakeRun(t, repo, taskID, time.Now().UTC())
	}

	task := &TaskSnapshot{
		ID:                     taskID,
		WorkspaceID:            "ws-1",
		AssigneeAgentProfileID: "agent-old",
	}
	newAssignee := "agent-new"
	gen := int64(1)
	change := TaskMutation{
		NewAssigneeID:        &newAssignee,
		AssignmentGeneration: &gen,
		ActorID:              "agent-old",
		ActorType:            "agent",
	}

	res, err := ss.ApplyTaskMutation(context.Background(), task, change)
	if err != nil {
		t.Fatalf("ApplyTaskMutation: %v", err)
	}

	if res.InterruptSessionID != task.ID {
		t.Fatalf("InterruptSessionID = %q, want %q (a refusal must not abort the mutation's other effects)",
			res.InterruptSessionID, task.ID)
	}
	for _, r := range res.Runs {
		if r.AgentID == "agent-new" && r.Reason == RunReasonTaskAssigned {
			t.Fatalf("expected the rate-limited reassignment wake to be refused (absent from res.Runs), got %+v", res.Runs)
		}
	}
}

// TestApplyTaskMutation_Unassignment_NeitherRefusesNorConsumesAllowance is
// AC-OFFICE-ASSIGN-RATE-001.9: reactToAssigneeChange returns before ever
// calling queue() when NewAssigneeID is "", so an unassignment on a task
// whose allowance is already exhausted must not be refused (there is no
// wake to refuse) and must not move the rate-limit counter (there is no
// wake to count). The previous assignee's session interrupt still fires —
// unassignment is a real handoff away, not a no-op.
func TestApplyTaskMutation_Unassignment_NeitherRefusesNorConsumesAllowance(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-old")

	taskID := "task-unassign-rate-limit"
	for i := 0; i < AssignmentWakeAllowanceN; i++ {
		createAssignmentWakeRun(t, repo, taskID, time.Now().UTC())
	}

	before := assignmentRateLimitCounterValue(t, "reason=allowance_exhausted")

	task := &TaskSnapshot{
		ID:                     taskID,
		WorkspaceID:            "ws-1",
		AssigneeAgentProfileID: "agent-old",
	}
	newAssignee := ""
	gen := int64(1)
	change := TaskMutation{
		NewAssigneeID:        &newAssignee,
		AssignmentGeneration: &gen,
		ActorID:              "agent-old",
		ActorType:            "agent",
	}

	res, err := ss.ApplyTaskMutation(context.Background(), task, change)
	if err != nil {
		t.Fatalf("ApplyTaskMutation: %v", err)
	}

	if res.InterruptSessionID != task.ID {
		t.Fatalf("InterruptSessionID = %q, want %q (unassigning the current assignee still interrupts their session)",
			res.InterruptSessionID, task.ID)
	}
	for _, r := range res.Runs {
		if r.Reason == RunReasonTaskAssigned {
			t.Fatalf("unassignment must never attempt a task_assigned wake, got %+v", res.Runs)
		}
	}

	after := assignmentRateLimitCounterValue(t, "reason=allowance_exhausted")
	if after != before {
		t.Fatalf("allowance_exhausted counter = %d, want unchanged at %d (unassignment must not be refused or counted)", after, before)
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
