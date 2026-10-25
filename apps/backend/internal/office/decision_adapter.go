package office

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/office/dashboard"
	officeruntime "github.com/kandev/kandev/internal/office/runtime"
)

type dashboardDecisionRecorder interface {
	RecordAgentDecision(ctx context.Context, input dashboard.RecordAgentDecisionInput) (*dashboard.RecordAgentDecisionResult, error)
}

type runtimeDecisionRecorder struct {
	dashboard dashboardDecisionRecorder
}

func newRuntimeDecisionRecorder(svc dashboardDecisionRecorder) officeruntime.DecisionRecorder {
	if svc == nil {
		return nil
	}
	return &runtimeDecisionRecorder{dashboard: svc}
}

func (r *runtimeDecisionRecorder) RecordAgentDecision(
	ctx context.Context,
	input officeruntime.RecordAgentDecisionInput,
) (officeruntime.RecordAgentDecisionResult, error) {
	result, err := r.dashboard.RecordAgentDecision(ctx, dashboard.RecordAgentDecisionInput{
		TaskID:         input.TaskID,
		AgentProfileID: input.AgentProfileID,
		SessionID:      input.SessionID,
		Decision:       input.Decision,
		Reason:         input.Reason,
	})
	if err != nil {
		if dashboard.IsAgentDecisionValidationError(err) {
			return officeruntime.RecordAgentDecisionResult{}, officeruntime.NewDecisionValidationError(err)
		}
		return officeruntime.RecordAgentDecisionResult{}, err
	}
	if result == nil {
		return officeruntime.RecordAgentDecisionResult{}, fmt.Errorf("record agent decision returned no result")
	}
	guards := make([]officeruntime.DecisionGuard, 0, len(result.Guards))
	for _, guard := range result.Guards {
		guards = append(guards, officeruntime.DecisionGuard{
			TargetStepID:  guard.TargetStepID,
			Role:          guard.Role,
			Threshold:     guard.Threshold,
			RequiredCount: guard.RequiredCount,
			ReceivedCount: guard.ReceivedCount,
			Satisfied:     guard.Satisfied,
			Reason:        guard.Reason,
			Error:         guard.Error,
		})
	}
	return officeruntime.RecordAgentDecisionResult{
		Decision:          result.Decision,
		Role:              result.Role,
		StepID:            result.StepID,
		DecisionID:        result.DecisionID,
		DecidedAt:         result.DecidedAt,
		TransitionApplied: result.TransitionApplied,
		Guards:            guards,
	}, nil
}
