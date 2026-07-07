package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrSameWorkspace is returned when a copy targets the same workspace it reads
// from.
var ErrSameWorkspace = errors.New("github: source and target workspaces are the same")

// CopyWorkspaceSettingsToWorkspace copies the per-workspace GitHub operational
// settings (repo scope + saved/default query presets) from sourceWorkspaceID to
// targetWorkspaceID. GitHub authentication is install-wide, so there are no
// credentials to copy — only the workspace-scoped settings. Watchers are
// intentionally out of scope.
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
	source, err := s.store.GetWorkspaceSettings(ctx, sourceWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("read source github settings: %w", err)
	}
	if source == nil {
		source = defaultWorkspaceSettings(sourceWorkspaceID)
	}
	target := &WorkspaceSettings{
		WorkspaceID:         targetWorkspaceID,
		RepoScopeMode:       source.RepoScopeMode,
		RepoScopeOrgs:       append([]string(nil), source.RepoScopeOrgs...),
		RepoScopeRepos:      append([]RepoFilter(nil), source.RepoScopeRepos...),
		SavedPresets:        append(json.RawMessage(nil), source.SavedPresets...),
		DefaultQueryPresets: append(json.RawMessage(nil), source.DefaultQueryPresets...),
	}
	if err := s.store.UpsertWorkspaceSettings(ctx, target); err != nil {
		return nil, fmt.Errorf("write target github settings: %w", err)
	}
	return s.store.GetWorkspaceSettings(ctx, targetWorkspaceID)
}
