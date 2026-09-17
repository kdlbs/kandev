package models

import "time"

type AgentMemory struct {
	OwnerUserID     string     `json:"owner_user_id" db:"owner_user_id"`
	Scope           string     `json:"scope" db:"scope"`
	ScopeID         string     `json:"scope_id" db:"scope_id"`
	SourceCommentID string     `json:"source_comment_id" db:"source_comment_id"`
	Revision        int64      `json:"revision" db:"revision"`
	Confirmed       bool       `json:"confirmed" db:"confirmed"`
	Priority        int        `json:"priority" db:"priority"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	ForgottenAt     *time.Time `json:"-" db:"forgotten_at"`
	ID              string     `json:"id" db:"id"`
	AgentProfileID  string     `json:"agent_profile_id" db:"agent_profile_id"`
	Layer           string     `json:"layer" db:"layer"`
	Key             string     `json:"key" db:"key"`
	Content         string     `json:"content" db:"content"`
	Metadata        string     `json:"metadata" db:"metadata"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}
type TaskComment struct {
	ClientMessageID string    `json:"client_message_id,omitempty" db:"client_message_id"`
	Sequence        int64     `json:"sequence,omitempty" db:"sequence"`
	IntentRevision  int64     `json:"intent_revision,omitempty" db:"intent_revision"`
	ReceiptStatus   string    `json:"receipt_status,omitempty" db:"receipt_status"`
	RunID           string    `json:"run_id,omitempty" db:"run_id"`
	RunStatus       string    `json:"run_status,omitempty" db:"-"`
	RunError        string    `json:"run_error,omitempty" db:"-"`
	ID              string    `json:"id" db:"id"`
	TaskID          string    `json:"task_id" db:"task_id"`
	AuthorType      string    `json:"author_type" db:"author_type"`
	AuthorID        string    `json:"author_id" db:"author_id"`
	Body            string    `json:"body" db:"body"`
	Source          string    `json:"source" db:"source"`
	ReplyChannelID  string    `json:"reply_channel_id" db:"reply_channel_id"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}
