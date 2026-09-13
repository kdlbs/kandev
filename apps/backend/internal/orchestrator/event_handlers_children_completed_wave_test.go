package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/office/waveidentity"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestChildCompletionPayload_DerivesWaveIdentityFromIDSortedRows is this
// task's acceptance test for P4's re-sort: ListChildCompletionRows orders
// by created_at first (see its doc comment), so feeding rows in an order
// that disagrees with ascending id must still produce the id-sorted wave
// identity (AC-OFFICE-WAKE-WAVE-IDENTITY-001.2).
func TestChildCompletionPayload_DerivesWaveIdentityFromIDSortedRows(t *testing.T) {
	// Deliberately out of id order (as if ordered by an earlier created_at).
	rows := []models.ChildCompletionRow{
		{ID: "child-b", State: v1.TaskStateCompleted},
		{ID: "child-a", State: v1.TaskStateCancelled},
	}

	payload := childCompletionPayload("parent-1", rows)

	wantIDs := []string{"child-a", "child-b"}
	if want := waveidentity.WaveKey("parent-1", wantIDs); payload.WaveKey != want {
		t.Fatalf("WaveKey = %q, want %q (rows must be re-sorted ascending by id)", payload.WaveKey, want)
	}
	if want := waveidentity.WaveString("parent-1", wantIDs); payload.WaveString != want {
		t.Fatalf("WaveString = %q, want %q (rows must be re-sorted ascending by id)", payload.WaveString, want)
	}
}

// TestChildCompletionPayload_MatchesCascadeDerivationForSameParentAndSet is
// this task's cross-producer derivation-parity check (proven fully,
// together with the other three producers, in Task 07): for a fixed
// parent and wave-member id set, P4 must derive the exact same wave key
// waveidentity.WaveKey itself would produce directly — i.e. P4 adds no
// transformation beyond the id sort.
func TestChildCompletionPayload_MatchesCascadeDerivationForSameParentAndSet(t *testing.T) {
	rows := []models.ChildCompletionRow{
		{ID: "child-1", State: v1.TaskStateCompleted},
		{ID: "child-2", State: v1.TaskStateCompleted},
	}
	payload := childCompletionPayload("parent-1", rows)

	want := waveidentity.WaveKey("parent-1", []string{"child-1", "child-2"})
	if payload.WaveKey != want {
		t.Fatalf("WaveKey = %q, want %q", payload.WaveKey, want)
	}
}
