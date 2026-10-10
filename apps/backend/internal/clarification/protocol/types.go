// Package protocol contains the wire types for clarification requests.
package protocol

import "time"

// Option represents a single choice option for a question.
type Option struct {
	ID          string `json:"option_id"`
	Label       string `json:"label"`       // Concise 1-5 words
	Description string `json:"description"` // Explanation of the option
}

// Question represents a single question with multiple choice options.
type Question struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`   // Short label (max 12 chars)
	Prompt          string   `json:"prompt"`  // Full question text
	Options         []Option `json:"options"` // 2-6 options
	AllowCustomText *bool    `json:"allow_custom_text,omitempty"`
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
