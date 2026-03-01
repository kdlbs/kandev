package models

import "time"

// UtilityAgent represents a configured utility agent for quick one-shot tasks.
// It references an inference-capable agent (like claude-code, amp) by ID.
type UtilityAgent struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Prompt      string    `json:"prompt" db:"prompt"`
	AgentID     string    `json:"agent_id" db:"agent_id"` // Reference to inference agent (e.g., "claude-code")
	Model       string    `json:"model" db:"model"`       // Model to use (e.g., "claude-haiku-4-5")
	Builtin     bool      `json:"builtin" db:"builtin"`   // Built-in agents cannot be deleted
	Enabled     bool      `json:"enabled" db:"enabled"`   // Whether agent is enabled
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// UtilityAgentCall represents a single invocation of a utility agent.
type UtilityAgentCall struct {
	ID             string     `json:"id" db:"id"`
	UtilityID      string     `json:"utility_id" db:"utility_id"`
	SessionID      string     `json:"session_id" db:"session_id"`
	ResolvedPrompt string     `json:"resolved_prompt" db:"resolved_prompt"`
	Response       string     `json:"response" db:"response"`
	Model          string     `json:"model" db:"model"`
	PromptTokens   int        `json:"prompt_tokens" db:"prompt_tokens"`
	ResponseTokens int        `json:"response_tokens" db:"response_tokens"`
	DurationMs     int        `json:"duration_ms" db:"duration_ms"`
	Status         string     `json:"status" db:"status"` // "pending", "completed", "failed"
	ErrorMessage   string     `json:"error_message" db:"error_message"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	CompletedAt    *time.Time `json:"completed_at" db:"completed_at"`
}
