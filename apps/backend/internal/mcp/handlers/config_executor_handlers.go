package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (h *Handlers) handleListExecutors(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	executors, err := h.taskSvc.ListExecutors(ctx)
	if err != nil {
		h.logger.Error("failed to list executors", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list executors", nil)
	}
	dtos := make([]dto.ExecutorDTO, 0, len(executors))
	for _, e := range executors {
		dtos = append(dtos, dto.FromExecutor(e))
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.ListExecutorsResponse{Executors: dtos, Total: len(dtos)})
}

func (h *Handlers) handleListExecutorProfiles(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.handleListByField(ctx, msg, "executor_id", "failed to list executor profiles", "Failed to list executor profiles",
		func(ctx context.Context, executorID string) (any, error) {
			profiles, err := h.taskSvc.ListExecutorProfiles(ctx, executorID)
			if err != nil {
				return nil, err
			}
			dtos := make([]dto.ExecutorProfileDTO, 0, len(profiles))
			for _, p := range profiles {
				dtos = append(dtos, dto.FromExecutorProfile(p))
			}
			return dto.ListExecutorProfilesResponse{Profiles: dtos, Total: len(dtos)}, nil
		})
}

func (h *Handlers) handleCreateExecutorProfile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		ExecutorID    string                 `json:"executor_id"`
		Name          string                 `json:"name"`
		McpPolicy     string                 `json:"mcp_policy"`
		Config        map[string]string      `json:"config"`
		PrepareScript string                 `json:"prepare_script"`
		CleanupScript string                 `json:"cleanup_script"`
		EnvVars       []models.ProfileEnvVar `json:"env_vars"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ExecutorID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "executor_id is required", nil)
	}
	if req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}

	profile, err := h.taskSvc.CreateExecutorProfile(ctx, &service.CreateExecutorProfileRequest{
		ExecutorID:    req.ExecutorID,
		Name:          req.Name,
		McpPolicy:     req.McpPolicy,
		Config:        req.Config,
		PrepareScript: req.PrepareScript,
		CleanupScript: req.CleanupScript,
		EnvVars:       req.EnvVars,
	})
	if err != nil {
		h.logger.Error("failed to create executor profile", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create executor profile: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromExecutorProfile(profile))
}

func (h *Handlers) handleUpdateExecutorProfile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		ProfileID     string                 `json:"profile_id"`
		Name          *string                `json:"name"`
		McpPolicy     *string                `json:"mcp_policy"`
		Config        map[string]string      `json:"config"`
		PrepareScript *string                `json:"prepare_script"`
		CleanupScript *string                `json:"cleanup_script"`
		EnvVars       []models.ProfileEnvVar `json:"env_vars"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ProfileID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "profile_id is required", nil)
	}

	profile, err := h.taskSvc.UpdateExecutorProfile(ctx, req.ProfileID, &service.UpdateExecutorProfileRequest{
		Name:          req.Name,
		McpPolicy:     req.McpPolicy,
		Config:        req.Config,
		PrepareScript: req.PrepareScript,
		CleanupScript: req.CleanupScript,
		EnvVars:       req.EnvVars,
	})
	if err != nil {
		h.logger.Error("failed to update executor profile", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update executor profile: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromExecutorProfile(profile))
}

func (h *Handlers) handleDeleteExecutorProfile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.handleDeleteByField(ctx, msg, "profile_id", "failed to delete executor profile", "Failed to delete executor profile",
		func(ctx context.Context, id string) error { return h.taskSvc.DeleteExecutorProfile(ctx, id) })
}
