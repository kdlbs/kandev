// Package hostutility manages long-lived per-agent-type agentctl instances
// used for boot-time capability probes, on-demand refresh, and sessionless
// utility prompt execution (e.g. "enhance prompt" before a task/session exists).
//
// Each enabled ACP-capable agent type gets one warm agentctl instance in a
// process-scoped tmp directory. Calls spawn ephemeral ACP subprocesses through
// that instance, exactly like task-scoped utility calls, but without needing a
// real workspace or task session.
package hostutility

import "time"

// Status reports the state of a host utility instance for a given agent type.
type Status string

const (
	StatusOK            Status = "ok"
	StatusAuthRequired  Status = "auth_required"
	StatusNotInstalled  Status = "not_installed"
	StatusFailed        Status = "failed"
	StatusNotConfigured Status = "not_configured"
)

// AgentCapabilities is the cached result of probing an agent type.
type AgentCapabilities struct {
	AgentType    string `json:"agent_type"`
	AgentName    string `json:"agent_name,omitempty"`
	AgentVersion string `json:"agent_version,omitempty"`

	Status Status `json:"status"`
	Error  string `json:"error,omitempty"`

	ProtocolVersion    int                `json:"protocol_version,omitempty"`
	LoadSession        bool               `json:"load_session,omitempty"`
	PromptCapabilities PromptCapabilities `json:"prompt_capabilities,omitempty"`

	AuthMethods []AuthMethod `json:"auth_methods,omitempty"`

	Models         []Model `json:"models,omitempty"`
	CurrentModelID string  `json:"current_model_id,omitempty"`

	Modes         []Mode `json:"modes,omitempty"`
	CurrentModeID string `json:"current_mode_id,omitempty"`

	LastCheckedAt time.Time `json:"last_checked_at"`
	DurationMs    int       `json:"duration_ms,omitempty"`
}

// PromptCapabilities reports which prompt content block types the agent accepts.
type PromptCapabilities struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embedded_context,omitempty"`
}

// AuthMethod is a single advertised authentication method.
type AuthMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Model is a single advertised model.
type Model struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Mode is a single advertised session mode.
type Mode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PromptResult is returned from ExecutePrompt and RawPrompt calls.
type PromptResult struct {
	Response       string
	Model          string
	PromptTokens   int
	ResponseTokens int
	DurationMs     int
}
