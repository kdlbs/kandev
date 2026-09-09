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
