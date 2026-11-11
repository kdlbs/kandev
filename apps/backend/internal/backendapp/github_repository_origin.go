package backendapp

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/github"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type gitHubRepositoryHostBinder interface {
	BindWorkspaceGitHubHost(context.Context, string, string) (context.Context, error)
}

type gitHubRepositoryOriginVerifier struct{ connections gitHubRepositoryHostBinder }

func (v gitHubRepositoryOriginVerifier) VerifyGitHubRepositoryOrigin(ctx context.Context, workspaceID, origin string) error {
	_, err := v.connections.BindWorkspaceGitHubHost(ctx, workspaceID, origin)
	if err == nil {
		return nil
	}
	code := taskservice.RepositorySelectionErrorUnavailable
	if errors.Is(err, github.ErrGitHubHostMismatch) || errors.Is(err, github.ErrGitHubNotConfigured) {
		code = taskservice.RepositorySelectionErrorInvalid
	}
	return taskservice.NewRepositorySelectionError(code, err)
}
