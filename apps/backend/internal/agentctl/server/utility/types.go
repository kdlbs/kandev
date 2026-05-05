// Package utility provides one-shot prompt execution via inference-capable agents.
// This is a simplified interface compared to the full session-based adapters,
// designed for quick tasks like generating commit messages or PR descriptions.
package utility

// PromptRequest is the request for executing an inference prompt.
type PromptRequest struct {
	// Prompt is the fully resolved prompt text to send to the LLM.
	Prompt string `json:"prompt" binding:"required"`

	// AgentID is the agent to use (e.g., "claude-code", "amp").
	AgentID string `json:"agent_id" binding:"required"`

	// Model is the model to use (e.g., "claude-haiku-4-5").
	Model string `json:"model,omitempty"`

	// Mode is the optional session mode to set before sending the prompt.
	// If empty, no session/set_mode call is made and the agent default is used.
	Mode string `json:"mode,omitempty"`

	// InferenceConfig is the agent's inference configuration.
	// This is passed from the backend which has access to the agent registry.
	InferenceConfig *InferenceConfigDTO `json:"inference_config,omitempty"`

	// MaxTokens is the maximum tokens for the response (default: 1024).
	MaxTokens int `json:"max_tokens,omitempty"`

	// MCPServers are the MCP servers to expose to the agent for the duration
	// of this single inference call. Forwarded to ACP `session/new` so the
	// agent can call tools mid-prompt. Empty (the common case) keeps the
	// existing "pure inference, no tools" behaviour for utility-agent
	// callers that only need text-in/text-out (PR title generation, commit
	// messages, etc.).
	MCPServers []MCPServerDTO `json:"mcp_servers,omitempty"`
}

// MCPServerDTO describes a single MCP server endpoint to wire into the
// agent's session. Mirrors the ACP McpServer shape but kept JSON-flat so it
// crosses the agentctl HTTP boundary cleanly. Today we use HTTP transport
// only (Type="http"); stdio/sse can be added when a caller needs them.
type MCPServerDTO struct {
	Name string `json:"name"`
	Type string `json:"type"`          // "http" | "sse" | "stdio"
	URL  string `json:"url,omitempty"` // for http/sse
	// HeaderKVs is an optional set of HTTP headers (key, value pairs).
	HeaderKVs []HTTPHeaderDTO `json:"headers,omitempty"`
}

// HTTPHeaderDTO is a single HTTP header for HTTP/SSE-transport MCP servers.
type HTTPHeaderDTO struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ProbeRequest is the request for probing an agent's capabilities.
// It runs initialize + session/new against an ephemeral ACP subprocess and
// returns the discovered agent info, auth methods, models, and modes without
// sending any prompt.
type ProbeRequest struct {
	// AgentID is the agent to probe (e.g., "claude-acp", "codex-acp").
	AgentID string `json:"agent_id" binding:"required"`

	// InferenceConfig is the agent's inference configuration.
	// Command and WorkDir are required; Model is intentionally omitted for probes.
	InferenceConfig *InferenceConfigDTO `json:"inference_config,omitempty"`
}

// ProbeResponse is the response from probing an agent.
type ProbeResponse struct {
	// Success indicates if the probe completed successfully.
	Success bool `json:"success"`

	// Error is the error message if the probe failed.
	Error string `json:"error,omitempty"`

	// DurationMs is the probe duration in milliseconds.
	DurationMs int `json:"duration_ms,omitempty"`

	// AgentName is the agent name reported in initialize response (if any).
	AgentName string `json:"agent_name,omitempty"`
	// AgentVersion is the agent version reported in initialize response (if any).
	AgentVersion string `json:"agent_version,omitempty"`

	// ProtocolVersion is the negotiated ACP protocol version.
	ProtocolVersion int `json:"protocol_version,omitempty"`

	// AuthMethods are the authentication methods advertised by the agent.
	AuthMethods []ProbeAuthMethod `json:"auth_methods,omitempty"`

	// Models are the models the agent advertises from session/new.
	Models []ProbeModel `json:"models,omitempty"`
	// CurrentModelID is the default/current model selected by the agent.
	CurrentModelID string `json:"current_model_id,omitempty"`

	// Modes are the session modes the agent advertises from session/new.
	Modes []ProbeMode `json:"modes,omitempty"`
	// CurrentModeID is the default/current mode selected by the agent.
	CurrentModeID string `json:"current_mode_id,omitempty"`

	// Commands are the slash commands the agent advertises via the
	// `available_commands_update` session notification (drained briefly
	// after session/new).
	Commands []ProbeCommand `json:"commands,omitempty"`

	// LoadSession indicates if the agent supports session/load.
	LoadSession bool `json:"load_session,omitempty"`
	// PromptCapabilities reports which content block types the agent accepts.
	PromptCapabilities ProbePromptCapabilities `json:"prompt_capabilities,omitempty"`
}

// ProbeAuthMethod is a single advertised authentication method.
type ProbeAuthMethod struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
}

// ProbeModel is a single advertised model. Meta carries agent-specific
// extras from ACP's `_meta` field — e.g. GitHub Copilot exposes
// `copilotUsage` ("1x", "0.33x", "0x") and `copilotEnablement` here.
type ProbeModel struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
}

// ProbeMode is a single advertised session mode.
type ProbeMode struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
}

// ProbeCommand is a single slash command advertised by the agent via the
// ACP `available_commands_update` session notification.
type ProbeCommand struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ProbePromptCapabilities reports the agent's prompt input capabilities.
type ProbePromptCapabilities struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embedded_context,omitempty"`
}

// InferenceConfigDTO is the inference configuration passed from backend to agentctl.
type InferenceConfigDTO struct {
	// Command is the ACP command for one-shot inference.
	// e.g., ["npx", "-y", "@agentclientprotocol/claude-agent-acp"]
	Command []string `json:"command"`
	// ModelFlag is the flag template for specifying the model.
	ModelFlag []string `json:"model_flag,omitempty"`
	// WorkDir is the working directory for the agent process.
	WorkDir string `json:"work_dir"`
}

// PromptResponse is the response from executing a utility prompt.
type PromptResponse struct {
	// Success indicates if the prompt completed successfully.
	Success bool `json:"success"`

	// Response is the generated text.
	Response string `json:"response,omitempty"`

	// Model is the model that was used.
	Model string `json:"model,omitempty"`

	// PromptTokens is the number of input tokens.
	PromptTokens int `json:"prompt_tokens,omitempty"`

	// ResponseTokens is the number of output tokens.
	ResponseTokens int `json:"response_tokens,omitempty"`

	// DurationMs is the execution duration in milliseconds.
	DurationMs int `json:"duration_ms,omitempty"`

	// Error is the error message if the prompt failed.
	Error string `json:"error,omitempty"`
}

// ModelInfo represents an available model.
type ModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ModelsResponse is the response for listing available models.
type ModelsResponse struct {
	Models []ModelInfo `json:"models"`
}
