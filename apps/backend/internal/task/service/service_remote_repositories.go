package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
)

// RegisterRemoteRepositoryRequest registers a provider-hosted repository in a
// workspace without a task. The locator is verified the same way task creation
// verifies one: built-in hosts through the URL parser, plugin providers through
// the workspace's repository selection resolver.
type RegisterRemoteRepositoryRequest struct {
	WorkspaceID    string
	RemoteURL      string
	Provider       string
	ProviderHost   string
	ProviderScope  string
	ProviderRepoID string
	ProviderOwner  string
	ProviderName   string
	DefaultBranch  string
}

// RegisterRemoteRepository resolves the locator to a workspace repository,
// creating it when no repository with that provider identity exists yet. The
// returned flag reports whether this call inserted the row.
func (s *Service) RegisterRemoteRepository(
	ctx context.Context, req *RegisterRemoteRepositoryRequest,
) (*models.Repository, bool, error) {
	if err := s.AuthorizeWorkspaceScope(ctx, req.WorkspaceID, authz.ScopeRepositoryManage); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(req.RemoteURL) == "" {
		return nil, false, fmt.Errorf("%w: remote_url is required", ErrInvalidRepositorySettings)
	}
	inputs := []TaskRepositoryInput{{
		RemoteURL:               strings.TrimSpace(req.RemoteURL),
		Provider:                strings.TrimSpace(req.Provider),
		ProviderHost:            strings.TrimSpace(req.ProviderHost),
		ProviderScope:           strings.TrimSpace(req.ProviderScope),
		ProviderRepoID:          strings.TrimSpace(req.ProviderRepoID),
		ProviderOwner:           strings.TrimSpace(req.ProviderOwner),
		ProviderName:            strings.TrimSpace(req.ProviderName),
		DefaultBranch:           strings.TrimSpace(req.DefaultBranch),
		ResolveProviderDefaults: true,
	}}
	if err := s.preflightRepositoryInputs(ctx, req.WorkspaceID, inputs); err != nil {
		return nil, false, err
	}
	if err := validateBuiltInRemoteHints(inputs[0]); err != nil {
		return nil, false, err
	}
	repositoryID, _, created, err := s.ResolveRepositoryRef(ctx, req.WorkspaceID, inputs[0])
	if err != nil {
		return nil, false, err
	}
	if repositoryID == "" {
		return nil, false, fmt.Errorf("%w: remote repository locator did not resolve", ErrInvalidRepositorySettings)
	}
	repository, err := s.repoEntities.GetRepository(ctx, repositoryID)
	if err != nil {
		return nil, false, err
	}
	return repository, created, nil
}

// validateBuiltInRemoteHints applies the same provider, owner, name, and host
// agreement checks that built-in resolution enforces, but as a typed
// selection error so a caller-supplied hint that disagrees with the locator
// is a client error rather than an internal failure. Plugin descriptors are
// already authoritative by the time this runs.
func validateBuiltInRemoteHints(input TaskRepositoryInput) error {
	if input.TrustedProviderDescriptor {
		return nil
	}
	provider, owner, name, canonicalURL, err := parseRemoteRepositoryURL(effectiveRemoteURL(input), input.Provider)
	if err != nil {
		return NewRepositorySelectionError(RepositorySelectionErrorInvalid, err)
	}
	if input.Provider != "" && !strings.EqualFold(input.Provider, provider) {
		return NewRepositorySelectionError(RepositorySelectionErrorInvalid,
			fmt.Errorf("remote_url provider %q does not match provider %q", provider, input.Provider))
	}
	if _, err := validateRemoteRepositoryMetadata(input, provider, owner, name); err != nil {
		return NewRepositorySelectionError(RepositorySelectionErrorInvalid, err)
	}
	providerHost := remoteProviderHost(provider, canonicalURL)
	if input.ProviderHost != "" && !strings.EqualFold(strings.TrimRight(input.ProviderHost, "/"), providerHost) {
		return NewRepositorySelectionError(RepositorySelectionErrorInvalid,
			fmt.Errorf("remote_url provider host %q does not match %q", providerHost, input.ProviderHost))
	}
	if provider == providerGitLab && providerHost != "https://gitlab.com" {
		return NewRepositorySelectionError(RepositorySelectionErrorInvalid,
			fmt.Errorf("untrusted GitLab origin %q", providerHost))
	}
	return nil
}
