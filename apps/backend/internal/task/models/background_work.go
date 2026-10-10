package models

import (
	"time"
)

// BackgroundWorkload is the durable projection model for an agent background workload.
type BackgroundWorkload struct {
	ID              string     `json:"id" db:"id"`
	TaskID          string     `json:"task_id" db:"task_id"`
	SessionID       string     `json:"session_id" db:"session_id"`
	Kind            string     `json:"kind" db:"kind"`
	Title           string     `json:"title" db:"title"`
	State           string     `json:"state" db:"state"`
	ParentWorkID    *string    `json:"parent_work_id,omitempty" db:"parent_work_id"`
	OriginTurnID    string     `json:"origin_turn_id,omitempty" db:"origin_turn_id"`
	SourceMessageID string     `json:"source_message_id,omitempty" db:"source_message_id"`
	SourceCallID    string     `json:"source_call_id,omitempty" db:"source_call_id"`
	ExitCode        *int       `json:"exit_code,omitempty" db:"exit_code"`
	Output          string     `json:"output,omitempty" db:"output"`
	OutputTruncated bool       `json:"output_truncated,omitempty" db:"output_truncated"`
	OutputOffset    int64      `json:"output_offset,omitempty" db:"output_offset"`
	StartedAt       *time.Time `json:"started_at,omitempty" db:"started_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty" db:"finished_at"`
	Revision        int64      `json:"revision" db:"revision"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// BackgroundRun tracks an individual execution run of a workload.
type BackgroundRun struct {
	ID             string     `json:"id" db:"id"`
	WorkloadID     string     `json:"workload_id" db:"workload_id"`
	SessionID      string     `json:"session_id" db:"session_id"`
	ProviderRunKey string     `json:"provider_run_key,omitempty" db:"provider_run_key"`
	State          string     `json:"state" db:"state"`
	ExitCode       *int       `json:"exit_code,omitempty" db:"exit_code"`
	StartedAt      *time.Time `json:"started_at,omitempty" db:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty" db:"finished_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

// BackgroundActionReceipt records an action execution attempt on a workload.
type BackgroundActionReceipt struct {
	ID          string    `json:"id" db:"id"`
	SessionID   string    `json:"session_id" db:"session_id"`
	WorkloadID  string    `json:"workload_id" db:"workload_id"`
	RunID       string    `json:"run_id,omitempty" db:"run_id"`
	Action      string    `json:"action" db:"action"`
	OperationID string    `json:"operation_id" db:"operation_id"`
	Status      string    `json:"status" db:"status"`
	Error       string    `json:"error,omitempty" db:"error"`
	Uncertain   bool      `json:"uncertain" db:"uncertain"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}
