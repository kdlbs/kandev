package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

func isHostWorkspaceSourceExecutor(executor string) bool {
	return isLocalWorkspaceExecutor(executor) || executor == string(models.ExecutorTypeWorktree)
}

// Older callers omit placement. Provisioned host environments attach beneath
// their established root; only explicit expansion may change that root.
func (s *Service) applyDefaultWorkspaceSourcePlacement(ctx context.Context, task *models.Task, batch *models.WorkspaceSourceBatch) error {
	if s.taskEnvironments == nil || batch == nil || len(batch.Sources) == 0 {
		return nil
	}
	env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, task.ID)
	if err != nil {
		return err
	}
	if env == nil || env.Status == models.TaskEnvironmentStatusCreating || !isHostWorkspaceSourceExecutor(env.ExecutorType) || env.WorkspacePath == "" {
		return nil
	}
	targets, paths, err := s.buildWorkspaceRepositoryPlacementTargets(ctx, task.WorkspaceID, env, batch.Sources, WorkspacePlacementCurrentRoot)
	if err != nil {
		return err
	}
	if err := preflightWorkspaceRepositoryDestinations(paths); err != nil {
		return err
	}
	for _, target := range targets {
		if target.repository != nil {
			target.repository.WorkspaceRelativePath = target.relative
		}
		if target.folder != nil {
			target.folder.WorkspaceRelativePath = target.relative
		}
	}
	batch.RepositoryPlacement = string(WorkspacePlacementCurrentRoot)
	return nil
}

// Preview resolves identity and naming without registering, backfilling, or
// cloning a repository. Attachment repeats resolution under its mutation lock.
func (s *Service) previewWorkspaceRepositoryEntity(ctx context.Context, workspaceID string, source WorkspaceSourceInput) (*models.Repository, error) {
	if source.RepositoryID != "" {
		entity, err := s.repositoryEntityInWorkspace(ctx, workspaceID, source.RepositoryID)
		if err != nil {
			return nil, err
		}
		if err := s.validateRepositoryWorkspaceSourceInput(ctx, &models.Task{WorkspaceID: workspaceID}, source); err != nil {
			return nil, err
		}
		return entity, nil
	}
	if err := s.validateRepositoryWorkspaceSourceInput(ctx, &models.Task{WorkspaceID: workspaceID}, source); err != nil {
		return nil, err
	}
	if source.LocalPath != "" {
		return s.previewLocalWorkspaceRepository(ctx, workspaceID, source.LocalPath)
	}
	return s.previewRemoteWorkspaceRepository(ctx, workspaceID, source)
}

func (s *Service) previewLocalWorkspaceRepository(ctx context.Context, workspaceID, path string) (*models.Repository, error) {
	canonical, branch, err := resolveExplicitLocalRepositoryPath(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWorkspaceSource, err)
	}
	if isKandevTaskWorktreePath(canonical, s.discoveryConfig.TaskWorktreeRoots) {
		return nil, fmt.Errorf("%w: use the source repository instead of a task worktree", ErrInvalidWorkspaceSource)
	}
	repos, err := s.repoEntities.ListRepositories(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, repo := range repos {
		if repo.LocalPath == canonical {
			return repo, nil
		}
	}
	return &models.Repository{WorkspaceID: workspaceID, Name: filepath.Base(path), LocalPath: canonical, DefaultBranch: branch}, nil
}

func (s *Service) previewRemoteWorkspaceRepository(ctx context.Context, workspaceID string, source WorkspaceSourceInput) (*models.Repository, error) {
	input := TaskRepositoryInput{RemoteURL: source.RemoteURL, GitHubURL: source.GitHubURL, Provider: source.Provider, ProviderOwner: source.ProviderOwner, ProviderName: source.ProviderName}
	provider, owner, name, canonical, err := parseRemoteRepositoryURL(effectiveRemoteURL(input), input.Provider)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWorkspaceSource, err)
	}
	owner, err = validateRemoteRepositoryMetadata(input, provider, owner, name)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWorkspaceSource, err)
	}
	host := remoteProviderHost(provider, canonical)
	if (source.Provider != "" && !strings.EqualFold(source.Provider, provider)) ||
		(source.ProviderHost != "" && !strings.EqualFold(strings.TrimRight(source.ProviderHost, "/"), host)) ||
		(provider == providerGitLab && host != "https://gitlab.com") {
		return nil, fmt.Errorf("%w: unsupported remote repository origin", ErrInvalidWorkspaceSource)
	}
	existing, err := s.repoEntities.GetRepositoryByProviderIdentity(ctx, models.ProviderRepositoryIdentity{WorkspaceID: workspaceID, Provider: provider, Host: host, RepositoryID: source.ProviderRepoID, Owner: owner, Name: name})
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	branch := source.BaseBranch
	if branch == "" && provider == providerGitHub && s.providerProber != nil {
		branch = s.probeProviderDefaultBranchIfMissing(ctx, workspaceID, provider, owner, name)
	}
	return &models.Repository{WorkspaceID: workspaceID, Name: owner + "/" + name, Provider: provider, ProviderHost: host, ProviderOwner: owner, ProviderName: name, RemoteURL: canonical, DefaultBranch: branch}, nil
}
