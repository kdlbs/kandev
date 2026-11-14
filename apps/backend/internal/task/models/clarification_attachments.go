package models

// ClarificationAttachmentClaim identifies the ordered answer files attached to
// one question in a clarification bundle. The task service derives ownership
// from authenticated state; the repository validates every ID in the same
// transaction that claims the response.
type ClarificationAttachmentClaim struct {
	QuestionID    string
	AttachmentIDs []string
}

// ClarificationAttachmentScope is the authenticated and persisted identity a
// clarification response claim must match. It is never taken from a client
// supplied answer descriptor.
type ClarificationAttachmentScope struct {
	OwnerID     string
	WorkspaceID string
	TaskID      string
	SessionID   string
}

// ClarificationAnswerAttachment is the canonical, durable projection of one
// file-backed answer attachment. Executor paths are deliberately absent.
type ClarificationAnswerAttachment struct {
	AttachmentID string `json:"attachment_id"`
	Type         string `json:"type"`
	Name         string `json:"name"`
	MimeType     string `json:"mime_type"`
	SizeBytes    int64  `json:"size_bytes"`
	DeliveryMode string `json:"delivery_mode"`
}
