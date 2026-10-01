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
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
)

type Action string

const (
	ActionRetrySame Action = "retry_same"
	ActionTryNext   Action = "try_next"
	ActionStop      Action = "stop"
)

var (
	ErrStaleGeneration                        = errors.New("dynamic route generation is stale")
	ErrNoEligibleCandidate                    = errors.New("dynamic profile has no eligible candidate")
	ErrRouteStateNotFound                     = errors.New("dynamic route state not found")
	ErrRecoveryPending                        = errors.New("dynamic route recovery is pending")
	ErrRecoveryNotDue                         = errors.New("dynamic route recovery is not due")
	ErrUnclassifiedWorkflowContextChanged     = errors.New("workflow context changed during unclassified fallback")
	ErrUnclassifiedWorkflowContextUnavailable = errors.New("workflow context unavailable during unclassified fallback")
	// ErrStatusClaimUnsupported means the persistence implementation cannot
	// fence a same-generation status transition. Status-fenced claims must
	// fail closed rather than silently degrade to a generation-only claim,
	// which would let two concurrent callers both win the same transition.
	ErrStatusClaimUnsupported = errors.New("dynamic route persistence does not support status-fenced claims")
)

type Candidate struct {
	ID         string
	Enabled    bool
	BindingKey string
	Rules      map[string]Action
	Policies   routingpolicy.Document
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
	PolicyStateJSON    string
	UpdatedAt          time.Time
}

// PolicyState is the durable snapshot of one evaluated provider failure. It
// is stored as JSON so older installations can add fields without changing
// candidate or session identity columns.
type PolicyState struct {
	FailureCode      routingerr.Code           `json:"failure_code"`
	FailureClass     routingerr.Class          `json:"failure_class"`
	CatalogueVersion string                    `json:"catalogue_version"`
	PolicyJSON       string                    `json:"policy_json"`
	RetryOrdinal     int64                     `json:"retry_ordinal"`
	ResetWaitUsed    bool                      `json:"reset_wait_used"`
	ResetWaitClasses map[routingerr.Class]bool `json:"reset_wait_classes,omitempty"`
	Deadline         *time.Time                `json:"deadline,omitempty"`
	PendingOutcome   routingpolicy.Outcome     `json:"pending_outcome"`
	Unclassified     *UnclassifiedStreak       `json:"unclassified_streak,omitempty"`
}

// UnclassifiedStreak is the bounded durable identity for one sequence of
// matching, effect-safe failures. It intentionally stores only a diagnostic
// fingerprint, never provider-controlled diagnostic text.
type UnclassifiedStreak struct {
	Version             int                       `json:"version"`
	LogicalProfileID    string                    `json:"logical_profile_id"`
	ExecutionProfileID  string                    `json:"execution_profile_id"`
	ProfileVersion      int64                     `json:"profile_version"`
	StepID              string                    `json:"step_id,omitempty"`
	StepUpdatedAt       time.Time                 `json:"step_updated_at,omitempty"`
	Fingerprint         string                    `json:"fingerprint"`
	Origin              UnclassifiedFailureOrigin `json:"origin"`
	Phase               routingerr.Phase          `json:"phase"`
	Count               int64                     `json:"count"`
	LastAttemptID       string                    `json:"last_attempt_id"`
	LastRouteGeneration int64                     `json:"last_route_generation"`
}

func (s *UnclassifiedStreak) valid() bool {
	return s != nil && s.Version == 1 && s.LogicalProfileID != "" &&
		s.ExecutionProfileID != "" && s.ProfileVersion >= 0 && s.Fingerprint != "" &&
		s.Origin != "" && s.Phase != "" && s.Count >= 1 && s.Count <= 10 &&
		s.LastAttemptID != "" && s.LastRouteGeneration > 0
}

