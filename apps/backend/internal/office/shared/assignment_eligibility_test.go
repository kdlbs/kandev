package shared

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type assignmentEligibilityTestStepIDs struct {
	stepID string
}

func (f assignmentEligibilityTestStepIDs) GetTaskWorkflowStepID(context.Context, string) (string, error) {
	return f.stepID, nil
}

type assignmentEligibilityTestSteps struct {
	step *wfmodels.WorkflowStep
}

func (f assignmentEligibilityTestSteps) GetStep(context.Context, string) (*wfmodels.WorkflowStep, error) {
	return f.step, nil
}

func TestIsAssignmentWakeEligible_LogsEligibleOutcomeAtInfo(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("observer logger: %v", err)
	}

	step := &wfmodels.WorkflowStep{
		ID: "step-work",
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}
	if !IsAssignmentWakeEligible(
		context.Background(),
		log,
		assignmentEligibilityTestStepIDs{stepID: step.ID},
		assignmentEligibilityTestSteps{step: step},
		"task-1",
		"test.source",
	) {
		t.Fatal("eligible step was rejected")
	}

	entries := logs.FilterMessage("office.assignment_wake.step_eligible").All()
	if len(entries) != 1 {
		t.Fatalf("eligible outcome logs = %d, want 1: %+v", len(entries), logs.All())
	}
	if entries[0].Level != zapcore.InfoLevel {
		t.Fatalf("eligible outcome level = %v, want info", entries[0].Level)
	}
	fields := entries[0].ContextMap()
	if fields["task_id"] != "task-1" || fields["step_id"] != "step-work" || fields["source"] != "test.source" {
		t.Fatalf("eligible outcome fields = %#v, want task, step, and source", fields)
	}
}

func TestIsAssignmentWakeEligible_LogsUnboundOutcomeAtInfo(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("observer logger: %v", err)
	}

	if !IsAssignmentWakeEligible(
		context.Background(),
		log,
		assignmentEligibilityTestStepIDs{},
		assignmentEligibilityTestSteps{},
		"task-2",
		"test.source",
	) {
		t.Fatal("unbound task was rejected")
	}

	entries := logs.FilterMessage("office.assignment_wake.step_eligible").All()
	if len(entries) != 1 {
		t.Fatalf("unbound outcome logs = %d, want 1: %+v", len(entries), logs.All())
	}
	if entries[0].Level != zapcore.InfoLevel {
		t.Fatalf("unbound outcome level = %v, want info", entries[0].Level)
	}
	fields := entries[0].ContextMap()
	if fields["task_id"] != "task-2" || fields["step_id"] != "" || fields["source"] != "test.source" {
		t.Fatalf("unbound outcome fields = %#v, want task, empty step, and source", fields)
	}
}
