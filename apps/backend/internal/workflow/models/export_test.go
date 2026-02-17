package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestBuildWorkflowExport(t *testing.T) {
	t.Run("converts step IDs to positions", func(t *testing.T) {
		wf := &taskmodels.Workflow{ID: "wf-1", Name: "My Workflow", Description: "desc"}
		steps := []*WorkflowStep{
			{ID: "step-a", Name: "Todo", Position: 0, Color: "blue"},
			{
				ID: "step-b", Name: "In Progress", Position: 1, Color: "yellow",
				Events: StepEvents{
					OnTurnComplete: []OnTurnCompleteAction{
						{Type: OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": "step-a"}},
					},
				},
			},
		}
		stepMap := map[string][]*WorkflowStep{"wf-1": steps}

		export := BuildWorkflowExport([]*taskmodels.Workflow{wf}, stepMap)

		require.Equal(t, ExportVersion, export.Version)
		require.Equal(t, ExportType, export.Type)
		require.Len(t, export.Workflows, 1)

		pw := export.Workflows[0]
		assert.Equal(t, "My Workflow", pw.Name)
		assert.Equal(t, "desc", pw.Description)
		require.Len(t, pw.Steps, 2)

		// The move_to_step should reference position 0 (step-a's position), not step-a ID.
		action := pw.Steps[1].Events.OnTurnComplete[0]
		assert.Equal(t, OnTurnCompleteMoveToStep, action.Type)
		assert.Equal(t, 0, action.Config["step_position"])
		assert.Nil(t, action.Config["step_id"])
	})

	t.Run("preserves non-move events", func(t *testing.T) {
		wf := &taskmodels.Workflow{ID: "wf-1", Name: "WF"}
		steps := []*WorkflowStep{
			{
				ID: "step-a", Name: "Step", Position: 0,
				Events: StepEvents{
					OnEnter: []OnEnterAction{{Type: OnEnterAutoStartAgent}},
					OnExit:  []OnExitAction{{Type: OnExitDisablePlanMode}},
					OnTurnStart: []OnTurnStartAction{
						{Type: OnTurnStartMoveToNext},
					},
				},
			},
		}
		export := BuildWorkflowExport([]*taskmodels.Workflow{wf}, map[string][]*WorkflowStep{"wf-1": steps})

		sp := export.Workflows[0].Steps[0]
		assert.Len(t, sp.Events.OnEnter, 1)
		assert.Equal(t, OnEnterAutoStartAgent, sp.Events.OnEnter[0].Type)
		assert.Len(t, sp.Events.OnExit, 1)
		assert.Len(t, sp.Events.OnTurnStart, 1)
		assert.Equal(t, OnTurnStartMoveToNext, sp.Events.OnTurnStart[0].Type)
	})
}

func TestValidate(t *testing.T) {
	validExport := func() *WorkflowExport {
		return &WorkflowExport{
			Version: ExportVersion,
			Type:    ExportType,
			Workflows: []WorkflowPortable{
				{
					Name: "Test",
					Steps: []StepPortable{
						{Name: "Todo", Position: 0, Color: "blue"},
						{Name: "Done", Position: 1, Color: "green"},
					},
				},
			},
		}
	}

	t.Run("valid export passes", func(t *testing.T) {
		assert.NoError(t, validExport().Validate())
	})

	t.Run("wrong version fails", func(t *testing.T) {
		e := validExport()
		e.Version = 99
		err := e.Validate()
		assert.ErrorContains(t, err, "unsupported export version")
	})

	t.Run("wrong type fails", func(t *testing.T) {
		e := validExport()
		e.Type = "wrong"
		err := e.Validate()
		assert.ErrorContains(t, err, "unsupported export type")
	})

	t.Run("empty workflows fails", func(t *testing.T) {
		e := validExport()
		e.Workflows = nil
		assert.ErrorContains(t, e.Validate(), "no workflows")
	})

	t.Run("empty workflow name fails", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Name = ""
		assert.ErrorContains(t, e.Validate(), "name is required")
	})

	t.Run("empty step name fails", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Name = ""
		assert.ErrorContains(t, e.Validate(), "step 0: name is required")
	})

	t.Run("duplicate positions fails", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[1].Position = 0
		assert.ErrorContains(t, e.Validate(), "duplicate step position 0")
	})

	t.Run("valid move_to_step position ref passes", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Events = StepEvents{
			OnTurnComplete: []OnTurnCompleteAction{
				{Type: OnTurnCompleteMoveToStep, Config: map[string]any{"step_position": 1}},
			},
		}
		assert.NoError(t, e.Validate())
	})

	t.Run("invalid move_to_step position ref fails", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Events = StepEvents{
			OnTurnComplete: []OnTurnCompleteAction{
				{Type: OnTurnCompleteMoveToStep, Config: map[string]any{"step_position": 99}},
			},
		}
		assert.ErrorContains(t, e.Validate(), "does not match any step")
	})

	t.Run("missing config on move_to_step fails", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Events = StepEvents{
			OnTurnStart: []OnTurnStartAction{
				{Type: OnTurnStartMoveToStep, Config: nil},
			},
		}
		assert.ErrorContains(t, e.Validate(), "missing config")
	})

	t.Run("missing step_position key fails", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Events = StepEvents{
			OnTurnStart: []OnTurnStartAction{
				{Type: OnTurnStartMoveToStep, Config: map[string]any{"other": "val"}},
			},
		}
		assert.ErrorContains(t, e.Validate(), "missing step_position")
	})

	t.Run("float64 position ref passes (JSON unmarshal)", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Events = StepEvents{
			OnTurnComplete: []OnTurnCompleteAction{
				{Type: OnTurnCompleteMoveToStep, Config: map[string]any{"step_position": float64(1)}},
			},
		}
		assert.NoError(t, e.Validate())
	})

	t.Run("invalid position type fails", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Events = StepEvents{
			OnTurnComplete: []OnTurnCompleteAction{
				{Type: OnTurnCompleteMoveToStep, Config: map[string]any{"step_position": "not-a-number"}},
			},
		}
		assert.ErrorContains(t, e.Validate(), "unexpected type")
	})
}

