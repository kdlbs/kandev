package models

import "time"

const TaskTransferPreservationPolicyV1 = "preserve-task-identity-v1"

type TaskTransferActorKind string

const (
	TaskTransferActorHuman       TaskTransferActorKind = "human"
	TaskTransferActorCoordinator TaskTransferActorKind = "coordinator"
	TaskTransferActorRejected    TaskTransferActorKind = "rejected"
)

// TaskTransferActor is server-attested caller identity. MCP request payloads
// never populate this structure.
type TaskTransferActor struct {
	Kind         TaskTransferActorKind `json:"kind"`
	ID           string                `json:"id"`
	SessionID    string                `json:"session_id,omitempty"`
	CallerTaskID string                `json:"-"`
}

// TaskTransferCommand binds every mutable placement predicate plus the exact
// destination and idempotency identity for one cross-workspace transfer.
type TaskTransferCommand struct {
	TaskID                    string            `json:"task_id"`
	ExpectedSourceWorkspaceID string            `json:"expected_source_workspace_id"`
	ExpectedSourceWorkflowID  string            `json:"expected_source_workflow_id"`
	ExpectedSourceStepID      string            `json:"expected_source_workflow_step_id"`
	ExpectedTaskUpdatedAt     time.Time         `json:"expected_task_updated_at"`
	DestinationWorkspaceID    string            `json:"destination_workspace_id"`
	DestinationWorkflowID     string            `json:"destination_workflow_id"`
	DestinationStepID         string            `json:"destination_workflow_step_id,omitempty"`
	DestinationStepName       string            `json:"destination_workflow_step_name,omitempty"`
	IdempotencyKey            string            `json:"idempotency_key"`
	PreservationPolicy        string            `json:"preservation_policy"`
	Actor                     TaskTransferActor `json:"-"`
	AuthorizedOwnerID         string            `json:"-"`
	OwnerPredicateSet         bool              `json:"-"`
}

type TaskTransferSessionReceipt struct {
	ID                string           `json:"id"`
	State             TaskSessionState `json:"state"`
	IsPrimary         bool             `json:"is_primary"`
	TaskEnvironmentID string           `json:"task_environment_id,omitempty"`
	TurnID            string           `json:"turn_id,omitempty"`
}

// TaskTransferReceipt is the immutable idempotent result persisted with the
// operation. It intentionally contains identifiers and counts, never message
// bodies, secrets, or other transcript content.
type TaskTransferReceipt struct {
	OperationID            string                       `json:"operation_id"`
	TaskID                 string                       `json:"task_id"`
	SourceWorkspaceID      string                       `json:"source_workspace_id"`
	SourceWorkflowID       string                       `json:"source_workflow_id"`
	SourceStepID           string                       `json:"source_workflow_step_id"`
	DestinationWorkspaceID string                       `json:"destination_workspace_id"`
	DestinationWorkflowID  string                       `json:"destination_workflow_id"`
	DestinationStepID      string                       `json:"destination_workflow_step_id"`
	DestinationStepName    string                       `json:"destination_workflow_step_name"`
	TaskGeneration         time.Time                    `json:"task_generation"`
	StepTransitionID       int64                        `json:"step_transition_id"`
	Sessions               []TaskTransferSessionReceipt `json:"sessions"`
	PreservationCounts     map[string]int               `json:"preservation_counts"`
	PreservationDigest     string                       `json:"preservation_digest"`
	IdempotencyKey         string                       `json:"idempotency_key"`
	PreservationPolicy     string                       `json:"preservation_policy"`
	TransferredAt          time.Time                    `json:"transferred_at"`
	IdempotentReplay       bool                         `json:"-"`
}
