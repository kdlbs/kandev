package dto

import "testing"

type fakeParkedProvider struct {
	parked   bool
	epoch    int64
	revision uint64
}

func (f fakeParkedProvider) ParkedProjectionSnapshot(string) (bool, int64, uint64) {
	return f.parked, f.epoch, f.revision
}

type fakeTaskParkedProvider struct {
	parked   bool
	epoch    int64
	revision uint64
}

func (f fakeTaskParkedProvider) TaskParkedProjectionSnapshot(string) (bool, int64, uint64) {
	return f.parked, f.epoch, f.revision
}

func TestEnrichParked_StampsAllThreeFields(t *testing.T) {
	session := &TaskSessionDTO{ID: "sess-1"}
	EnrichParked(session, fakeParkedProvider{parked: true, epoch: 42, revision: 3})

	if !session.ParkedOnBackgroundWork || session.ParkedEpoch != 42 || session.ParkedRevision != 3 {
		t.Fatalf("got (%v, %d, %d), want (true, 42, 3)",
			session.ParkedOnBackgroundWork, session.ParkedEpoch, session.ParkedRevision)
	}
}

func TestEnrichParked_NilProviderLeavesZeroValues(t *testing.T) {
	session := &TaskSessionDTO{ID: "sess-1", ParkedOnBackgroundWork: true, ParkedEpoch: 1, ParkedRevision: 1}
	EnrichParked(session, nil)

	// D9: a nil (not-yet-wired) provider must not be treated as an authoritative
	// "clear" — EnrichParked is documented as a no-op when provider is nil, so
	// whatever the caller already set on the DTO survives untouched.
	if !session.ParkedOnBackgroundWork || session.ParkedEpoch != 1 || session.ParkedRevision != 1 {
		t.Fatalf("nil provider mutated the DTO: got (%v, %d, %d)",
			session.ParkedOnBackgroundWork, session.ParkedEpoch, session.ParkedRevision)
	}
}

func TestEnrichParkedSummary_StampsAllThreeFields(t *testing.T) {
	summary := &TaskSessionSummaryDTO{ID: "sess-1"}
	EnrichParkedSummary(summary, fakeParkedProvider{parked: true, epoch: 7, revision: 9})

	if !summary.ParkedOnBackgroundWork || summary.ParkedEpoch != 7 || summary.ParkedRevision != 9 {
		t.Fatalf("got (%v, %d, %d), want (true, 7, 9)",
			summary.ParkedOnBackgroundWork, summary.ParkedEpoch, summary.ParkedRevision)
	}
}

// TestEnrichTaskParked_StampsAllThreeFieldsIncludingEpoch is MUST-FIX #2's
// backend half (Review round 3): TaskDTO previously had no ParkedEpoch field
// at all, so even a correct frontend discard rule had nothing to compare
// against on the task carrier (D1: "Every carrier that carries a revision or
// a parked_revision also carries parked_epoch").
func TestEnrichTaskParked_StampsAllThreeFieldsIncludingEpoch(t *testing.T) {
	task := &TaskDTO{ID: "task-1"}
	EnrichTaskParked(task, fakeTaskParkedProvider{parked: true, epoch: 42, revision: 5})

	if !task.ParkedOnBackgroundWork {
		t.Fatal("expected ParkedOnBackgroundWork=true")
	}
	if task.ParkedEpoch != 42 {
		t.Fatalf("ParkedEpoch = %d, want 42", task.ParkedEpoch)
	}
	if task.ParkedRevision != 5 {
		t.Fatalf("ParkedRevision = %d, want 5", task.ParkedRevision)
	}
}

func TestEnrichTaskParked_NilProviderLeavesZeroValues(t *testing.T) {
	task := &TaskDTO{ID: "task-1"}
	EnrichTaskParked(task, nil)

	if task.ParkedOnBackgroundWork || task.ParkedEpoch != 0 || task.ParkedRevision != 0 {
		t.Fatalf("got (%v, %d, %d), want (false, 0, 0)",
			task.ParkedOnBackgroundWork, task.ParkedEpoch, task.ParkedRevision)
	}
}

func TestEnrichTaskParked_NilTaskIsNoOp(t *testing.T) {
	// Must not panic.
	EnrichTaskParked(nil, fakeTaskParkedProvider{parked: true, epoch: 1, revision: 1})
}
