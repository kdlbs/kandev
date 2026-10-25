package scheduler

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// fakeParticipantStore is a minimal engine.ParticipantStore test double: a
// flat row set, filtered the same way the two ParticipantStore methods
// partition template (task_id="") vs per-task rows. It does not implement
// engine.WorkflowScopedParticipantStore, so gatherParticipantSlate always
// takes the plain ListTaskParticipants branch regardless of the workflowID
// argument passed to it — sufficient for these single-task, single-step
// fixtures.
type fakeParticipantStore struct {
	rows []engine.ParticipantInfo
	err  error
}

func (f *fakeParticipantStore) ListStepParticipants(_ context.Context, stepID, taskID string) ([]engine.ParticipantInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []engine.ParticipantInfo
	for _, p := range f.rows {
		if p.StepID == stepID && p.TaskID == taskID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeParticipantStore) ListTaskParticipants(_ context.Context, taskID string) ([]engine.ParticipantInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []engine.ParticipantInfo
	for _, p := range f.rows {
		if p.TaskID == taskID {
			out = append(out, p)
		}
	}
	return out, nil
}

// bindForEachParticipantStep wires a fake workflow step whose
// on_children_completed trigger authors one queue_run_for_each_participant
// action for role/reason/payload, plus a participant store standing in for
// the engine's own ParticipantAdapter.
func bindForEachParticipantStep(
	t *testing.T, ss *SchedulerService, parentID, role, reason string, payload map[string]any, store engine.ParticipantStore,
) {
	t.Helper()
	ctx := context.Background()
	if _, err := ss.repo.ExecRaw(ctx,
		`UPDATE tasks SET workflow_step_id = 'step-x' WHERE id = ?`, parentID); err != nil {
		t.Fatalf("bind parent step: %v", err)
	}
	cfg := map[string]any{"role": role, "payload": payload}
	if reason != "" {
		cfg["reason"] = reason
	}
	ss.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-x": {
			ID: "step-x",
			Events: wfmodels.StepEvents{
				OnChildrenCompleted: []wfmodels.GenericAction{
					{
						Type:   wfmodels.GenericActionQueueRunForEachParticipant,
						Config: cfg,
					},
				},
			},
		},
	}})
	ss.SetParticipantStore(store)
}

// TestCascadeChildrenCompleted_PayloadParity_MergesForEachParticipantPayload
// is AC-002.10/.16's queue_run_for_each_participant half: a step authoring
// only a for-each-participant action (no plain queue_run) still gets its
// payload attached to the cascade wake when the fanned-out role resolves a
// seat matching the parent's assignee — the exact agent cascade always
// wakes, and therefore the exact (wave, agent) pair idx_run_wake_wave would
// collapse an engine-routed fan-out run into.
func TestCascadeChildrenCompleted_PayloadParity_MergesForEachParticipantPayload(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	ctx := context.Background()

	store := &fakeParticipantStore{rows: []engine.ParticipantInfo{
		{ID: "seat-1", StepID: "step-x", TaskID: "parent-1", Role: "reviewer", AgentProfileID: "agent-1"},
	}}
	bindForEachParticipantStep(t, ss, "parent-1", "reviewer", RunReasonTaskChildrenCompleted,
		map[string]any{"escalate_to": "lead-agent"}, store)

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")

	var payload string
	queue := func(_ string, c RunContext) {
		encoded, err := encodeRunContext(c)
		if err != nil {
			t.Fatalf("encode run context: %v", err)
		}
		payload = encoded
	}
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	if !strings.Contains(payload, `"escalate_to":"lead-agent"`) {
		t.Fatalf("payload = %s, want it to contain the for-each-participant action's escalate_to key", payload)
	}
}

// TestCascadeChildrenCompleted_PayloadParity_ForEachParticipant_NoMatchingSeat_NoPayload
// covers the non-colliding half: the fanned-out role resolves seats, but
// none of them is the parent's assignee, so the engine-routed fan-out would
// never produce a run for the (wave, parentAssignee) pair cascade wakes —
// attaching this action's payload here would carry a foreign recipient's
// content onto cascade's wake.
func TestCascadeChildrenCompleted_PayloadParity_ForEachParticipant_NoMatchingSeat_NoPayload(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	ctx := context.Background()

	store := &fakeParticipantStore{rows: []engine.ParticipantInfo{
		{ID: "seat-1", StepID: "step-x", TaskID: "parent-1", Role: "reviewer", AgentProfileID: "agent-2"},
	}}
	bindForEachParticipantStep(t, ss, "parent-1", "reviewer", RunReasonTaskChildrenCompleted,
		map[string]any{"escalate_to": "lead-agent"}, store)

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")

	var payload string
	queue := func(_ string, c RunContext) {
		encoded, err := encodeRunContext(c)
		if err != nil {
			t.Fatalf("encode run context: %v", err)
		}
		payload = encoded
	}
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	if strings.Contains(payload, "lead-agent") {
		t.Fatalf("payload = %s, must not contain the for-each-participant payload when no seat matches the assignee", payload)
	}
}

// TestCascadeChildrenCompleted_PayloadParity_ForEachParticipant_NoStoreWired_StillQueues
// covers the omission path (mirroring
// TestCascadeChildrenCompleted_PayloadParity_StepLookupFails_StillQueues): no
// ParticipantStore wired must not block the wake itself, it just queues
// without the merged payload.
func TestCascadeChildrenCompleted_PayloadParity_ForEachParticipant_NoStoreWired_StillQueues(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	ctx := context.Background()

	bindForEachParticipantStep(t, ss, "parent-1", "reviewer", RunReasonTaskChildrenCompleted,
		map[string]any{"escalate_to": "lead-agent"}, nil)

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")

	var queued int
	var payload string
	queue := func(_ string, c RunContext) {
		queued++
		encoded, err := encodeRunContext(c)
		if err != nil {
			t.Fatalf("encode run context: %v", err)
		}
		payload = encoded
	}
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	if queued != 1 {
		t.Fatalf("queued = %d, want 1 (no participant store must not block the wake)", queued)
	}
	if strings.Contains(payload, "lead-agent") {
		t.Fatalf("payload = %s, must not contain the for-each-participant payload with no store wired", payload)
	}
}

// TestCascadeChildrenCompleted_PayloadParity_ForEachParticipant_IgnoresDifferentReasonAction
// mirrors the queue_run reason-matching test: a for-each-participant action
// left at its default (non-task_children_completed) reason must not have
// its payload merged onto this wave, even when a seat matches the assignee.
func TestCascadeChildrenCompleted_PayloadParity_ForEachParticipant_IgnoresDifferentReasonAction(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	ctx := context.Background()

	store := &fakeParticipantStore{rows: []engine.ParticipantInfo{
		{ID: "seat-1", StepID: "step-x", TaskID: "parent-1", Role: "reviewer", AgentProfileID: "agent-1"},
	}}
	// reason left empty: readQueueRunForEachParticipantConfig then falls
	// back to the trigger name ("on_children_completed"), never
	// task_children_completed.
	bindForEachParticipantStep(t, ss, "parent-1", "reviewer", "",
		map[string]any{"escalate_to": "lead-agent"}, store)

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")

	var payload string
	queue := func(_ string, c RunContext) {
		encoded, err := encodeRunContext(c)
		if err != nil {
			t.Fatalf("encode run context: %v", err)
		}
		payload = encoded
	}
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	if strings.Contains(payload, "lead-agent") {
		t.Fatalf("payload = %s, must not contain a differently-reasoned for-each-participant action's payload", payload)
	}
}
