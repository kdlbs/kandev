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
	Conflict        bool   `json:"conflict"`
	// AlternateContent is the version not in use: the kept local version of a
	// synced prompt, or the repository version of an override.
	AlternateContent string `json:"alternate_content,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
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
		ID:               prompt.ID,
		Name:             prompt.Name,
		Content:          prompt.Content,
		Builtin:          prompt.Builtin,
		AllowAgentEdits:  AgentEditable(prompt),
		Source:           Source(prompt),
		SourcePath:       prompt.SourcePath,
		Conflict:         prompt.Conflict,
		AlternateContent: prompt.AlternateContent,
		CreatedAt:        prompt.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        prompt.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

// AgentEditable reports whether agents may write the prompt: built-in and
// synced prompts are read-only to agents.
func AgentEditable(prompt *models.Prompt) bool {
	return !prompt.Builtin && !prompt.IsSynced()
}

// EditGuidance explains to an agent how a synced prompt or an override can
// be changed, so it can relay the steps to the user. Other prompts need none.
func EditGuidance(prompt *models.Prompt) string {
	switch {
	case prompt.IsSynced():
		return "Synced from the prompt repository and read-only. Only the user can make it editable: switch it to Local in Settings > Prompts and save. Agents cannot change that setting."
	case prompt.IsOverride() && prompt.Conflict:
		return "A local version that conflicts with the repository file of the same name. Edits change the local version, which is in use. Only the user can resolve the conflict, in Settings > Prompts."
	case prompt.IsOverride():
		return "A local override of a synced prompt. Edits change the local version, which is in use. Only the user can switch back to the repository version, in Settings > Prompts."
	default:
		return ""
	}
}

// Source returns the prompt's ownership, defaulting to manual.
func Source(prompt *models.Prompt) string {
	switch {
	case prompt.IsSynced():
		return models.PromptSourceSync
	case prompt.IsOverride():
		return models.PromptSourceOverride
	default:
		return models.PromptSourceManual
	}
}
