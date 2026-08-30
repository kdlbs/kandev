// Package routing defines the durable identity and outcome contract shared by
// every workflow-route producer. It intentionally contains no repository or
// orchestrator dependencies so the operation can travel on context into the
// task transaction that owns the physical lane change.
package routing

import (
	"context"
	"errors"
)

var ErrOperationIdentityConflict = errors.New("route operation identity conflict")

type Producer string

const (
	ProducerManualMove   Producer = "manual_move"
	ProducerDeferredMove Producer = "deferred_move"
	ProducerStepComplete Producer = "step_complete"
	ProducerWorkflow     Producer = "workflow_auto_advance"
	ProducerMergedPR     Producer = "merged_pr"
)

type Outcome string

const (
	OutcomePending          Outcome = "pending"
	OutcomeCommitted        Outcome = "committed"
	OutcomeAlreadySatisfied Outcome = "already_satisfied"
	OutcomeStaleSource      Outcome = "stale_source"
	OutcomeConflict         Outcome = "conflict"
)

// Operation contains only server-attested routing data. Request payloads must
// never be decoded directly into this type.
type Operation struct {
	ID              string
	TaskID          string
	WorkspaceID     string
	Producer        Producer
	ExpectedStepID  string
	ObservedStepID  string
	TargetStepID    string
	SessionID       string
	TurnID          string
	ActorKind       string
	ActorID         string
	ExternalCause   string
	ExternalCauseID string
	Outcome         Outcome
	SupersedesID    string
	TransitionID    int64
	EffectID        string
}

// Effect is the durable destination-entry allocation owned by a committed
// route operation. Engine workers retain their existing workflow-step entry
// claims; this record makes the route-to-entry correlation and status readable.
type Effect struct {
	ID           string
	OperationID  string
	TaskID       string
	TransitionID int64
	TargetStepID string
	Status       string
	ClaimToken   string
}

const (
	EffectPending   = "pending"
	EffectClaimed   = "claimed"
	EffectCompleted = "completed"
)

type operationContextKey struct{}

func WithOperation(ctx context.Context, operation Operation) context.Context {
	return context.WithValue(ctx, operationContextKey{}, operation)
}

func FromContext(ctx context.Context) (Operation, bool) {
	operation, ok := ctx.Value(operationContextKey{}).(Operation)
	return operation, ok && operation.ID != ""
}
