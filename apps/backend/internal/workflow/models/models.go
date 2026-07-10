package models

import (
	"maps"
	"time"
)

// OnEnterActionType represents the type of action to execute when entering a step.
type OnEnterActionType string

const (
	OnEnterEnablePlanMode    OnEnterActionType = "enable_plan_mode"
	OnEnterAutoStartAgent    OnEnterActionType = "auto_start_agent"
	OnEnterResetAgentContext OnEnterActionType = "reset_agent_context"
	// OnEnterSetSessionMode declares the agent's session permission mode (e.g.
	// "default", "acceptEdits") for a step on entry. The target mode is carried
	// in the action Config under the "mode" key. See issue #1183.
	OnEnterSetSessionMode OnEnterActionType = "set_session_mode"

	// Phase 2 (ADR-0004) — generic actions are also permitted on on_enter
	// so review/approval steps can clear decisions and fan out runs to
	// participants when the task arrives at the step.
	OnEnterClearDecisions             OnEnterActionType = "clear_decisions"
	OnEnterQueueRunForEachParticipant OnEnterActionType = "queue_run_for_each_participant"
	OnEnterQueueRun                   OnEnterActionType = "queue_run"
)

// OnTurnStartActionType represents the type of action to execute when a user sends a message.
type OnTurnStartActionType string

const (
	OnTurnStartMoveToNext     OnTurnStartActionType = "move_to_next"
	OnTurnStartMoveToPrevious OnTurnStartActionType = "move_to_previous"
	OnTurnStartMoveToStep     OnTurnStartActionType = "move_to_step"
)

// OnTurnStartAction represents an action to execute when a user sends a message.
type OnTurnStartAction struct {
	Type   OnTurnStartActionType  `json:"type" yaml:"type"`
	Config map[string]interface{} `json:"config,omitempty" yaml:"config,omitempty"`
}

// OnTurnCompleteActionType represents the type of action to execute when an agent turn completes.
type OnTurnCompleteActionType string

const (
	OnTurnCompleteMoveToNext      OnTurnCompleteActionType = "move_to_next"
	OnTurnCompleteMoveToPrevious  OnTurnCompleteActionType = "move_to_previous"
	OnTurnCompleteMoveToStep      OnTurnCompleteActionType = "move_to_step"
	OnTurnCompleteDisablePlanMode OnTurnCompleteActionType = "disable_plan_mode"
)

// OnEnterAction represents an action to execute when entering a step.
type OnEnterAction struct {
	Type   OnEnterActionType      `json:"type" yaml:"type"`
	Config map[string]interface{} `json:"config,omitempty" yaml:"config,omitempty"`
}

// OnTurnCompleteAction represents an action to execute when an agent turn completes.
type OnTurnCompleteAction struct {
	Type   OnTurnCompleteActionType `json:"type" yaml:"type"`
	Config map[string]interface{}   `json:"config,omitempty" yaml:"config,omitempty"`
}

// OnExitActionType represents the type of action to execute when leaving a step.
type OnExitActionType string

const (
	OnExitDisablePlanMode OnExitActionType = "disable_plan_mode"
)

// OnExitAction represents an action to execute when leaving a step.
type OnExitAction struct {
	Type   OnExitActionType       `json:"type" yaml:"type"`
	Config map[string]interface{} `json:"config,omitempty" yaml:"config,omitempty"`
}

// GenericActionType represents the type of a Phase 2 (ADR-0004) action that
// can appear under any of the new event-driven triggers (on_comment,
// on_blocker_resolved, on_children_completed, on_approval_resolved,
// on_heartbeat, on_budget_alert, on_agent_error). Actions are compiled into
// the engine's typed Action structs by engine.CompileStep.
type GenericActionType string

const (
	// GenericActionMoveToNext transitions to the next workflow step.
	GenericActionMoveToNext GenericActionType = "move_to_next"
	// GenericActionMoveToPrevious transitions to the previous workflow step.
	GenericActionMoveToPrevious GenericActionType = "move_to_previous"
	// GenericActionMoveToStep transitions to a configured workflow step.
	GenericActionMoveToStep GenericActionType = "move_to_step"
	// GenericActionAutoStartAgent starts the step's agent.
	GenericActionAutoStartAgent GenericActionType = "auto_start_agent"
	// GenericActionQueueRun queues a run on a target task/agent.
	GenericActionQueueRun GenericActionType = "queue_run"
	// GenericActionClearDecisions clears recorded decisions for the
	// (task, step) pair. Typically used by a Review step's on_enter to
	// start fresh after a rejection round.
	GenericActionClearDecisions GenericActionType = "clear_decisions"
	// GenericActionQueueRunForEachParticipant fans out queue_run over
	// every participant of the step matching a configured role.
	GenericActionQueueRunForEachParticipant GenericActionType = "queue_run_for_each_participant"
)

