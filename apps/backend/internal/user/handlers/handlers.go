package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/controller"
	"github.com/kandev/kandev/internal/user/dto"
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

type httpUpdateUserSettingsRequest struct {
	WorkspaceID   *string  `json:"workspace_id,omitempty"`
	BoardID       *string  `json:"board_id,omitempty"`
	RepositoryIDs *[]string `json:"repository_ids,omitempty"`
}

func (h *Handlers) httpUpdateUserSettings(c *gin.Context) {
	var body httpUpdateUserSettingsRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.UpdateUserSettings(c.Request.Context(), dto.UpdateUserSettingsRequest{
		WorkspaceID:   body.WorkspaceID,
		BoardID:       body.BoardID,
		RepositoryIDs: body.RepositoryIDs,
	})
	if err != nil {
		h.logger.Error("failed to update user settings", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user settings"})
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

type wsUpdateUserSettingsRequest struct {
	WorkspaceID   *string  `json:"workspace_id,omitempty"`
	BoardID       *string  `json:"board_id,omitempty"`
	RepositoryIDs *[]string `json:"repository_ids,omitempty"`
}

func (h *Handlers) wsUpdateUserSettings(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateUserSettingsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	resp, err := h.controller.UpdateUserSettings(ctx, dto.UpdateUserSettingsRequest{
		WorkspaceID:   req.WorkspaceID,
		BoardID:       req.BoardID,
		RepositoryIDs: req.RepositoryIDs,
	})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update user settings", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
