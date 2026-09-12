package engine

import "testing"

// TestQueueRunCallback_OnChildrenCompletedCopiesWaveIdentity is
// AC-OFFICE-WAKE-WAVE-IDENTITY-002.16's engine-seam half: a
// TriggerOnChildrenCompleted dispatch carrying a wave identity on its
// OnChildrenCompletedPayload must have that identity copied onto the
// QueueRunRequest the adapter receives, unchanged.
func TestQueueRunCallback_OnChildrenCompletedCopiesWaveIdentity(t *testing.T) {
	q := &fakeRunQueue{}
	cb := QueueRunCallback{Adapter: q, Primary: fakePrimary{id: "agent-primary"}}
	in := newQueueRunInput("primary", "this")
	in.Trigger = TriggerOnChildrenCompleted
	in.Action.QueueRun.Reason = reasonTaskChildrenCompleted
	in.Payload = OnChildrenCompletedPayload{
		WaveKey:    "task_children_completed:parent-1:deadbeef",
		WaveString: "parent-1|child-1,child-2",
	}

	if _, err := cb.Execute(t.Context(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 queue run call, got %d", len(q.calls))
	}
	got := q.calls[0]
	if got.WaveKey != "task_children_completed:parent-1:deadbeef" {
		t.Fatalf("WaveKey = %q, want it copied from OnChildrenCompletedPayload", got.WaveKey)
	}
	if got.WaveString != "parent-1|child-1,child-2" {
		t.Fatalf("WaveString = %q, want it copied from OnChildrenCompletedPayload", got.WaveString)
	}
}

// TestQueueRunCallback_OtherTriggersLeaveWaveIdentityEmpty is the
// regression guard: any trigger other than on_children_completed must
// never populate WaveKey/WaveString, even though ActionInput.Payload is a
// bare `any` the callback type-asserts.
func TestQueueRunCallback_OtherTriggersLeaveWaveIdentityEmpty(t *testing.T) {
	q := &fakeRunQueue{}
	cb := QueueRunCallback{Adapter: q, Primary: fakePrimary{id: "agent-primary"}}
	in := newQueueRunInput("primary", "this")
	in.OperationID = "task_comment:c-1"
	in.Payload = OnCommentPayload{CommentID: "c-1", AuthorID: "user-1"}
	in.Action.QueueRun.Payload = nil

	if _, err := cb.Execute(t.Context(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 queue run call, got %d", len(q.calls))
	}
	got := q.calls[0]
	if got.WaveKey != "" || got.WaveString != "" {
		t.Fatalf("WaveKey/WaveString = %q/%q, want both empty for a non-children-completed trigger",
			got.WaveKey, got.WaveString)
	}
}

// TestQueueRunForEachParticipantCallback_OnChildrenCompletedCopiesWaveIdentity
// is QueueRunForEachParticipantCallback's half of AC-002.16: it must attach
// the trigger's wave identity to every request it queues, exactly like its
// QueueRunCallback sibling above. The participant slate seats the same
// reviewer twice (a per-task row and a step-template row for the same
// AgentProfileID) so collapseByRoleAgent's within-loop dedupe still applies
// — proving the fix does not depend on there being only one seat — while
// AC-002.13's cross-action duplicate (two different roles resolving to the
// same agent, deduped instead by idx_run_wake_wave once both requests carry
// this identical wave identity) is exercised at the runs/service layer.
func TestQueueRunForEachParticipantCallback_OnChildrenCompletedCopiesWaveIdentity(t *testing.T) {
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
		Trigger:     TriggerOnChildrenCompleted,
		State:       MachineState{TaskID: "task-1", WorkflowID: "wf-1"},
		Step:        StepSpec{ID: "step-review"},
		OperationID: "op-1",
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role:   "reviewer",
				Reason: reasonTaskChildrenCompleted,
			},
		},
		Payload: OnChildrenCompletedPayload{
			WaveKey:    "task_children_completed:parent-1:deadbeef",
			WaveString: "parent-1|child-1,child-2",
		},
	}

	if _, err := cb.Execute(t.Context(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected exactly 1 queue run call (duplicate seats collapsed), got %d", len(q.calls))
	}
	got := q.calls[0]
	if got.AgentProfileID != "rev-A" {
		t.Fatalf("agent_profile_id = %q, want rev-A", got.AgentProfileID)
	}
	if got.WaveKey != "task_children_completed:parent-1:deadbeef" {
		t.Fatalf("WaveKey = %q, want it copied from OnChildrenCompletedPayload", got.WaveKey)
	}
	if got.WaveString != "parent-1|child-1,child-2" {
		t.Fatalf("WaveString = %q, want it copied from OnChildrenCompletedPayload", got.WaveString)
	}
}

