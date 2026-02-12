package orchestrator

import v1 "github.com/kandev/kandev/pkg/api/v1"

// UserMessageMeta holds metadata fields for a user message.
// Use NewUserMessageMeta to construct and ToMap to serialize.
type UserMessageMeta struct {
	PlanMode          bool
	HasReviewComments bool
	Attachments       []v1.MessageAttachment
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

// ToMap returns the metadata as a map suitable for message creation.
// Returns nil if no metadata fields are set.
func (m *UserMessageMeta) ToMap() map[string]interface{} {
	if !m.PlanMode && !m.HasReviewComments && len(m.Attachments) == 0 {
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
	return meta
}
