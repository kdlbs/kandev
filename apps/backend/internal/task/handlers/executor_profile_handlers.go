package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type ExecutorProfileHandlers struct {
	service *service.Service
	logger  *logger.Logger
}

func NewExecutorProfileHandlers(svc *service.Service, log *logger.Logger) *ExecutorProfileHandlers {
	return &ExecutorProfileHandlers{service: svc, logger: log}
}

func RegisterExecutorProfileRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *service.Service, log *logger.Logger) {
	handlers := NewExecutorProfileHandlers(svc, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *ExecutorProfileHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/executors/:id/profiles", h.httpListProfiles)
	api.POST("/executors/:id/profiles", h.httpCreateProfile)
	api.GET("/executors/:id/profiles/:profileId", h.httpGetProfile)
	api.PATCH("/executors/:id/profiles/:profileId", h.httpUpdateProfile)
	api.DELETE("/executors/:id/profiles/:profileId", h.httpDeleteProfile)
}

func (h *ExecutorProfileHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionExecutorProfileList, h.wsListProfiles)
	dispatcher.RegisterFunc(ws.ActionExecutorProfileCreate, h.wsCreateProfile)
	dispatcher.RegisterFunc(ws.ActionExecutorProfileGet, h.wsGetProfile)
	dispatcher.RegisterFunc(ws.ActionExecutorProfileUpdate, h.wsUpdateProfile)
	dispatcher.RegisterFunc(ws.ActionExecutorProfileDelete, h.wsDeleteProfile)
}

