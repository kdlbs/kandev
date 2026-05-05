// Package clarification provides types and services for agent clarification requests.
// This allows agents to ask structured questions to users and wait for responses.
package clarification

import (
	"time"
)

// Option represents a single choice option for a question.
type Option struct {
	ID          string `json:"option_id"`
	Label       string `json:"label"`       // Concise 1-5 words
	Description string `json:"description"` // Explanation of the option
}

// Question represents a single question with multiple choice options.
type Question struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`   // Short label (max 12 chars)
	Prompt  string   `json:"prompt"`  // Full question text
	Options []Option `json:"options"` // 2-6 options
}

// Request represents a clarification request from an agent. A request bundles
// one or more questions; the agent stays blocked until every question has been
// answered (or the bundle is rejected as a whole).
type Request struct {
	PendingID string     `json:"pending_id"`
	SessionID string     `json:"session_id"`
	TaskID    string     `json:"task_id"`
	Questions []Question `json:"questions"`         // 1-N questions, all required
	Context   string     `json:"context,omitempty"` // Optional shared context for all questions
	CreatedAt time.Time  `json:"created_at"`
}

// Answer represents the user's answer to a single question.
type Answer struct {
	QuestionID      string   `json:"question_id"`
	SelectedOptions []string `json:"selected_options,omitempty"` // Option IDs (single-choice ⇒ at most one)
	CustomText      string   `json:"custom_text,omitempty"`      // Free-text input
}

// Response represents the user's response to a clarification request.
// On success, Answers has exactly one entry per question in the request.
// On rejection, Answers may be nil and Rejected/RejectReason describe the skip.
type Response struct {
	PendingID    string    `json:"pending_id"`
	Answers      []Answer  `json:"answers,omitempty"`
	Rejected     bool      `json:"rejected,omitempty"`
	RejectReason string    `json:"reject_reason,omitempty"` // If rejected
	RespondedAt  time.Time `json:"responded_at"`
}

// PendingClarification represents a clarification request waiting for a response.
type PendingClarification struct {
	Request    *Request
	ResponseCh chan *Response
	CancelCh   chan struct{} // Closed when session's turn completes (agent moved on)
	CreatedAt  time.Time
}

// Status represents the status of a clarification request.
type Status string

const (
	StatusPending   Status = "pending"
	StatusAnswered  Status = "answered"
	StatusRejected  Status = "rejected"
	StatusExpired   Status = "expired"
	StatusCancelled Status = "cancelled"
)