// GenericAction is the persisted shape of a Phase 2 action used in the
// new event-driven triggers. Config carries the action-specific parameters
// (target, task_id, reason, payload, role, …) interpreted by
// engine.CompileStep.
type GenericAction struct {
	Type   GenericActionType      `json:"type" yaml:"type"`
	Config map[string]interface{} `json:"config,omitempty" yaml:"config,omitempty"`
}

// StepEvents contains event-driven actions for a workflow step.
//
// The four kanban-era triggers (OnEnter, OnTurnStart, OnTurnComplete, OnExit)
// keep their typed action slices for backwards compatibility. The Phase 2
// (ADR-0004) triggers all use GenericAction so new action kinds can be added
// without further model changes — engine.CompileStep is the single point of
// translation into typed engine.Action structs.
type StepEvents struct {
	OnEnter        []OnEnterAction        `json:"on_enter,omitempty" yaml:"on_enter,omitempty"`
	OnTurnStart    []OnTurnStartAction    `json:"on_turn_start,omitempty" yaml:"on_turn_start,omitempty"`
	OnTurnComplete []OnTurnCompleteAction `json:"on_turn_complete,omitempty" yaml:"on_turn_complete,omitempty"`
	OnExit         []OnExitAction         `json:"on_exit,omitempty" yaml:"on_exit,omitempty"`

	// Phase 2 (ADR-0004) — new event-driven triggers. Empty slices keep
	// today's kanban behaviour; the engine simply finds no actions for the
	// trigger and exits.
	OnComment           []GenericAction `json:"on_comment,omitempty" yaml:"on_comment,omitempty"`
	OnBlockerResolved   []GenericAction `json:"on_blocker_resolved,omitempty" yaml:"on_blocker_resolved,omitempty"`
	OnChildrenCompleted []GenericAction `json:"on_children_completed,omitempty" yaml:"on_children_completed,omitempty"`
	OnApprovalResolved  []GenericAction `json:"on_approval_resolved,omitempty" yaml:"on_approval_resolved,omitempty"`
	OnHeartbeat         []GenericAction `json:"on_heartbeat,omitempty" yaml:"on_heartbeat,omitempty"`
	OnBudgetAlert       []GenericAction `json:"on_budget_alert,omitempty" yaml:"on_budget_alert,omitempty"`
	OnAgentError        []GenericAction `json:"on_agent_error,omitempty" yaml:"on_agent_error,omitempty"`
}

// ReviewStatus represents the review state of a session
type ReviewStatus string

const (
	ReviewStatusPending          ReviewStatus = "pending"
	ReviewStatusChangesRequested ReviewStatus = "changes_requested"
	ReviewStatusApproved         ReviewStatus = "approved"
)

// WorkflowTemplate represents a pre-defined workflow type that workflows can adopt
type WorkflowTemplate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsSystem    bool   `json:"is_system"`
	// Hidden marks templates that produce hidden workflow instances
	// (excluded from management UI and pickers). Used by system-only flows.
	// Not persisted to DB; sourced from the embedded YAML at load time.
	Hidden    bool             `json:"hidden,omitempty"`
	Steps     []StepDefinition `json:"steps"` // JSON stored
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// StepDefinition represents a step in a workflow template (stored as JSON in WorkflowTemplate)
type StepDefinition struct {
	ID                    string     `json:"id"`
	Name                  string     `json:"name"`
	Position              int        `json:"position"`
	Color                 string     `json:"color"`
	Prompt                string     `json:"prompt,omitempty"`
	Events                StepEvents `json:"events"`
	AllowManualMove       bool       `json:"allow_manual_move"`
	IsStartStep           bool       `json:"is_start_step"`
	ShowInCommandPanel    bool       `json:"show_in_command_panel"`
	AutoArchiveAfterHours int        `json:"auto_archive_after_hours,omitempty"`
	AgentProfileID        string     `json:"agent_profile_id,omitempty"`
	// StageType mirrors WorkflowStep.StageType for templates so the office
	// default + coordination workflows can declare their UX role
	// ("work", "review", "approval", "custom") in YAML.
	StageType StageType `json:"stage_type,omitempty"`
	// AutoAdvanceRequiresSignal gates on_turn_complete transitions on an
	// explicit `step_complete_kandev` MCP signal from the agent (ADR 0015).
	AutoAdvanceRequiresSignal bool `json:"auto_advance_requires_signal,omitempty" yaml:"auto_advance_requires_signal,omitempty"`
}

