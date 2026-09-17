package models

import (
	"errors"
	"time"
)

var ErrConflict = errors.New("assistant revision or idempotency conflict")

type AssistantBinding struct {
	ID             string    `json:"id" db:"id"`
	OwnerUserID    string    `json:"owner_user_id" db:"owner_user_id"`
	OrchestratorID string    `json:"orchestrator_id" db:"orchestrator_id"`
	WorkspaceID    string    `json:"home_workspace_id" db:"workspace_id"`
	ConversationID string    `json:"conversation_id" db:"conversation_id"`
	Version        int64     `json:"version" db:"version"`
	ExecutionMode  string    `json:"execution_mode" db:"execution_mode"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// AssistantAuthority is the immutable admission evidence captured by a run.
// Revision fingerprints configuration without exposing configuration values.
type AssistantAuthority struct {
	BindingID         string `json:"binding_id"`
	BindingVersion    int64  `json:"binding_version"`
	IntentRevision    int64  `json:"intent_revision"`
	Revision          string `json:"revision"`
	Restriction       string `json:"restriction"`
	UnsupportedReason string `json:"unsupported_reason,omitempty"`
	ProfileID         string `json:"profile_id"`
	ExecutorID        string `json:"executor_id"`
	Mode              string `json:"mode"`
}

type Intake struct {
	CommentID       string `db:"comment_id"`
	TaskID          string `db:"task_id"`
	AgentID         string `db:"agent_id"`
	OwnerUserID     string `db:"owner_user_id"`
	ClientMessageID string `db:"client_message_id"`
	PayloadHash     string `db:"payload_hash"`
	Sequence        int64  `db:"sequence"`
	Status          string `db:"status"`
	RunID           string `db:"run_id"`
}
