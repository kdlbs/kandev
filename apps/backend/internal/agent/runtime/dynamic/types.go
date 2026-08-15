// Package dynamic owns provider-neutral routing for dynamic agent profiles.
// It deliberately depends only on the normalized runtime error contract, so
// concrete launch adapters and Office do not need to import each other.
package dynamic

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

type Action string

const (
	ActionRetrySame Action = "retry_same"
	ActionTryNext   Action = "try_next"
	ActionStop      Action = "stop"
)

var (
	ErrStaleGeneration     = errors.New("dynamic route generation is stale")
	ErrNoEligibleCandidate = errors.New("dynamic profile has no eligible candidate")
	ErrRouteStateNotFound  = errors.New("dynamic route state not found")
)

type Candidate struct {
	ID         string
	Enabled    bool
	BindingKey string
	Rules      map[string]Action
}

type Profile struct {
	ID         string
	Version    int64
	Candidates []Candidate
}

type RouteState struct {
	SessionID          string
	LogicalProfileID   string
	ExecutionProfileID string
	Generation         int64
	ProfileVersion     int64
	Status             string
	ContinuationJSON   string
	UpdatedAt          time.Time
}

type RouteDecision struct {
	SessionID          string
	LogicalProfileID   string
	ExecutionProfileID string
	Generation         int64
	ProfileVersion     int64
	Reason             string
}

type RouteAttempt struct {
	SessionID          string
	LogicalProfileID   string
	ExecutionProfileID string
	Generation         int64
	ProfileVersion     int64
	Reason             string
	CreatedAt          time.Time
}

// ContinuationRecord is the bounded handoff package persisted for a route
// generation before a successor launch. It contains context, not provider
// native session state.
type ContinuationRecord struct {
	SessionID    string
	Generation   int64
	Continuation Continuation
	UpdatedAt    time.Time
}

// ContinuationPersistence stores the handoff package with the route
// generation that owns it. Implementations must reject a stale generation.
type ContinuationPersistence interface {
	SaveRouteContinuation(context.Context, ContinuationRecord) error
}

// Persistence is the narrow durable seam for route state and immutable
// attempts. The task repository implements it without importing the routing
// engine's selection logic.
type Persistence interface {
	SaveRouteState(context.Context, RouteState) error
	AppendRouteAttempt(context.Context, RouteAttempt) error
}

// StateLoader is an optional restart-recovery seam. Implementations return
// (nil, nil) when no route state exists for the session.
type StateLoader interface {
	LoadRouteState(context.Context, string) (*RouteState, error)
}

// GenerationClaimer is the durable compare-and-swap seam. A false result
// means another worker already advanced the session generation.
type GenerationClaimer interface {
	ClaimRouteState(context.Context, int64, RouteState) (bool, error)
}

// DecisionRecorder lets a repository commit the state row and immutable
// attempt row in one transaction. Persistence remains backwards compatible
// for callers that only need the narrow Save/Append contract.
type DecisionRecorder interface {
	RecordRouteDecision(context.Context, RouteDecision, RouteState) error
}

type NoEligibleCandidateError struct {
	SessionID      string
	LogicalProfile string
	Generation     int64
}

func (e *NoEligibleCandidateError) Error() string {
	return fmt.Sprintf("%s: session=%s profile=%s generation=%d", ErrNoEligibleCandidate, e.SessionID, e.LogicalProfile, e.Generation)
}

func (e *NoEligibleCandidateError) Unwrap() error { return ErrNoEligibleCandidate }

func actionForRules(rules map[string]Action, code routingerr.Code) Action {
	if action, ok := rules[string(code)]; ok {
		return action
	}
	if action, ok := rules["on_provider_error"]; ok {
		return action
	}
	return ActionStop
}
