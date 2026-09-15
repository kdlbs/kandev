package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

// normalizeWorkspaceSourcesForCreation makes the presence-aware source list
// the only repository input used by creation. A nil list keeps the legacy
// repositories/workspace_path contract, while an empty list deliberately
// creates an empty scratch workspace and suppresses parent repository
// inheritance.
func normalizeWorkspaceSourcesForCreation(req *CreateTaskRequest) error {
	if req.WorkspaceSources == nil {
		return nil
	}
	if len(req.Repositories) > 0 || strings.TrimSpace(req.WorkspacePath) != "" {
		return fmt.Errorf("%w: workspace_sources cannot be combined with repositories or workspace_path", ErrInvalidWorkspaceSource)
	}
	if req.WorkspacePolicy != nil && req.WorkspacePolicy.Mode == workspaceModeInheritParent {
		return fmt.Errorf("%w: workspace sources cannot be changed for an inherited workspace", ErrInvalidWorkspaceSource)
	}

	repositories := make([]TaskRepositoryInput, 0, len(*req.WorkspaceSources))
	for _, source := range *req.WorkspaceSources {
		if source.Kind == WorkspaceSourceFolder && (source.CheckoutSource != "" || source.ExpectedOrigin != "") {
			return fmt.Errorf("%w: checkout source fields are only valid for repository sources", ErrInvalidWorkspaceSource)
		}
		if source.Kind != WorkspaceSourceRepository {
			continue
		}
		if source.CheckoutSource != "" && source.CheckoutSource != checkoutSourceRemoteOrigin {
			return fmt.Errorf("%w: unsupported checkout_source %q", ErrInvalidWorkspaceSource, source.CheckoutSource)
		}
		if source.CheckoutSource == "" && source.ExpectedOrigin != "" {
			return fmt.Errorf("%w: expected_origin requires checkout_source", ErrInvalidWorkspaceSource)
		}
		repositories = append(repositories, TaskRepositoryInput{
			RepositoryID:   source.RepositoryID,
			BaseBranch:     source.BaseBranch,
			CheckoutBranch: source.CheckoutBranch,
			BranchPolicyID: source.BranchPolicyID,
			PRNumber:       source.PRNumber,
			LocalPath:      source.LocalPath,
			GitHubURL:      source.GitHubURL,
			RemoteURL:      source.RemoteURL,
			Provider:       source.Provider,
			ProviderHost:   source.ProviderHost,
			ProviderScope:  source.ProviderScope,
			ProviderRepoID: source.ProviderRepoID,
			ProviderOwner:  source.ProviderOwner,
			ProviderName:   source.ProviderName,
			CheckoutSource: source.CheckoutSource,
			ExpectedOrigin: source.ExpectedOrigin,
		})
	}
	req.Repositories = repositories
	return nil
}

func (s *Service) prepareWorkspaceSourceBatchForCreation(
	ctx context.Context,
	task *models.Task,
	inputs []WorkspaceSourceInput,
	repositories []*models.TaskRepository,
) (*models.WorkspaceSourceBatch, error) {
	if len(inputs) == 0 {
		return &models.WorkspaceSourceBatch{TaskID: task.ID}, nil
	}
	if s.workspaceSourceStore() == nil {
		return nil, fmt.Errorf("%w: workspace source persistence is unavailable", ErrWorkspaceSourceMaterialize)
	}

	batch := &models.WorkspaceSourceBatch{TaskID: task.ID, Sources: make([]models.WorkspaceSource, 0, len(inputs))}
	seenPaths := make(map[string]bool, len(inputs))
	seenNames := make(map[string]bool, len(inputs))
	repositoryIndex := 0
	for _, input := range inputs {
		switch input.Kind {
		case WorkspaceSourceFolder:
			folder, err := prepareFolderWorkspaceSource(input, seenPaths, seenNames)
			if err != nil {
				return nil, err
			}
			batch.Sources = append(batch.Sources, models.WorkspaceSource{Folder: folder})
		case WorkspaceSourceRepository:
			if repositoryIndex >= len(repositories) {
				return nil, fmt.Errorf("%w: repository source resolution is incomplete", ErrInvalidWorkspaceSource)
			}
			batch.Sources = append(batch.Sources, models.WorkspaceSource{Repository: repositories[repositoryIndex]})
			repositoryIndex++
		default:
			return nil, fmt.Errorf("%w: unsupported workspace source kind %q", ErrInvalidWorkspaceSource, input.Kind)
		}
	}
	if repositoryIndex != len(repositories) {
		return nil, fmt.Errorf("%w: repository source resolution produced an unexpected count", ErrInvalidWorkspaceSource)
	}
	if err := s.rejectRuntimeNameCollisions(ctx, repositories, batch); err != nil {
		return nil, err
	}
	return batch, nil
}
