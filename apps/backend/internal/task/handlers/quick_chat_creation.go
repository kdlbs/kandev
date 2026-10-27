package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
)

func quickChatCreationExternalID(ctx context.Context, creationID string) (string, error) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok {
		return "", service.ErrQuickChatCreationIDInvalid
	}
	return service.NewQuickChatCreationExternalID(identity.UserID, creationID)
}

func isQuickChatCreationTask(task *models.Task) bool {
	if !service.IsRestorableQuickChatTask(task) || task.Metadata == nil {
		return service.IsRestorableQuickChatTask(task)
	}
	configMode, _ := task.Metadata["config_mode"].(bool)
	return !configMode
}

func (h *TaskHandlers) quickChatCreationDescriptor(ctx context.Context, task *models.Task, creationID string) (httpStartQuickChatResponse, error) {
	if !isQuickChatCreationTask(task) {
		return httpStartQuickChatResponse{}, repoerrors.ErrTaskNotFound
	}
	response := httpStartQuickChatResponse{
		TaskID:           task.ID,
		CreationID:       creationID,
		CreationComplete: quickChatBoolPointer(task.ExternalIDSettledAt != nil),
		Archived:         task.ArchivedAt != nil,
	}
	session, err := h.service.GetPrimarySession(ctx, task.ID)
	if errors.Is(err, taskrepo.ErrNoPrimarySession) || errors.Is(err, models.ErrTaskSessionNotFound) {
		session, err = nil, nil
	}
	if err != nil {
		return httpStartQuickChatResponse{}, err
	}
	if session != nil {
		response.SessionID = session.ID
		response.SessionState = string(session.State)
		response.AgentProfileID = firstNonEmpty(session.AgentProfileID, quickChatTaskAgentProfileID(task))
	}
	return response, nil
}

func quickChatBoolPointer(value bool) *bool { return &value }

func quickChatTaskAgentProfileID(task *models.Task) string {
	if task == nil || task.Metadata == nil {
		return ""
	}
	profileID, _ := task.Metadata[models.MetaKeyAgentProfileID].(string)
	return profileID
}

func (h *TaskHandlers) writeQuickChatCreation(c *gin.Context, task *models.Task, creationID string) {
	response, err := h.quickChatCreationDescriptor(c.Request.Context(), task, creationID)
	if err != nil {
		handleNotFound(c, h.logger, err, "quick chat creation not found")
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *TaskHandlers) existingQuickChatCreation(
	c *gin.Context,
	workspaceID string,
	body httpStartQuickChatRequest,
) (string, bool) {
	if body.CreationID == "" {
		return "", false
	}
	ctx := c.Request.Context()
	if err := h.service.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeTaskWrite); err != nil {
		handleNotFound(c, h.logger, err, "workspace not found")
		return "", true
	}
	externalID, err := quickChatCreationExternalID(ctx, body.CreationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "creation_id must be a canonical UUID"})
		return "", true
	}
	existing, err := h.service.GetTaskByExternalID(ctx, workspaceID, externalID)
	if errors.Is(err, taskrepo.ErrTaskNotFound) {
		return externalID, false
	}
	if err != nil {
		handleNotFound(c, h.logger, err, "quick chat creation not found")
		return "", true
	}
	if existing == nil || existing.ExternalID != externalID ||
		existing.WorkspaceID != workspaceID || !isQuickChatCreationTask(existing) {
		c.JSON(http.StatusConflict, gin.H{"code": "quick_chat_creation_conflict"})
		return "", true
	}
	h.writeQuickChatCreation(c, existing, body.CreationID)
	return "", true
}

func (h *TaskHandlers) httpGetQuickChatCreation(c *gin.Context) {
	creationID := c.Param("creation_id")
	externalID, err := quickChatCreationExternalID(c.Request.Context(), creationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "creation_id must be a canonical UUID"})
		return
	}
	workspaceID := c.Param("id")
	task, err := h.service.GetTaskByExternalID(c.Request.Context(), workspaceID, externalID)
	if err != nil {
		handleNotFound(c, h.logger, err, "quick chat creation not found")
		return
	}
	if task == nil || task.ExternalID != externalID || task.WorkspaceID != workspaceID || !isQuickChatCreationTask(task) {
		c.JSON(http.StatusNotFound, gin.H{"error": "quick chat creation not found"})
		return
	}
	response, err := h.quickChatCreationDescriptor(c.Request.Context(), task, creationID)
	if err != nil {
		handleNotFound(c, h.logger, err, "quick chat creation not found")
		return
	}
	c.JSON(http.StatusOK, response)
}
