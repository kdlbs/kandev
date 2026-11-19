package dto

import (
	"time"

	"github.com/kandev/kandev/internal/prompts/models"
)

type PromptDTO struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Content         string `json:"content"`
	Builtin         bool   `json:"builtin"`
	AllowAgentEdits bool   `json:"allow_agent_edits"`
	Source          string `json:"source"`
	SourcePath      string `json:"source_path"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type PromptsResponse struct {
	Prompts []PromptDTO `json:"prompts"`
}

type CreatePromptRequest struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type UpdatePromptRequest struct {
	// Retained for compatibility; updates ignore this field.
	AllowAgentEdits   *bool      `json:"allow_agent_edits,omitempty"`
	ExpectedUpdatedAt *time.Time `json:"expected_updated_at,omitempty"`
	Name              *string    `json:"name,omitempty"`
	Content           *string    `json:"content,omitempty"`
}

func FromPrompt(prompt *models.Prompt) PromptDTO {
	return PromptDTO{
		ID:              prompt.ID,
		Name:            prompt.Name,
		Content:         prompt.Content,
		Builtin:         prompt.Builtin,
		AllowAgentEdits: AgentEditable(prompt),
		Source:          Source(prompt),
		SourcePath:      prompt.SourcePath,
		CreatedAt:       prompt.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:       prompt.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

// AgentEditable reports whether agents may write the prompt: built-in and
// synced prompts are read-only to agents.
func AgentEditable(prompt *models.Prompt) bool {
	return !prompt.Builtin && !prompt.IsSynced()
}

// Source returns the prompt's ownership, defaulting to manual.
func Source(prompt *models.Prompt) string {
	if prompt.IsSynced() {
		return models.PromptSourceSync
	}
	return models.PromptSourceManual
}
