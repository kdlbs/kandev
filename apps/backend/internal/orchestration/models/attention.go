package models

import "time"

const AttentionPending = "pending"
const AttentionExpired = "expired"

// AttentionSource contains references and bounded safe text, never tool arguments.
type AttentionSource struct {
	SourceID       string `json:"source_id" db:"source_id"`
	SessionID      string `json:"session_id" db:"session_id"`
	Kind           string `json:"kind" db:"kind"`
	State          string `json:"state" db:"state"`
	SourceRevision string `json:"source_revision" db:"source_revision"`
	Summary        string `json:"summary" db:"summary"`
}
type Attention struct {
	AttentionSource
	ID                   string    `json:"id" db:"id"`
	BindingID            string    `json:"binding_id" db:"binding_id"`
	WorkspaceID          string    `json:"workspace_id" db:"workspace_id"`
	TaskID               string    `json:"task_id" db:"task_id"`
	Revision             int64     `json:"revision" db:"revision"`
	LastNotifiedRevision int64     `json:"last_notified_revision" db:"last_notified_revision"`
	UpdatedAt            time.Time `json:"updated_at" db:"updated_at"`
}
type AttentionTarget struct {
	BindingID string `db:"binding_id"`
	TaskID    string `db:"task_id"`
	Cursor    string `db:"cursor"`
}
type AttentionWake struct {
	ID             string `db:"id"`
	AttentionID    string `db:"attention_id"`
	SourceRevision string `db:"source_revision"`
	Revision       int64  `db:"revision"`
	State          string `db:"state"`
}