type RouteDecision struct {
	SessionID          string
	LogicalProfileID   string
	ExecutionProfileID string
	Generation         int64
	ProfileVersion     int64
	Reason             string
	Status             string
	Deadline           *time.Time
	ErrorCode          routingerr.Code
	ErrorClass         routingerr.Class
	CatalogueVersion   string
	RetryOrdinal       int64
	PendingOutcome     routingpolicy.Outcome
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

// UnclassifiedFailureOrigin names one trusted producer boundary that can
// supply complete evidence for the narrow repeated-failure policy.
type UnclassifiedFailureOrigin string

const (
	UnclassifiedOriginTerminalProvider UnclassifiedFailureOrigin = "terminal_provider_result"
	UnclassifiedOriginAgentStartup     UnclassifiedFailureOrigin = "agent_startup"
)

// UnclassifiedFailureEvidence is the typed, default-deny context for the
// unclassified fallback exception. Callers must populate it from current
// task and runtime evidence; a zero value never authorizes fallback.
type UnclassifiedFailureEvidence struct {
	TaskScope          bool
	TaskID             string
	WorkflowID         string
	SessionID          string
	LogicalProfileID   string
	ExecutionProfileID string
	RouteGeneration    int64
	StepID             string
	StepUpdatedAt      time.Time
	StepKnown          bool
	StepVeto           bool
	AttemptID          string
	Origin             UnclassifiedFailureOrigin
	Phase              routingerr.Phase
	ProviderID         string
	DiagnosticText     string
	DiagnosticComplete bool
	CurrentAttempt     bool
	EvidenceKnown      bool
	OutputObserved     bool
	EffectObserved     bool
}

func (e UnclassifiedFailureEvidence) currentFor(
	sessionID string,
	profile Profile,
	candidateID string,
	generation int64,
) bool {
	return e.TaskScope && e.TaskID != "" && e.CurrentAttempt && e.SessionID == sessionID &&
		e.LogicalProfileID == profile.ID && e.ExecutionProfileID == candidateID &&
		e.RouteGeneration == generation && e.AttemptID != ""
}

func (e UnclassifiedFailureEvidence) permits(failure *routingerr.Error) bool {
	if failure == nil || !e.safeBeforeResult() || !e.trustedFailureShape(failure) {
		return false
	}
	return e.failureMatchesOrigin(failure)
}

func (e UnclassifiedFailureEvidence) safeBeforeResult() bool {
	return e.TaskScope && e.TaskID != "" && e.StepKnown && !e.StepVeto &&
		e.EvidenceKnown && !e.OutputObserved && !e.EffectObserved && e.DiagnosticComplete
}

func (e UnclassifiedFailureEvidence) trustedFailureShape(failure *routingerr.Error) bool {
	if failure.UserAction {
		return false
	}
	if failure.Class != "" && failure.Class != routingerr.ClassUnclassified {
		return false
	}
	if routingerr.ClassForCode(failure.Code) != routingerr.ClassUnclassified ||
		failure.Phase != e.Phase || e.ProviderID == "" {
		return false
	}
	return true
}

func (e UnclassifiedFailureEvidence) failureMatchesOrigin(failure *routingerr.Error) bool {
	switch e.Origin {
	case UnclassifiedOriginTerminalProvider:
		return failure.Code == routingerr.CodeUnknownProvider && e.Phase == routingerr.PhasePromptSend
	case UnclassifiedOriginAgentStartup:
		return failure.Code == routingerr.CodeAgentRuntime &&
			(e.Phase == routingerr.PhaseProcessStart || e.Phase == routingerr.PhaseSessionInit)
	default:
		return false
	}
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

// GenerationStatusClaimer updates one generation only while its status still
// matches the caller's observation. This closes the same-generation race
// between a launch becoming active and a concurrent recovery transition, and
// between two concurrent callers observing the same due retry/wait state.
type GenerationStatusClaimer interface {
	ClaimRouteStateFrom(context.Context, int64, string, RouteState) (bool, error)
}

// RouteStateSnapshotClaimer fences a same-generation update to the exact
// policy snapshot that was observed. This is required for idempotent streak
// increments when status remains action_required across failures.
type RouteStateSnapshotClaimer interface {
	ClaimRouteStateFromSnapshot(context.Context, int64, string, string, RouteState) (bool, error)
}

// DecisionRecorder lets a repository commit the state row and immutable
// attempt row in one transaction. Persistence remains backwards compatible
// for callers that only need the narrow Save/Append contract.
type DecisionRecorder interface {
	RecordRouteDecision(context.Context, RouteDecision, RouteState) error
}

// UnclassifiedRouteDecisionRecorder commits an automatically selected
// successor only while the task's workflow step identity, revision, and veto
// still match the evidence used to count the failure.
type UnclassifiedRouteDecisionRecorder interface {
	RecordUnclassifiedRouteDecision(
		context.Context, RouteDecision, RouteState, UnclassifiedFailureEvidence,
	) error
}

// UnclassifiedFallbackLaunchClaimer fences the final automatic launch against
// workflow-step mutations after the route decision has been persisted.
type UnclassifiedFallbackLaunchClaimer interface {
	ClaimUnclassifiedFallbackLaunch(
		context.Context, RouteDecision, UnclassifiedFailureEvidence,
	) error
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
