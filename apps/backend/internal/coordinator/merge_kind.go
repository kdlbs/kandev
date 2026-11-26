package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// mergeKind is the executor of a guarded merge. Its Execute settles the
// proposal itself because only the merge kind distinguishes "never attempted"
// from "outcome unknown".
type mergeKind struct{ svc *Service }

func (k *mergeKind) Kind() string   { return ProposalKindMerge }
func (k *mergeKind) Action() Action { return ActionMerge }

// ReRunsOnStaleClaim is false: a merge is never replayed.
func (k *mergeKind) ReRunsOnStaleClaim() bool { return false }

// ValidatePropose is unused: a merge proposal is created by ProposeMerge,
// which validates against the provider.
func (k *mergeKind) ValidatePropose(_ context.Context, _ *Coordinator, spec json.RawMessage) (json.RawMessage, error) {
	return spec, nil
}

// ValidateEdits refuses any edit: a merge approves exactly the frozen spec.
func (k *mergeKind) ValidateEdits(base, _ json.RawMessage) (json.RawMessage, error) {
	return base, nil
}

// Execute runs the recheck, the dispatch and the provider request, and settles
// the proposal. It always returns errKindSettled.
func (k *mergeKind) Execute(ctx context.Context, claim Claim) (Outcome, error) {
	k.svc.runMerge(ctx, claim)
	return Outcome{}, errKindSettled
}

// SettleStale recovers a stale approving claim without any provider call:
// back to pending when no dispatch was recorded, uncertain when one was.
func (k *mergeKind) SettleStale(ctx context.Context, current *Proposal, cutoff time.Time) (*Proposal, error) {
	return k.svc.settleStaleMerge(ctx, current, cutoff)
}

// errKindSettled tells executeKind that Execute settled the row itself.
var errKindSettled = errors.New("coordinator: kind settled the proposal")

// staleSettler is implemented by a kind that recovers a stale claim itself.
type staleSettler interface {
	SettleStale(ctx context.Context, current *Proposal, cutoff time.Time) (*Proposal, error)
}

type mergeAutomaticKey struct{}

// withMergeAutomatic marks a claim as an automatic attempt.
func withMergeAutomatic(ctx context.Context) context.Context {
	return context.WithValue(ctx, mergeAutomaticKey{}, true)
}

func mergeAutomaticFrom(ctx context.Context) bool {
	v, _ := ctx.Value(mergeAutomaticKey{}).(bool)
	return v
}
