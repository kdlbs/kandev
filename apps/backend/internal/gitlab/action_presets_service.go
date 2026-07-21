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
// Nil fields are left unchanged. Untouched kinds are NOT filled with current
// defaults before persistence — that would freeze stale defaults into the
// workspace row, masking future default changes. The reader
// (GetActionPresetsOrDefault) substitutes defaults on read instead.
func (s *Service) UpdateActionPresets(ctx context.Context, req *UpdateActionPresetsRequest) (*ActionPresets, error) {
	if req == nil || req.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id required")
	}
	store := s.requireStore()
	if store == nil {
		return nil, fmt.Errorf("gitlab store not configured")
	}
	current, err := store.GetActionPresets(ctx, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("get action presets: %w", err)
	}
	if current == nil {
		current = &ActionPresets{WorkspaceID: req.WorkspaceID}
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
	// Return the rendered view (defaults substituted) so the caller sees the
	// same shape the read endpoint produces.
	return s.GetActionPresetsOrDefault(ctx, req.WorkspaceID)
}

// ResetActionPresets removes a workspace's stored presets, falling back to defaults.
func (s *Service) ResetActionPresets(ctx context.Context, workspaceID string) (*ActionPresets, error) {
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id required")
	}
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
