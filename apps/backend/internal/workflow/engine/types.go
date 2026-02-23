package engine

import (
	"fmt"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// Trigger identifies when a step event should be evaluated.
type Trigger string

const (
	TriggerOnEnter        Trigger = "on_enter"
	TriggerOnTurnStart    Trigger = "on_turn_start"
	TriggerOnTurnComplete Trigger = "on_turn_complete"
	TriggerOnExit         Trigger = "on_exit"
)

// ActionKind identifies a typed workflow action.
type ActionKind string

const (
	ActionMoveToNext        ActionKind = "move_to_next"
	ActionMoveToPrevious    ActionKind = "move_to_previous"
	ActionMoveToStep        ActionKind = "move_to_step"
	ActionEnablePlanMode    ActionKind = "enable_plan_mode"
	ActionDisablePlanMode   ActionKind = "disable_plan_mode"
	ActionAutoStartAgent    ActionKind = "auto_start_agent"
	ActionResetAgentContext ActionKind = "reset_agent_context"
	ActionSetWorkflowData   ActionKind = "set_workflow_data"
)

// Action is the typed internal representation of workflow actions.
type Action struct {
	Kind             ActionKind
	RequiresApproval bool

	MoveToStep      *MoveToStepAction
	AutoStartAgent  *AutoStartAgentAction
	SetWorkflowData *SetWorkflowDataAction
}

// MoveToStepAction defines target step transitions.
type MoveToStepAction struct {
	StepID string
}

// AutoStartAgentAction defines auto-start prompt behavior for a step.
type AutoStartAgentAction struct {
	PromptOverride *string
	QueueIfBusy    bool
}

// SetWorkflowDataAction writes a key/value into the workflow data bag.
type SetWorkflowDataAction struct {
	Key   string
	Value any
}

// StepSpec is the engine's compiled step shape.
type StepSpec struct {
	ID         string
	WorkflowID string
	Name       string
	Position   int
	Prompt     string
	Events     map[Trigger][]Action
}

// CompileStep translates workflow models into typed step specs for the engine.
func CompileStep(step *wfmodels.WorkflowStep) StepSpec {
	events := map[Trigger][]Action{
		TriggerOnEnter:        compileOnEnter(step),
		TriggerOnTurnStart:    compileOnTurnStart(step),
		TriggerOnTurnComplete: compileOnTurnComplete(step),
		TriggerOnExit:         compileOnExit(step),
	}
	return StepSpec{
		ID:         step.ID,
		WorkflowID: step.WorkflowID,
		Name:       step.Name,
		Position:   step.Position,
		Prompt:     step.Prompt,
		Events:     events,
	}
}

func compileOnEnter(step *wfmodels.WorkflowStep) []Action {
	actions := make([]Action, 0, len(step.Events.OnEnter))
	for _, action := range step.Events.OnEnter {
		switch action.Type {
		case wfmodels.OnEnterEnablePlanMode:
			actions = append(actions, Action{Kind: ActionEnablePlanMode})
		case wfmodels.OnEnterAutoStartAgent:
			actions = append(actions, Action{Kind: ActionAutoStartAgent, AutoStartAgent: &AutoStartAgentAction{QueueIfBusy: true}})
		case wfmodels.OnEnterResetAgentContext:
			actions = append(actions, Action{Kind: ActionResetAgentContext})
		}
	}
	return actions
}

func compileOnTurnStart(step *wfmodels.WorkflowStep) []Action {
	actions := make([]Action, 0, len(step.Events.OnTurnStart))
	for _, action := range step.Events.OnTurnStart {
		switch action.Type {
		case wfmodels.OnTurnStartMoveToNext:
			actions = append(actions, Action{Kind: ActionMoveToNext})
		case wfmodels.OnTurnStartMoveToPrevious:
			actions = append(actions, Action{Kind: ActionMoveToPrevious})
		case wfmodels.OnTurnStartMoveToStep:
			stepID, err := readStepID(action.Config)
			if err != nil {
				continue // skip malformed move_to_step actions
			}
			actions = append(actions, Action{Kind: ActionMoveToStep, MoveToStep: &MoveToStepAction{StepID: stepID}})
		}
	}
	return actions
}

func compileOnTurnComplete(step *wfmodels.WorkflowStep) []Action {
	actions := make([]Action, 0, len(step.Events.OnTurnComplete))
	for _, action := range step.Events.OnTurnComplete {
		ra := ConfigRequiresApproval(action.Config)
		switch action.Type {
		case wfmodels.OnTurnCompleteMoveToNext:
			actions = append(actions, Action{Kind: ActionMoveToNext, RequiresApproval: ra})
		case wfmodels.OnTurnCompleteMoveToPrevious:
			actions = append(actions, Action{Kind: ActionMoveToPrevious, RequiresApproval: ra})
		case wfmodels.OnTurnCompleteMoveToStep:
			stepID, err := readStepID(action.Config)
			if err != nil {
				continue // skip malformed move_to_step actions
			}
			actions = append(actions, Action{Kind: ActionMoveToStep, RequiresApproval: ra, MoveToStep: &MoveToStepAction{StepID: stepID}})
		case wfmodels.OnTurnCompleteDisablePlanMode:
			actions = append(actions, Action{Kind: ActionDisablePlanMode})
		}
	}
	return actions
}

func compileOnExit(step *wfmodels.WorkflowStep) []Action {
	actions := make([]Action, 0, len(step.Events.OnExit))
	for _, action := range step.Events.OnExit {
		if action.Type == wfmodels.OnExitDisablePlanMode {
			actions = append(actions, Action{Kind: ActionDisablePlanMode})
		}
	}
	return actions
}

func readStepID(config map[string]any) (string, error) {
	if config == nil {
		return "", fmt.Errorf("missing move_to_step config")
	}
	stepID, _ := config["step_id"].(string)
	if stepID == "" {
		return "", fmt.Errorf("missing move_to_step step_id")
	}
	return stepID, nil
}

// ConfigRequiresApproval returns true if an action config has requires_approval set to true.
func ConfigRequiresApproval(config map[string]any) bool {
	if config == nil {
		return false
	}
	ra, ok := config["requires_approval"].(bool)
	return ok && ra
}
