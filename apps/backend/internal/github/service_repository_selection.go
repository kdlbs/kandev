package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type repositorySelectionReader interface {
	GetRepository(context.Context, string, string) (*GitHubRepository, error)
}

// InspectRepositoryForWorkspace verifies one exact GitHub repository through
// the workspace-scoped personal and automation visibility boundaries.
func (s *Service) InspectRepositoryForWorkspace(
	ctx context.Context, workspaceID, userID, owner, name string,
) (*GitHubRepository, error) {
	resolved, err := s.resolvePersonalReadClient(ctx, workspaceID, userID, owner, name)
	if err != nil {
		return nil, err
	}
	reader, ok := resolved.Client.(repositorySelectionReader)
	if !ok {
		return nil, fmt.Errorf("GitHub client cannot inspect repositories")
	}
	repository, err := reader.GetRepository(ctx, owner, name)
	if err != nil {
		return nil, classifyRepositoryRootReadError(err)
	}
	if repository == nil || !strings.EqualFold(repository.Owner, owner) || !strings.EqualFold(repository.Name, name) {
		return nil, ErrRepoNotResolvable
	}
	return repository, nil
}

func classifyRepositoryRootReadError(err error) error {
	var apiErr *GitHubAPIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %w", ErrRepoNotResolvable, err)
	}
	return err
}