func TestConvertStepIDToPosition(t *testing.T) {
	idToPos := map[string]int{"step-a": 0, "step-b": 1}

	t.Run("rewrites step_id to step_position in OnTurnStart", func(t *testing.T) {
		events := StepEvents{
			OnTurnStart: []OnTurnStartAction{
				{Type: OnTurnStartMoveToStep, Config: map[string]any{"step_id": "step-b"}},
			},
		}
		result := convertStepIDToPosition(events, idToPos)
		require.Len(t, result.OnTurnStart, 1)
		assert.Equal(t, 1, result.OnTurnStart[0].Config["step_position"])
		assert.Nil(t, result.OnTurnStart[0].Config["step_id"])
	})

	t.Run("rewrites step_id to step_position in OnTurnComplete", func(t *testing.T) {
		events := StepEvents{
			OnTurnComplete: []OnTurnCompleteAction{
				{Type: OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": "step-a"}},
			},
		}
		result := convertStepIDToPosition(events, idToPos)
		require.Len(t, result.OnTurnComplete, 1)
		assert.Equal(t, 0, result.OnTurnComplete[0].Config["step_position"])
		assert.Nil(t, result.OnTurnComplete[0].Config["step_id"])
	})

	t.Run("preserves non-move actions unchanged", func(t *testing.T) {
		events := StepEvents{
			OnTurnStart:    []OnTurnStartAction{{Type: OnTurnStartMoveToNext}},
			OnTurnComplete: []OnTurnCompleteAction{{Type: OnTurnCompleteMoveToNext}},
			OnEnter:        []OnEnterAction{{Type: OnEnterAutoStartAgent}},
			OnExit:         []OnExitAction{{Type: OnExitDisablePlanMode}},
		}
		result := convertStepIDToPosition(events, idToPos)
		assert.Len(t, result.OnTurnStart, 1)
		assert.Equal(t, OnTurnStartMoveToNext, result.OnTurnStart[0].Type)
		assert.Len(t, result.OnTurnComplete, 1)
		assert.Len(t, result.OnEnter, 1)
		assert.Len(t, result.OnExit, 1)
	})

	t.Run("unknown step_id left unchanged", func(t *testing.T) {
		events := StepEvents{
			OnTurnStart: []OnTurnStartAction{
				{Type: OnTurnStartMoveToStep, Config: map[string]any{"step_id": "unknown"}},
			},
		}
		result := convertStepIDToPosition(events, idToPos)
		assert.Equal(t, "unknown", result.OnTurnStart[0].Config["step_id"])
	})
}

func TestConvertPositionToStepID(t *testing.T) {
	posToID := map[int]string{0: "new-a", 1: "new-b"}

	t.Run("rewrites step_position to step_id in OnTurnStart", func(t *testing.T) {
		events := StepEvents{
			OnTurnStart: []OnTurnStartAction{
				{Type: OnTurnStartMoveToStep, Config: map[string]any{"step_position": 1}},
			},
		}
		result := ConvertPositionToStepID(events, posToID)
		require.Len(t, result.OnTurnStart, 1)
		assert.Equal(t, "new-b", result.OnTurnStart[0].Config["step_id"])
		assert.Nil(t, result.OnTurnStart[0].Config["step_position"])
	})

	t.Run("rewrites step_position to step_id in OnTurnComplete", func(t *testing.T) {
		events := StepEvents{
			OnTurnComplete: []OnTurnCompleteAction{
				{Type: OnTurnCompleteMoveToStep, Config: map[string]any{"step_position": 0}},
			},
		}
		result := ConvertPositionToStepID(events, posToID)
		require.Len(t, result.OnTurnComplete, 1)
		assert.Equal(t, "new-a", result.OnTurnComplete[0].Config["step_id"])
		assert.Nil(t, result.OnTurnComplete[0].Config["step_position"])
	})

	t.Run("handles float64 position (JSON unmarshal)", func(t *testing.T) {
		events := StepEvents{
			OnTurnStart: []OnTurnStartAction{
				{Type: OnTurnStartMoveToStep, Config: map[string]any{"step_position": float64(0)}},
			},
		}
		result := ConvertPositionToStepID(events, posToID)
		assert.Equal(t, "new-a", result.OnTurnStart[0].Config["step_id"])
	})

	t.Run("unknown position left unchanged", func(t *testing.T) {
		events := StepEvents{
			OnTurnComplete: []OnTurnCompleteAction{
				{Type: OnTurnCompleteMoveToStep, Config: map[string]any{"step_position": 99}},
			},
		}
		result := ConvertPositionToStepID(events, posToID)
		assert.Equal(t, 99, result.OnTurnComplete[0].Config["step_position"])
	})
}

func TestRoundTrip(t *testing.T) {
	t.Run("export then import preserves events", func(t *testing.T) {
		// Build domain steps with step_id references.
		steps := []*WorkflowStep{
			{ID: "orig-a", Name: "Backlog", Position: 0, Color: "gray"},
			{
				ID: "orig-b", Name: "In Progress", Position: 1, Color: "blue",
				Events: StepEvents{
					OnTurnComplete: []OnTurnCompleteAction{
						{Type: OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": "orig-c"}},
					},
				},
			},
			{
				ID: "orig-c", Name: "Done", Position: 2, Color: "green",
				Events: StepEvents{
					OnTurnStart: []OnTurnStartAction{
						{Type: OnTurnStartMoveToStep, Config: map[string]any{"step_id": "orig-b"}},
					},
				},
			},
		}
		wf := &taskmodels.Workflow{ID: "wf-1", Name: "Pipeline"}
		export := BuildWorkflowExport([]*taskmodels.Workflow{wf}, map[string][]*WorkflowStep{"wf-1": steps})

		require.NoError(t, export.Validate())

		// Simulate import: assign new IDs by position.
		posToID := map[int]string{0: "new-a", 1: "new-b", 2: "new-c"}
		for _, sp := range export.Workflows[0].Steps {
			imported := ConvertPositionToStepID(sp.Events, posToID)

			switch sp.Name {
			case "In Progress":
				require.Len(t, imported.OnTurnComplete, 1)
				assert.Equal(t, "new-c", imported.OnTurnComplete[0].Config["step_id"],
					"In Progress should now reference new Done ID")
			case "Done":
				require.Len(t, imported.OnTurnStart, 1)
				assert.Equal(t, "new-b", imported.OnTurnStart[0].Config["step_id"],
					"Done should now reference new In Progress ID")
			}
		}
	})
}

func TestToInt(t *testing.T) {
	t.Run("float64", func(t *testing.T) {
		v, ok := toInt(float64(42))
		assert.True(t, ok)
		assert.Equal(t, 42, v)
	})
	t.Run("int", func(t *testing.T) {
		v, ok := toInt(7)
		assert.True(t, ok)
		assert.Equal(t, 7, v)
	})
	t.Run("string returns false", func(t *testing.T) {
		_, ok := toInt("nope")
		assert.False(t, ok)
	})
	t.Run("nil returns false", func(t *testing.T) {
		_, ok := toInt(nil)
		assert.False(t, ok)
	})
}
