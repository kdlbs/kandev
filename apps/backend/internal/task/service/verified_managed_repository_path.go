package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// UpdateVerifiedManagedRepositoryPath is an internal attachment-only write.
// Only repoLocalPathUpdater's verified attachment lane may call it, after
// repoclone/executor admission has established managed path and Git ownership.
// Ordinary repository editing must use UpdateRepository and its canonicalization.
func (s *Service) UpdateVerifiedManagedRepositoryPath(ctx context.Context, admitted *models.Repository, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateVerifiedManagedRepositorySnapshot(admitted); err != nil {
		return err
	}
	if err := validateVerifiedManagedDirectory(path); err != nil {
		return err
	}
	current, err := s.repoEntities.GetRepository(ctx, admitted.ID)
	if err != nil {
		return err
	}
	if current == nil || current.DeletedAt != nil {
		return repoerrors.ErrRepositoryNotFound
	}
	if err := s.AuthorizeWorkspaceScope(ctx, current.WorkspaceID, authz.ScopeRepositoryManage); err != nil {
		return repoerrors.ErrRepositoryNotFound
	}
	if !current.UpdatedAt.Equal(admitted.UpdatedAt) || !sameVerifiedManagedRepositoryIdentity(current, admitted) {
		return repoerrors.ErrTaskVersionConflict
	}
	updater, ok := s.repoEntities.(exactRepositoryUpdater)
	if !ok {
		return fmt.Errorf("verified managed repository version fencing is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	current.LocalPath = path
	current.UpdatedAt = time.Now().UTC()
	if err := updater.UpdateRepositoryIfUnchanged(ctx, current, admitted.UpdatedAt); err != nil {
		return fmt.Errorf("persist verified managed repository path: %w", err)
	}
	s.publishRepositoryEvent(ctx, events.RepositoryUpdated, current)
	return nil
}

func validateVerifiedManagedRepositorySnapshot(admitted *models.Repository) error {
	if admitted == nil || strings.TrimSpace(admitted.ID) == "" || strings.TrimSpace(admitted.WorkspaceID) == "" || admitted.UpdatedAt.IsZero() || admitted.DeletedAt != nil {
		return ErrInvalidRepositorySettings
	}
	if admitted.SourceType == sourceTypeLocal || (admitted.SourceType != sourceTypeProvider && admitted.LocalPath != "") {
		return ErrInvalidRepositorySettings
	}
	for _, value := range []string{admitted.ID, admitted.WorkspaceID, admitted.SourceType, admitted.Provider, admitted.ProviderHost, admitted.ProviderScope, admitted.ProviderRepoID, admitted.ProviderOwner, admitted.ProviderName, admitted.RemoteURL} {
		if strings.ContainsRune(value, '\x00') {
			return ErrInvalidRepositorySettings
		}
	}
	return validateVerifiedManagedProviderIdentity(admitted)
}

func validateVerifiedManagedProviderIdentity(admitted *models.Repository) error {
	if strings.TrimSpace(admitted.Provider) == "" || strings.TrimSpace(admitted.ProviderHost) == "" || normalizeProviderHost(admitted.Provider, admitted.ProviderHost) == "" {
		return ErrInvalidRepositorySettings
	}
	scope, err := validateProviderScope(admitted.ProviderScope)
	if err != nil {
		return err
	}
	if scope != admitted.ProviderScope {
		return ErrInvalidRepositorySettings
	}
	if scope != "" {
		return validateProviderScopeAndRepoIDPair(scope, admitted.ProviderRepoID)
	}
	if admitted.ProviderOwner == "" || admitted.ProviderName == "" {
		return ErrInvalidRepositorySettings
	}
	return nil
}

func validateVerifiedManagedDirectory(path string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsRune(path, '\x00') {
		return ErrInvalidRepositorySettings
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect verified managed directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrInvalidRepositorySettings
	}
	return nil
}

func sameVerifiedManagedRepositoryIdentity(current, admitted *models.Repository) bool {
	return current.ID == admitted.ID && current.WorkspaceID == admitted.WorkspaceID && current.SourceType == admitted.SourceType && current.LocalPath == admitted.LocalPath && current.Provider == admitted.Provider && current.ProviderHost == admitted.ProviderHost && current.ProviderScope == admitted.ProviderScope && current.ProviderRepoID == admitted.ProviderRepoID && current.ProviderOwner == admitted.ProviderOwner && current.ProviderName == admitted.ProviderName && current.RemoteURL == admitted.RemoteURL
}
