// Package stepentry carries the durable "step-entry" allocation contract from
// the caller that resolves a workflow step transition (and therefore knows
// the landing step's on_enter declaration) down to the task repository
// transaction that persists the step change. It intentionally has no
// dependency on the task repository package; the repository reads only the
// already-resolved PendingAllocation/AllocationResult shapes carried on
// context, mirroring how internal/steptelemetry threads attribution.
//
// Scope note: this package only recognises clear_decisions and
// queue_run_for_each_participant as engine-owned on_enter kinds for now.
// queue_run and run_code_review on_enter dispatch (AC-A7/AC-A8 in
// docs/specs/workflow-on-enter-action-dispatch/spec.md) are explicitly
// deferred to a later Build round — see that spec's Verification section and
// the task plan's scope note.
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

// IsEngineOwnedOnEnter reports whether an on_enter action kind is dispatched
// by the step-entry marker system as of this Build round. See the package
// doc's scope note for the kinds deliberately excluded for now.
func IsEngineOwnedOnEnter(t wfmodels.OnEnterActionType) bool {
	switch t {
	case wfmodels.OnEnterClearDecisions, wfmodels.OnEnterQueueRunForEachParticipant:
		return true
	default:
		return false
	}
}

// BuildPendingAllocation inspects a step's on_enter declaration and returns
// the PendingAllocation the write site should allocate, plus false when the
// step declares no engine-owned on_enter actions (nothing to allocate).
func BuildPendingAllocation(stepID string, actions []wfmodels.OnEnterAction) (PendingAllocation, bool) {
	positions := make([]EnginePosition, 0, len(actions))
	for i, action := range actions {
		if IsEngineOwnedOnEnter(action.Type) {
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
