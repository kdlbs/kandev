package models

import (
	"errors"
	"time"
)

var (
	ErrConversationForkSourceUnavailable      = errors.New("conversation fork source is unavailable")
	ErrConversationForkCutoffUnavailable      = errors.New("conversation fork cutoff is unavailable")
	ErrConversationForkLimitExceeded          = errors.New("conversation fork exceeds storage limits")
	ErrConversationForkQuotaExceeded          = errors.New("conversation fork draft quota exceeded")
	ErrConversationForkExpired                = errors.New("conversation fork draft has expired")
	ErrConversationForkNotFound               = errors.New("conversation fork was not found")
	ErrConversationForkConflict               = errors.New("conversation fork request conflicts with an existing request")
	ErrConversationForkAttachmentMissing      = errors.New("selected conversation fork attachment is unavailable")
	ErrConversationForkToolEvidence           = errors.New("selected conversation fork tool evidence is unavailable")
	ErrConversationForkUnsupportedDestination = errors.New("conversation fork destination is unsupported")
)

type ConversationForkSourceRequest struct {
	SessionID             string
	CutoffMessageID       string
	StartMessageID        string
	IncludeToolEvidence   bool
	AttachmentCursor      string
	SelectedAttachmentIDs []string
}

type ConversationForkSource struct {
	SessionID            string
	TaskID               string
	WorkspaceID          string
	TaskTitle            string
	Revision             int64
	Messages             []*Message
	Attachments          []*TaskMessageAttachment
	AttachmentsHasMore   bool
	AttachmentCursor     string
	AttachmentCandidates []ConversationForkAttachment
	CutoffTurnComplete   bool
}

type ConversationForkCreateRequest struct {
	Source              ConversationForkSourceRequest
	DraftRequestID      string
	IncludeToolEvidence bool
	ModelID             string
	AttachmentIDs       []string
}

type ConversationForkAdmission struct {
	OwnerID              string
	WorkspaceID          string
	ForkID               string
	DestinationKind      string
	DestinationRequestID string
	RequestFingerprint   string
	DestinationTaskID    string
	DestinationSessionID string
}

type ConversationForkDescriptor struct {
	ID                    string                       `json:"id"`
	SourceTaskID          string                       `json:"source_task_id"`
	SourceSessionID       string                       `json:"source_session_id"`
	SourceMessageID       string                       `json:"source_message_id"`
	StartMessageID        string                       `json:"start_message_id,omitempty"`
	SourceTaskTitle       string                       `json:"source_task_title,omitempty"`
	SourceRevision        int64                        `json:"source_revision"`
	CompilerVersion       string                       `json:"compiler_version"`
	ContentHash           string                       `json:"content_hash"`
	MessageCount          int                          `json:"message_count"`
	TextBytes             int                          `json:"text_bytes"`
	Omissions             map[string]int               `json:"omissions"`
	AttachmentDescriptors []ConversationForkAttachment `json:"attachments"`
	Estimate              ConversationForkEstimate     `json:"estimate"`
	CreatedAt             time.Time                    `json:"created_at"`
	ExpiresAt             time.Time                    `json:"expires_at"`
	State                 string                       `json:"state"`
	DestinationKind       string                       `json:"destination_kind,omitempty"`
	DestinationTaskID     string                       `json:"destination_task_id,omitempty"`
	DestinationSessionID  string                       `json:"destination_session_id,omitempty"`
	DestinationComplete   bool                         `json:"-"`
}

type ConversationForkAttachment struct {
	SourceID     string `json:"source_id,omitempty"`
	ID           string `json:"id,omitempty"`
	Name         string `json:"name"`
	MediaType    string `json:"media_type,omitempty"`
	Kind         string `json:"kind,omitempty"`
	DeliveryMode string `json:"delivery_mode,omitempty"`
	Size         int64  `json:"size,omitempty"`
	Available    bool   `json:"available"`
}

type ConversationForkEstimate struct {
	EstimatedTokens       int64  `json:"estimated_tokens"`
	Method                string `json:"method"`
	ModelID               string `json:"model_id,omitempty"`
	ContextLimit          *int64 `json:"context_limit,omitempty"`
	LimitSource           string `json:"limit_source,omitempty"`
	AttachmentsUnmeasured bool   `json:"attachments_unmeasured"`
}

type ConversationForkDraft struct {
	Descriptor             ConversationForkDescriptor `json:"descriptor"`
	CompiledText           string                     `json:"compiled_text"`
	OwnerID                string                     `json:"-"`
	WorkspaceID            string                     `json:"-"`
	SelectionJSON          string                     `json:"-"`
	DraftRequestID         string                     `json:"-"`
	RequestFingerprint     string                     `json:"-"`
	DestinationRequestID   string                     `json:"-"`
	DestinationFingerprint string                     `json:"-"`
}

type ConversationForkExpiredDraftAttachments struct {
	OwnerID     string
	Attachments []ConversationForkAttachment
}
