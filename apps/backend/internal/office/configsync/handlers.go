package configsync

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// errorKey is the JSON error-message field shared by every failure response.
const errorKey = "error"

// Handler holds HTTP route handlers for config sync. Registered on the
// Office API group so :wsId is scope-checked by the existing
// officeWorkspaceScopeMiddleware (see docs/specs/office/system-design/
// config-sync.md's HTTP section) — this package does not re-authorize.
type Handler struct {
	service *Service
	logger  *logger.Logger
}

// NewHandler constructs a config sync Handler.
func NewHandler(svc *Service, log *logger.Logger) *Handler {
	return &Handler{service: svc, logger: log}
}

// RegisterRoutes wires the config sync HTTP endpoints onto the Office API
// route group.
func RegisterRoutes(api *gin.RouterGroup, h *Handler) {
	api.GET("/workspaces/:wsId/config-sync/config", h.httpGetConfig)
	api.POST("/workspaces/:wsId/config-sync/config", h.httpSetConfig)
	api.DELETE("/workspaces/:wsId/config-sync/config", h.httpDeleteConfig)
	api.POST("/workspaces/:wsId/config-sync/sync", h.httpForceSync)
}

func (h *Handler) internalError(c *gin.Context, msg string, err error) {
	h.logger.Error(msg, zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{errorKey: msg})
}

func (h *Handler) httpGetConfig(c *gin.Context) {
	cfg, err := h.service.GetConfigForWorkspace(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		h.internalError(c, "failed to load config sync config", err)
		return
	}
	if cfg == nil {
		c.Status(http.StatusNoContent)
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func (h *Handler) httpSetConfig(c *gin.Context) {
	var req SetConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errorKey: "invalid payload"})
		return
	}
	cfg, err := h.service.SetConfigForWorkspace(c.Request.Context(), c.Param("wsId"), &req)
	if errors.Is(err, ErrInvalidConfig) {
		c.JSON(http.StatusBadRequest, gin.H{errorKey: err.Error()})
		return
	}
	if errors.Is(err, ErrWorkspaceGone) {
		c.JSON(http.StatusNotFound, gin.H{errorKey: err.Error()})
		return
	}
	if err != nil {
		h.internalError(c, "failed to save config sync config", err)
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func (h *Handler) httpDeleteConfig(c *gin.Context) {
	if err := h.service.DeleteConfigForWorkspace(c.Request.Context(), c.Param("wsId")); err != nil {
		h.internalError(c, "failed to remove config sync config", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// httpForceSync runs a sync immediately instead of waiting for the poller.
// A sync failure still returns 200 with the error embedded beside the
// updated config, mirroring the shipped workflow-sync contract — the
// outcome is also recorded on the config row for the status card.
func (h *Handler) httpForceSync(c *gin.Context) {
	workspaceID := c.Param("wsId")
	result, syncErr := h.service.SyncWorkspace(c.Request.Context(), workspaceID)
	if errors.Is(syncErr, ErrNotConfigured) {
		c.JSON(http.StatusNotFound, gin.H{errorKey: syncErr.Error()})
		return
	}
	cfg, err := h.service.GetConfigForWorkspace(c.Request.Context(), workspaceID)
	if err != nil {
		h.internalError(c, "failed to load config sync config", err)
		return
	}
	response := gin.H{"config": cfg}
	if syncErr != nil {
		response[errorKey] = syncErr.Error()
	}
	if result != nil {
		response["result"] = result
	}
	c.JSON(http.StatusOK, response)
}
