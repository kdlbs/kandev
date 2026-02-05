package v1

import "time"

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

// WorkflowStep represents a step in a board's workflow.
// This is the shared type used across task, orchestrator, and workflow packages.
type WorkflowStep struct {
	ID               string    `json:"id"`
	BoardID          string    `json:"board_id"`
	Name             string    `json:"name"`
	StepType         StepType  `json:"step_type"`
	Position         int       `json:"position"`
	Color            string    `json:"color"`
	TaskState        TaskState `json:"task_state"`
	AutoStartAgent   bool      `json:"auto_start_agent"`
	PlanMode         bool      `json:"plan_mode"`
	RequireApproval  bool      `json:"require_approval"`
	PromptPrefix     string    `json:"prompt_prefix,omitempty"`
	PromptSuffix     string    `json:"prompt_suffix,omitempty"`
	OnCompleteStepID *string   `json:"on_complete_step_id,omitempty"`
	OnApprovalStepID *string   `json:"on_approval_step_id,omitempty"`
	AllowManualMove  bool      `json:"allow_manual_move"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// WorkflowStepGetter retrieves workflow step information.
// This interface is implemented by workflow service and used by task and orchestrator services.
type WorkflowStepGetter interface {
	GetStep(stepID string) (*WorkflowStep, error)
	GetNextStepByPosition(boardID string, currentPosition int) (*WorkflowStep, error)
	GetSourceStep(boardID, targetStepID string) (*WorkflowStep, error)
}

// ReviewStatus represents the review state of a session
type ReviewStatus string

const (
	ReviewStatusPending          ReviewStatus = "pending"
	ReviewStatusChangesRequested ReviewStatus = "changes_requested"
	ReviewStatusApproved         ReviewStatus = "approved"
)

// StepTransitionTrigger represents how a session moved between steps
type StepTransitionTrigger string

const (
	StepTransitionTriggerManual       StepTransitionTrigger = "manual"
	StepTransitionTriggerAutoComplete StepTransitionTrigger = "auto_complete"
	StepTransitionTriggerApproval     StepTransitionTrigger = "approval"
)
