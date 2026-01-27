package models

import (
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// StepType represents the semantic type of a workflow step
type StepType string

const (
	StepTypeBacklog        StepType = "backlog"
	StepTypePlanning       StepType = "planning"
	StepTypeImplementation StepType = "implementation"
	StepTypeReview         StepType = "review"
	StepTypeVerification   StepType = "verification"
	StepTypeDone           StepType = "done"
	StepTypeBlocked        StepType = "blocked"
)

// ReviewStatus represents the review state of a session
type ReviewStatus string

const (
	ReviewStatusPending          ReviewStatus = "pending"
	ReviewStatusChangesRequested ReviewStatus = "changes_requested"
	ReviewStatusApproved         ReviewStatus = "approved"
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

// WorkflowStep represents a step in a board's workflow (replaces Column)
type WorkflowStep struct {
	ID               string       `json:"id"`
	BoardID          string       `json:"board_id"`
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
	OnCompleteStepID *string      `json:"on_complete_step_id,omitempty"`
	OnApprovalStepID *string      `json:"on_approval_step_id,omitempty"`
	AllowManualMove  bool         `json:"allow_manual_move"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
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

