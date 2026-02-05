package models

import (
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Type aliases from shared v1 package to maintain backward compatibility
type (
	StepType              = v1.StepType
	ReviewStatus          = v1.ReviewStatus
	StepTransitionTrigger = v1.StepTransitionTrigger
	WorkflowStep          = v1.WorkflowStep
)

// Re-export constants from v1 for backward compatibility
const (
	StepTypeBacklog        = v1.StepTypeBacklog
	StepTypePlanning       = v1.StepTypePlanning
	StepTypeImplementation = v1.StepTypeImplementation
	StepTypeReview         = v1.StepTypeReview
	StepTypeVerification   = v1.StepTypeVerification
	StepTypeDone           = v1.StepTypeDone
	StepTypeBlocked        = v1.StepTypeBlocked

	ReviewStatusPending          = v1.ReviewStatusPending
	ReviewStatusChangesRequested = v1.ReviewStatusChangesRequested
	ReviewStatusApproved         = v1.ReviewStatusApproved

	StepTransitionTriggerManual       = v1.StepTransitionTriggerManual
	StepTransitionTriggerAutoComplete = v1.StepTransitionTriggerAutoComplete
	StepTransitionTriggerApproval     = v1.StepTransitionTriggerApproval
)

// WorkflowTemplate represents a pre-defined workflow type that boards can adopt
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
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	StepType         StepType     `json:"step_type"`
	Position         int          `json:"position"`
	Color            string       `json:"color"`
	TaskState        v1.TaskState `json:"task_state"`
	AutoStartAgent   bool         `json:"auto_start_agent"`
	PlanMode         bool         `json:"plan_mode"`
	RequireApproval  bool         `json:"require_approval"`
	PromptPrefix     string       `json:"prompt_prefix,omitempty"`
	PromptSuffix     string       `json:"prompt_suffix,omitempty"`
	OnCompleteStepID string       `json:"on_complete_step_id,omitempty"`
	OnApprovalStepID string       `json:"on_approval_step_id,omitempty"`
	AllowManualMove  bool         `json:"allow_manual_move"`
}

// WorkflowStep is now an alias to v1.WorkflowStep (defined above in type aliases)

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

