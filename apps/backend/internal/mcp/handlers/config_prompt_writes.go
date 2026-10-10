package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/prompts/models"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type PromptWriter interface {
	CreatePromptForAgent(context.Context, string, string) (*models.Prompt, error)
	UpdatePromptContentForAgent(context.Context, string, string) (*models.Prompt, error)
}

func (h *Handlers) SetPromptWriter(writer PromptWriter, authEnabled func() bool) {
	h.promptWriter, h.promptAuthEnabled = writer, authEnabled
}

func (h *Handlers) handleCreateSharedPrompt(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.writeSharedPrompt(ctx, msg, h.promptWriter.CreatePromptForAgent)
}

func (h *Handlers) handleUpdateSharedPrompt(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.writeSharedPrompt(ctx, msg, h.promptWriter.UpdatePromptContentForAgent)
}

func (h *Handlers) canWriteSharedPrompts(ctx context.Context) bool {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok {
		return h.promptAuthEnabled != nil && !h.promptAuthEnabled()
	}
	if identity.Synthetic {
		return true
	}
	return authz.SubjectOrgScopes(authz.Subject{
		UserID: identity.UserID, OrgID: identity.OrgID,
		OrgRole: authz.NormalizeOrgRole(string(identity.Role)),
	}).Has(authz.ScopeOrgConfigManage)
}

func (h *Handlers) writeSharedPrompt(ctx context.Context, msg *ws.Message,
	write func(context.Context, string, string) (*models.Prompt, error),
) (*ws.Message, error) {
	if !h.canWriteSharedPrompts(ctx) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "Shared prompt writes require org.config.manage", nil)
	}
	var req struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid shared prompt payload", nil)
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Content) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name and content are required", nil)
	}
	prompt, err := write(ctx, req.Name, req.Content)
	if err != nil {
		return h.sharedPromptWriteError(msg, err)
	}
	if prompt == nil {
		return h.sharedPromptWriteError(msg, errors.New("prompt write returned no result"))
	}
	return ws.NewResponse(msg.ID, msg.Action, sharedPromptResult(prompt))
}

func (h *Handlers) sharedPromptWriteError(msg *ws.Message, err error) (*ws.Message, error) {
	code, message := ws.ErrorCodeInternalError, "Failed to save shared prompt"
	switch {
	case errors.Is(err, promptservice.ErrPromptNotFound):
		code, message = ws.ErrorCodeNotFound, "Shared prompt not found"
	case errors.Is(err, promptservice.ErrInvalidPrompt):
		code, message = ws.ErrorCodeValidation, "name must be 1-512 bytes and content 1-1048576 bytes after trimming"
	case errors.Is(err, promptservice.ErrPromptAlreadyExists), errors.Is(err, promptservice.ErrPromptListLimit):
		code, message = ws.ErrorCodeValidation, err.Error()
	case errors.Is(err, promptservice.ErrBuiltinPrompt), errors.Is(err, promptservice.ErrPromptAgentEditsDisabled):
		code, message = ws.ErrorCodeForbidden, err.Error()
	case errors.Is(err, promptstore.ErrPromptWriteRejected):
		code, message = ws.ErrorCodeConflict, err.Error()
	default:
		h.logger.Error("failed to save shared prompt", zap.Error(err))
	}
	return ws.NewError(msg.ID, msg.Action, code, message, nil)
}
