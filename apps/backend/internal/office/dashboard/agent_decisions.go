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
		return nil, fmt.Errorf("task %s has no workflow step bound", in.TaskID)
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
		return nil, fmt.Errorf("invalid decision: %q", in.Decision)
	}
	// AC-55(4)/AC-6.
	if strings.TrimSpace(in.Reason) == "" {
		return nil, fmt.Errorf("%s", agentDecisionReasonRequiredErr)
	}

	result, err := dispatcher.RecordDecision(ctx, officeenginedispatcher.RecordDecisionInput{
		TaskID:        in.TaskID,
		StepID:        stepID,
		ParticipantID: participantID,
		Decision:      in.Decision,
		DeciderType:   models.DeciderTypeAgent,
		DeciderID:     in.AgentProfileID,
		Role:          role,
		Comment:       in.Reason,
	})
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
// successful write. AC-64/F36: only attempts the AC-57d snapshot when this
// decision's own re-evaluation did NOT already apply a transition. When
// result.Transitioned is true, the task has necessarily left the validated
// step by the time any snapshot read would run, so comparing "current step
// != validated step" would spuriously read as the AC-37 race and wrongly
// downgrade a genuine transition to guards:[]/transition_applied:false.
// Scoping the comparison to the branch where Transitioned is already false
// avoids that self-contradiction — transition_applied is already false
// there, so the AC-15/AC-37 fallback (guards:[], transition_applied:false)
// is a true no-op, never an override of a real transition.
func (s *DashboardService) agentDecisionGuardsSnapshot(
	ctx context.Context, taskID string, result officeenginedispatcher.RecordDecisionResult,
) []GuardStateDTO {
	guards := []GuardStateDTO{}
	if result.Transitioned {
		return guards
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
