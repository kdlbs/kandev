package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrSameWorkspace is returned when a copy targets the same workspace it reads
// from.
var ErrSameWorkspace = errors.New("github: source and target workspaces are the same")

// CopyWorkspaceSettingsToWorkspace copies the per-workspace GitHub operational
// settings (repo scope + saved/default query presets) and the workspace's
// quick-action presets from sourceWorkspaceID to targetWorkspaceID. GitHub
// authentication is install-wide, so there are no credentials to copy — only the
// workspace-scoped settings. Watchers are intentionally out of scope.
func (s *Service) CopyWorkspaceSettingsToWorkspace(ctx context.Context, sourceWorkspaceID, targetWorkspaceID string) (*WorkspaceSettings, error) {
	sourceWorkspaceID = strings.TrimSpace(sourceWorkspaceID)
	targetWorkspaceID = strings.TrimSpace(targetWorkspaceID)
	if sourceWorkspaceID == "" || targetWorkspaceID == "" {
		return nil, fmt.Errorf("%w: source and target workspace ids are required", ErrWorkspaceSettingsValidation)
	}
	if sourceWorkspaceID == targetWorkspaceID {
		return nil, ErrSameWorkspace
	}
	if s.store == nil {
		return nil, fmt.Errorf("github store not configured")
	}
	// GetWorkspaceSettings returns defaults (never nil) for a missing row.
	source, err := s.store.GetWorkspaceSettings(ctx, sourceWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("read source github settings: %w", err)
	}
	target := &WorkspaceSettings{
		WorkspaceID:         targetWorkspaceID,
		RepoScopeMode:       source.RepoScopeMode,
		RepoScopeOrgs:       append([]string(nil), source.RepoScopeOrgs...),
		RepoScopeRepos:      append([]RepoFilter(nil), source.RepoScopeRepos...),
		SavedPresets:        cloneRawMessage(source.SavedPresets),
		DefaultQueryPresets: cloneRawMessage(source.DefaultQueryPresets),
	}
	if err := s.store.UpsertWorkspaceSettings(ctx, target); err != nil {
		return nil, fmt.Errorf("write target github settings: %w", err)
	}
	if err := s.copyActionPresets(ctx, sourceWorkspaceID, targetWorkspaceID); err != nil {
		return nil, err
	}
	return s.store.GetWorkspaceSettings(ctx, targetWorkspaceID)
}

// copyActionPresets duplicates the source workspace's stored quick-action
// presets onto the target. When the source has no stored row it relies on
// built-in defaults, so the target's row is cleared as well — otherwise a copy
// would leave the target on its own customised presets while reporting success,
// diverging from the source. Both then fall back to the same defaults on read.
func (s *Service) copyActionPresets(ctx context.Context, sourceWorkspaceID, targetWorkspaceID string) error {
	stored, err := s.store.GetActionPresets(ctx, sourceWorkspaceID)
	if err != nil {
		return fmt.Errorf("read source github action presets: %w", err)
	}
	if stored == nil {
		if err := s.store.DeleteActionPresets(ctx, targetWorkspaceID); err != nil {
			return fmt.Errorf("clear target github action presets: %w", err)
		}
		return nil
	}
	target := &ActionPresets{
		WorkspaceID: targetWorkspaceID,
		PR:          append([]ActionPreset(nil), stored.PR...),
		Issue:       append([]ActionPreset(nil), stored.Issue...),
	}
	if err := s.store.UpsertActionPresets(ctx, target); err != nil {
		return fmt.Errorf("write target github action presets: %w", err)
	}
	return nil
}
