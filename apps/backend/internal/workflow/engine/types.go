package engine

import (
	"fmt"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// Trigger identifies when a step event should be evaluated.
type Trigger string

const (
	// Existing kanban-era triggers — DO NOT CHANGE.
	TriggerOnEnter        Trigger = "on_enter"
	TriggerOnTurnStart    Trigger = "on_turn_start"
	TriggerOnTurnComplete Trigger = "on_turn_complete"
	TriggerOnExit         Trigger = "on_exit"

	// New triggers for Phase 2 of the task model unification (ADR-0004).
	// Declared here so payloads and step compilation can reference them;
	// they are NOT yet handled by Engine.HandleTrigger. Wiring lands once
	// Phase 1 (agent runtime extraction) merges and the engine integration
	// slice of Phase 2 begins.
	TriggerOnComment           Trigger = "on_comment"
	TriggerOnBlockerResolved   Trigger = "on_blocker_resolved"
	TriggerOnChildrenCompleted Trigger = "on_children_completed"
	TriggerOnApprovalResolved  Trigger = "on_approval_resolved"
	TriggerOnHeartbeat         Trigger = "on_heartbeat"
	TriggerOnBudgetAlert       Trigger = "on_budget_alert"
	TriggerOnAgentError        Trigger = "on_agent_error"
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

	// New Phase 2 action kinds (ADR-0004). Defined and exposed via callbacks
	// that intentionally return ErrActionNotYetWired — they will be wired
	// into Engine.HandleTrigger and registered against real implementations
	// as part of the Phase 2 engine-integration slice. Until then they are
	// declared so that downstream code (workflow templates, tests) can
	// reference the kind constants without behaviour changing.
	ActionQueueRun                   ActionKind = "queue_run"
	ActionClearDecisions             ActionKind = "clear_decisions"
	ActionQueueRunForEachParticipant ActionKind = "queue_run_for_each_participant"

	// Phase 8 (ADR-0004) cross-strategy delegation actions.
	//
	// ActionCreateChildTask creates a new task with parent_id == current
	// task. Typical use: an Office agent emits a delegation signal that
	// fires this action, which spawns a kanban subtask carrying its own
	// workflow + first runnable step.
	//
	// ActionSwitchWorkflow swaps a task's workflow_id / workflow_step_id in
	// place. Used to promote a kanban task to office (long conversation
	// needed) or vice versa. Engine fires on_exit on the old step + on_enter
	// on the new step.
	ActionCreateChildTask ActionKind = "create_child_task"
	ActionSwitchWorkflow  ActionKind = "switch_workflow"
)

// Action is the typed internal representation of workflow actions.
type Action struct {
	Kind             ActionKind
	RequiresApproval bool

	// Guard, when non-nil, gates a transition action on a condition that the
	// engine evaluates before resolving the transition target. Today only
	// wait_for_quorum is supported. Non-transition actions ignore Guard.
	Guard *TransitionGuard

	MoveToStep                 *MoveToStepAction
	AutoStartAgent             *AutoStartAgentAction
	SetWorkflowData            *SetWorkflowDataAction
	QueueRun                   *QueueRunAction
	ClearDecisions             *ClearDecisionsAction
	QueueRunForEachParticipant *QueueRunForEachParticipantAction
	CreateChildTask            *CreateChildTaskAction
	SwitchWorkflow             *SwitchWorkflowAction
}

// TransitionGuard is the typed `if:` clause attached to a transition action.
// At most one guard variant is set per Action; nil guards mean "always
// permit the transition" (preserving today's kanban semantics).
type TransitionGuard struct {
	WaitForQuorum *WaitForQuorumGuard
}

// WaitForQuorumGuard gates a transition on the state of recorded decisions
// for the (task, step) pair.
//
// Threshold values:
//   - "all_approve"     — every required participant has decision == "approved".
//   - "all_decide"      — every required participant has any non-empty decision.
//   - "any_reject"      — at least one decision == "rejected".
//   - "majority_approve" — strictly more than half of required participants approved.
//   - "n_approve:<N>"   — at least N decisions == "approved".
//
// "Required participants" are the current participants of the step whose
// role matches Role and whose decision_required flag is true. Participants
// removed mid-flight drop out of the count (their stale decisions are
// ignored for quorum purposes).
type WaitForQuorumGuard struct {
	Role      string
	Threshold string
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

// QueueRunAction represents the Phase 2 "queue a run on a target task/agent"
// action. The action is declared but its callback is not yet wired into the
// engine; see PlaceholderQueueRunCallback.
//
// Targets: "primary" | "participant_role:<role>" | "agent_profile_id:<id>" |
// "workspace.ceo_agent" (resolved at engine evaluation time, post-wire-up).
//
// TaskID: "this" (default — the trigger's task) | a literal task id |
// "{coordination_task.id}" template.
type QueueRunAction struct {
	Target  string
	TaskID  string
	Reason  string
	Payload map[string]any
}

// ClearDecisionsAction clears workflow_step_decisions rows for the trigger's
// (task, step) pair. Used by Review.on_enter so quorum starts fresh after a
// rejection round. Declared but not yet wired.
type ClearDecisionsAction struct{}

// QueueRunForEachParticipantAction fans out QueueRun against every step
// participant matching the configured role. Declared but not yet wired.
type QueueRunForEachParticipantAction struct {
	Role    string
	Reason  string
	Payload map[string]any
}

// CreateChildTaskAction defines a "spawn a child task" action. The new
// task has parent_id == the trigger's task id, runs under the configured
// workflow + step, and is assigned to the configured agent profile (when
// non-empty).
//
// All fields except Title are optional:
//   - WorkflowID: when empty, the engine adapter falls back to the parent
//     task's workspace default workflow (typically the Kanban Default).
//   - StepID: when empty, the adapter resolves the workflow's first
//     runnable step.
//   - AgentProfileID: when empty, the adapter inherits the parent task's
//     assignee.
type CreateChildTaskAction struct {
	Title          string
	Description    string
	WorkflowID     string
	StepID         string
	AgentProfileID string
}

// SwitchWorkflowAction mutates a task's workflow_id and workflow_step_id
// in place. The engine fires on_exit on the old step before the swap and
// on_enter on the new step after.
//
// StepID is optional — when empty, the engine adapter resolves the new
// workflow's first runnable step.
type SwitchWorkflowAction struct {
	WorkflowID string
	StepID     string
}

// ParticipantRole, StageType and WorkflowStyle are defined in
// internal/workflow/models. The engine surfaces small validators here so
// callers (templates, engine integration code) can sanity-check
// configuration values without pulling the model package directly.

// ValidParticipantRole reports whether the supplied string is one of the
// participant roles supported by the workflow_step_participants CHECK
// constraint.
func ValidParticipantRole(role string) bool {
	switch wfmodels.ParticipantRole(role) {
	case wfmodels.ParticipantRoleReviewer,
		wfmodels.ParticipantRoleApprover,
		wfmodels.ParticipantRoleWatcher,
		wfmodels.ParticipantRoleCollaborator:
		return true
	}
	return false
}

// ValidStageType reports whether the supplied string matches the schema's
// CHECK constraint for workflow_steps.stage_type.
func ValidStageType(stage string) bool {
	switch wfmodels.StageType(stage) {
	case wfmodels.StageTypeWork,
		wfmodels.StageTypeReview,
		wfmodels.StageTypeApproval,
		wfmodels.StageTypeCustom:
		return true
	}
	return false
}

// ValidWorkflowStyle reports whether the supplied string matches the schema's
// CHECK constraint for workflows.style.
func ValidWorkflowStyle(style string) bool {
	switch wfmodels.WorkflowStyle(style) {
	case wfmodels.WorkflowStyleKanban,
		wfmodels.WorkflowStyleOffice,
		wfmodels.WorkflowStyleCustom:
		return true
	}
	return false
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
		// Phase 2 (ADR-0004) event-driven triggers — empty slices when the
		// step has no GenericAction entries for the trigger.
		TriggerOnComment:           compileGenericActions(step.Events.OnComment),
		TriggerOnBlockerResolved:   compileGenericActions(step.Events.OnBlockerResolved),
		TriggerOnChildrenCompleted: compileGenericActions(step.Events.OnChildrenCompleted),
		TriggerOnApprovalResolved:  compileGenericActions(step.Events.OnApprovalResolved),
		TriggerOnHeartbeat:         compileGenericActions(step.Events.OnHeartbeat),
		TriggerOnBudgetAlert:       compileGenericActions(step.Events.OnBudgetAlert),
		TriggerOnAgentError:        compileGenericActions(step.Events.OnAgentError),
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
		case wfmodels.OnEnterClearDecisions:
			actions = append(actions, Action{
				Kind:           ActionClearDecisions,
				ClearDecisions: &ClearDecisionsAction{},
			})
		case wfmodels.OnEnterQueueRunForEachParticipant:
			actions = append(actions, Action{
				Kind:                       ActionQueueRunForEachParticipant,
				QueueRunForEachParticipant: readQueueRunForEachParticipantConfig(action.Config),
			})
		case wfmodels.OnEnterQueueRun:
			actions = append(actions, Action{
				Kind:     ActionQueueRun,
				QueueRun: readQueueRunConfig(action.Config),
			})
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
		guard := ConfigTransitionGuard(action.Config)
		switch action.Type {
		case wfmodels.OnTurnCompleteMoveToNext:
			actions = append(actions, Action{Kind: ActionMoveToNext, RequiresApproval: ra, Guard: guard})
		case wfmodels.OnTurnCompleteMoveToPrevious:
			actions = append(actions, Action{Kind: ActionMoveToPrevious, RequiresApproval: ra, Guard: guard})
		case wfmodels.OnTurnCompleteMoveToStep:
			stepID, err := readStepID(action.Config)
			if err != nil {
				continue // skip malformed move_to_step actions
			}
			actions = append(actions, Action{Kind: ActionMoveToStep, RequiresApproval: ra, Guard: guard, MoveToStep: &MoveToStepAction{StepID: stepID}})
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

// compileGenericActions translates the persisted GenericAction list into the
// engine's typed Action structs. Unknown action types are skipped so a future
// action kind shipped via YAML doesn't crash older binaries.
func compileGenericActions(actions []wfmodels.GenericAction) []Action {
	out := make([]Action, 0, len(actions))
	for _, a := range actions {
		switch a.Type {
		case wfmodels.GenericActionQueueRun:
			out = append(out, Action{
				Kind:     ActionQueueRun,
				QueueRun: readQueueRunConfig(a.Config),
			})
		case wfmodels.GenericActionClearDecisions:
			out = append(out, Action{
				Kind:           ActionClearDecisions,
				ClearDecisions: &ClearDecisionsAction{},
			})
		case wfmodels.GenericActionQueueRunForEachParticipant:
			out = append(out, Action{
				Kind:                       ActionQueueRunForEachParticipant,
				QueueRunForEachParticipant: readQueueRunForEachParticipantConfig(a.Config),
			})
		}
	}
	return out
}

// readQueueRunConfig reads a `queue_run` action's config map. Missing keys
// fall back to engine defaults ("primary" target, "this" task id).
func readQueueRunConfig(config map[string]any) *QueueRunAction {
	if config == nil {
		return &QueueRunAction{Target: TargetPrimary, TaskID: TaskIDThis}
	}
	target, _ := config["target"].(string)
	if target == "" {
		target = TargetPrimary
	}
	taskID, _ := config["task_id"].(string)
	if taskID == "" {
		taskID = TaskIDThis
	}
	reason, _ := config["reason"].(string)
	payload, _ := config["payload"].(map[string]any)
	return &QueueRunAction{
		Target:  target,
		TaskID:  taskID,
		Reason:  reason,
		Payload: payload,
	}
}

// readQueueRunForEachParticipantConfig reads the role/reason/payload for a
// queue_run_for_each_participant action.
func readQueueRunForEachParticipantConfig(config map[string]any) *QueueRunForEachParticipantAction {
	if config == nil {
		return &QueueRunForEachParticipantAction{}
	}
	role, _ := config["role"].(string)
	reason, _ := config["reason"].(string)
	payload, _ := config["payload"].(map[string]any)
	return &QueueRunForEachParticipantAction{
		Role:    role,
		Reason:  reason,
		Payload: payload,
	}
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

// ConfigTransitionGuard reads the `wait_for_quorum` guard from an action's
// config map, if present. Returns nil when the config is missing, malformed,
// or has no guard — preserving today's "always allow" semantics for kanban
// steps that never set the key.
//
// Expected shape:
//
//	{
//	    "wait_for_quorum": {
//	        "role":      "reviewer",
//	        "threshold": "all_approve"
//	    }
//	}
func ConfigTransitionGuard(config map[string]any) *TransitionGuard {
	if config == nil {
		return nil
	}
	raw, ok := config["wait_for_quorum"].(map[string]any)
	if !ok {
		return nil
	}
	role, _ := raw["role"].(string)
	threshold, _ := raw["threshold"].(string)
	if role == "" || threshold == "" {
		return nil
	}
	return &TransitionGuard{
		WaitForQuorum: &WaitForQuorumGuard{Role: role, Threshold: threshold},
	}
}
