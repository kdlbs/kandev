package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type RepositoryHandlers struct {
	service *service.Service
	logger  *logger.Logger
}

func NewRepositoryHandlers(svc *service.Service, log *logger.Logger) *RepositoryHandlers {
	return &RepositoryHandlers{
		service: svc,
		logger:  log.WithFields(zap.String("component", "task-repository-handlers")),
	}
}

func RegisterRepositoryRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *service.Service, log *logger.Logger) {
	handlers := NewRepositoryHandlers(svc, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *RepositoryHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/workspaces/:id/repositories", h.httpListRepositories)
	api.POST("/workspaces/:id/repositories", h.httpCreateRepository)
	api.GET("/workspaces/:id/repositories/discover", h.httpDiscoverRepositories)
	api.GET("/workspaces/:id/repositories/branches", h.httpListLocalRepositoryBranches)
	api.GET("/workspaces/:id/repositories/validate", h.httpValidateRepositoryPath)
	api.GET("/repositories/:id", h.httpGetRepository)
	api.GET("/repositories/:id/branches", h.httpListRepositoryBranches)
	api.PATCH("/repositories/:id", h.httpUpdateRepository)
	api.DELETE("/repositories/:id", h.httpDeleteRepository)
	api.GET("/repositories/:id/scripts", h.httpListRepositoryScripts)
	api.POST("/repositories/:id/scripts", h.httpCreateRepositoryScript)
	api.GET("/scripts/:id", h.httpGetRepositoryScript)
	api.PUT("/scripts/:id", h.httpUpdateRepositoryScript)
	api.DELETE("/scripts/:id", h.httpDeleteRepositoryScript)
}

func (h *RepositoryHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionRepositoryList, h.wsListRepositories)
	dispatcher.RegisterFunc(ws.ActionRepositoryCreate, h.wsCreateRepository)
	dispatcher.RegisterFunc(ws.ActionRepositoryGet, h.wsGetRepository)
	dispatcher.RegisterFunc(ws.ActionRepositoryUpdate, h.wsUpdateRepository)
	dispatcher.RegisterFunc(ws.ActionRepositoryDelete, h.wsDeleteRepository)
	dispatcher.RegisterFunc(ws.ActionRepositoryScriptList, h.wsListRepositoryScripts)
	dispatcher.RegisterFunc(ws.ActionRepositoryScriptCreate, h.wsCreateRepositoryScript)
	dispatcher.RegisterFunc(ws.ActionRepositoryScriptGet, h.wsGetRepositoryScript)
	dispatcher.RegisterFunc(ws.ActionRepositoryScriptUpdate, h.wsUpdateRepositoryScript)
	dispatcher.RegisterFunc(ws.ActionRepositoryScriptDelete, h.wsDeleteRepositoryScript)
}

// HTTP handlers

func (h *RepositoryHandlers) httpListRepositories(c *gin.Context) {
	workspaceID := c.Param("id")
	includeScripts := c.Query("include_scripts") == queryValueTrue

	repositories, err := h.service.ListRepositories(c.Request.Context(), workspaceID)
	if err != nil {
		h.logger.Error("failed to list repositories", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list repositories"})
		return
	}

	resp := dto.ListRepositoriesResponse{
		Repositories: make([]dto.RepositoryDTO, 0, len(repositories)),
		Total:        len(repositories),
	}
	for _, repository := range repositories {
		repoDTO := dto.FromRepository(repository)
		if includeScripts {
			scripts, scriptsErr := h.service.ListRepositoryScripts(c.Request.Context(), repository.ID)
			if scriptsErr == nil {
				repoDTO.Scripts = make([]dto.RepositoryScriptDTO, 0, len(scripts))
				for _, script := range scripts {
					repoDTO.Scripts = append(repoDTO.Scripts, dto.FromRepositoryScript(script))
				}
			}
		}
		resp.Repositories = append(resp.Repositories, repoDTO)
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RepositoryHandlers) httpDiscoverRepositories(c *gin.Context) {
	root := c.Query("root")
	result, err := h.service.DiscoverLocalRepositories(c.Request.Context(), root)
	if err != nil {
		if errors.Is(err, service.ErrPathNotAllowed) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "root is not within allowed paths"})
			return
		}
		h.logger.Error("failed to discover repositories", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to discover repositories"})
		return
	}

	resp := dto.RepositoryDiscoveryResponse{
		Roots:        result.Roots,
		Repositories: make([]dto.LocalRepositoryDTO, 0, len(result.Repositories)),
		Total:        len(result.Repositories),
	}
	for _, repo := range result.Repositories {
		resp.Repositories = append(resp.Repositories, dto.FromLocalRepository(repo))
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RepositoryHandlers) httpValidateRepositoryPath(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	result, err := h.service.ValidateLocalRepositoryPath(c.Request.Context(), path)
	if err != nil {
		h.logger.Error("failed to validate repository path", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate repository path"})
		return
	}
	c.JSON(http.StatusOK, dto.RepositoryPathValidationResponse{
		Path:          result.Path,
		Exists:        result.Exists,
		IsGitRepo:     result.IsGitRepo,
		Allowed:       result.Allowed,
		DefaultBranch: result.DefaultBranch,
		Message:       result.Message,
	})
}

func (h *RepositoryHandlers) httpListLocalRepositoryBranches(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	branches, err := h.service.ListLocalRepositoryBranches(c.Request.Context(), path)
	if err != nil {
		if errors.Is(err, service.ErrPathNotAllowed) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "path is not within allowed roots"})
			return
		}
		h.logger.Error("failed to list local repository branches", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list local repository branches"})
		return
	}
	dtoBranches := make([]dto.BranchDTO, len(branches))
	for i, branch := range branches {
		dtoBranches[i] = dto.FromBranch(branch)
	}
	c.JSON(http.StatusOK, dto.RepositoryBranchesResponse{
		Branches: dtoBranches,
		Total:    len(dtoBranches),
	})
}