// TestQueueRunCallback_OnChildrenCompletedWithOtherReason_LeavesWaveIdentityEmpty
// is AC-002.9's regression test: an on_children_completed queue_run action
// configured with a reason other than task_children_completed must not
// receive the trigger's wave identity, even though the trigger carries one.
// idx_run_wake_wave is keyed on (wake_wave_key, agent_profile_id) only, with
// no reason component, so a differently-reasoned run pulled into that domain
// could collide with the parent's real completion wake.
func TestQueueRunCallback_OnChildrenCompletedWithOtherReason_LeavesWaveIdentityEmpty(t *testing.T) {
	q := &fakeRunQueue{}
	cb := QueueRunCallback{Adapter: q, Primary: fakePrimary{id: "agent-primary"}}
	in := newQueueRunInput("primary", "this")
	in.Trigger = TriggerOnChildrenCompleted
	in.Action.QueueRun.Reason = "follow_up"
	in.Payload = OnChildrenCompletedPayload{
		WaveKey:    "task_children_completed:parent-1:deadbeef",
		WaveString: "parent-1|child-1,child-2",
	}

	if _, err := cb.Execute(t.Context(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 queue run call, got %d", len(q.calls))
	}
	got := q.calls[0]
	if got.WaveKey != "" || got.WaveString != "" {
		t.Fatalf("WaveKey/WaveString = %q/%q, want both empty for a non-task_children_completed reason",
			got.WaveKey, got.WaveString)
	}
}

// TestQueueRunForEachParticipantCallback_OnChildrenCompletedWithOtherReason_LeavesWaveIdentityEmpty
// is QueueRunForEachParticipantCallback's half of the same AC-002.9
// regression: a fan-out action on this trigger configured with a reason
// other than task_children_completed must not receive wave identity either.
func TestQueueRunForEachParticipantCallback_OnChildrenCompletedWithOtherReason_LeavesWaveIdentityEmpty(t *testing.T) {
	q := &fakeRunQueue{}
	parts := scopedParticipants{
		perTask: []ParticipantInfo{
			{ID: "p1", StepID: "step-work", TaskID: "task-1", Role: "reviewer", AgentProfileID: "rev-A"},
		},
	}
	cb := QueueRunForEachParticipantCallback{Adapter: q, Participants: parts}
	in := ActionInput{
		Trigger:     TriggerOnChildrenCompleted,
		State:       MachineState{TaskID: "task-1", WorkflowID: "wf-1"},
		Step:        StepSpec{ID: "step-review"},
		OperationID: "op-1",
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role:   "reviewer",
				Reason: "follow_up",
			},
		},
		Payload: OnChildrenCompletedPayload{
			WaveKey:    "task_children_completed:parent-1:deadbeef",
			WaveString: "parent-1|child-1,child-2",
		},
	}

	if _, err := cb.Execute(t.Context(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 queue run call, got %d", len(q.calls))
	}
	got := q.calls[0]
	if got.WaveKey != "" || got.WaveString != "" {
		t.Fatalf("WaveKey/WaveString = %q/%q, want both empty for a non-task_children_completed reason",
			got.WaveKey, got.WaveString)
	}
}
