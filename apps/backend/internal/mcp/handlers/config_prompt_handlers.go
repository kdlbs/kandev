package handlers

import (
	"context"
	"errors"
	"strings"
	"time"

	promptservice "github.com/kandev/kandev/internal/prompts/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type sharedPromptSummary struct {
	Name         string `json:"name"`
	Builtin      bool   `json:"builtin"`
	ContentBytes int    `json:"content_bytes"`
}

type sharedPromptRead struct {
	Name         string `json:"name"`
	Content      string `json:"content"`
	Builtin      bool   `json:"builtin"`
	ContentBytes int    `json:"content_bytes"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func (h *Handlers) registerPromptHandlers(d *guardedMCPDispatcher) {
	d.RegisterFunc(ws.ActionMCPListSharedPrompts, h.handleListSharedPrompts)
	d.RegisterFunc(ws.ActionMCPGetSharedPrompt, h.handleGetSharedPrompt)
}

func (h *Handlers) handleListSharedPrompts(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	prompts, err := h.promptReader.ListPrompts(ctx)
	if err != nil {
		h.logger.Error("failed to list shared prompts", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list shared prompts", nil)
	}

	summaries := make([]sharedPromptSummary, 0, len(prompts))
	for _, prompt := range prompts {
		if prompt == nil {
			continue
		}
		summaries = append(summaries, sharedPromptSummary{
			Name:         prompt.Name,
			Builtin:      prompt.Builtin,
			ContentBytes: len(prompt.Content),
		})
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"shared_prompts": summaries,
		"total":          len(summaries),
	})
}

func (h *Handlers) handleGetSharedPrompt(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	name, err := unmarshalStringField(msg.Payload, "name")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}

	prompt, err := h.promptReader.GetPromptByName(ctx, name)
	// Keep the nil-result guard for alternate PromptReader implementations. The
	// production service maps nil results to ErrPromptNotFound.
	if errors.Is(err, promptservice.ErrPromptNotFound) || (err == nil && prompt == nil) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Shared prompt not found", nil)
	}
	if err != nil {
		h.logger.Error("failed to get shared prompt", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get shared prompt", nil)
	}

	result := sharedPromptRead{
		Name:         prompt.Name,
		Content:      prompt.Content,
		Builtin:      prompt.Builtin,
		ContentBytes: len(prompt.Content),
		CreatedAt:    prompt.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:    prompt.UpdatedAt.UTC().Format(time.RFC3339),
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}
