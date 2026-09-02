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

// TestOwnershipTableMatchesDesign asserts the ownership declaration's
// membership matches the system design's table exactly, both dispatcher and
// marker-bearing columns classified independently for every reachable kind.
func TestOwnershipTableMatchesDesign(t *testing.T) {
	cases := map[wfmodels.OnEnterActionType]Ownership{
		wfmodels.OnEnterClearDecisions:             {Dispatcher: DispatcherLedger, MarkerBearing: true},
		wfmodels.OnEnterQueueRunForEachParticipant: {Dispatcher: DispatcherLedger, MarkerBearing: true},
		wfmodels.OnEnterQueueRun:                   {Dispatcher: DispatcherLedger, MarkerBearing: false},
		wfmodels.OnEnterRunCodeReview:              {Dispatcher: DispatcherLedger, MarkerBearing: false},
		wfmodels.OnEnterEnsureParticipantSeat:      {Dispatcher: DispatcherLedger, MarkerBearing: false},
		wfmodels.OnEnterEnablePlanMode:             {Dispatcher: DispatcherMarker, MarkerBearing: false},
		wfmodels.OnEnterAutoStartAgent:             {Dispatcher: DispatcherMarker, MarkerBearing: false},
		wfmodels.OnEnterResetAgentContext:          {Dispatcher: DispatcherMarker, MarkerBearing: false},
		wfmodels.OnEnterSetSessionMode:             {Dispatcher: DispatcherMarker, MarkerBearing: false},
		wfmodels.OnEnterConfigureSession:           {Dispatcher: DispatcherMarker, MarkerBearing: false},
	}
	for kind, want := range cases {
		got, ok := Owner(string(kind))
		if !ok {
			t.Errorf("Owner(%s): expected a classification, found none", kind)
			continue
		}
		if got != want {
			t.Errorf("Owner(%s) = %+v, want %+v", kind, got, want)
		}
		if OwnedByLedger(string(kind)) != (want.Dispatcher == DispatcherLedger) {
			t.Errorf("OwnedByLedger(%s) disagrees with Owner", kind)
		}
		if OwnedByMarker(string(kind)) != (want.Dispatcher == DispatcherMarker) {
			t.Errorf("OwnedByMarker(%s) disagrees with Owner", kind)
		}
		if MarkerBearing(string(kind)) != want.MarkerBearing {
			t.Errorf("MarkerBearing(%s) disagrees with Owner", kind)
		}
	}
	if len(cases) != 10 {
		t.Fatalf("expected exactly 10 classified kinds (the design's table), got %d", len(cases))
	}
}

// TestOwnershipTableUnclassifiedKind asserts an unknown kind is classified by
// neither accessor, rather than defaulting to either dispatcher.
func TestOwnershipTableUnclassifiedKind(t *testing.T) {
	if _, ok := Owner("move_to_next"); ok {
		t.Fatalf("expected move_to_next (a transition action, not an entry action) to be unclassified")
	}
	if OwnedByLedger("move_to_next") || OwnedByMarker("move_to_next") {
		t.Fatalf("expected an unclassified kind to be owned by neither dispatcher")
	}
	if MarkerBearing("move_to_next") {
		t.Fatalf("expected an unclassified kind to be non-marker-bearing")
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

// TestSerializePositions covers AC-OFFICE-STEP-ENTRY-DISPATCH-002.1/.9: the
// encoding allocateStepEntryIfPending persists into
// workflow_step_entries.marker_positions must actually carry the allocated
// positions, in order — a version that always returned "" would otherwise
// pass every other test in this package (BuildPendingAllocation's own
// callers never re-parse it).
func TestSerializePositions(t *testing.T) {
	cases := []struct {
		name      string
		positions []EnginePosition
		want      string
	}{
		{name: "empty", positions: nil, want: ""},
		{name: "single", positions: []EnginePosition{{Position: 0, Kind: "clear_decisions"}}, want: "0"},
		{
			name: "multi preserves order",
			positions: []EnginePosition{
				{Position: 0, Kind: "clear_decisions"},
				{Position: 2, Kind: "queue_run_for_each_participant"},
			},
			want: "0,2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SerializePositions(tc.positions); got != tc.want {
				t.Fatalf("SerializePositions(%+v) = %q, want %q", tc.positions, got, tc.want)
			}
		})
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
