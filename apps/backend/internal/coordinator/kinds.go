package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"time"
)

// KindExecutor is one non-create_task proposal kind: how a propose is
// validated, how an approve edit is validated, and what approval does.
// create_task keeps the phase-1 approve path and is not in the registry.
type KindExecutor interface {
	Kind() string
	Action() Action
	// ValidatePropose checks the target and the stored spec, returning the
	// spec JSON to store.
	ValidatePropose(ctx context.Context, c *Coordinator, spec json.RawMessage) (json.RawMessage, error)
	// ValidateEdits merges approve edits onto base, returning the frozen spec.
	ValidateEdits(base, edits json.RawMessage) (json.RawMessage, error)
	// Execute performs the side effect for a claimed proposal.
	Execute(ctx context.Context, claim Claim) (Outcome, error)
	// ReRunsOnStaleClaim reports whether Execute is safe to run again on a
	// stale claim. The non-create kinds are not.
	ReRunsOnStaleClaim() bool
}

// Claim is what Execute receives: the claimed proposal's identity and its
// frozen spec.
type Claim struct {
	ProposalID   string
	Token        string
	WorkspaceID  string
	Coordinator  *Coordinator
	TargetTaskID string
	Spec         json.RawMessage
	StartsAgent  bool
	ApprovedBy   string
	TextEdited   bool
}

// Outcome is a successful Execute: the task the change applied to, the
// outcome_json to store (empty keeps what is stored) and the activity detail.
type Outcome struct {
	TaskID      string
	OutcomeJSON string
	Detail      string
	Noop        bool
}

const (
	outcomeUnknownError = "outcome_unknown"
	executeDeadline     = 60 * time.Second
	settleBound         = 5 * time.Second
)

// execFailure is a definite failure: Execute knows the side effect did not
// happen (or, for a refused step, that it was not attempted), so the row
// settles failed with this text even when the deadline has fired.
type execFailure struct{ text string }

func (e *execFailure) Error() string { return e.text }

func failWith(text string) error { return &execFailure{text: text} }

// errSettleFenced reports that the claim was already settled by another path;
// Execute made no further call and the request returns the current row.
var errSettleFenced = errors.New("coordinator: claim already settled")

// Failure texts an Execute settles a row with.
const (
	failTaskArchived    = "task_archived"
	failNotResumable    = "not_resumable"
	failNotAccepting    = "not_accepting"
	failQueueFull       = "queue_full"
	failTaskLeftWF      = "task_left_workflow"
	failStepMissing     = "step_missing"
	failStepIsDone      = "step_is_done"
	failStepStartsAgent = "step_starts_agent"
	failAgentRunning    = "agent_running"
	failStepFull        = "step_full"
	failMoved           = "moved"
)

// kindExecutor returns the executor of a non-create_task kind, or nil.
func (s *Service) kindExecutor(kind string) KindExecutor {
	return s.kinds[kind]
}

// KindExecutors lists the registered executors in a stable order.
func (s *Service) KindExecutors() []KindExecutor {
	out := make([]KindExecutor, 0, len(s.kinds))
	for _, k := range []string{ProposalKindResume, ProposalKindMessage, ProposalKindMove, ProposalKindImprovement} {
		if e, ok := s.kinds[k]; ok {
			out = append(out, e)
		}
	}
	return out
}

func (s *Service) registerKinds() {
	s.kinds = map[string]KindExecutor{
		ProposalKindResume:      &resumeKind{svc: s},
		ProposalKindMessage:     &messageKind{svc: s},
		ProposalKindMove:        &moveKind{svc: s},
		ProposalKindImprovement: &improvementKind{svc: s},
	}
}

// Field names the kind validators report in a *FieldError.
const (
	fieldText             = "text"
	fieldBody             = "body"
	fieldStandingOrderIDs = "standing_order_ids"
	fieldTaskID           = "task_id"
	fieldRationale        = "rationale"
	fieldStepID           = "step_id"
)

// checkExecuteTarget refuses a target that no longer belongs where the proposal
// was validated: another workspace, a coordinator conversation, or a task other
// than the one the stored spec names.
func checkExecuteTarget(target *TargetTask, claim Claim, specTaskID string) error {
	if target.WorkspaceID != claim.WorkspaceID || target.Origin == string(taskmodels.TaskOriginCoordinator) ||
		specTaskID != claim.TargetTaskID {
		return failWith(failTaskArchived)
	}
	return nil
}
