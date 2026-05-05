package orchestrator

import v1 "github.com/kandev/kandev/pkg/api/v1"

// UserMessageMeta holds metadata fields for a user message.
// Use NewUserMessageMeta to construct and ToMap to serialize.
type UserMessageMeta struct {
	PlanMode          bool
	HasReviewComments bool
	Attachments       []v1.MessageAttachment
	ContextFiles      []v1.ContextFileMeta
	SenderTaskID      string
	SenderTaskTitle   string
	SenderSessionID   string
}

// NewUserMessageMeta creates a UserMessageMeta builder.
func NewUserMessageMeta() *UserMessageMeta {
	return &UserMessageMeta{}
}

// WithPlanMode marks the message as having plan mode enabled.
func (m *UserMessageMeta) WithPlanMode(enabled bool) *UserMessageMeta {
	m.PlanMode = enabled
	return m
}

// WithReviewComments marks the message as containing review comments.
func (m *UserMessageMeta) WithReviewComments(has bool) *UserMessageMeta {
	m.HasReviewComments = has
	return m
}

// WithAttachments sets the message attachments.
func (m *UserMessageMeta) WithAttachments(attachments []v1.MessageAttachment) *UserMessageMeta {
	m.Attachments = attachments
	return m
}

// WithContextFiles sets the context files attached to the message.
func (m *UserMessageMeta) WithContextFiles(files []v1.ContextFileMeta) *UserMessageMeta {
	m.ContextFiles = files
	return m
}

// WithSenderTask records that the message originated from another task's agent
// (via the message_task_kandev MCP tool). Title is a snapshot at send time;
// session ID is optional and identifies the sender's specific session.
func (m *UserMessageMeta) WithSenderTask(taskID, taskTitle, sessionID string) *UserMessageMeta {
	m.SenderTaskID = taskID
	m.SenderTaskTitle = taskTitle
	m.SenderSessionID = sessionID
	return m
}

// ToMap returns the metadata as a map suitable for message creation.
// Returns nil if no metadata fields are set.
func (m *UserMessageMeta) ToMap() map[string]interface{} {
	if !m.PlanMode && !m.HasReviewComments && len(m.Attachments) == 0 && len(m.ContextFiles) == 0 && m.SenderTaskID == "" {
		return nil
	}
	meta := make(map[string]interface{})
	if m.PlanMode {
		meta["plan_mode"] = true
	}
	if m.HasReviewComments {
		meta["has_review_comments"] = true
	}
	if len(m.Attachments) > 0 {
		meta["attachments"] = m.Attachments
	}
	if len(m.ContextFiles) > 0 {
		meta["context_files"] = m.ContextFiles
	}
	if m.SenderTaskID != "" {
		meta["sender_task_id"] = m.SenderTaskID
		meta["sender_task_title"] = m.SenderTaskTitle
		if m.SenderSessionID != "" {
			meta["sender_session_id"] = m.SenderSessionID
		}
	}
	return meta
}

// mergeMetadata returns a single metadata map combining base and extra. extra
// values take precedence on key collision. Returns nil only when both inputs
// are nil/empty so callers can keep the "no metadata" branch in the message
// store.
func mergeMetadata(base, extra map[string]interface{}) map[string]interface{} {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	if len(extra) == 0 {
		return base
	}
	if len(base) == 0 {
		return extra
	}
	merged := make(map[string]interface{}, len(base)+len(extra))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range extra {
		merged[k] = v
	}
	return merged
}
