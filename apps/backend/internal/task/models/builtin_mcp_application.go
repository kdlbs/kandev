package models

import "time"

// BuiltinMCPApplication records saved intent separately from execution evidence.
type BuiltinMCPApplication struct {
	SessionID              string     `json:"session_id" db:"session_id"`
	TaskID                 string     `json:"task_id" db:"task_id"`
	WorkspaceID            string     `json:"workspace_id" db:"workspace_id"`
	TargetRevision         int64      `json:"target_revision" db:"target_revision"`
	PolicyRevision         int64      `json:"policy_revision" db:"policy_revision"`
	SelectionHash          string     `json:"-" db:"selection_hash"`
	RequestedBy            string     `json:"-" db:"requested_by"`
	OperationID            string     `json:"operation_id,omitempty" db:"operation_id"`
	AttemptSource          string     `json:"-" db:"attempt_source"`
	AttemptActorID         string     `json:"-" db:"attempt_actor_id"`
	AttemptSelectionHash   string     `json:"-" db:"attempt_selection_hash"`
	AttemptPolicyRevision  int64      `json:"-" db:"attempt_policy_revision"`
	TargetBindingJSON      string     `json:"-" db:"target_binding_json"`
	TargetBindingID        string     `json:"-" db:"target_binding_id"`
	VerificationDeadline   *time.Time `json:"-" db:"verification_deadline"`
	RetryOfOperationID     string     `json:"-" db:"retry_of_operation_id"`
	RequestID              string     `json:"request_id" db:"request_id"`
	AttemptGeneration      int64      `json:"attempt_generation" db:"attempt_generation"`
	AttemptRevision        int64      `json:"attempt_revision" db:"attempt_revision"`
	SourceExecutionID      string     `json:"source_execution_id,omitempty" db:"source_execution_id"`
	ReplacementExecutionID string     `json:"replacement_execution_id,omitempty" db:"replacement_execution_id"`
	Stage                  string     `json:"stage" db:"stage"`
	Reason                 string     `json:"reason,omitempty" db:"reason"`
	RowVersion             int64      `json:"row_version" db:"row_version"`
	CreatedAt              time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at" db:"updated_at"`
}

// BuiltinMCPRetryTarget comes from the saved task selection and current policy.
type BuiltinMCPRetryTarget struct {
	TaskRevision   int64
	PolicyRevision int64
	SelectionHash  string
}

const (
	BuiltinMCPApplicationPending    = "pending"
	BuiltinMCPApplicationPreparing  = "preparing"
	BuiltinMCPApplicationRetiring   = "retiring"
	BuiltinMCPApplicationStarting   = "starting"
	BuiltinMCPApplicationVerifying  = "verifying"
	BuiltinMCPApplicationAccepting  = "accepting"
	BuiltinMCPApplicationApplied    = "applied"
	BuiltinMCPApplicationFailed     = "failed"
	BuiltinMCPApplicationSuperseded = "superseded"
)
