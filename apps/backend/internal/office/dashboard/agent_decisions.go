package dashboard

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	officeenginedispatcher "github.com/kandev/kandev/internal/office/engine_dispatcher"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/workflow/engine"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

const agentDecisionReasonRequiredErr = "reason is required"

// RecordAgentDecisionInput bundles the agent decision tool's caller-supplied
// fields. Per the tool contract, task/step/role are never caller-supplied —
// they are resolved server-side from the calling session and the AC-50
// slate.
type RecordAgentDecisionInput struct {
	TaskID         string
	AgentProfileID string
	Decision       string
	Reason         string
	// SessionID, when features.officeSessionIdentity is on, names the
	// decider's own calling session so RecordDecision re-evaluates against
	// it instead of the task's most-recently-started ("active") session.
	// Populated unconditionally by the MCP handler; gated here because the
	// flag decision belongs with the rest of this service's behavior, not
	// the transport layer.
	SessionID string
}

// AgentDecisionValidationError marks an error caused by a caller-provided
// decision field or a task precondition that the caller can correct. The MCP
// transport uses this type to keep unexpected repository and engine failures
// opaque to agents.
type AgentDecisionValidationError struct {
	Err error
}

func (e *AgentDecisionValidationError) Error() string {
	if e == nil || e.Err == nil {
		return "invalid agent decision"
	}
	return e.Err.Error()
}

