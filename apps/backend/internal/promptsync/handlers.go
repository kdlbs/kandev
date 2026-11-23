package promptsync

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	promptdto "github.com/kandev/kandev/internal/prompts/dto"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type controller struct {
	service *Service
	logger  *logger.Logger
}

// RegisterRoutes mounts the prompt sync HTTP API. Reads follow saved-prompt
// read access; mutations and Sync now require configuration management.
func RegisterRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	ctrl := &controller{service: svc, logger: log.WithFields(zap.String("component", "promptsync-handlers"))}
	api := router.Group("/api/v1/prompt-sync")
	manageConfig := authz.RequireOrgScope(authz.ScopeOrgConfigManage)
	api.GET("/config", ctrl.httpGetConfig)
	api.GET("/kept-versions", ctrl.httpRemovalReview)
	api.POST("/config", manageConfig, ctrl.httpSetConfig)
	api.DELETE("/config", manageConfig, ctrl.httpDeleteConfig)
	api.POST("/check", manageConfig, ctrl.httpCheck)
	api.POST("/sync", manageConfig, ctrl.httpSync)
	api.PUT("/prompts/:id/versions", manageConfig, ctrl.httpSaveVersions)
}

// versionsRequest is the operator's saved Local | Repository choice and edited
// local version for a synced prompt or an override.
type versionsRequest struct {
	UseLocal          bool       `json:"use_local"`
	LocalContent      *string    `json:"local_content,omitempty"`
	ExpectedUpdatedAt *time.Time `json:"expected_updated_at,omitempty"`
}

func (c *controller) httpSaveVersions(ctx *gin.Context) {
	var req versionsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	prompt, err := c.service.SaveVersions(ctx.Request.Context(), ctx.Param("id"), promptservice.VersionSave{
		UseLocal: req.UseLocal, LocalContent: req.LocalContent, ExpectedUpdatedAt: req.ExpectedUpdatedAt,
	})
	switch {
	case err == nil:
		ctx.JSON(http.StatusOK, promptdto.FromPrompt(prompt))
	case workspaceDenied(err), errors.Is(err, promptservice.ErrPromptNotFound):
		ctx.JSON(http.StatusNotFound, gin.H{"error": "prompt not found"})
	case errors.Is(err, ErrNotConfigured):
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, promptservice.ErrInvalidPrompt):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, promptservice.ErrPromptNotLinked), errors.Is(err, promptservice.ErrNoRepositoryVersion),
		errors.Is(err, promptstore.ErrPromptWriteRejected):
		ctx.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.internalError(ctx, "failed to save prompt versions", err)
	}
}

func workspaceDenied(err error) bool {
	return errors.Is(err, repoerrors.ErrWorkspaceNotFound)
}

func (c *controller) internalError(ctx *gin.Context, msg string, err error) {
	c.logger.Error(msg, zap.Error(err))
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": msg})
}

func (c *controller) httpGetConfig(ctx *gin.Context) {
	cfg, err := c.service.GetConfig(ctx.Request.Context())
	if err != nil {
		c.internalError(ctx, "failed to load prompt sync config", err)
		return
	}
	if cfg == nil {
		ctx.Status(http.StatusNoContent)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *controller) httpSetConfig(ctx *gin.Context) {
	var req SetConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	outcome, err := c.service.SaveAndSync(ctx.Request.Context(), &req)
	switch {
	case workspaceDenied(err):
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
	case errors.Is(err, ErrInvalidConfig):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case err != nil:
		c.internalError(ctx, "failed to save prompt sync config", err)
	default:
		ctx.JSON(http.StatusOK, runResponse(outcome.Config, outcome.Result, outcome.SyncErr))
	}
}

// httpCheck reports the conflicts a source would cause without storing or
// changing anything. An empty body checks the stored configuration. A source
// that cannot be read returns 422 with a sanitized error.
func (c *controller) httpCheck(ctx *gin.Context) {
	var req *SetConfigRequest
	if ctx.Request.ContentLength != 0 {
		req = &SetConfigRequest{}
		if err := ctx.ShouldBindJSON(req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
	}
	result, err := c.service.Check(ctx.Request.Context(), req)
	switch {
	case err == nil:
		ctx.JSON(http.StatusOK, result)
	case workspaceDenied(err):
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
	case errors.Is(err, ErrNotConfigured):
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrInvalidConfig):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{"error": safeErrorMessage(err)})
	}
}

// syncRequest carries optional conflict choices for Sync now.
type syncRequest struct {
	Resolutions map[string]promptservice.ConflictChoice `json:"resolutions"`
}

func runResponse(cfg *Config, result *SyncResult, syncErr error) gin.H {
	response := gin.H{"config": cfg}
	if syncErr != nil {
		response["error"] = safeErrorMessage(syncErr)
		if errors.Is(syncErr, github.ErrGitHubAdmissionWaitRequired) {
			response["error_code"] = github.RateLimitErrorCode
		}
	}
	if result != nil {
		response["result"] = result
	}
	return response
}

// httpRemovalReview lists what removing the configuration asks about.
func (c *controller) httpRemovalReview(ctx *gin.Context) {
	review, err := c.service.RemovalReview(ctx.Request.Context())
	if err != nil {
		c.internalError(ctx, "failed to list prompts with two versions", err)
		return
	}
	ctx.JSON(http.StatusOK, review)
}

// deleteRequest picks, by prompt name, which version a prompt with both a
// local and a repository version keeps when Prompt Sync is removed, and names
// the reviewed prompts that only came from the repository to delete.
type deleteRequest struct {
	Choices              map[string]promptservice.ConflictChoice `json:"choices"`
	DeleteRepositoryOnly []string                                `json:"delete_repository_only"`
}

func (c *controller) httpDeleteConfig(ctx *gin.Context) {
	var req deleteRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
	}
	err := c.service.DeleteConfig(ctx.Request.Context(), Removal(req))
	switch {
	case errors.Is(err, ErrInvalidConfig):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case workspaceDenied(err):
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
	case err != nil:
		c.internalError(ctx, "failed to remove prompt sync config", err)
	default:
		ctx.JSON(http.StatusOK, gin.H{"deleted": true})
	}
}

// httpSync runs one sync. A completed run returns 200 even when it carries a
// sanitized error, so the client always receives the current status.
func (c *controller) httpSync(ctx *gin.Context) {
	var req syncRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
	}
	result, syncErr := c.service.SyncWithResolutions(ctx.Request.Context(), req.Resolutions)
	if ctx.Request.Context().Err() != nil {
		return
	}
	if errors.Is(syncErr, ErrInvalidConfig) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": syncErr.Error()})
		return
	}
	if workspaceDenied(syncErr) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}
	if errors.Is(syncErr, ErrNotConfigured) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": syncErr.Error()})
		return
	}
	cfg, err := c.service.GetConfig(ctx.Request.Context())
	if err != nil {
		c.internalError(ctx, "failed to load prompt sync config", err)
		return
	}
	ctx.JSON(http.StatusOK, runResponse(cfg, result, syncErr))
}
