package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (h *Handlers) handleListRepositories(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.handleListByField(ctx, msg, "workspace_id", "failed to list repositories", "Failed to list repositories",
		func(ctx context.Context, workspaceID string) (any, error) {
			repos, err := h.taskSvc.ListRepositories(ctx, workspaceID)
			if err != nil {
				return nil, err
			}
			dtos := make([]dto.RepositoryDTO, 0, len(repos))
			for _, r := range repos {
				dtos = append(dtos, dto.FromRepository(r))
			}
			return dto.ListRepositoriesResponse{Repositories: dtos, Total: len(dtos)}, nil
		})
}

func (h *Handlers) handleCreateRepository(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
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
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
	}
	if req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}

	repo, err := h.taskSvc.CreateRepository(ctx, &service.CreateRepositoryRequest{
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
	})
	if err != nil {
		h.logger.Error("failed to create repository", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create repository", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromRepository(repo))
}

func (h *Handlers) handleDeleteRepository(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.handleDeleteByField(ctx, msg, "repository_id", "failed to delete repository", "Failed to delete repository",
		func(ctx context.Context, repositoryID string) error {
			return h.taskSvc.DeleteRepository(ctx, repositoryID)
		})
}
