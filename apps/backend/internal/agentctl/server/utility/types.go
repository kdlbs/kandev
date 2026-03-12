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

	// InferenceConfig is the agent's inference configuration.
	// This is passed from the backend which has access to the agent registry.
	InferenceConfig *InferenceConfigDTO `json:"inference_config,omitempty"`

	// MaxTokens is the maximum tokens for the response (default: 1024).
	MaxTokens int `json:"max_tokens,omitempty"`
}

// InferenceConfigDTO is the inference configuration passed from backend to agentctl.
type InferenceConfigDTO struct {
	// Command is the ACP command for one-shot inference.
	// e.g., ["npx", "-y", "@zed-industries/claude-agent-acp"]
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
