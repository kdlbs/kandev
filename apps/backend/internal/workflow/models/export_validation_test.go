package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func validExportForAdditionalValidation() *WorkflowExport {
	return &WorkflowExport{
		Version: ExportVersion,
		Type:    ExportType,
		Workflows: []WorkflowPortable{{
			Name: "Test",
			Steps: []StepPortable{
				{Name: "Todo", Position: 0, Color: "blue"},
				{Name: "Done", Position: 1, Color: "green"},
			},
		}},
	}
}

func TestValidateRejectsFractionalGenericStepPosition(t *testing.T) {
	export := validExportForAdditionalValidation()
	export.Workflows[0].Steps[0].Events = StepEvents{
		OnChildrenCompleted: []GenericAction{{
			Type:   GenericActionMoveToStep,
			Config: map[string]any{"step_position": float64(1.5)},
		}},
	}

	assert.ErrorContains(t, export.Validate(), "unexpected type")
}

func TestValidateRejectsInvalidGenericStepPositions(t *testing.T) {
	triggers := []struct {
		name string
		set  func(*StepEvents)
	}{
		{
			name: "on_comment",
			set: func(events *StepEvents) {
				events.OnComment = []GenericAction{{Type: GenericActionMoveToStep, Config: map[string]any{"step_position": 99}}}
			},
		},
		{
			name: "on_blocker_resolved",
			set: func(events *StepEvents) {
				events.OnBlockerResolved = []GenericAction{{Type: GenericActionMoveToStep, Config: map[string]any{"step_position": 99}}}
			},
		},
		{
			name: "on_approval_resolved",
			set: func(events *StepEvents) {
				events.OnApprovalResolved = []GenericAction{{Type: GenericActionMoveToStep, Config: map[string]any{"step_position": 99}}}
			},
		},
		{
			name: "on_pr_merged",
			set: func(events *StepEvents) {
				events.OnPRMerged = []GenericAction{{Type: GenericActionMoveToStep, Config: map[string]any{"step_position": 99}}}
			},
		},
		{
			name: "on_pr_closed",
			set: func(events *StepEvents) {
				events.OnPRClosed = []GenericAction{{Type: GenericActionMoveToStep, Config: map[string]any{"step_position": 99}}}
			},
		},
	}

	for _, trigger := range triggers {
		t.Run(trigger.name, func(t *testing.T) {
			export := validExportForAdditionalValidation()
			trigger.set(&export.Workflows[0].Steps[0].Events)

			assert.ErrorContains(t, export.Validate(), "does not match any step")
		})
	}
}

func TestValidateStageTypes(t *testing.T) {
	tests := []struct {
		name      string
		stageType StageType
		wantError string
	}{
		{name: "omitted legacy value"},
		{name: "work", stageType: StageTypeWork},
		{name: "review", stageType: StageTypeReview},
		{name: "approval", stageType: StageTypeApproval},
		{name: "custom", stageType: StageTypeCustom},
		{name: "hold", stageType: StageTypeHold},
		{name: "unsupported", stageType: "unknown", wantError: "unsupported stage type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			export := validExportForAdditionalValidation()
			export.Workflows[0].Steps[0].StageType = tt.stageType

			err := export.Validate()
			if tt.wantError == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantError)
		})
	}
}
