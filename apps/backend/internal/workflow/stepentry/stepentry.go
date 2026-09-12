// Package stepentry carries the durable "step-entry" allocation contract from
// the caller that resolves a workflow step transition (and therefore knows
// the landing step's on_enter declaration) down to the task repository
// transaction that persists the step change. It intentionally has no
// dependency on the task repository package; the repository reads only the
// already-resolved PendingAllocation/AllocationResult shapes carried on
// context, mirroring how internal/steptelemetry threads attribution.
//
// Only clear_decisions and queue_run_for_each_participant are marker-bearing
// (carry a step-entry marker and an allocated position). queue_run and
// run_code_review are ledger-owned but not marker-bearing — see
// ownershipTable's doc comment for the full ten-kind classification.
package stepentry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// EnginePosition is one engine-owned on_enter action's declared position
// (0-based, index into the step's on_enter list) and kind, as resolved by
// the caller that has the workflow step in hand.
type EnginePosition struct {
	Position int
	Kind     string
}

// PendingAllocation is what the caller resolving a step transition passes
// down to the write-site transaction: which step is being entered, the
// digest of its full on_enter declaration, and the ordered list of
// engine-owned positions the write site should allocate markers for.
type PendingAllocation struct {
	StepID    string
	Digest    string
	Positions []EnginePosition
}

// AllocationResult is a mutable out-parameter: the caller creates one, wraps
// it onto the context via WithResultHolder, and reads it back after the
// write-site transaction commits. It is populated by the repository layer.
type AllocationResult struct {
	EntryID  int64
	EntrySeq int64
}

// MarkerState is a step-entry marker's terminal (or in-progress) state.
// Declared here rather than in the task repository package so that
// dispatch-side callers (internal/orchestrator) can reference it without
// importing the repository package's implementation types.
type MarkerState string

const (
	// MarkerInProgress is set the moment a claim succeeds.
	MarkerInProgress MarkerState = "in_progress"
	// MarkerDone marks a successfully executed action.
	MarkerDone MarkerState = "done"
	// MarkerFailed marks an action whose callback returned an error; the
	// error is recorded alongside the marker.
	MarkerFailed MarkerState = "failed"
)

type pendingAllocationKey struct{}
type resultHolderKey struct{}

// WithPendingAllocation attaches the resolved allocation input to ctx.
func WithPendingAllocation(ctx context.Context, p PendingAllocation) context.Context {
	return context.WithValue(ctx, pendingAllocationKey{}, p)
}

// FromContext reads back a PendingAllocation previously attached with
// WithPendingAllocation. ok is false when none was attached (the common
// case for callers that have not opted into step-entry allocation yet).
func FromContext(ctx context.Context) (PendingAllocation, bool) {
	p, ok := ctx.Value(pendingAllocationKey{}).(PendingAllocation)
	return p, ok
}

// WithResultHolder attaches a mutable AllocationResult pointer to ctx so the
// repository layer can report the allocated entry identity back to a caller
// several function calls away without changing every intermediate
// signature — the same shape steptelemetry uses for attribution, just with
// an output instead of an input.
func WithResultHolder(ctx context.Context, r *AllocationResult) context.Context {
	return context.WithValue(ctx, resultHolderKey{}, r)
}

// ResultHolderFromContext reads back the AllocationResult pointer previously
// attached with WithResultHolder.
func ResultHolderFromContext(ctx context.Context) (*AllocationResult, bool) {
	r, ok := ctx.Value(resultHolderKey{}).(*AllocationResult)
	return r, ok
}

// IsMarkerBearing reports whether an on_enter action kind carries a
// step-entry marker. Delegates to MarkerBearing, the single declaration
// AC-OFFICE-STEP-ENTRY-DISPATCH-002.1 requires — see Ownership's doc comment
// for why this must not be reseeded independently. Named distinctly from
// "engine-owned"/"ledger-owned" (OwnedByLedger): marker-bearing and
// ledger-owned are different, independent sets — three ledger-owned kinds
// (queue_run, run_code_review, ensure_participant_seat) carry no marker.
func IsMarkerBearing(t wfmodels.OnEnterActionType) bool {
	return MarkerBearing(string(t))
}

// Dispatcher identifies which of the two step-entry dispatchers owns an
// action kind.
type Dispatcher string

const (
	// DispatcherLedger is Repository.dispatchStepEntry -> Engine.DispatchStepEntry,
	// which runs synchronously after every registered step-transition
	// writer's own commit, on every route.
	DispatcherLedger Dispatcher = "ledger"
	// DispatcherMarker is processOnEnter -> dispatchOnEnterActions, which
	// runs only on the on_turn_complete route.
	DispatcherMarker Dispatcher = "marker"
)

// Ownership records, for one action kind, which dispatcher owns it and
// whether it is marker-bearing. The two properties are independent and must
// not be conflated (AC-OFFICE-STEP-ENTRY-DISPATCH-002.1): a kind can be
// owned and carry no marker, as three ledger-owned kinds already do.
type Ownership struct {
	Dispatcher    Dispatcher
	MarkerBearing bool
}

