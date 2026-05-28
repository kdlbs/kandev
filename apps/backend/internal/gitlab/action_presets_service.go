package gitlab

import (
	"context"
	"fmt"
)

// GetActionPresetsOrDefault returns the workspace's stored presets, falling
// back to the built-in defaults when none are stored.
func (s *Service) GetActionPresetsOrDefault(ctx context.Context, workspaceID string) (*ActionPresets, error) {
	store := s.requireStore()
	if store == nil {
		return defaultPresets(workspaceID), nil
	}
	presets, err := store.GetActionPresets(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get action presets: %w", err)
	}
	if len(presets.MR) == 0 {
		presets.MR = DefaultMRActionPresets()
	}
	if len(presets.Issue) == 0 {
		presets.Issue = DefaultIssueActionPresets()
	}
	return presets, nil
}

// UpdateActionPresets persists a partial update to a workspace's presets.
// Nil fields are left unchanged. The full updated row is returned.
func (s *Service) UpdateActionPresets(ctx context.Context, req *UpdateActionPresetsRequest) (*ActionPresets, error) {
	if req == nil || req.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id required")
	}
	store := s.requireStore()
	if store == nil {
		return nil, fmt.Errorf("gitlab store not configured")
	}
	current, err := s.GetActionPresetsOrDefault(ctx, req.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if req.MR != nil {
		current.MR = *req.MR
	}
	if req.Issue != nil {
		current.Issue = *req.Issue
	}
	if err := store.UpsertActionPresets(ctx, current); err != nil {
		return nil, fmt.Errorf("upsert action presets: %w", err)
	}
	return current, nil
}

// ResetActionPresets removes a workspace's stored presets, falling back to defaults.
func (s *Service) ResetActionPresets(ctx context.Context, workspaceID string) (*ActionPresets, error) {
	store := s.requireStore()
	if store == nil {
		return defaultPresets(workspaceID), nil
	}
	if err := store.DeleteActionPresets(ctx, workspaceID); err != nil {
		return nil, fmt.Errorf("reset action presets: %w", err)
	}
	return defaultPresets(workspaceID), nil
}

func defaultPresets(workspaceID string) *ActionPresets {
	return &ActionPresets{
		WorkspaceID: workspaceID,
		MR:          DefaultMRActionPresets(),
		Issue:       DefaultIssueActionPresets(),
	}
}
