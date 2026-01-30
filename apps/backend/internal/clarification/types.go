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
	Options []Option `json:"options"` // 2-4 options
}

// Request represents a clarification request from an agent.
type Request struct {
	PendingID string    `json:"pending_id"`
	SessionID string    `json:"session_id"`
	TaskID    string    `json:"task_id"`
	Question  Question  `json:"question"`          // Single question
	Context   string    `json:"context,omitempty"` // Optional context for the question
	CreatedAt time.Time `json:"created_at"`
}

// Answer represents the user's answer to a single question.
type Answer struct {
	QuestionID      string   `json:"question_id"`
	SelectedOptions []string `json:"selected_options,omitempty"` // Option IDs
	CustomText      string   `json:"custom_text,omitempty"`      // Free-text input
}

// Response represents the user's response to a clarification request.
type Response struct {
	PendingID    string    `json:"pending_id"`
	Answer       *Answer   `json:"answer,omitempty"`
	Rejected     bool      `json:"rejected,omitempty"`
	RejectReason string    `json:"reject_reason,omitempty"` // If rejected
	RespondedAt  time.Time `json:"responded_at"`
}

// PendingClarification represents a clarification request waiting for a response.
type PendingClarification struct {
	Request    *Request
	ResponseCh chan *Response
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

