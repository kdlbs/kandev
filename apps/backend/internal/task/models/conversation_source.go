package models

// ConversationMutationKind identifies the source operation represented by a
// transient mutation receipt.
type ConversationMutationKind string

const (
	ConversationMutationUpsert ConversationMutationKind = "upsert"
	ConversationMutationRemove ConversationMutationKind = "remove"
)

// ConversationEntityKind identifies which source table an operation changes.
type ConversationEntityKind string

const (
	ConversationEntityMessage ConversationEntityKind = "message"
	ConversationEntityTurn    ConversationEntityKind = "turn"
)

// ConversationMutationOperation is the in-memory projection of one source
// mutation. PreviousSessionID is set for a move so a subscriber can remove
// the old selection before applying the new one.
type ConversationMutationOperation struct {
	Kind              ConversationMutationKind `json:"kind"`
	Entity            ConversationEntityKind   `json:"entity"`
	ID                string                   `json:"id"`
	SessionID         string                   `json:"session_id"`
	TaskID            string                   `json:"task_id,omitempty"`
	AuthorType        string                   `json:"author_type,omitempty"`
	PreviousSessionID string                   `json:"previous_session_id,omitempty"`
	Message           *Message                 `json:"message,omitempty"`
	Turn              *Turn                    `json:"turn,omitempty"`
}

// ConversationMutationReceipt describes a complete revision interval for
// one session. Receipts are transient and are published only after the source
// transaction commits. Complete is false when a caller changed more rows
// than it could describe, in which case delivery must request reconciliation.
type ConversationMutationReceipt struct {
	SessionID    string                          `json:"session_id"`
	BaseRevision int64                           `json:"base_revision"`
	Revision     int64                           `json:"revision"`
	Operations   []ConversationMutationOperation `json:"operations"`
	Complete     bool                            `json:"complete"`
}

// ConversationMessagePageRequest describes one bounded source-backed message
// read. CursorID is an opaque boundary to callers; the repository uses it only
// to apply the requested keyset comparison inside the read transaction.
type ConversationMessagePageRequest struct {
	SessionID string
	TaskID    *string
	Authors   []string
	Sort      string
	CursorID  string
	Limit     int
}

// ConversationMessagePage is a source snapshot and its consistency revision.
type ConversationMessagePage struct {
	Messages []*Message
	HasMore  bool
	Revision int64
	CursorID string
}

// ConversationTurnPageRequest describes one bounded source-backed turn read.
type ConversationTurnPageRequest struct {
	SessionID string
	TaskID    *string
	Sort      string
	CursorID  string
	Limit     int
}

// ConversationTurnPage is a source snapshot and its consistency revision.
type ConversationTurnPage struct {
	Turns    []*Turn
	HasMore  bool
	Revision int64
	CursorID string
}

// ConversationRevision reports whether a session exists and its current
// source revision. Missing revision rows are revision zero.
type ConversationRevision struct {
	SessionID string
	Revision  int64
	Exists    bool
}
