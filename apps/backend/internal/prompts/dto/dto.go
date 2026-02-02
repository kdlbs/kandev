package dto

import (
	"time"

	"github.com/kandev/kandev/internal/prompts/models"
)

type PromptDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	Builtin   bool   `json:"builtin"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type PromptsResponse struct {
	Prompts []PromptDTO `json:"prompts"`
}

type CreatePromptRequest struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type UpdatePromptRequest struct {
	Name    *string `json:"name,omitempty"`
	Content *string `json:"content,omitempty"`
}

func FromPrompt(prompt *models.Prompt) PromptDTO {
	return PromptDTO{
		ID:        prompt.ID,
		Name:      prompt.Name,
		Content:   prompt.Content,
		Builtin:   prompt.Builtin,
		CreatedAt: prompt.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: prompt.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
