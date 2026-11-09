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
	"go.uber.org/zap"
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

func quickChatParamsForExisting(task *models.Task, fallbackAgentID, fallbackExecutorID string) quickChatParams {
	params := quickChatParams{metadata: make(map[string]interface{})}
	if task == nil {
		return params
	}
	for key, value := range task.Metadata {
		params.metadata[key] = value
	}
	params.agentProfileID = firstNonEmpty(quickChatTaskAgentProfileID(task), fallbackAgentID)
	params.executorID = firstNonEmpty(models.StringFromAny(task.Metadata[models.MetaKeyExecutorID]), fallbackExecutorID)
	params.title = firstNonEmpty(task.Title, "Quick Chat")
	for _, repository := range task.Repositories {
		if repository == nil {
			continue
		}
		params.repos = append(params.repos, service.TaskRepositoryInput{
			RepositoryID: repository.RepositoryID,
			BaseBranch:   repository.BaseBranch,
		})
	}
	if len(params.repos) > 0 {
		params.executorID = models.ExecutorIDWorktree
	}
	if params.agentProfileID != "" {
		params.metadata[models.MetaKeyAgentProfileID] = params.agentProfileID
	}
	if params.executorID != "" {
		params.metadata[models.MetaKeyExecutorID] = params.executorID
	}
	return params
}

func (h *TaskHandlers) writeQuickChatCreation(c *gin.Context, task *models.Task, creationID string) {
	response, err := h.quickChatCreationDescriptor(c.Request.Context(), task, creationID)
	if err != nil {
		handleNotFound(c, h.logger, err, "quick chat creation not found")
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *TaskHandlers) respondWithFoundQuickChatCreation(
	c *gin.Context,
	workspaceID, externalID, creationID string,
	result service.CreateTaskResult,
) bool {
	if result.Outcome == service.CreateTaskOutcomeCreated {
		return false
	}
	task := result.Task
	if task == nil || task.ExternalID != externalID || task.WorkspaceID != workspaceID || !isQuickChatCreationTask(task) {
		c.JSON(http.StatusConflict, gin.H{"code": "quick_chat_creation_conflict"})
		return true
	}
	h.writeQuickChatCreation(c, task, creationID)
	return true
}

func (h *TaskHandlers) respondQuickChatCreateError(c *gin.Context, err error) {
	if status, ok := quickChatCreateHTTPStatus(err); ok {
		errorBody := taskErrorBody(err)
		errorBody[errorDetailKeyErrorCode] = "quick_chat_creation_rejected"
		c.JSON(status, errorBody)
		return
	}
	h.logger.Error("failed to create ephemeral task", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create quick chat"})
}

func (h *TaskHandlers) settleQuickChatCreation(
	c *gin.Context,
	taskID, externalID, creationID string,
) bool {
	if creationID == "" {
		return true
	}
	settled, survivor, err := h.service.SettleExternalID(c.Request.Context(), taskID, externalID)
	if err != nil {
		handleNotFound(c, h.logger, err, "quick chat creation no longer exists")
		return false
	}
	if settled {
		return true
	}
	if survivor != nil && survivor.ExternalIDSettledAt != nil {
		h.writeQuickChatCreation(c, survivor, creationID)
		return false
	}
	if survivor == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "quick chat creation no longer exists"})
		return false
	}
	c.JSON(http.StatusConflict, gin.H{
		"code":              "quick_chat_creation_identity_lost",
		"task_id":           survivor.ID,
		"creation_id":       creationID,
		"creation_complete": false,
	})
	return false
}

func (h *TaskHandlers) existingQuickChatCreation(
	c *gin.Context,
	workspaceID string,
	body httpStartQuickChatRequest,
) (string, *models.Task, func(), bool) {
	if body.CreationID == "" {
		return "", nil, nil, false
	}
	ctx := c.Request.Context()
	if err := h.service.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeTaskWrite); err != nil {
		handleNotFound(c, h.logger, err, "workspace not found")
		return "", nil, nil, true
	}
	externalID, err := quickChatCreationExternalID(ctx, body.CreationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "creation_id must be a canonical UUID"})
		return "", nil, nil, true
	}
	release := h.lockQuickChatCreation(externalID)
	existing, err := h.service.GetTaskByExternalID(ctx, workspaceID, externalID)
	if errors.Is(err, taskrepo.ErrTaskNotFound) {
		return externalID, nil, release, false
	}
	if err != nil {
		handleNotFound(c, h.logger, err, "quick chat creation not found")
		return "", nil, release, true
	}
	if existing == nil || existing.ExternalID != externalID ||
		existing.WorkspaceID != workspaceID || !isQuickChatCreationTask(existing) {
		c.JSON(http.StatusConflict, gin.H{"code": "quick_chat_creation_conflict"})
		return "", nil, release, true
	}
	if existing.ExternalIDSettledAt != nil || existing.ArchivedAt != nil {
		h.writeQuickChatCreation(c, existing, body.CreationID)
		return "", nil, release, true
	}
	return externalID, existing, release, false
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