// WorkflowStep represents a step in a workflow
type WorkflowStep struct {
	ID                    string     `json:"id"`
	WorkflowID            string     `json:"workflow_id"`
	Name                  string     `json:"name"`
	Position              int        `json:"position"`
	Color                 string     `json:"color"`
	Prompt                string     `json:"prompt,omitempty"`
	Events                StepEvents `json:"events"`
	AllowManualMove       bool       `json:"allow_manual_move"`
	IsStartStep           bool       `json:"is_start_step"`
	ShowInCommandPanel    bool       `json:"show_in_command_panel"`
	AutoArchiveAfterHours int        `json:"auto_archive_after_hours,omitempty"`
	AgentProfileID        string     `json:"agent_profile_id,omitempty"`
	// StageType is a Phase 2 (ADR-0004) semantic hint for the frontend
	// ("work", "review", "approval", "custom"). The engine does not branch
	// on it. Stored as TEXT in workflow_steps.stage_type, defaulting to
	// "custom" so existing rows remain unchanged.
	StageType StageType `json:"stage_type,omitempty"`
	// AutoAdvanceRequiresSignal gates on_turn_complete transitions on an
	// explicit `step_complete_kandev` MCP signal from the agent (ADR 0015).
	// When true, bare turn-end does NOT trigger the step's transition
	// actions; instead the orchestrator waits for the agent (or a manual
	// UI fallback) to write the pending-signal bag on TaskSession.Metadata.
	AutoAdvanceRequiresSignal bool      `json:"auto_advance_requires_signal"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

// HasOnEnterAction checks if the step has a specific on_enter action type.
func (s *WorkflowStep) HasOnEnterAction(actionType OnEnterActionType) bool {
	for _, action := range s.Events.OnEnter {
		if action.Type == actionType {
			return true
		}
	}
	return false
}

// HasOnTurnStartAction checks if the step has any on_turn_start actions.
func (s *WorkflowStep) HasOnTurnStartAction() bool {
	return len(s.Events.OnTurnStart) > 0
}

// HasOnTurnCompleteAction checks if the step has a specific on_turn_complete action type.
func (s *WorkflowStep) HasOnTurnCompleteAction(actionType OnTurnCompleteActionType) bool {
	for _, action := range s.Events.OnTurnComplete {
		if action.Type == actionType {
			return true
		}
	}
	return false
}

// StepTransitionTrigger represents how a session moved between steps
type StepTransitionTrigger string

const (
	StepTransitionTriggerManual       StepTransitionTrigger = "manual"
	StepTransitionTriggerAutoComplete StepTransitionTrigger = "auto_complete"
	StepTransitionTriggerApproval     StepTransitionTrigger = "approval"
)

// SessionStepHistory represents an audit trail entry for session step transitions
type SessionStepHistory struct {
	ID         int64                  `json:"id"`
	SessionID  string                 `json:"session_id"`
	FromStepID *string                `json:"from_step_id,omitempty"`
	ToStepID   string                 `json:"to_step_id"`
	Trigger    StepTransitionTrigger  `json:"trigger"`
	ActorID    *string                `json:"actor_id,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

// RemapStepEvents returns a copy of events with all step_id references
// in move_to_step actions replaced using the provided ID mapping.
func RemapStepEvents(events StepEvents, idMap map[string]string) StepEvents {
	result := StepEvents{}
	result.OnEnter = append(result.OnEnter, events.OnEnter...)
	for _, a := range events.OnTurnStart {
		if a.Type == OnTurnStartMoveToStep {
			a.Config = remapActionStepID(a.Config, idMap)
		}
		result.OnTurnStart = append(result.OnTurnStart, a)
	}
	for _, a := range events.OnTurnComplete {
		if a.Type == OnTurnCompleteMoveToStep {
			a.Config = remapActionStepID(a.Config, idMap)
		}
		result.OnTurnComplete = append(result.OnTurnComplete, a)
	}
	result.OnExit = append(result.OnExit, events.OnExit...)
	result.OnComment = remapGenericStepEvents(events.OnComment, idMap)
	result.OnBlockerResolved = remapGenericStepEvents(events.OnBlockerResolved, idMap)
	result.OnChildrenCompleted = remapGenericStepEvents(events.OnChildrenCompleted, idMap)
	result.OnApprovalResolved = remapGenericStepEvents(events.OnApprovalResolved, idMap)
	result.OnHeartbeat = remapGenericStepEvents(events.OnHeartbeat, idMap)
	result.OnBudgetAlert = remapGenericStepEvents(events.OnBudgetAlert, idMap)
	result.OnAgentError = remapGenericStepEvents(events.OnAgentError, idMap)
	return result
}

func remapGenericStepEvents(actions []GenericAction, idMap map[string]string) []GenericAction {
	result := make([]GenericAction, 0, len(actions))
	for _, action := range actions {
		if action.Type == GenericActionMoveToStep {
			action.Config = remapActionStepID(action.Config, idMap)
		}
		result = append(result, action)
	}
	return result
}

func remapActionStepID(config map[string]any, idMap map[string]string) map[string]any {
	if config == nil {
		return nil
	}
	stepID, ok := config["step_id"].(string)
	if !ok {
		return config
	}
	newID, found := idMap[stepID]
	if !found {
		return config
	}
	cfg := make(map[string]any, len(config))
	maps.Copy(cfg, config)
	cfg["step_id"] = newID
	return cfg
}
