package models

import (
	"maps"
	"time"
)

// OnEnterActionType represents the type of action to execute when entering a step.
type OnEnterActionType string

const (
	OnEnterEnablePlanMode OnEnterActionType = "enable_plan_mode"
	OnEnterAutoStartAgent OnEnterActionType = "auto_start_agent"
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
	Type   OnTurnStartActionType  `json:"type"`
	Config map[string]interface{} `json:"config,omitempty"`
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
	Type   OnEnterActionType      `json:"type"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// OnTurnCompleteAction represents an action to execute when an agent turn completes.
type OnTurnCompleteAction struct {
	Type   OnTurnCompleteActionType `json:"type"`
	Config map[string]interface{}   `json:"config,omitempty"`
}

// OnExitActionType represents the type of action to execute when leaving a step.
type OnExitActionType string

const (
	OnExitDisablePlanMode OnExitActionType = "disable_plan_mode"
)

// OnExitAction represents an action to execute when leaving a step.
type OnExitAction struct {
	Type   OnExitActionType       `json:"type"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// StepEvents contains event-driven actions for a workflow step.
type StepEvents struct {
	OnEnter        []OnEnterAction        `json:"on_enter,omitempty"`
	OnTurnStart    []OnTurnStartAction    `json:"on_turn_start,omitempty"`
	OnTurnComplete []OnTurnCompleteAction `json:"on_turn_complete,omitempty"`
	OnExit         []OnExitAction         `json:"on_exit,omitempty"`
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
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	IsSystem    bool             `json:"is_system"`
	Steps       []StepDefinition `json:"steps"` // JSON stored
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
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
	AutoArchiveAfterHours int        `json:"auto_archive_after_hours,omitempty"`
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
	AutoArchiveAfterHours int        `json:"auto_archive_after_hours,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
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
		if a.Type == OnTurnStartMoveToStep && a.Config != nil {
			if stepID, ok := a.Config["step_id"].(string); ok {
				if newID, found := idMap[stepID]; found {
					cfg := make(map[string]any, len(a.Config))
					maps.Copy(cfg, a.Config)
					cfg["step_id"] = newID
					a.Config = cfg
				}
			}
		}
		result.OnTurnStart = append(result.OnTurnStart, a)
	}
	for _, a := range events.OnTurnComplete {
		if a.Type == OnTurnCompleteMoveToStep && a.Config != nil {
			if stepID, ok := a.Config["step_id"].(string); ok {
				if newID, found := idMap[stepID]; found {
					cfg := make(map[string]any, len(a.Config))
					maps.Copy(cfg, a.Config)
					cfg["step_id"] = newID
					a.Config = cfg
				}
			}
		}
		result.OnTurnComplete = append(result.OnTurnComplete, a)
	}
	result.OnExit = append(result.OnExit, events.OnExit...)
	return result
}
