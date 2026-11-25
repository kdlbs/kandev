package github

import (
	"context"
	"errors"
	"strings"
)

// ResolveRepoRefForWorkspace resolves a repository ref through the workspace's GitHub connection.
func (s *Service) ResolveRepoRefForWorkspace(
	ctx context.Context, workspaceID, owner, repo, ref string,
) (string, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return "", err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return "", err
	}
	resolver, ok := resolved.Client.(RepoRefSHAResolver)
	if !ok {
		return "", errors.New("GitHub client does not support repository ref resolution")
	}
	sha, err := resolver.ResolveRepoRefSHA(ctx, owner, repo, ref)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(sha) == "" {
		return "", errors.New("GitHub repository ref resolved to an empty commit SHA")
	}
	return sha, nil
}

func (s *Service) ListRepoDirectoryForWorkspace(
	ctx context.Context, workspaceID, owner, repo, path, ref string,
) ([]RepoContentEntry, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return nil, err
	}
	return resolved.Client.ListRepoDirectory(ctx, owner, repo, path, ref)
}

func (s *Service) GetRepoFileContentForWorkspace(
	ctx context.Context, workspaceID, owner, repo, path, ref string,
) ([]byte, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return nil, err
	}
	return resolved.Client.GetRepoFileContent(ctx, owner, repo, path, ref)
}
