package dto

import (
	"time"

	"github.com/kandev/kandev/internal/utility/models"
	"github.com/kandev/kandev/internal/utility/template"
)

// UtilityAgentDTO represents a utility agent for API responses.
type UtilityAgentDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
	AgentID     string `json:"agent_id"`
	Model       string `json:"model"`
	Builtin     bool   `json:"builtin"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// UtilityAgentsResponse is the response for listing utility agents.
type UtilityAgentsResponse struct {
	Agents []UtilityAgentDTO `json:"agents"`
}

// CreateUtilityAgentRequest is the request for creating a utility agent.
type CreateUtilityAgentRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Prompt      string `json:"prompt" binding:"required"`
	AgentID     string `json:"agent_id" binding:"required"`
	Model       string `json:"model" binding:"required"`
}

// UpdateUtilityAgentRequest is the request for updating a utility agent.
type UpdateUtilityAgentRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Prompt      *string `json:"prompt,omitempty"`
	AgentID     *string `json:"agent_id,omitempty"`
	Model       *string `json:"model,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
}

// UtilityAgentCallDTO represents a call for API responses.
type UtilityAgentCallDTO struct {
	ID             string  `json:"id"`
	UtilityID      string  `json:"utility_id"`
	SessionID      string  `json:"session_id"`
	ResolvedPrompt string  `json:"resolved_prompt"`
	Response       string  `json:"response"`
	Model          string  `json:"model"`
	PromptTokens   int     `json:"prompt_tokens"`
	ResponseTokens int     `json:"response_tokens"`
	DurationMs     int     `json:"duration_ms"`
	Status         string  `json:"status"`
	ErrorMessage   string  `json:"error_message"`
	CreatedAt      string  `json:"created_at"`
	CompletedAt    *string `json:"completed_at,omitempty"`
}

// UtilityAgentCallsResponse is the response for listing calls.
type UtilityAgentCallsResponse struct {
	Calls []UtilityAgentCallDTO `json:"calls"`
}

// TemplateVariablesResponse is the response for listing available template variables.
type TemplateVariablesResponse struct {
	Variables []template.VariableInfo `json:"variables"`
}

// FromUtilityAgent converts a model to a DTO.
func FromUtilityAgent(agent *models.UtilityAgent) UtilityAgentDTO {
	return UtilityAgentDTO{
		ID:          agent.ID,
		Name:        agent.Name,
		Description: agent.Description,
		Prompt:      agent.Prompt,
		AgentID:     agent.AgentID,
		Model:       agent.Model,
		Builtin:     agent.Builtin,
		Enabled:     agent.Enabled,
		CreatedAt:   agent.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   agent.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// FromUtilityAgentCall converts a call model to a DTO.
func FromUtilityAgentCall(call *models.UtilityAgentCall) UtilityAgentCallDTO {
	dto := UtilityAgentCallDTO{
		ID:             call.ID,
		UtilityID:      call.UtilityID,
		SessionID:      call.SessionID,
		ResolvedPrompt: call.ResolvedPrompt,
		Response:       call.Response,
		Model:          call.Model,
		PromptTokens:   call.PromptTokens,
		ResponseTokens: call.ResponseTokens,
		DurationMs:     call.DurationMs,
		Status:         call.Status,
		ErrorMessage:   call.ErrorMessage,
		CreatedAt:      call.CreatedAt.UTC().Format(time.RFC3339),
	}
	if call.CompletedAt != nil {
		formatted := call.CompletedAt.UTC().Format(time.RFC3339)
		dto.CompletedAt = &formatted
	}
	return dto
}

// ExecutePromptRequest is the request for executing a utility prompt.
type ExecutePromptRequest struct {
	// UtilityAgentID is the ID of the utility agent to execute.
	UtilityAgentID string `json:"utility_agent_id" binding:"required"`

	// SessionID is the active session to piggyback on.
	SessionID string `json:"session_id"`

	// Context variables for template resolution.
	GitDiff             string `json:"git_diff,omitempty"`
	CommitLog           string `json:"commit_log,omitempty"`
	ChangedFiles        string `json:"changed_files,omitempty"`
	DiffSummary         string `json:"diff_summary,omitempty"`
	BranchName          string `json:"branch_name,omitempty"`
	BaseBranch          string `json:"base_branch,omitempty"`
	TaskTitle           string `json:"task_title,omitempty"`
	TaskDescription     string `json:"task_description,omitempty"`
	UserPrompt          string `json:"user_prompt,omitempty"`
	ConversationHistory string `json:"conversation_history,omitempty"`
}

// ExecutePromptResponse is the response from executing a utility prompt.
type ExecutePromptResponse struct {
	Success        bool   `json:"success"`
	CallID         string `json:"call_id,omitempty"`
	Response       string `json:"response,omitempty"`
	Model          string `json:"model,omitempty"`
	PromptTokens   int    `json:"prompt_tokens,omitempty"`
	ResponseTokens int    `json:"response_tokens,omitempty"`
	DurationMs     int    `json:"duration_ms,omitempty"`
	Error          string `json:"error,omitempty"`
}

// ModelInfoDTO represents a model for API responses.
type ModelInfoDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ModelsResponse is the response for listing models.
type ModelsResponse struct {
	Models []ModelInfoDTO `json:"models"`
}

// InferenceModelDTO represents a model available for inference.
type InferenceModelDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
	// Meta carries agent-specific extras from ACP's `_meta` field. For
	// GitHub Copilot this includes `copilotUsage` (e.g. "1x", "0.33x",
	// "0x" — the premium-request multiplier) which the UI renders as a
	// cost badge next to the model name.
	Meta map[string]any `json:"meta,omitempty"`
}

// InferenceAgentDTO represents an agent that supports inference.
type InferenceAgentDTO struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	DisplayName string              `json:"display_name"`
	Models      []InferenceModelDTO `json:"models"`
}

// InferenceAgentsResponse is the response for listing inference agents.
type InferenceAgentsResponse struct {
	Agents []InferenceAgentDTO `json:"agents"`
}
