package models

import v1 "github.com/kandev/kandev/pkg/api/v1"

const SessionMetaKeyInitialPromptPreview = "initial_prompt_preview"

// InitialPromptPreview is display-only submission data, never an agent prompt.
type InitialPromptPreview struct {
	Content     string                 `json:"content"`
	Attachments []v1.MessageAttachment `json:"attachments"`
}

// NewInitialPromptPreview copies file-backed descriptors without inline bytes.
// Callers must complete attachment claim admission before preparing the session.
func NewInitialPromptPreview(content string, attachments []v1.MessageAttachment) *InitialPromptPreview {
	preview := &InitialPromptPreview{Content: content, Attachments: []v1.MessageAttachment{}}
	for _, attachment := range attachments {
		if attachment.AttachmentID == "" {
			continue
		}
		preview.Attachments = append(preview.Attachments, v1.MessageAttachment{
			AttachmentID: attachment.AttachmentID, Type: attachment.Type,
			Name: attachment.Name, MimeType: attachment.MimeType,
			SizeBytes: attachment.SizeBytes, DeliveryMode: attachment.DeliveryMode,
		})
	}
	return preview
}