type httpCreateRepositoryRequest struct {
	Name                 string `json:"name"`
	SourceType           string `json:"source_type"`
	LocalPath            string `json:"local_path"`
	Provider             string `json:"provider"`
	ProviderRepoID       string `json:"provider_repo_id"`
	ProviderOwner        string `json:"provider_owner"`
	ProviderName         string `json:"provider_name"`
	DefaultBranch        string `json:"default_branch"`
	WorktreeBranchPrefix string `json:"worktree_branch_prefix"`
	PullBeforeWorktree   *bool  `json:"pull_before_worktree"`
	SetupScript          string `json:"setup_script"`
	CleanupScript        string `json:"cleanup_script"`
	DevScript            string `json:"dev_script"`
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
	repository, err := h.service.CreateRepository(c.Request.Context(), &service.CreateRepositoryRequest{
		WorkspaceID:          c.Param("id"),
		Name:                 body.Name,
		SourceType:           body.SourceType,
		LocalPath:            body.LocalPath,
		Provider:             body.Provider,
		ProviderRepoID:       body.ProviderRepoID,
		ProviderOwner:        body.ProviderOwner,
		ProviderName:         body.ProviderName,
		DefaultBranch:        body.DefaultBranch,
		WorktreeBranchPrefix: body.WorktreeBranchPrefix,
		PullBeforeWorktree:   body.PullBeforeWorktree,
		SetupScript:          body.SetupScript,
		CleanupScript:        body.CleanupScript,
		DevScript:            body.DevScript,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidRepositorySettings) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repository settings"})
			return
		}
		h.logger.Error("failed to create repository", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create repository"})
		return
	}
	c.JSON(http.StatusCreated, dto.FromRepository(repository))
}

func (h *RepositoryHandlers) httpGetRepository(c *gin.Context) {
	repository, err := h.service.GetRepository(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "repository not found")
		return
	}
	c.JSON(http.StatusOK, dto.FromRepository(repository))
}

func (h *RepositoryHandlers) httpListRepositoryBranches(c *gin.Context) {
	branches, err := h.service.ListRepositoryBranches(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, service.ErrPathNotAllowed) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "repository path is not allowed"})
			return
		}
		h.logger.Error("failed to list repository branches", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list repository branches"})
		return
	}
	dtoBranches := make([]dto.BranchDTO, len(branches))
	for i, branch := range branches {
		dtoBranches[i] = dto.FromBranch(branch)
	}
	c.JSON(http.StatusOK, dto.RepositoryBranchesResponse{
		Branches: dtoBranches,
		Total:    len(dtoBranches),
	})
}

type httpUpdateRepositoryRequest struct {
	Name                 *string `json:"name"`
	SourceType           *string `json:"source_type"`
	LocalPath            *string `json:"local_path"`
	Provider             *string `json:"provider"`
	ProviderRepoID       *string `json:"provider_repo_id"`
	ProviderOwner        *string `json:"provider_owner"`
	ProviderName         *string `json:"provider_name"`
	DefaultBranch        *string `json:"default_branch"`
	WorktreeBranchPrefix *string `json:"worktree_branch_prefix"`
	PullBeforeWorktree   *bool   `json:"pull_before_worktree"`
	SetupScript          *string `json:"setup_script"`
	CleanupScript        *string `json:"cleanup_script"`
	DevScript            *string `json:"dev_script"`
}

func (h *RepositoryHandlers) httpUpdateRepository(c *gin.Context) {
	var body httpUpdateRepositoryRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	repository, err := h.service.UpdateRepository(c.Request.Context(), c.Param("id"), &service.UpdateRepositoryRequest{
		Name:                 body.Name,
		SourceType:           body.SourceType,
		LocalPath:            body.LocalPath,
		Provider:             body.Provider,
		ProviderRepoID:       body.ProviderRepoID,
		ProviderOwner:        body.ProviderOwner,
		ProviderName:         body.ProviderName,
		DefaultBranch:        body.DefaultBranch,
		WorktreeBranchPrefix: body.WorktreeBranchPrefix,
		PullBeforeWorktree:   body.PullBeforeWorktree,
		SetupScript:          body.SetupScript,
		CleanupScript:        body.CleanupScript,
		DevScript:            body.DevScript,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidRepositorySettings) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repository settings"})
			return
		}
		handleNotFound(c, h.logger, err, "repository not found")
		return
	}
	c.JSON(http.StatusOK, dto.FromRepository(repository))
}

