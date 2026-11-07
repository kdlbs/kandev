package executor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/kandev/kandev/internal/task/models"
)

type managedAttachmentCloner interface {
	InspectManagedAttachmentPath(context.Context, *models.Repository, string) (bool, error)
	ManagedAttachmentContext(context.Context, *models.Repository) (context.Context, error)
	WorkspaceProviderRepositoryPath(string, string, string, string, string, string, string) (string, error)
}

func (e *Executor) materializeHostAttachment(ctx context.Context, taskID, sessionID string, repository *models.Repository) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if repository.SourceType == sourceTypeLocal || (repository.LocalPath != "" && repository.SourceType != "provider") {
		return repository.LocalPath, nil
	}
	if taskID == "" || sessionID == "" {
		return "", errors.New("managed attachment requires task and session identity")
	}
	cloner, ok := e.repoCloner.(managedAttachmentCloner)
	if !ok {
		return "", errors.New("managed attachment path validation is unavailable")
	}
	path := repository.LocalPath
	if path == "" {
		var err error
		path, err = attachmentDestination(cloner, repository.WorkspaceID, repository.Provider, repository.ProviderHost, repository.ProviderScope, repository.ProviderRepoID, repository.ProviderOwner, repository.ProviderName)
		if err != nil {
			return "", err
		}
	}
	exists, err := cloner.InspectManagedAttachmentPath(ctx, repository, path)
	if err != nil {
		return "", err
	}
	if !exists {
		path, err = e.cloneHostAttachment(ctx, taskID, sessionID, repository, cloner)
		if err != nil {
			return "", err
		}
	}
	return e.persistHostAttachment(ctx, repository, path, !exists)
}

func (e *Executor) persistHostAttachment(ctx context.Context, repository *models.Repository, path string, cloned bool) (string, error) {
	if cloned || path != repository.LocalPath {
		updater, ok := e.repoUpdater.(interface {
			UpdateVerifiedRepositoryLocalPath(context.Context, *models.Repository, string) error
		})
		if !ok {
			return "", errors.New("verified attachment path persistence is unavailable")
		}
		if err := updater.UpdateVerifiedRepositoryLocalPath(ctx, repository, path); err != nil {
			return "", fmt.Errorf("persist verified attachment path: %w", err)
		}
	}
	repository.LocalPath = path
	return path, nil
}

func (e *Executor) cloneHostAttachment(ctx context.Context, taskID, sessionID string, repository *models.Repository, cloner managedAttachmentCloner) (string, error) {
	scoped, err := cloner.ManagedAttachmentContext(ctx, repository)
	if err != nil {
		return "", err
	}
	cloneURL := repository.RemoteURL
	if cloneURL == "" {
		cloneURL, err = e.repoCloner.BuildCloneURLWithHost(scoped, repository.Provider, repository.ProviderHost, repository.ProviderOwner, repository.ProviderName)
		if err != nil {
			return "", err
		}
	}
	if cloneURL == "" {
		return "", ErrNoCloneURL
	}
	path, _, err := e.ensureClonedWithWorkspaceAuthForSessionAndState(scoped, taskID, sessionID, repository, cloneURL)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", errors.New("managed attachment clone produced no path")
	}
	expected, err := attachmentDestination(cloner, repository.WorkspaceID, repository.Provider, repository.ProviderHost, repository.ProviderScope, repository.ProviderRepoID, repository.ProviderOwner, repository.ProviderName)
	path, pathErr := filepath.Abs(path)
	if err != nil || pathErr != nil || path != expected {
		return "", errors.New("managed attachment clone returned an unexpected path")
	}
	exists, err := cloner.InspectManagedAttachmentPath(ctx, repository, path)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", errors.New("managed attachment clone checkout is missing")
	}
	return path, nil
}

func attachmentDestination(cloner managedAttachmentCloner, workspaceID, provider, host, scope, repoID, owner, name string) (string, error) {
	path, err := cloner.WorkspaceProviderRepositoryPath(workspaceID, provider, host, scope, repoID, owner, name)
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}
