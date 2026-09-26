package models

import "time"

const (
	ProfileBindingInherit      = "inherit"
	ProfileBindingExplicit     = "explicit"
	ProfileBindingUnconfigured = "unconfigured"
)

// UtilityAgent represents a configured utility agent for quick one-shot tasks.
// It references an inference-capable agent (like claude-acp, amp) by ID.
type UtilityAgent struct {
	ID                  string    `json:"id" db:"id"`
	Name                string    `json:"name" db:"name"`
	Description         string    `json:"description" db:"description"`
	Prompt              string    `json:"prompt" db:"prompt"`
	AgentID             string    `json:"agent_id" db:"agent_id"` // Reference to inference agent (e.g., "claude-acp")
	Model               string    `json:"model" db:"model"`       // Model to use (e.g., "claude-haiku-4-5")
	AgentProfileID      string    `json:"agent_profile_id" db:"agent_profile_id"`
	ProfileBindingState string    `json:"profile_binding_state" db:"profile_binding_state"`
	Builtin             bool      `json:"builtin" db:"builtin"` // Built-in agents cannot be deleted
	Enabled             bool      `json:"enabled" db:"enabled"` // Whether agent is enabled
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time `json:"updated_at" db:"updated_at"`
}

// UsesDefaultProfile reports whether a built-in utility action has an explicit
// inherited binding and therefore resolves through the global default. An
// empty unconfigured row may be a stale binding from an older release that
// erased its profile ID, so it must remain fail-closed.
func UsesDefaultProfile(agent *UtilityAgent) bool {
	return agent != nil && agent.Builtin && agent.AgentProfileID == "" &&
		agent.ProfileBindingState == ProfileBindingInherit
}

// UtilityAgentCall represents a single invocation of a utility agent.
type UtilityAgentCall struct {
	ID                 string     `json:"id" db:"id"`
	UtilityID          string     `json:"utility_id" db:"utility_id"`
	SessionID          string     `json:"session_id" db:"session_id"`
	ResolvedPrompt     string     `json:"resolved_prompt" db:"resolved_prompt"`
	Response           string     `json:"response" db:"response"`
	Model              string     `json:"model" db:"model"`
	AgentProfileID     string     `json:"agent_profile_id" db:"agent_profile_id"`
	ExecutionProfileID string     `json:"execution_profile_id,omitempty" db:"execution_profile_id"`
	PromptTokens       int        `json:"prompt_tokens" db:"prompt_tokens"`
	ResponseTokens     int        `json:"response_tokens" db:"response_tokens"`
	DurationMs         int        `json:"duration_ms" db:"duration_ms"`
	Status             string     `json:"status" db:"status"` // "pending", "completed", "failed"
	ErrorMessage       string     `json:"error_message" db:"error_message"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	CompletedAt        *time.Time `json:"completed_at" db:"completed_at"`
}
