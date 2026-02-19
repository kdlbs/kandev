package messagequeue

import "time"

// QueuedMessage represents a message queued for a session
type QueuedMessage struct {
	ID          string              `json:"id"`          // Unique queue entry ID
	SessionID   string              `json:"session_id"`  // Task session ID
	TaskID      string              `json:"task_id"`     // Task ID
	Content     string              `json:"content"`     // Message content
	Model       string              `json:"model"`       // Optional model override
	PlanMode    bool                `json:"plan_mode"`   // Plan mode enabled
	Attachments []MessageAttachment `json:"attachments"` // Image attachments
	QueuedAt    time.Time           `json:"queued_at"`   // When queued
	QueuedBy    string              `json:"queued_by"`   // User ID who queued
}

// MessageAttachment represents an attachment (image) in a queued message
type MessageAttachment struct {
	Type     string `json:"type"`      // "image"
	Data     string `json:"data"`      // Base64 data
	MimeType string `json:"mime_type"` // MIME type
}

// QueueStatus represents the queue status for a session
type QueueStatus struct {
	IsQueued bool           `json:"is_queued"` // Whether a message is queued
	Message  *QueuedMessage `json:"message"`   // The queued message (nil if not queued)
}
