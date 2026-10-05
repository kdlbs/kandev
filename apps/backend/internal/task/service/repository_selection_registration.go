package service

import (
	"context"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
)

// InspectRemoteRepositorySelection returns the server-verified provider
// descriptor without changing workspace state.
func (s *Service) InspectRemoteRepositorySelection(
	ctx context.Context, workspaceID string, input TaskRepositoryInput,
) (TaskRepositoryInput, error) {
	if err := s.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return TaskRepositoryInput{}, err
	}
	return s.verifyRemoteRepositorySelection(ctx, workspaceID, input)
}

// RegisterRemoteRepositorySelection re-verifies a client selection before
// persisting its canonical provider identity. FindOrCreateRepository owns
// concurrent deduplication; registration does not clone or create a task.
func (s *Service) RegisterRemoteRepositorySelection(
	ctx context.Context, workspaceID string, input TaskRepositoryInput,
) (*models.Repository, error) {
	if err := s.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeRepositoryManage); err != nil {
		return nil, err
	}
	verified, err := s.verifyRemoteRepositorySelection(ctx, workspaceID, input)
	if err != nil {
		return nil, err
	}
	id, _, _, err := s.resolveTrustedRemoteRepository(ctx, workspaceID, verified, "")
	if err != nil {
		return nil, err
	}
	return s.GetRepository(ctx, id)
}

func (s *Service) verifyRemoteRepositorySelection(
	ctx context.Context, workspaceID string, input TaskRepositoryInput,
) (TaskRepositoryInput, error) {
	input.RemoteURL = strings.TrimSpace(input.RemoteURL)
	input.Provider = strings.TrimSpace(input.Provider)
	if input.RemoteURL == "" || input.Provider == "" || input.RepositoryID != "" || input.LocalPath != "" {
		return TaskRepositoryInput{}, NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
	}
	if s.repositorySelectionResolver == nil {
		return TaskRepositoryInput{}, NewRepositorySelectionError(RepositorySelectionErrorUnavailable, nil)
	}
	resolved, err := s.repositorySelectionResolver.ResolveRepositorySelection(ctx, workspaceID, input)
	if err != nil {
		return TaskRepositoryInput{}, normalizeRepositorySelectionError(err)
	}
	if !repositorySelectionHintsMatch(input, resolved) {
		return TaskRepositoryInput{}, NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
	}
	verified, err := trustResolvedRepositorySelection(input, resolved)
	if err != nil {
		return TaskRepositoryInput{}, err
	}
	if err := validateTrustedRemoteRepository(verified); err != nil {
		return TaskRepositoryInput{}, NewRepositorySelectionError(RepositorySelectionErrorInvalid, err)
	}
	if strings.TrimSpace(verified.DefaultBranch) == "" {
		return TaskRepositoryInput{}, NewRepositorySelectionError(RepositorySelectionErrorInvalid, errors.New("provider default branch is required"))
	}
	return verified, nil
}
