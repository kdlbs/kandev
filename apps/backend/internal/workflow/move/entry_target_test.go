package move

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

// AC-TASKS-WORKFLOW-STAGE-MOVES-004.1, 004.2
func TestValidateEntryTargetRejectsOnlyManualSteps(t *testing.T) {
	onEnter := func(kind models.OnEnterActionType) *models.WorkflowStep {
		return &models.WorkflowStep{Events: models.StepEvents{OnEnter: []models.OnEnterAction{{Type: kind}}}}
	}
	cases := map[string]struct {
		step   *models.WorkflowStep
		manual bool
	}{
		"unknown step":                   {step: nil},
		"no automation":                  {step: &models.WorkflowStep{}, manual: true},
		"non-starting on_enter action":   {step: onEnter(models.OnEnterResetAgentContext), manual: true},
		"auto_start_agent":               {step: onEnter(models.OnEnterAutoStartAgent)},
		"queue_run":                      {step: onEnter(models.OnEnterQueueRun)},
		"queue_run_for_each_participant": {step: onEnter(models.OnEnterQueueRunForEachParticipant)},
		"run_code_review":                {step: onEnter(models.OnEnterRunCodeReview)},
		"pull source":                    {step: &models.WorkflowStep{PullFromStepID: "upstream-step"}},
	}
	for name, tc := range cases {
		err := ValidateEntryTarget(tc.step)
		if tc.manual != errors.Is(err, ErrEntryTargetIsManual) {
			t.Errorf("%s: ValidateEntryTarget() = %v, want manual=%t", name, err, tc.manual)
		}
	}
}
