package engine

import (
	"context"
	"strings"
	"testing"
)

// crossTaskParticipants emulates the real repository's two queries closely
// enough to catch a cross-workflow leak: ListStepParticipants filters
// `step_id = ? AND (task_id = ” OR task_id = ?)` (step-scoped, as the repo
// actually enforces); ListTaskParticipants filters only `task_id = ?`,
// ignoring step_id and workflow entirely — the unscoped, any-step query a
// cross-task participant_role target must NOT use. Unlike scopedParticipants
// (quorum_test.go), whose ListStepParticipants always returns nil for a
// non-empty taskID, this fake answers a real step+task lookup so a leak from
// the wrong query is actually observable.
type crossTaskParticipants struct {
	rows []ParticipantInfo
}

func (c crossTaskParticipants) ListStepParticipants(_ context.Context, stepID, taskID string) ([]ParticipantInfo, error) {
	out := make([]ParticipantInfo, 0)
	for _, p := range c.rows {
		if p.StepID != stepID {
			continue
		}
		if p.TaskID != "" && p.TaskID != taskID {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (c crossTaskParticipants) ListTaskParticipants(_ context.Context, taskID string) ([]ParticipantInfo, error) {
	out := make([]ParticipantInfo, 0)
	for _, p := range c.rows {
		if p.TaskID == taskID {
			out = append(out, p)
		}
	}
	return out, nil
}

// WO-30: a participant added while the task sat on an earlier step
// (AddTaskParticipant stamps step_id at write time) must still be woken by
// the fan-out when the task has since advanced — the quorum guard already
// counts that participant via its any-step read (requiredSeatsForWorkflow),
// so the fan-out reading a narrower, step-scoped population left a seat the
// guard requires but the fan-out could never wake, and all_approve waited
// forever. Uses scopedParticipants (quorum_test.go), which — unlike
// fakeParticipants — correctly distinguishes a per-task row's real StepID
// from a step-template row, exercising the case fakeParticipants cannot.
func TestQueueRunForEachParticipantCallback_WakesParticipantAddedOnEarlierStep(t *testing.T) {
	q := &fakeRunQueue{}
	parts := scopedParticipants{
		perTask: []ParticipantInfo{
			{ID: "p1", StepID: "step-work", TaskID: "task-1", Role: "reviewer", AgentProfileID: "rev-A"},
		},
	}
	cb := QueueRunForEachParticipantCallback{Adapter: q, Participants: parts}
	in := ActionInput{
		Trigger:     TriggerOnEnter,
		State:       MachineState{TaskID: "task-1", WorkflowID: "wf-1"},
		Step:        StepSpec{ID: "step-review"},
		OperationID: "op-1",
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role: "reviewer",
			},
		},
	}
	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 queue run call for a reviewer added on an earlier step, got %d", len(q.calls))
	}
	if q.calls[0].AgentProfileID != "rev-A" {
		t.Fatalf("agent_profile_id = %q, want rev-A", q.calls[0].AgentProfileID)
	}
}

// WO-30 regression guard: a reviewer present at BOTH the current step (as a
// template row) and an earlier step (as a per-task row) must be woken
// exactly once, not twice — the any-step gather can return the same
// reviewer from both queries, and collapseByRoleAgent's per-task-wins-over-
// template dedupe is what keeps that from double-queuing a run.
func TestQueueRunForEachParticipantCallback_ReviewerAtBothStepsQueuesOnce(t *testing.T) {
	q := &fakeRunQueue{}
	parts := scopedParticipants{
		perTask: []ParticipantInfo{
			{ID: "p1", StepID: "step-work", TaskID: "task-1", Role: "reviewer", AgentProfileID: "rev-A"},
		},
		template: []ParticipantInfo{
			{ID: "p2", StepID: "step-review", TaskID: "", Role: "reviewer", AgentProfileID: "rev-A"},
		},
	}
	cb := QueueRunForEachParticipantCallback{Adapter: q, Participants: parts}
	in := ActionInput{
		Trigger:     TriggerOnEnter,
		State:       MachineState{TaskID: "task-1", WorkflowID: "wf-1"},
		Step:        StepSpec{ID: "step-review"},
		OperationID: "op-1",
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role: "reviewer",
			},
		},
	}
	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected exactly 1 queue run call for a reviewer seated at both steps, got %d", len(q.calls))
	}
}

// WO-30: the second call site (queue_run with target=participant_role:<role>)
// has the identical defect — on the same arrangement it currently hard-errors
// with "no participants with role", failing the whole action instead of
// silently queuing zero.
func TestQueueRunCallback_ParticipantRoleWakesParticipantAddedOnEarlierStep(t *testing.T) {
	q := &fakeRunQueue{}
	parts := scopedParticipants{
		perTask: []ParticipantInfo{
			{ID: "p1", StepID: "step-work", TaskID: "task-1", Role: "reviewer", AgentProfileID: "rev-A"},
		},
	}
	cb := QueueRunCallback{Adapter: q, Participants: parts}
	in := ActionInput{
		Trigger: TriggerOnComment,
		State:   MachineState{TaskID: "task-1", WorkflowID: "wf-1"},
		Step:    StepSpec{ID: "step-review"},
		Action: Action{
			Kind: ActionQueueRun,
			QueueRun: &QueueRunAction{
				Target: "participant_role:reviewer",
			},
		},
		OperationID: "op-1",
	}
	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 queue run call for a reviewer added on an earlier step, got %d", len(q.calls))
	}
	if q.calls[0].AgentProfileID != "rev-A" {
		t.Fatalf("agent_profile_id = %q, want rev-A", q.calls[0].AgentProfileID)
	}
}

// WO-30 review round 1 regression: applying the any-step slate to a
// CROSS-task participant_role target reads across workflows. A per-task row
// stamped on a step the target task no longer uses (because it changed
// workflow, or simply advanced) must not be woken by a cross-task fan-out —
// unlike the same-task case, the engine has no target-task workflow id to
// scope the any-step query with, so widening it here can only leak. The
// cross-task path must stay step-scoped to the target task's CURRENT step,
// exactly as it was before WO-30.
func TestQueueRunCallback_ParticipantRoleCrossTaskDoesNotLeakStaleStepRow(t *testing.T) {
	q := &fakeRunQueue{}
	parts := crossTaskParticipants{
		rows: []ParticipantInfo{
			{ID: "p-stale", StepID: "step-old", TaskID: "target-task", Role: "reviewer", AgentProfileID: "rev-leaked"},
		},
	}
	cb := QueueRunCallback{Adapter: q, Participants: parts, TaskSteps: fakeTaskSteps{id: "step-new"}}
	in := newQueueRunInput("participant_role:reviewer", "target-task")

	_, err := cb.Execute(context.Background(), in)
	if err == nil {
		t.Fatalf("expected error: cross-task fan-out must not find the stale per-task row from step-old")
	}
	if !strings.Contains(err.Error(), `no participants with role "reviewer"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 0 {
		t.Fatalf("expected 0 queue run calls (stale row must not leak), got %d: %#v", len(q.calls), q.calls)
	}
}
