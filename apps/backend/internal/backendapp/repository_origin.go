package backendapp

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/gitlab"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type gitLabWorkspaceConnectionReader interface {
	GetConfigForWorkspace(context.Context, string) (*gitlab.GitLabConfig, error)
}

type gitLabRepositoryOriginVerifier struct {
	connections gitLabWorkspaceConnectionReader
}

func (v gitLabRepositoryOriginVerifier) VerifyGitLabRepositoryOrigin(ctx context.Context, workspaceID, origin string) error {
	config, err := v.connections.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorUnavailable, err)
	}
	if config == nil || config.AuthMethod == "" || config.AuthMethod == gitlab.AuthMethodNone || !strings.EqualFold(strings.TrimRight(config.Host, "/"), origin) {
		return taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorInvalid, nil)
	}
	return nil
}