func (h *RepositoryHandlers) httpDeleteRepository(c *gin.Context) {
	if err := h.service.DeleteRepository(c.Request.Context(), c.Param("id")); err != nil {
		if errors.Is(err, service.ErrActiveTaskSessions) {
			c.JSON(http.StatusConflict, gin.H{"error": "repository is used by an active agent session"})
			return
		}
		handleNotFound(c, h.logger, err, "repository not found")
		return
	}
	c.Status(http.StatusNoContent)
}

// WS handlers

func reposToListResponse(repositories []*models.Repository) dto.ListRepositoriesResponse {
	resp := dto.ListRepositoriesResponse{
		Repositories: make([]dto.RepositoryDTO, 0, len(repositories)),
		Total:        len(repositories),
	}
	for _, repository := range repositories {
		resp.Repositories = append(resp.Repositories, dto.FromRepository(repository))
	}
	return resp
}

func (h *RepositoryHandlers) wsListRepositories(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	repositories, err := h.service.ListRepositories(ctx, req.WorkspaceID)
	if err != nil {
		h.logger.Error("failed to list repositories", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list repositories", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, reposToListResponse(repositories))
}

type wsCreateRepositoryRequest struct {
	WorkspaceID          string `json:"workspace_id"`
	Name                 string `json:"name"`
	SourceType           string `json:"source_type"`
	LocalPath            string `json:"local_path"`
	Provider             string `json:"provider"`
	ProviderRepoID       string `json:"provider_repo_id"`
	ProviderOwner        string `json:"provider_owner"`
	ProviderName         string `json:"provider_name"`
	DefaultBranch        string `json:"default_branch"`
	WorktreeBranchPrefix string `json:"worktree_branch_prefix"`
	SetupScript          string `json:"setup_script"`
	CleanupScript        string `json:"cleanup_script"`
	DevScript            string `json:"dev_script"`
}

func (h *RepositoryHandlers) wsCreateRepository(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateRepositoryRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkspaceID == "" || req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id and name are required", nil)
	}
	repository, err := h.service.CreateRepository(ctx, &service.CreateRepositoryRequest{
		WorkspaceID:          req.WorkspaceID,
		Name:                 req.Name,
		SourceType:           req.SourceType,
		LocalPath:            req.LocalPath,
		Provider:             req.Provider,
		ProviderRepoID:       req.ProviderRepoID,
		ProviderOwner:        req.ProviderOwner,
		ProviderName:         req.ProviderName,
		DefaultBranch:        req.DefaultBranch,
		WorktreeBranchPrefix: req.WorktreeBranchPrefix,
		SetupScript:          req.SetupScript,
		CleanupScript:        req.CleanupScript,
		DevScript:            req.DevScript,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidRepositorySettings) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Invalid repository settings", nil)
		}
		h.logger.Error("failed to create repository", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create repository", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromRepository(repository))
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
	repository, err := h.service.GetRepository(ctx, req.ID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Repository not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromRepository(repository))
}

type wsUpdateRepositoryRequest struct {
	ID                   string  `json:"id"`
	Name                 *string `json:"name,omitempty"`
	SourceType           *string `json:"source_type,omitempty"`
	LocalPath            *string `json:"local_path,omitempty"`
	Provider             *string `json:"provider,omitempty"`
	ProviderRepoID       *string `json:"provider_repo_id,omitempty"`
	ProviderOwner        *string `json:"provider_owner,omitempty"`
	ProviderName         *string `json:"provider_name,omitempty"`
	DefaultBranch        *string `json:"default_branch,omitempty"`
	WorktreeBranchPrefix *string `json:"worktree_branch_prefix,omitempty"`
	SetupScript          *string `json:"setup_script,omitempty"`
	CleanupScript        *string `json:"cleanup_script,omitempty"`
	DevScript            *string `json:"dev_script,omitempty"`
}

func (h *RepositoryHandlers) wsUpdateRepository(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateRepositoryRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	repository, err := h.service.UpdateRepository(ctx, req.ID, &service.UpdateRepositoryRequest{
		Name:                 req.Name,
		SourceType:           req.SourceType,
		LocalPath:            req.LocalPath,
		Provider:             req.Provider,
		ProviderRepoID:       req.ProviderRepoID,
		ProviderOwner:        req.ProviderOwner,
		ProviderName:         req.ProviderName,
		DefaultBranch:        req.DefaultBranch,
		WorktreeBranchPrefix: req.WorktreeBranchPrefix,
		SetupScript:          req.SetupScript,
		CleanupScript:        req.CleanupScript,
		DevScript:            req.DevScript,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidRepositorySettings) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Invalid repository settings", nil)
		}
		h.logger.Error("failed to update repository", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update repository", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromRepository(repository))
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
	if err := h.service.DeleteRepository(ctx, req.ID); err != nil {
		h.logger.Error("failed to delete repository", zap.Error(err))
		if errors.Is(err, service.ErrActiveTaskSessions) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "repository is used by an active agent session", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Repository not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, gin.H{"deleted": true})
}
