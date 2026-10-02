package service

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/kandev/kandev/internal/task/models"
)

// findRepositoryForRemoteSelection runs under repoResolveMu so skipped local
// registrations cannot cause concurrent remote selections to insert duplicates.
func (s *Service) findRepositoryForRemoteSelection(
	ctx context.Context, req *FindOrCreateRepositoryRequest,
) (*models.Repository, error) {
	identity := models.ProviderRepositoryIdentity{
		WorkspaceID: req.WorkspaceID, Provider: req.Provider, Scope: req.ProviderScope,
		RepositoryID: req.ProviderRepoID, Host: req.ProviderHost, Owner: req.ProviderOwner, Name: req.ProviderName,
	}
	existing, err := s.repoEntities.GetRepositoryByProviderIdentity(ctx, identity)
	if err != nil || existing == nil || req.RemoteURL == "" || req.LocalPath != "" || s.isKandevTaskWorktreeRepository(existing) {
		return existing, err
	}
	eligible, err := remoteRepositoryCandidateAvailable(ctx, existing)
	if err != nil || eligible {
		return existing, err
	}
	return s.findAvailableRemoteRepository(ctx, identity, existing.ID)
}

func (s *Service) findAvailableRemoteRepository(
	ctx context.Context, identity models.ProviderRepositoryIdentity, skippedID string,
) (*models.Repository, error) {
	repositories, err := s.repoEntities.ListRepositories(ctx, identity.WorkspaceID)
	if err != nil {
		return nil, err
	}
	sort.Slice(repositories, func(i, j int) bool {
		left, right := repositories[i], repositories[j]
		if left.CreatedAt.Equal(right.CreatedAt) {
			return left.ID < right.ID
		}
		return left.CreatedAt.Before(right.CreatedAt)
	})
	for _, candidate := range repositories {
		if candidate.ID == skippedID || !matchesRemoteRepositoryIdentity(candidate, identity) || s.isKandevTaskWorktreeRepository(candidate) {
			continue
		}
		eligible, err := remoteRepositoryCandidateAvailable(ctx, candidate)
		if err != nil {
			return nil, err
		}
		if eligible {
			return candidate, nil
		}
	}
	return nil, nil
}

func matchesRemoteRepositoryIdentity(repo *models.Repository, identity models.ProviderRepositoryIdentity) bool {
	if repo.DeletedAt != nil || repo.WorkspaceID != identity.WorkspaceID || repo.Provider != identity.Provider {
		return false
	}
	if identity.Scope != "" {
		return identity.RepositoryID != "" && repo.ProviderScope == identity.Scope && repo.ProviderRepoID == identity.RepositoryID
	}
	return repo.ProviderHost == identity.Host && repo.ProviderOwner == identity.Owner && repo.ProviderName == identity.Name
}

func remoteRepositoryCandidateAvailable(ctx context.Context, repo *models.Repository) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if repo.SourceType != sourceTypeLocal {
		return true, nil
	}
	// Lstat distinguishes a deleted checkout from a dangling or replaced link.
	if _, err := os.Lstat(repo.LocalPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("%w: inspect local checkout: %w", ErrInvalidRepositorySettings, err)
	}
	canonical, err := canonicalRepositoryLocalPath(repo.LocalPath)
	if err != nil {
		return false, err
	}
	if !sameCanonicalPath(canonical, repo.LocalPath) {
		return false, fmt.Errorf("%w: local checkout no longer resolves to its registered path", ErrInvalidRepositorySettings)
	}
	return true, nil
}
