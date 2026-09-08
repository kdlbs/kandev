package stepentry

import (
	"context"
	"reflect"
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestComputeDigestDeterministicAndPositionSensitive(t *testing.T) {
	a := []wfmodels.OnEnterAction{
		{Type: wfmodels.OnEnterClearDecisions},
		{Type: wfmodels.OnEnterQueueRunForEachParticipant},
	}
	b := []wfmodels.OnEnterAction{
		{Type: wfmodels.OnEnterClearDecisions},
		{Type: wfmodels.OnEnterQueueRunForEachParticipant},
	}
	if ComputeDigest(a) != ComputeDigest(b) {
		t.Fatalf("expected identical declarations to produce identical digests")
	}

	reordered := []wfmodels.OnEnterAction{
		{Type: wfmodels.OnEnterQueueRunForEachParticipant},
		{Type: wfmodels.OnEnterClearDecisions},
	}
	if ComputeDigest(a) == ComputeDigest(reordered) {
		t.Fatalf("expected reordered declaration to change the digest")
	}

	withConfigOnly := []wfmodels.OnEnterAction{
		{Type: wfmodels.OnEnterClearDecisions, Config: map[string]interface{}{"unused": "value"}},
		{Type: wfmodels.OnEnterQueueRunForEachParticipant},
	}
	if ComputeDigest(a) != ComputeDigest(withConfigOnly) {
		t.Fatalf("expected a config-only change to leave the digest unchanged (shape digest, not config digest)")
	}
}

func TestIsEngineOwnedOnEnter(t *testing.T) {
	cases := map[wfmodels.OnEnterActionType]bool{
		wfmodels.OnEnterClearDecisions:             true,
		wfmodels.OnEnterQueueRunForEachParticipant: true,
		wfmodels.OnEnterQueueRun:                   false,
		wfmodels.OnEnterRunCodeReview:              false,
		wfmodels.OnEnterEnablePlanMode:             false,
		wfmodels.OnEnterAutoStartAgent:             false,
		wfmodels.OnEnterResetAgentContext:          false,
		wfmodels.OnEnterSetSessionMode:             false,
	}
	for kind, want := range cases {
		if got := IsEngineOwnedOnEnter(kind); got != want {
			t.Errorf("IsEngineOwnedOnEnter(%s) = %v, want %v", kind, got, want)
		}
	}
}

func TestBuildPendingAllocationNoEngineOwnedActions(t *testing.T) {
	actions := []wfmodels.OnEnterAction{
		{Type: wfmodels.OnEnterEnablePlanMode},
		{Type: wfmodels.OnEnterAutoStartAgent},
	}
	_, ok := BuildPendingAllocation("step-1", actions)
	if ok {
		t.Fatalf("expected no allocation for a step with no engine-owned on_enter actions")
	}
}

func TestBuildPendingAllocationTracksDeclaredPositions(t *testing.T) {
	actions := []wfmodels.OnEnterAction{
		{Type: wfmodels.OnEnterEnablePlanMode},
		{Type: wfmodels.OnEnterClearDecisions},
		{Type: wfmodels.OnEnterQueueRunForEachParticipant},
	}
	pending, ok := BuildPendingAllocation("step-1", actions)
	if !ok {
		t.Fatalf("expected an allocation for a step declaring engine-owned actions")
	}
	if pending.StepID != "step-1" {
		t.Fatalf("expected StepID to be carried through, got %q", pending.StepID)
	}
	if len(pending.Positions) != 2 {
		t.Fatalf("expected 2 engine-owned positions, got %d: %+v", len(pending.Positions), pending.Positions)
	}
	if pending.Positions[0].Position != 1 || pending.Positions[0].Kind != string(wfmodels.OnEnterClearDecisions) {
		t.Errorf("unexpected first position: %+v", pending.Positions[0])
	}
	if pending.Positions[1].Position != 2 || pending.Positions[1].Kind != string(wfmodels.OnEnterQueueRunForEachParticipant) {
		t.Errorf("unexpected second position: %+v", pending.Positions[1])
	}
	if pending.Digest != ComputeDigest(actions) {
		t.Errorf("expected pending.Digest to match ComputeDigest(actions)")
	}
}

func TestPendingAllocationContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	if _, ok := FromContext(ctx); ok {
		t.Fatalf("expected no pending allocation on a bare context")
	}
	pending := PendingAllocation{StepID: "step-1", Digest: "abc"}
	ctx = WithPendingAllocation(ctx, pending)
	got, ok := FromContext(ctx)
	if !ok {
		t.Fatalf("expected pending allocation to round-trip through context")
	}
	if !reflect.DeepEqual(got, pending) {
		t.Errorf("got %+v, want %+v", got, pending)
	}
}

func TestResultHolderContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	if _, ok := ResultHolderFromContext(ctx); ok {
		t.Fatalf("expected no result holder on a bare context")
	}
	holder := &AllocationResult{}
	ctx = WithResultHolder(ctx, holder)
	got, ok := ResultHolderFromContext(ctx)
	if !ok {
		t.Fatalf("expected result holder to round-trip through context")
	}
	got.EntryID = 42
	got.EntrySeq = 3
	if holder.EntryID != 42 || holder.EntrySeq != 3 {
		t.Fatalf("expected mutations through the round-tripped pointer to reach the original holder")
	}
}
