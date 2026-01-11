package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/controller"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type RepositoryHandlers struct {
	controller *controller.RepositoryController
	logger     *logger.Logger
}

func NewRepositoryHandlers(ctrl *controller.RepositoryController, log *logger.Logger) *RepositoryHandlers {
	return &RepositoryHandlers{
		controller: ctrl,
		logger:     log.WithFields(zap.String("component", "task-repository-handlers")),
	}
}

func RegisterRepositoryRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, ctrl *controller.RepositoryController, log *logger.Logger) {
	handlers := NewRepositoryHandlers(ctrl, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *RepositoryHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/workspaces/:id/repositories", h.httpListRepositories)
	api.POST("/workspaces/:id/repositories", h.httpCreateRepository)
	api.GET("/workspaces/:id/repositories/discover", h.httpDiscoverRepositories)
	api.GET("/workspaces/:id/repositories/validate", h.httpValidateRepositoryPath)
	api.GET("/repositories/:id", h.httpGetRepository)
	api.PATCH("/repositories/:id", h.httpUpdateRepository)
	api.DELETE("/repositories/:id", h.httpDeleteRepository)
	api.GET("/repositories/:id/scripts", h.httpListRepositoryScripts)
	api.POST("/repositories/:id/scripts", h.httpCreateRepositoryScript)
	api.GET("/scripts/:id", h.httpGetRepositoryScript)
	api.PUT("/scripts/:id", h.httpUpdateRepositoryScript)
	api.DELETE("/scripts/:id", h.httpDeleteRepositoryScript)
}

func (h *RepositoryHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc("repository.list", h.wsListRepositories)
	dispatcher.RegisterFunc("repository.create", h.wsCreateRepository)
	dispatcher.RegisterFunc("repository.get", h.wsGetRepository)
	dispatcher.RegisterFunc("repository.update", h.wsUpdateRepository)
	dispatcher.RegisterFunc("repository.delete", h.wsDeleteRepository)
	dispatcher.RegisterFunc("repository.script.list", h.wsListRepositoryScripts)
	dispatcher.RegisterFunc("repository.script.create", h.wsCreateRepositoryScript)
	dispatcher.RegisterFunc("repository.script.get", h.wsGetRepositoryScript)
	dispatcher.RegisterFunc("repository.script.update", h.wsUpdateRepositoryScript)
	dispatcher.RegisterFunc("repository.script.delete", h.wsDeleteRepositoryScript)
}

// HTTP handlers

func (h *RepositoryHandlers) httpListRepositories(c *gin.Context) {
	resp, err := h.controller.ListRepositories(c.Request.Context(), dto.ListRepositoriesRequest{WorkspaceID: c.Param("id")})
	if err != nil {
		h.logger.Error("failed to list repositories", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list repositories"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RepositoryHandlers) httpDiscoverRepositories(c *gin.Context) {
	root := c.Query("root")
	resp, err := h.controller.DiscoverRepositories(c.Request.Context(), dto.DiscoverRepositoriesRequest{
		WorkspaceID: c.Param("id"),
		Root:        root,
	})
	if err != nil {
		if errors.Is(err, service.ErrPathNotAllowed) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "root is not within allowed paths"})
			return
		}
		h.logger.Error("failed to discover repositories", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to discover repositories"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RepositoryHandlers) httpValidateRepositoryPath(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	resp, err := h.controller.ValidateRepositoryPath(c.Request.Context(), dto.ValidateRepositoryPathRequest{
		WorkspaceID: c.Param("id"),
		Path:        path,
	})
	if err != nil {
		h.logger.Error("failed to validate repository path", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate repository path"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpCreateRepositoryRequest struct {
	Name           string `json:"name"`
	SourceType     string `json:"source_type"`
	LocalPath      string `json:"local_path"`
	Provider       string `json:"provider"`
	ProviderRepoID string `json:"provider_repo_id"`
	ProviderOwner  string `json:"provider_owner"`
	ProviderName   string `json:"provider_name"`
	DefaultBranch  string `json:"default_branch"`
	SetupScript    string `json:"setup_script"`
	CleanupScript  string `json:"cleanup_script"`
}

func (h *RepositoryHandlers) httpCreateRepository(c *gin.Context) {
	var body httpCreateRepositoryRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	resp, err := h.controller.CreateRepository(c.Request.Context(), dto.CreateRepositoryRequest{
		WorkspaceID:    c.Param("id"),
		Name:           body.Name,
		SourceType:     body.SourceType,
		LocalPath:      body.LocalPath,
		Provider:       body.Provider,
		ProviderRepoID: body.ProviderRepoID,
		ProviderOwner:  body.ProviderOwner,
		ProviderName:   body.ProviderName,
		DefaultBranch:  body.DefaultBranch,
		SetupScript:    body.SetupScript,
		CleanupScript:  body.CleanupScript,
	})
	if err != nil {
		h.logger.Error("failed to create repository", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create repository"})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *RepositoryHandlers) httpGetRepository(c *gin.Context) {
	resp, err := h.controller.GetRepository(c.Request.Context(), dto.GetRepositoryRequest{ID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "repository not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpUpdateRepositoryRequest struct {
	Name           *string `json:"name"`
	SourceType     *string `json:"source_type"`
	LocalPath      *string `json:"local_path"`
	Provider       *string `json:"provider"`
	ProviderRepoID *string `json:"provider_repo_id"`
	ProviderOwner  *string `json:"provider_owner"`
	ProviderName   *string `json:"provider_name"`
	DefaultBranch  *string `json:"default_branch"`
	SetupScript    *string `json:"setup_script"`
	CleanupScript  *string `json:"cleanup_script"`
}

func (h *RepositoryHandlers) httpUpdateRepository(c *gin.Context) {
	var body httpUpdateRepositoryRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	resp, err := h.controller.UpdateRepository(c.Request.Context(), dto.UpdateRepositoryRequest{
		ID:             c.Param("id"),
		Name:           body.Name,
		SourceType:     body.SourceType,
		LocalPath:      body.LocalPath,
		Provider:       body.Provider,
		ProviderRepoID: body.ProviderRepoID,
		ProviderOwner:  body.ProviderOwner,
		ProviderName:   body.ProviderName,
		DefaultBranch:  body.DefaultBranch,
		SetupScript:    body.SetupScript,
		CleanupScript:  body.CleanupScript,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "repository not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RepositoryHandlers) httpDeleteRepository(c *gin.Context) {
	if _, err := h.controller.DeleteRepository(c.Request.Context(), dto.DeleteRepositoryRequest{ID: c.Param("id")}); err != nil {
		handleNotFound(c, h.logger, err, "repository not found")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *RepositoryHandlers) httpListRepositoryScripts(c *gin.Context) {
	resp, err := h.controller.ListRepositoryScripts(c.Request.Context(), dto.ListRepositoryScriptsRequest{RepositoryID: c.Param("id")})
	if err != nil {
		h.logger.Error("failed to list repository scripts", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list repository scripts"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpCreateRepositoryScriptRequest struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Position int    `json:"position"`
}

func (h *RepositoryHandlers) httpCreateRepositoryScript(c *gin.Context) {
	var body httpCreateRepositoryScriptRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if body.Name == "" || body.Command == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and command are required"})
		return
	}
	resp, err := h.controller.CreateRepositoryScript(c.Request.Context(), dto.CreateRepositoryScriptRequest{
		RepositoryID: c.Param("id"),
		Name:         body.Name,
		Command:      body.Command,
		Position:     body.Position,
	})
	if err != nil {
		h.logger.Error("failed to create repository script", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create repository script"})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *RepositoryHandlers) httpGetRepositoryScript(c *gin.Context) {
	resp, err := h.controller.GetRepositoryScript(c.Request.Context(), dto.GetRepositoryScriptRequest{ID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "repository script not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpUpdateRepositoryScriptRequest struct {
	Name     *string `json:"name"`
	Command  *string `json:"command"`
	Position *int    `json:"position"`
}

func (h *RepositoryHandlers) httpUpdateRepositoryScript(c *gin.Context) {
	var body httpUpdateRepositoryScriptRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	resp, err := h.controller.UpdateRepositoryScript(c.Request.Context(), dto.UpdateRepositoryScriptRequest{
		ID:       c.Param("id"),
		Name:     body.Name,
		Command:  body.Command,
		Position: body.Position,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "repository script not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RepositoryHandlers) httpDeleteRepositoryScript(c *gin.Context) {
	if _, err := h.controller.DeleteRepositoryScript(c.Request.Context(), dto.DeleteRepositoryScriptRequest{ID: c.Param("id")}); err != nil {
		handleNotFound(c, h.logger, err, "repository script not found")
		return
	}
	c.Status(http.StatusNoContent)
}

// WS handlers

func (h *RepositoryHandlers) wsListRepositories(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	resp, err := h.controller.ListRepositories(ctx, dto.ListRepositoriesRequest{WorkspaceID: req.WorkspaceID})
	if err != nil {
		h.logger.Error("failed to list repositories", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list repositories", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateRepositoryRequest struct {
	WorkspaceID    string `json:"workspace_id"`
	Name           string `json:"name"`
	SourceType     string `json:"source_type"`
	LocalPath      string `json:"local_path"`
	Provider       string `json:"provider"`
	ProviderRepoID string `json:"provider_repo_id"`
	ProviderOwner  string `json:"provider_owner"`
	ProviderName   string `json:"provider_name"`
	DefaultBranch  string `json:"default_branch"`
	SetupScript    string `json:"setup_script"`
	CleanupScript  string `json:"cleanup_script"`
}

func (h *RepositoryHandlers) wsCreateRepository(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateRepositoryRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkspaceID == "" || req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id and name are required", nil)
	}
	resp, err := h.controller.CreateRepository(ctx, dto.CreateRepositoryRequest{
		WorkspaceID:    req.WorkspaceID,
		Name:           req.Name,
		SourceType:     req.SourceType,
		LocalPath:      req.LocalPath,
		Provider:       req.Provider,
		ProviderRepoID: req.ProviderRepoID,
		ProviderOwner:  req.ProviderOwner,
		ProviderName:   req.ProviderName,
		DefaultBranch:  req.DefaultBranch,
		SetupScript:    req.SetupScript,
		CleanupScript:  req.CleanupScript,
	})
	if err != nil {
		h.logger.Error("failed to create repository", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create repository", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsGetRepositoryRequest struct {
	ID string `json:"id"`
}

func (h *RepositoryHandlers) wsGetRepository(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetRepositoryRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	resp, err := h.controller.GetRepository(ctx, dto.GetRepositoryRequest{ID: req.ID})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Repository not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsUpdateRepositoryRequest struct {
	ID             string  `json:"id"`
	Name           *string `json:"name,omitempty"`
	SourceType     *string `json:"source_type,omitempty"`
	LocalPath      *string `json:"local_path,omitempty"`
	Provider       *string `json:"provider,omitempty"`
	ProviderRepoID *string `json:"provider_repo_id,omitempty"`
	ProviderOwner  *string `json:"provider_owner,omitempty"`
	ProviderName   *string `json:"provider_name,omitempty"`
	DefaultBranch  *string `json:"default_branch,omitempty"`
	SetupScript    *string `json:"setup_script,omitempty"`
	CleanupScript  *string `json:"cleanup_script,omitempty"`
}

func (h *RepositoryHandlers) wsUpdateRepository(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateRepositoryRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	resp, err := h.controller.UpdateRepository(ctx, dto.UpdateRepositoryRequest{
		ID:             req.ID,
		Name:           req.Name,
		SourceType:     req.SourceType,
		LocalPath:      req.LocalPath,
		Provider:       req.Provider,
		ProviderRepoID: req.ProviderRepoID,
		ProviderOwner:  req.ProviderOwner,
		ProviderName:   req.ProviderName,
		DefaultBranch:  req.DefaultBranch,
		SetupScript:    req.SetupScript,
		CleanupScript:  req.CleanupScript,
	})
	if err != nil {
		h.logger.Error("failed to update repository", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update repository", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsDeleteRepositoryRequest struct {
	ID string `json:"id"`
}

func (h *RepositoryHandlers) wsDeleteRepository(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsDeleteRepositoryRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	if _, err := h.controller.DeleteRepository(ctx, dto.DeleteRepositoryRequest{ID: req.ID}); err != nil {
		h.logger.Error("failed to delete repository", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Repository not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, gin.H{"deleted": true})
}

func (h *RepositoryHandlers) wsListRepositoryScripts(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		RepositoryID string `json:"repository_id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	resp, err := h.controller.ListRepositoryScripts(ctx, dto.ListRepositoryScriptsRequest{RepositoryID: req.RepositoryID})
	if err != nil {
		h.logger.Error("failed to list repository scripts", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list repository scripts", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateRepositoryScriptRequest struct {
	RepositoryID string `json:"repository_id"`
	Name         string `json:"name"`
	Command      string `json:"command"`
	Position     int    `json:"position"`
}

func (h *RepositoryHandlers) wsCreateRepositoryScript(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateRepositoryScriptRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.RepositoryID == "" || req.Name == "" || req.Command == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "repository_id, name, and command are required", nil)
	}
	resp, err := h.controller.CreateRepositoryScript(ctx, dto.CreateRepositoryScriptRequest{
		RepositoryID: req.RepositoryID,
		Name:         req.Name,
		Command:      req.Command,
		Position:     req.Position,
	})
	if err != nil {
		h.logger.Error("failed to create repository script", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create repository script", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsGetRepositoryScriptRequest struct {
	ID string `json:"id"`
}

func (h *RepositoryHandlers) wsGetRepositoryScript(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetRepositoryScriptRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	resp, err := h.controller.GetRepositoryScript(ctx, dto.GetRepositoryScriptRequest{ID: req.ID})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Repository script not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsUpdateRepositoryScriptRequest struct {
	ID       string  `json:"id"`
	Name     *string `json:"name,omitempty"`
	Command  *string `json:"command,omitempty"`
	Position *int    `json:"position,omitempty"`
}

func (h *RepositoryHandlers) wsUpdateRepositoryScript(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateRepositoryScriptRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	resp, err := h.controller.UpdateRepositoryScript(ctx, dto.UpdateRepositoryScriptRequest{
		ID:       req.ID,
		Name:     req.Name,
		Command:  req.Command,
		Position: req.Position,
	})
	if err != nil {
		h.logger.Error("failed to update repository script", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update repository script", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsDeleteRepositoryScriptRequest struct {
	ID string `json:"id"`
}

func (h *RepositoryHandlers) wsDeleteRepositoryScript(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsDeleteRepositoryScriptRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	if _, err := h.controller.DeleteRepositoryScript(ctx, dto.DeleteRepositoryScriptRequest{ID: req.ID}); err != nil {
		h.logger.Error("failed to delete repository script", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Repository script not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, gin.H{"deleted": true})
}