func (e *AgentDecisionValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsAgentDecisionValidationError reports whether err is safe to expose as a
// validation response at the MCP boundary.
func IsAgentDecisionValidationError(err error) bool {
	var target *AgentDecisionValidationError
	return errors.As(err, &target)
}

func agentDecisionValidation(err error) error {
	return &AgentDecisionValidationError{Err: err}
}

// RecordAgentDecisionResult is the AC-64 tool-contract return shape.
type RecordAgentDecisionResult struct {
	Decision          string
	Role              string
	StepID            string
	DecisionID        string
	DecidedAt         time.Time
	TransitionApplied bool
	Guards            []GuardStateDTO
}

// roleResolvingDispatcher is the AC-2/3/4/4a additive capability this
// function needs from s.engineDispatcher. Named locally and reached via a
// type assertion rather than widening shared.WorkflowEngineDispatcher,
// mirroring decisionRecordingDispatcher above.
type roleResolvingDispatcher interface {
	ResolveParticipantRole(ctx context.Context, taskID, stepID, agentProfileID string) (role, participantID string, err error)
}

// quorumEvaluatingDispatcher is the AC-57d additive capability used to
// build the AC-64 guards field after a successful write.
type quorumEvaluatingDispatcher interface {
	EvaluateStepQuorum(ctx context.Context, taskID string) (engine.QuorumSnapshot, error)
}

// RecordAgentDecision is the agent/MCP counterpart to ApproveTask /
// RequestTaskChanges (`record_step_decision_kandev`). Unlike the human
// path — whose resolveDeciderRole/resolveParticipantID stay unchanged and
// step-scoped per AC-57b — this path resolves role and seat over the
// AC-50 population via the engine's ResolveParticipantRole (AC-4a): a new
// capability, not a rewiring of the human path's resolution.
//
// Validates preconditions in the AC-55 order: (1) bound workflow_step_id
// (AC-7), (2) participation (AC-3), (3) verdict validity (AC-5), (4)
// non-empty reason (AC-6).
func (s *DashboardService) RecordAgentDecision(
	ctx context.Context, in RecordAgentDecisionInput,
) (*RecordAgentDecisionResult, error) {
	if s.decisions == nil {
		return nil, fmt.Errorf("%s", decisionStoreNotWiredErr)
	}
	dispatcher, ok := s.engineDispatcher.(decisionRecordingDispatcher)
	if !ok {
		return nil, fmt.Errorf("%s", decisionStoreNotWiredErr)
	}
	roleDispatcher, ok := s.engineDispatcher.(roleResolvingDispatcher)
	if !ok {
		return nil, fmt.Errorf("%s", decisionStoreNotWiredErr)
	}

	// AC-55(1)/AC-7.
	stepID, err := s.repo.GetTaskWorkflowStepID(ctx, in.TaskID)
	if err != nil {
		return nil, fmt.Errorf("resolve task workflow_step_id: %w", err)
	}
	if stepID == "" {
		return nil, agentDecisionValidation(fmt.Errorf("task %s has no workflow step bound", in.TaskID))
	}

	// AC-55(2)/AC-3/AC-4/AC-4a.
	role, participantID, err := roleDispatcher.ResolveParticipantRole(ctx, in.TaskID, stepID, in.AgentProfileID)
	if err != nil {
		if errors.Is(err, engine.ErrParticipantNotFound) {
			return nil, shared.ErrForbidden
		}
		return nil, fmt.Errorf("resolve participant role: %w", err)
	}

	// AC-55(3)/AC-5.
	if in.Decision != engine.DecisionApproved && in.Decision != engine.DecisionRejected {
		return nil, agentDecisionValidation(fmt.Errorf("invalid decision: %q", in.Decision))
	}
	// AC-55(4)/AC-6.
	if strings.TrimSpace(in.Reason) == "" {
		return nil, agentDecisionValidation(fmt.Errorf("%s", agentDecisionReasonRequiredErr))
	}

	decisionInput := officeenginedispatcher.RecordDecisionInput{
		TaskID:        in.TaskID,
		StepID:        stepID,
		ParticipantID: participantID,
		Decision:      in.Decision,
		DeciderType:   models.DeciderTypeAgent,
		DeciderID:     in.AgentProfileID,
		Role:          role,
		Comment:       in.Reason,
	}
	if s.officeSessionIdentity {
		decisionInput.SessionID = in.SessionID
	}
	result, err := dispatcher.RecordDecision(ctx, decisionInput)
	if err != nil {
		return nil, fmt.Errorf("record decision: %w", err)
	}

	rec := fromWorkflowDecision(&workflowmodels.WorkflowStepDecision{
		ID:            result.DecisionID,
		TaskID:        in.TaskID,
		StepID:        result.StepID,
		ParticipantID: participantID,
		Decision:      in.Decision,
		DecidedAt:     result.DecidedAt,
		DeciderType:   models.DeciderTypeAgent,
		DeciderID:     in.AgentProfileID,
		Role:          role,
		Comment:       in.Reason,
	})
	s.publishDecisionRecorded(ctx, rec)
	s.logDecisionActivity(ctx, rec)
	s.runReactivityForDecision(ctx, rec)

	out := &RecordAgentDecisionResult{
		Decision:          in.Decision,
		Role:              role,
		StepID:            result.StepID,
		DecisionID:        result.DecisionID,
		DecidedAt:         result.DecidedAt,
		TransitionApplied: result.Transitioned,
		Guards:            s.agentDecisionGuardsSnapshot(ctx, in.TaskID, result),
	}
	return out, nil
}

// agentDecisionGuardsSnapshot builds the AC-64 guards field after a
// successful write. A transition result already carries the engine's
// pre-transition snapshot, so it must not issue a second read after the task
// has moved to another step. For non-transition results, the read remains a
// best-effort diagnostic and is discarded if the task moved concurrently.
func (s *DashboardService) agentDecisionGuardsSnapshot(
	ctx context.Context, taskID string, result officeenginedispatcher.RecordDecisionResult,
) []GuardStateDTO {
	guards := []GuardStateDTO{}
	if result.Transitioned {
		return guardStateDTOsFromSnapshot(result.Guards)
	}
	qd, ok := s.engineDispatcher.(quorumEvaluatingDispatcher)
	if !ok {
		return guards
	}
	// AC-15/AC-64: a failed snapshot read or an AC-37 step mismatch reports
	// guards:[]/transition_applied:false (already the zero state here)
	// rather than an error — the write already succeeded and a diagnostic
	// read must not put it at risk.
	snapshot, snapErr := qd.EvaluateStepQuorum(ctx, taskID)
	if snapErr != nil || snapshot.StepID != result.StepID {
		return guards
	}
	return guardStateDTOsFromSnapshot(snapshot.Guards)
}
