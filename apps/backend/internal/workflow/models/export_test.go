package models

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"gopkg.in/yaml.v3"
)

func TestBuildWorkflowExport(t *testing.T) {
	t.Run("converts step IDs to positions", func(t *testing.T) {
		wf := &taskmodels.Workflow{
			ID: "wf-1", Name: "My Workflow", Description: "desc",
			Prompt: "If the PR is merged or closed, move the Task to Done.",
		}
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

		export := BuildWorkflowExport([]*taskmodels.Workflow{wf}, stepMap, nil)

		require.Equal(t, ExportVersion, export.Version)
		require.Equal(t, ExportType, export.Type)
		require.Len(t, export.Workflows, 1)

		pw := export.Workflows[0]
		assert.Equal(t, "My Workflow", pw.Name)
		assert.Equal(t, "desc", pw.Description)
		assert.Equal(t, "If the PR is merged or closed, move the Task to Done.", pw.Prompt)
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
		export := BuildWorkflowExport([]*taskmodels.Workflow{wf}, map[string][]*WorkflowStep{"wf-1": steps}, nil)

		sp := export.Workflows[0].Steps[0]
		assert.Len(t, sp.Events.OnEnter, 1)
		assert.Equal(t, OnEnterAutoStartAgent, sp.Events.OnEnter[0].Type)
		assert.Len(t, sp.Events.OnExit, 1)
		assert.Len(t, sp.Events.OnTurnStart, 1)
		assert.Equal(t, OnTurnStartMoveToNext, sp.Events.OnTurnStart[0].Type)
	})

	t.Run("exports pull source as portable step position", func(t *testing.T) {
		wf := &taskmodels.Workflow{ID: "wf-1", Name: "Pull Workflow"}
		steps := []*WorkflowStep{
			{ID: "queue", Name: "Queue", Position: 0, Color: "gray"},
			{
				ID:             "work",
				Name:           "Work",
				Position:       1,
				Color:          "blue",
				WIPLimit:       1,
				PullFromStepID: "queue",
			},
		}

		export := BuildWorkflowExport(
			[]*taskmodels.Workflow{wf},
			map[string][]*WorkflowStep{"wf-1": steps},
			nil,
		)

		require.Len(t, export.Workflows[0].Steps, 2)
		assert.Equal(t, 1, export.Workflows[0].Steps[1].WIPLimit)
		require.NotNil(t, export.Workflows[0].Steps[1].PullFromStepPosition)
		assert.Equal(t, 0, *export.Workflows[0].Steps[1].PullFromStepPosition)
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

	t.Run("set_session_mode with mode passes", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Events = StepEvents{
			OnEnter: []OnEnterAction{
				{Type: OnEnterSetSessionMode, Config: map[string]any{"mode": "acceptEdits"}},
			},
		}
		assert.NoError(t, e.Validate())
	})

	t.Run("set_session_mode without mode fails", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Events = StepEvents{
			OnEnter: []OnEnterAction{{Type: OnEnterSetSessionMode}},
		}
		assert.ErrorContains(t, e.Validate(), "set_session_mode requires a non-empty string")
	})

	t.Run("set_session_mode with non-string mode fails", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Events = StepEvents{
			OnEnter: []OnEnterAction{
				{Type: OnEnterSetSessionMode, Config: map[string]any{"mode": 3}},
			},
		}
		assert.ErrorContains(t, e.Validate(), "set_session_mode requires a non-empty string")
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

	t.Run("invalid pull source position fails", func(t *testing.T) {
		e := validExport()
		pos := 99
		e.Workflows[0].Steps[1].PullFromStepPosition = &pos
		assert.ErrorContains(t, e.Validate(), "pull_from_step_position 99 does not match any step")
	})

	t.Run("self pull source position fails", func(t *testing.T) {
		e := validExport()
		pos := 1
		e.Workflows[0].Steps[1].PullFromStepPosition = &pos
		assert.ErrorContains(t, e.Validate(), "cannot reference itself")
	})

	t.Run("pull source cycle fails", func(t *testing.T) {
		e := validExport()
		firstPosition := 0
		secondPosition := 1
		e.Workflows[0].Steps[0].PullFromStepPosition = &secondPosition
		e.Workflows[0].Steps[1].PullFromStepPosition = &firstPosition
		assert.ErrorContains(t, e.Validate(), "cannot create a pull cycle")
	})

	t.Run("valid on_children_completed move_to_step position ref passes", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Events = StepEvents{
			OnChildrenCompleted: []GenericAction{
				{Type: GenericActionMoveToStep, Config: map[string]any{"step_position": 1}},
			},
		}
		assert.NoError(t, e.Validate())
	})

	t.Run("invalid on_children_completed move_to_step position ref fails", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Events = StepEvents{
			OnChildrenCompleted: []GenericAction{
				{Type: GenericActionMoveToStep, Config: map[string]any{"step_position": 99}},
			},
		}
		assert.ErrorContains(t, e.Validate(), "does not match any step")
	})

	t.Run("missing step_position on generic move_to_step fails", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Events = StepEvents{
			OnHeartbeat: []GenericAction{
				{Type: GenericActionMoveToStep, Config: map[string]any{"other": "val"}},
			},
		}
		assert.ErrorContains(t, e.Validate(), "missing step_position")
	})

	t.Run("missing config on generic move_to_step fails", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Events = StepEvents{
			OnBudgetAlert: []GenericAction{
				{Type: GenericActionMoveToStep, Config: nil},
			},
		}
		assert.ErrorContains(t, e.Validate(), "missing config")
	})

	t.Run("non-move_to_step generic action needs no position ref", func(t *testing.T) {
		e := validExport()
		e.Workflows[0].Steps[0].Events = StepEvents{
			OnAgentError: []GenericAction{
				{Type: GenericActionAutoStartAgent},
			},
		}
		assert.NoError(t, e.Validate())
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

func TestConvertPositionToStepID_PhaseTwoTriggers(t *testing.T) {
	posToID := map[int]string{0: "new-a", 1: "new-b"}

	t.Run("all seven Phase-2 GenericAction triggers survive conversion", func(t *testing.T) {
		moveTo := func(pos int) []GenericAction {
			return []GenericAction{{Type: GenericActionMoveToStep, Config: map[string]any{"step_position": pos}}}
		}
		events := StepEvents{
			OnComment:           moveTo(0),
			OnBlockerResolved:   moveTo(0),
			OnChildrenCompleted: moveTo(0),
			OnApprovalResolved:  moveTo(0),
			OnHeartbeat:         moveTo(0),
			OnBudgetAlert:       moveTo(0),
			OnAgentError:        moveTo(0),
		}
		result := ConvertPositionToStepID(events, posToID)

		triggers := map[string][]GenericAction{
			"on_comment":            result.OnComment,
			"on_blocker_resolved":   result.OnBlockerResolved,
			"on_children_completed": result.OnChildrenCompleted,
			"on_approval_resolved":  result.OnApprovalResolved,
			"on_heartbeat":          result.OnHeartbeat,
			"on_budget_alert":       result.OnBudgetAlert,
			"on_agent_error":        result.OnAgentError,
		}
		for name, actions := range triggers {
			require.Lenf(t, actions, 1, "%s dropped during ConvertPositionToStepID", name)
			assert.Equalf(t, "new-a", actions[0].Config["step_id"], "%s did not remap step_position to step_id", name)
			assert.Nilf(t, actions[0].Config["step_position"], "%s should no longer carry step_position", name)
		}
	})

	t.Run("non-move_to_step generic action passes through untouched", func(t *testing.T) {
		events := StepEvents{
			OnHeartbeat: []GenericAction{{Type: GenericActionAutoStartAgent}},
		}
		result := ConvertPositionToStepID(events, posToID)
		require.Len(t, result.OnHeartbeat, 1)
		assert.Equal(t, GenericActionAutoStartAgent, result.OnHeartbeat[0].Type)
		assert.Nil(t, result.OnHeartbeat[0].Config)
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
		export := BuildWorkflowExport([]*taskmodels.Workflow{wf}, map[string][]*WorkflowStep{"wf-1": steps}, nil)

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

func TestRoundTripPhaseTwoTrigger(t *testing.T) {
	t.Run("on_children_completed move_to_step round trips through export/import", func(t *testing.T) {
		// Build domain steps: In Progress moves to Backlog once its children complete.
		steps := []*WorkflowStep{
			{ID: "orig-a", Name: "Backlog", Position: 0, Color: "gray"},
			{
				ID: "orig-b", Name: "In Progress", Position: 1, Color: "blue",
				Events: StepEvents{
					OnChildrenCompleted: []GenericAction{
						{Type: GenericActionMoveToStep, Config: map[string]any{"step_id": "orig-a"}},
					},
				},
			},
		}
		wf := &taskmodels.Workflow{ID: "wf-1", Name: "Pipeline"}
		export := BuildWorkflowExport([]*taskmodels.Workflow{wf}, map[string][]*WorkflowStep{"wf-1": steps}, nil)

		require.NoError(t, export.Validate())

		var exportedInProgress *StepPortable
		for i := range export.Workflows[0].Steps {
			if export.Workflows[0].Steps[i].Name == "In Progress" {
				exportedInProgress = &export.Workflows[0].Steps[i]
			}
		}
		require.NotNil(t, exportedInProgress)
		require.Len(t, exportedInProgress.Events.OnChildrenCompleted, 1,
			"on_children_completed must survive export, not be silently dropped")
		assert.Equal(t, 0, exportedInProgress.Events.OnChildrenCompleted[0].Config["step_position"],
			"export direction must remap step_id to step_position")
		assert.Nil(t, exportedInProgress.Events.OnChildrenCompleted[0].Config["step_id"])

		// Simulate import: assign new IDs by position.
		posToID := map[int]string{0: "new-a", 1: "new-b"}
		imported := ConvertPositionToStepID(exportedInProgress.Events, posToID)
		require.Len(t, imported.OnChildrenCompleted, 1,
			"on_children_completed must survive import, not be silently dropped")
		assert.Equal(t, "new-a", imported.OnChildrenCompleted[0].Config["step_id"],
			"import direction must remap step_position back to step_id")
		assert.Nil(t, imported.OnChildrenCompleted[0].Config["step_position"])
	})

	t.Run("non-move_to_step generic action round trips untouched", func(t *testing.T) {
		steps := []*WorkflowStep{
			{
				ID: "orig-a", Name: "Backlog", Position: 0, Color: "gray",
				Events: StepEvents{
					OnAgentError: []GenericAction{{Type: GenericActionAutoStartAgent}},
				},
			},
		}
		wf := &taskmodels.Workflow{ID: "wf-1", Name: "Pipeline"}
		export := BuildWorkflowExport([]*taskmodels.Workflow{wf}, map[string][]*WorkflowStep{"wf-1": steps}, nil)

		require.NoError(t, export.Validate())
		require.Len(t, export.Workflows[0].Steps[0].Events.OnAgentError, 1,
			"on_agent_error must survive export, not be silently dropped")
		assert.Equal(t, GenericActionAutoStartAgent, export.Workflows[0].Steps[0].Events.OnAgentError[0].Type)

		posToID := map[int]string{0: "new-a"}
		imported := ConvertPositionToStepID(export.Workflows[0].Steps[0].Events, posToID)
		require.Len(t, imported.OnAgentError, 1)
		assert.Equal(t, GenericActionAutoStartAgent, imported.OnAgentError[0].Type)
	})
}

func TestAutoAdvanceRequiresSignalExport(t *testing.T) {
	t.Run("preserves auto_advance_requires_signal in export", func(t *testing.T) {
		wf := &taskmodels.Workflow{ID: "wf-1", Name: "WF"}
		steps := []*WorkflowStep{
			{ID: "s1", Name: "Legacy", Position: 0, Color: "gray", AutoAdvanceRequiresSignal: false},
			{ID: "s2", Name: "Gated", Position: 1, Color: "blue", AutoAdvanceRequiresSignal: true},
		}
		export := BuildWorkflowExport(
			[]*taskmodels.Workflow{wf},
			map[string][]*WorkflowStep{"wf-1": steps},
			nil,
		)

		require.Len(t, export.Workflows[0].Steps, 2)
		assert.False(t, export.Workflows[0].Steps[0].AutoAdvanceRequiresSignal)
		assert.True(t, export.Workflows[0].Steps[1].AutoAdvanceRequiresSignal)
	})
}

func TestCancelTriggersTurnCompleteExport(t *testing.T) {
	wf := &taskmodels.Workflow{ID: "wf-cancel", Name: "Cancel workflow"}
	steps := []*WorkflowStep{
		{ID: "s1", Name: "Paused", Position: 0},
		{ID: "s2", Name: "Advance", Position: 1},
	}
	field := reflect.ValueOf(steps[1]).Elem().FieldByName("CancelTriggersTurnComplete")
	if !field.IsValid() {
		t.Fatal("WorkflowStep is missing CancelTriggersTurnComplete")
	}
	field.SetBool(true)

	export := BuildWorkflowExport([]*taskmodels.Workflow{wf}, map[string][]*WorkflowStep{"wf-cancel": steps}, nil)
	require.Len(t, export.Workflows[0].Steps, 2)
	exportedField := reflect.ValueOf(&export.Workflows[0].Steps[1]).Elem().FieldByName("CancelTriggersTurnComplete")
	if !exportedField.IsValid() {
		t.Fatal("StepPortable is missing CancelTriggersTurnComplete")
	}
	assert.True(t, exportedField.Bool())
}

func TestCancelTriggersTurnCompleteYAMLExportIncludesFalse(t *testing.T) {
	wf := &taskmodels.Workflow{ID: "wf-yaml-cancel", Name: "YAML cancellation"}
	export := BuildWorkflowExport(
		[]*taskmodels.Workflow{wf},
		map[string][]*WorkflowStep{"wf-yaml-cancel": {
			{ID: "s1", Name: "Todo", Position: 0},
		}},
		nil,
	)

	encoded, err := yaml.Marshal(export)
	if err != nil {
		t.Fatalf("marshal workflow export: %v", err)
	}
	if got := string(encoded); !strings.Contains(got, "cancel_triggers_turn_complete: false") {
		t.Fatalf("YAML export omitted false cancellation policy:\n%s", got)
	}
}

func TestPullFromStepPositionToID(t *testing.T) {
	position := 0
	step := StepPortable{
		Name:                 "Work",
		Position:             1,
		WIPLimit:             1,
		PullFromStepPosition: &position,
	}
	posToID := map[int]string{0: "queue-id", 1: "work-id"}

	assert.Equal(t, "queue-id", step.PullFromStepID(posToID))
}

func TestShowInCommandPanelExport(t *testing.T) {
	t.Run("preserves show_in_command_panel in export", func(t *testing.T) {
		wf := &taskmodels.Workflow{ID: "wf-1", Name: "Test"}
		steps := []*WorkflowStep{
			{ID: "s1", Name: "Backlog", Position: 0, ShowInCommandPanel: false},
			{ID: "s2", Name: "Active", Position: 1, ShowInCommandPanel: true},
			{ID: "s3", Name: "Done", Position: 2, ShowInCommandPanel: false},
		}
		export := BuildWorkflowExport(
			[]*taskmodels.Workflow{wf},
			map[string][]*WorkflowStep{"wf-1": steps},
			nil,
		)

		require.Len(t, export.Workflows[0].Steps, 3)
		assert.False(t, export.Workflows[0].Steps[0].ShowInCommandPanel)
		assert.True(t, export.Workflows[0].Steps[1].ShowInCommandPanel)
		assert.False(t, export.Workflows[0].Steps[2].ShowInCommandPanel)
	})
}

func TestAgentProfileExport(t *testing.T) {
	resolver := func(profileID string) *AgentProfilePortable {
		profiles := map[string]*AgentProfilePortable{
			"prof-1": {AgentName: "Claude Code", Model: "opus", Mode: "code"},
			"prof-2": {AgentName: "Codex", Model: "o3"},
		}
		return profiles[profileID]
	}

	t.Run("includes agent profile on workflow and steps", func(t *testing.T) {
		wf := &taskmodels.Workflow{ID: "wf-1", Name: "WithProfiles", AgentProfileID: "prof-1"}
		steps := []*WorkflowStep{
			{ID: "s1", Name: "Dev", Position: 0, Color: "blue", AgentProfileID: "prof-2"},
			{ID: "s2", Name: "Review", Position: 1, Color: "green"},
		}
		export := BuildWorkflowExport(
			[]*taskmodels.Workflow{wf},
			map[string][]*WorkflowStep{"wf-1": steps},
			resolver,
		)

		pw := export.Workflows[0]
		require.NotNil(t, pw.AgentProfile)
		assert.Equal(t, "Claude Code", pw.AgentProfile.AgentName)
		assert.Equal(t, "opus", pw.AgentProfile.Model)
		assert.Equal(t, "code", pw.AgentProfile.Mode)

		require.NotNil(t, pw.Steps[0].AgentProfile)
		assert.Equal(t, "Codex", pw.Steps[0].AgentProfile.AgentName)
		assert.Equal(t, "o3", pw.Steps[0].AgentProfile.Model)
		assert.Empty(t, pw.Steps[0].AgentProfile.Mode)

		assert.Nil(t, pw.Steps[1].AgentProfile, "step without profile should be nil")
	})

	t.Run("omits agent profile when resolver is nil", func(t *testing.T) {
		wf := &taskmodels.Workflow{ID: "wf-1", Name: "NoResolver", AgentProfileID: "prof-1"}
		steps := []*WorkflowStep{
			{ID: "s1", Name: "Step", Position: 0, Color: "gray", AgentProfileID: "prof-2"},
		}
		export := BuildWorkflowExport(
			[]*taskmodels.Workflow{wf},
			map[string][]*WorkflowStep{"wf-1": steps},
			nil,
		)

		pw := export.Workflows[0]
		assert.Nil(t, pw.AgentProfile)
		assert.Nil(t, pw.Steps[0].AgentProfile)
	})

	t.Run("omits agent profile when IDs are empty", func(t *testing.T) {
		wf := &taskmodels.Workflow{ID: "wf-1", Name: "EmptyIDs"}
		steps := []*WorkflowStep{
			{ID: "s1", Name: "Step", Position: 0, Color: "gray"},
		}
		export := BuildWorkflowExport(
			[]*taskmodels.Workflow{wf},
			map[string][]*WorkflowStep{"wf-1": steps},
			resolver,
		)

		pw := export.Workflows[0]
		assert.Nil(t, pw.AgentProfile)
		assert.Nil(t, pw.Steps[0].AgentProfile)
	})

	t.Run("handles unknown profile ID gracefully", func(t *testing.T) {
		wf := &taskmodels.Workflow{ID: "wf-1", Name: "Unknown", AgentProfileID: "prof-unknown"}
		steps := []*WorkflowStep{
			{ID: "s1", Name: "Step", Position: 0, Color: "gray"},
		}
		export := BuildWorkflowExport(
			[]*taskmodels.Workflow{wf},
			map[string][]*WorkflowStep{"wf-1": steps},
			resolver,
		)

		pw := export.Workflows[0]
		assert.Nil(t, pw.AgentProfile, "unknown profile should resolve to nil")
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