func (h *ExecutorProfileHandlers) httpListProfiles(c *gin.Context) {
	executorID := c.Param("id")
	profiles, err := h.service.ListExecutorProfiles(c.Request.Context(), executorID)
	if err != nil {
		h.logger.Error("failed to list executor profiles", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list profiles"})
		return
	}
	resp := dto.ListExecutorProfilesResponse{
		Profiles: make([]dto.ExecutorProfileDTO, 0, len(profiles)),
		Total:    len(profiles),
	}
	for _, p := range profiles {
		resp.Profiles = append(resp.Profiles, dto.FromExecutorProfile(p))
	}
	c.JSON(http.StatusOK, resp)
}

type httpCreateProfileRequest struct {
	Name        string                 `json:"name"`
	IsDefault   bool                   `json:"is_default"`
	Config      map[string]string      `json:"config,omitempty"`
	SetupScript string                 `json:"setup_script"`
	EnvVars     []models.ProfileEnvVar `json:"env_vars,omitempty"`
}

func (h *ExecutorProfileHandlers) httpCreateProfile(c *gin.Context) {
	var body httpCreateProfileRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	profile, err := h.service.CreateExecutorProfile(c.Request.Context(), &service.CreateExecutorProfileRequest{
		ExecutorID:  c.Param("id"),
		Name:        body.Name,
		IsDefault:   body.IsDefault,
		Config:      body.Config,
		SetupScript: body.SetupScript,
		EnvVars:     body.EnvVars,
	})
	if err != nil {
		h.logger.Error("failed to create executor profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "profile not created"})
		return
	}
	c.JSON(http.StatusOK, dto.FromExecutorProfile(profile))
}

func (h *ExecutorProfileHandlers) httpGetProfile(c *gin.Context) {
	profile, err := h.service.GetExecutorProfile(c.Request.Context(), c.Param("profileId"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}
	c.JSON(http.StatusOK, dto.FromExecutorProfile(profile))
}

type httpUpdateProfileRequest struct {
	Name        *string                `json:"name,omitempty"`
	IsDefault   *bool                  `json:"is_default,omitempty"`
	Config      map[string]string      `json:"config,omitempty"`
	SetupScript *string                `json:"setup_script,omitempty"`
	EnvVars     []models.ProfileEnvVar `json:"env_vars,omitempty"`
}

func (h *ExecutorProfileHandlers) httpUpdateProfile(c *gin.Context) {
	var body httpUpdateProfileRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	profile, err := h.service.UpdateExecutorProfile(c.Request.Context(), c.Param("profileId"), &service.UpdateExecutorProfileRequest{
		Name:        body.Name,
		IsDefault:   body.IsDefault,
		Config:      body.Config,
		SetupScript: body.SetupScript,
		EnvVars:     body.EnvVars,
	})
	if err != nil {
		h.logger.Error("failed to update executor profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "profile not updated"})
		return
	}
	c.JSON(http.StatusOK, dto.FromExecutorProfile(profile))
}

func (h *ExecutorProfileHandlers) httpDeleteProfile(c *gin.Context) {
	if err := h.service.DeleteExecutorProfile(c.Request.Context(), c.Param("profileId")); err != nil {
		h.logger.Error("failed to delete executor profile", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile not deleted"})
		return
	}
	c.JSON(http.StatusOK, dto.SuccessResponse{Success: true})
}

// WebSocket handlers

type wsListProfilesRequest struct {
	ExecutorID string `json:"executor_id"`
}

func (h *ExecutorProfileHandlers) wsListProfiles(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsListProfilesRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ExecutorID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "executor_id is required", nil)
	}
	profiles, err := h.service.ListExecutorProfiles(ctx, req.ExecutorID)
	if err != nil {
		h.logger.Error("failed to list executor profiles", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list profiles", nil)
	}
	resp := dto.ListExecutorProfilesResponse{
		Profiles: make([]dto.ExecutorProfileDTO, 0, len(profiles)),
		Total:    len(profiles),
	}
	for _, p := range profiles {
		resp.Profiles = append(resp.Profiles, dto.FromExecutorProfile(p))
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateProfileRequest struct {
	ExecutorID  string                 `json:"executor_id"`
	Name        string                 `json:"name"`
	IsDefault   bool                   `json:"is_default"`
	Config      map[string]string      `json:"config,omitempty"`
	SetupScript string                 `json:"setup_script"`
	EnvVars     []models.ProfileEnvVar `json:"env_vars,omitempty"`
}

func (h *ExecutorProfileHandlers) wsCreateProfile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateProfileRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ExecutorID == "" || req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "executor_id and name are required", nil)
	}
	profile, err := h.service.CreateExecutorProfile(ctx, &service.CreateExecutorProfileRequest{
		ExecutorID:  req.ExecutorID,
		Name:        req.Name,
		IsDefault:   req.IsDefault,
		Config:      req.Config,
		SetupScript: req.SetupScript,
		EnvVars:     req.EnvVars,
	})
	if err != nil {
		h.logger.Error("failed to create executor profile", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create profile", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromExecutorProfile(profile))
}

type wsGetProfileRequest struct {
	ID string `json:"id"`
}

func (h *ExecutorProfileHandlers) wsGetProfile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetProfileRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	profile, err := h.service.GetExecutorProfile(ctx, req.ID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Profile not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromExecutorProfile(profile))
}

type wsUpdateProfileRequest struct {
	ID          string                 `json:"id"`
	Name        *string                `json:"name,omitempty"`
	IsDefault   *bool                  `json:"is_default,omitempty"`
	Config      map[string]string      `json:"config,omitempty"`
	SetupScript *string                `json:"setup_script,omitempty"`
	EnvVars     []models.ProfileEnvVar `json:"env_vars,omitempty"`
}

func (h *ExecutorProfileHandlers) wsUpdateProfile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateProfileRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	profile, err := h.service.UpdateExecutorProfile(ctx, req.ID, &service.UpdateExecutorProfileRequest{
		Name:        req.Name,
		IsDefault:   req.IsDefault,
		Config:      req.Config,
		SetupScript: req.SetupScript,
		EnvVars:     req.EnvVars,
	})
	if err != nil {
		h.logger.Error("failed to update executor profile", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update profile", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromExecutorProfile(profile))
}

func (h *ExecutorProfileHandlers) wsDeleteProfile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetProfileRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	if err := h.service.DeleteExecutorProfile(ctx, req.ID); err != nil {
		h.logger.Error("failed to delete executor profile", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete profile", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.SuccessResponse{Success: true})
}
