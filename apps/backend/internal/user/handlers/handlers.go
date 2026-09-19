package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/controller"
	"github.com/kandev/kandev/internal/user/dto"
	"github.com/kandev/kandev/internal/user/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type Handlers struct {
	controller *controller.Controller
	logger     *logger.Logger
}

func NewHandlers(ctrl *controller.Controller, log *logger.Logger) *Handlers {
	return &Handlers{
		controller: ctrl,
		logger:     log.WithFields(zap.String("component", "user-handlers")),
	}
}

func RegisterRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, ctrl *controller.Controller, log *logger.Logger) {
	h := NewHandlers(ctrl, log)
	h.registerHTTP(router)
	h.registerWS(dispatcher)
}

func (h *Handlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/user", h.httpGetUser)
	api.GET("/user/settings", h.httpGetUserSettings)
	api.PATCH("/user/settings", h.httpUpdateUserSettings)
	api.GET("/user/agent-profile-recent-use", h.httpGetAgentProfileRecentUse)
	api.PUT("/user/agent-profile-recent-use/:context", h.httpRecordAgentProfileRecentUse)
}

func (h *Handlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionUserGet, h.wsGetUser)
	dispatcher.RegisterFunc(ws.ActionUserSettingsUpdate, h.wsUpdateUserSettings)
}

func (h *Handlers) httpGetUser(c *gin.Context) {
	resp, err := h.controller.GetCurrentUser(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpGetUserSettings(c *gin.Context) {
	resp, err := h.controller.GetUserSettings(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get user settings", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user settings"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpGetAgentProfileRecentUse(c *gin.Context) {
	records, err := h.controller.GetAgentProfileRecentUse(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get agent profile recent use", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get agent profile recent use"})
		return
	}
	c.JSON(http.StatusOK, records)
}

const maxRecordAgentProfileRecentUseBodyBytes = 4 * 1024

func (h *Handlers) httpRecordAgentProfileRecentUse(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRecordAgentProfileRecentUseBodyBytes)
	var req dto.RecordAgentProfileRecentUseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	record, err := h.controller.RecordAgentProfileRecentUse(
		c.Request.Context(),
		c.Param("context"),
		req,
	)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrValidation) {
			status = http.StatusBadRequest
		} else if errors.Is(err, service.ErrAgentProfileRecentUseConflict) {
			status = http.StatusConflict
		}
		h.logger.Error("failed to record agent profile recent use", zap.Error(err))
		c.JSON(status, gin.H{"error": "failed to record agent profile recent use"})
		return
	}
	c.JSON(http.StatusOK, record)
}

// maxUpdateUserSettingsBodyBytes bounds the update-settings request body via
// http.MaxBytesReader, writing the 413 response below when exceeded. The
// settings payload can legitimately carry several capped-but-large fields
// (saved layouts, sidebar views, preference blobs, kanban_hidden_step_ids),
// so the limit is generous relative to those per-field caps rather than tight.
const maxUpdateUserSettingsBodyBytes = 2 * 1024 * 1024

func (h *Handlers) httpUpdateUserSettings(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUpdateUserSettingsBodyBytes)
	var req dto.UpdateUserSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.UpdateUserSettings(c.Request.Context(), req)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrValidation) {
			status = http.StatusBadRequest
		} else if errors.Is(err, service.ErrUserSettingsConflict) {
			status = http.StatusConflict
		}
		h.logger.Error("failed to update user settings", zap.Error(err))
		c.JSON(status, gin.H{"error": "failed to update user settings"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) wsGetUser(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	resp, err := h.controller.GetCurrentUser(ctx)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get user", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *Handlers) wsUpdateUserSettings(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req dto.UpdateUserSettingsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	resp, err := h.controller.UpdateUserSettings(ctx, req)
	if err != nil {
		code := ws.ErrorCodeInternalError
		if errors.Is(err, service.ErrValidation) {
			code = ws.ErrorCodeBadRequest
		} else if errors.Is(err, service.ErrUserSettingsConflict) {
			code = ws.ErrorCodeConflict
		}
		return ws.NewError(msg.ID, msg.Action, code, "Failed to update user settings", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
