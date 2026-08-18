package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// operatorOnlyConfigKeys are executor profile Config keys that the
// agent-exposed MCP tools (create_executor_profile, update_executor_profile)
// must reject. These keys can only be set through the operator HTTP/WS API.
// This is a narrow security guard: it prevents self-enablement of container
// security relaxations from within a task, without widening the surface onto
// the operator path (which already exposes prepare_script — strictly more
// powerful).
var operatorOnlyConfigKeys = []string{
	lifecycle.MetadataKeyAllowUserNamespaces,
}

// rejectOperatorConfigKeys returns an error if the config map contains any
// key that is reserved for the operator path. The error names the offending
// key and returns ws.ErrorCodeValidation.
func rejectOperatorConfigKeys(config map[string]string) error {
	var offending []string
	for _, key := range operatorOnlyConfigKeys {
		if _, ok := config[key]; ok {
			offending = append(offending, key)
		}
	}
	if len(offending) > 0 {
		return fmt.Errorf("config key(s) reserved for operator: %s", strings.Join(offending, ", "))
	}
	return nil
}

func (h *Handlers) handleListExecutors(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	executors, err := h.taskSvc.ListExecutors(ctx)
	if err != nil {
		h.logger.Error("failed to list executors", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list executors", nil)
	}

	profiles, err := h.taskSvc.ListAllExecutorProfiles(ctx)
	if err != nil {
		h.logger.Error("failed to list executor profiles for executor listing", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list executor profiles", nil)
	}
	profilesByExecutor := make(map[string][]dto.ExecutorProfileDTO, len(executors))
	for _, profile := range profiles {
		profilesByExecutor[profile.ExecutorID] = append(profilesByExecutor[profile.ExecutorID], dto.FromExecutorProfile(profile))
	}

	dtos := make([]dto.ExecutorDTO, 0, len(executors))
	for _, executor := range executors {
		executorDTO := dto.FromExecutor(executor)
		executorDTO.Profiles = profilesByExecutor[executor.ID]
		dtos = append(dtos, executorDTO)
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

	if err := rejectOperatorConfigKeys(req.Config); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
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

	if err := rejectOperatorConfigKeys(req.Config); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
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