// ownershipTable is the single declaration both dispatchers read instead of
// keeping a private list (AC-OFFICE-STEP-ENTRY-DISPATCH-002.1). Ownership is
// seeded from the pre-convergence sessionIndependentActionKinds membership
// (the five kinds Engine.DispatchStepEntry already executes); marker-bearing
// is seeded from the pre-convergence IsMarkerBearing membership (the two
// kinds the marker system already records). The two seeds are different
// functions over different histories and are not interchangeable: seeding
// ownership from the marker-bearing seed would silently drop queue_run,
// run_code_review and ensure_participant_seat; seeding marker-bearing from
// the ownership seed would promise a marker for three kinds that have never
// carried one. configure_session is written in by hand — CompileOnEnterAction
// has no case for it, so no engine ActionKind ever reaches this table for it,
// but AC-OFFICE-STEP-ENTRY-DISPATCH-002.1 still requires it classified in
// both columns as one of the five session-shaped kinds Out of scope names.
var ownershipTable = map[string]Ownership{
	string(wfmodels.OnEnterClearDecisions):             {Dispatcher: DispatcherLedger, MarkerBearing: true},
	string(wfmodels.OnEnterQueueRunForEachParticipant): {Dispatcher: DispatcherLedger, MarkerBearing: true},
	string(wfmodels.OnEnterQueueRun):                   {Dispatcher: DispatcherLedger, MarkerBearing: false},
	string(wfmodels.OnEnterRunCodeReview):              {Dispatcher: DispatcherLedger, MarkerBearing: false},
	string(wfmodels.OnEnterEnsureParticipantSeat):      {Dispatcher: DispatcherLedger, MarkerBearing: false},
	string(wfmodels.OnEnterEnablePlanMode):             {Dispatcher: DispatcherMarker, MarkerBearing: false},
	string(wfmodels.OnEnterAutoStartAgent):             {Dispatcher: DispatcherMarker, MarkerBearing: false},
	string(wfmodels.OnEnterResetAgentContext):          {Dispatcher: DispatcherMarker, MarkerBearing: false},
	string(wfmodels.OnEnterSetSessionMode):             {Dispatcher: DispatcherMarker, MarkerBearing: false},
	string(wfmodels.OnEnterConfigureSession):           {Dispatcher: DispatcherMarker, MarkerBearing: false},
}

// Owner returns the declared ownership for kind and false when kind is not
// classified in the table (neither dispatcher reaches it).
func Owner(kind string) (Ownership, bool) {
	o, ok := ownershipTable[kind]
	return o, ok
}

// OwnedByLedger reports whether kind's owning dispatcher is the ledger
// dispatcher. False for a kind the table does not classify.
func OwnedByLedger(kind string) bool {
	o, ok := ownershipTable[kind]
	return ok && o.Dispatcher == DispatcherLedger
}

// OwnedByMarker reports whether kind's owning dispatcher is the marker
// dispatcher. False for a kind the table does not classify.
func OwnedByMarker(kind string) bool {
	o, ok := ownershipTable[kind]
	return ok && o.Dispatcher == DispatcherMarker
}

// MarkerBearing reports whether kind is marker-bearing, per the single
// declaration this package owns (AC-OFFICE-STEP-ENTRY-DISPATCH-002.1). False
// for a kind the table does not classify.
func MarkerBearing(kind string) bool {
	o, ok := ownershipTable[kind]
	return ok && o.MarkerBearing
}

// KnownKinds returns every action kind the ownership table classifies,
// filtered to dispatcher (pass "" for every kind regardless of dispatcher).
// Exported for tests that need to assert the table's membership stays in
// sync with what the engine actually compiles, without reaching into this
// package's private map.
func KnownKinds(dispatcher Dispatcher) []string {
	kinds := make([]string, 0, len(ownershipTable))
	for kind, o := range ownershipTable {
		if dispatcher == "" || o.Dispatcher == dispatcher {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

// BuildPendingAllocation inspects a step's on_enter declaration and returns
// the PendingAllocation the write site should allocate, plus false when the
// step declares no engine-owned on_enter actions (nothing to allocate).
func BuildPendingAllocation(stepID string, actions []wfmodels.OnEnterAction) (PendingAllocation, bool) {
	positions := make([]EnginePosition, 0, len(actions))
	for i, action := range actions {
		if IsMarkerBearing(action.Type) {
			positions = append(positions, EnginePosition{Position: i, Kind: string(action.Type)})
		}
	}
	if len(positions) == 0 {
		return PendingAllocation{}, false
	}
	return PendingAllocation{
		StepID:    stepID,
		Digest:    ComputeDigest(actions),
		Positions: positions,
	}, true
}

// SerializePositions encodes an ordered list of marker-bearing positions as
// the comma-separated string workflow_step_entries.marker_positions stores —
// the allocated position set an entry's markers were actually claimed for,
// persisted once at allocation time rather than re-derived later from the
// step's current (possibly since-edited) on_enter declaration. Empty when
// positions is empty (a step with no marker-bearing on_enter action is never
// allocated an entry at all — see BuildPendingAllocation — so this only
// happens for a caller that skips that guard).
func SerializePositions(positions []EnginePosition) string {
	if len(positions) == 0 {
		return ""
	}
	parts := make([]string, len(positions))
	for i, p := range positions {
		parts[i] = strconv.Itoa(p.Position)
	}
	return strings.Join(parts, ",")
}

// ComputeDigest returns a stable digest of a step's full on_enter
// declaration (every action, not just the engine-owned ones), so a
// mid-flight template edit can eventually be detected by comparing an
// entry's stored digest against a freshly computed one. The algorithm and
// encoding are this package's choice (AC-H7): sha256 hex over the ordered
// "position:kind" pairs. Config payloads are deliberately excluded — this
// digest only needs to notice a shape change (actions added/removed/
// reordered/retyped), not a config tweak.
func ComputeDigest(actions []wfmodels.OnEnterAction) string {
	var b strings.Builder
	for i, action := range actions {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strconv.Itoa(i))
		b.WriteByte(':')
		b.WriteString(string(action.Type))
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
