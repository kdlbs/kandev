package office

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/dashboard"
	officeruntime "github.com/kandev/kandev/internal/office/runtime"
)

type fakeDashboardDecisionRecorder struct {
	input  dashboard.RecordAgentDecisionInput
	result *dashboard.RecordAgentDecisionResult
	err    error
}

func (f *fakeDashboardDecisionRecorder) RecordAgentDecision(
	_ context.Context,
	input dashboard.RecordAgentDecisionInput,
) (*dashboard.RecordAgentDecisionResult, error) {
	f.input = input
	return f.result, f.err
}

func TestRuntimeDecisionRecorderForwardsIdentityAndMapsResult(t *testing.T) {
	decidedAt := time.Date(2026, 9, 7, 20, 0, 0, 0, time.UTC)
	fake := &fakeDashboardDecisionRecorder{result: &dashboard.RecordAgentDecisionResult{
		Decision:          "approved",
		Role:              "approver",
		StepID:            "step-1",
		DecisionID:        "decision-1",
		DecidedAt:         decidedAt,
		TransitionApplied: true,
		Guards: []dashboard.GuardStateDTO{{
			TargetStepID:  "step-2",
			Role:          "approver",
			Threshold:     "all_approve",
			RequiredCount: 1,
			ReceivedCount: 1,
			Satisfied:     true,
		}},
	}}
	recorder := newRuntimeDecisionRecorder(fake)

	got, err := recorder.RecordAgentDecision(context.Background(), officeruntime.RecordAgentDecisionInput{
		TaskID:         "task-1",
		AgentProfileID: "agent-1",
		SessionID:      "session-1",
		Decision:       "approved",
		Reason:         "ready",
	})
	if err != nil {
		t.Fatalf("RecordAgentDecision: %v", err)
	}
	if fake.input.TaskID != "task-1" || fake.input.AgentProfileID != "agent-1" || fake.input.SessionID != "session-1" ||
		fake.input.Decision != "approved" || fake.input.Reason != "ready" {
		t.Fatalf("dashboard input = %+v", fake.input)
	}
	if got.Decision != "approved" || got.Role != "approver" || got.StepID != "step-1" ||
		got.DecisionID != "decision-1" || !got.DecidedAt.Equal(decidedAt) || !got.TransitionApplied || len(got.Guards) != 1 {
		t.Fatalf("runtime result = %+v", got)
	}
	if got.Guards[0].TargetStepID != "step-2" || got.Guards[0].RequiredCount != 1 || !got.Guards[0].Satisfied {
		t.Fatalf("runtime guard = %+v", got.Guards[0])
	}
}

func TestRuntimeDecisionRecorderClassifiesDashboardValidation(t *testing.T) {
	fake := &fakeDashboardDecisionRecorder{err: &dashboard.AgentDecisionValidationError{Err: errors.New("reason is required")}}
	recorder := newRuntimeDecisionRecorder(fake)

	_, err := recorder.RecordAgentDecision(context.Background(), officeruntime.RecordAgentDecisionInput{})
	var validation *officeruntime.DecisionValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want runtime validation error", err)
	}
}
