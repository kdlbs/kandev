package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// TestPREventsRoundTripThroughYAML pins that on_pr_merged and on_pr_closed
// survive export, YAML encoding, decoding, and import with their move_to_step
// targets remapped by position.
func TestPREventsRoundTripThroughYAML(t *testing.T) {
	steps := []*WorkflowStep{
		{ID: "orig-exec", Name: "Execution", Position: 0, Color: "blue"},
		{
			ID: "orig-accept", Name: "Acceptance", Position: 1, Color: "yellow",
			Events: StepEvents{
				OnPRMerged: []GenericAction{
					{Type: GenericActionMoveToStep, Config: map[string]any{"step_id": "orig-done"}},
				},
				OnPRClosed: []GenericAction{
					{Type: GenericActionMoveToStep, Config: map[string]any{"step_id": "orig-exec"}},
				},
			},
		},
		{ID: "orig-done", Name: "Done", Position: 2, Color: "green"},
	}
	wf := &taskmodels.Workflow{ID: "wf-1", Name: "Pipeline"}
	export := BuildWorkflowExport([]*taskmodels.Workflow{wf}, map[string][]*WorkflowStep{"wf-1": steps}, nil)
	require.NoError(t, export.Validate())

	encoded, err := yaml.Marshal(export)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), "on_pr_merged:")
	assert.Contains(t, string(encoded), "on_pr_closed:")

	var decoded WorkflowExport
	require.NoError(t, yaml.Unmarshal(encoded, &decoded))
	require.NoError(t, decoded.Validate())

	var accept *StepPortable
	for i := range decoded.Workflows[0].Steps {
		if decoded.Workflows[0].Steps[i].Name == "Acceptance" {
			accept = &decoded.Workflows[0].Steps[i]
		}
	}
	require.NotNil(t, accept)

	imported := ConvertPositionToStepID(accept.Events, map[int]string{0: "new-exec", 1: "new-accept", 2: "new-done"})
	require.Len(t, imported.OnPRMerged, 1)
	assert.Equal(t, GenericActionMoveToStep, imported.OnPRMerged[0].Type)
	assert.Equal(t, "new-done", imported.OnPRMerged[0].Config["step_id"])
	require.Len(t, imported.OnPRClosed, 1)
	assert.Equal(t, "new-exec", imported.OnPRClosed[0].Config["step_id"])
}

func TestStepEventReferencesIncludePREvents(t *testing.T) {
	refs := CollectStepEventReferences(StepEvents{
		OnPRMerged: []GenericAction{{Type: GenericActionMoveToStep, Config: map[string]any{"step_id": "done"}}},
		OnPRClosed: []GenericAction{{Type: GenericActionMoveToStep, Config: map[string]any{"step_id": "exec"}}},
	})
	assert.ElementsMatch(t, []string{"done", "exec"}, refs.StepIDs)

	remapped := RemapStepEvents(StepEvents{
		OnPRMerged: []GenericAction{{Type: GenericActionMoveToStep, Config: map[string]any{"step_id": "done"}}},
		OnPRClosed: []GenericAction{{Type: GenericActionMoveToStep, Config: map[string]any{"step_id": "exec"}}},
	}, map[string]string{"done": "done-copy", "exec": "exec-copy"})
	assert.Equal(t, "done-copy", remapped.OnPRMerged[0].Config["step_id"])
	assert.Equal(t, "exec-copy", remapped.OnPRClosed[0].Config["step_id"])
}
