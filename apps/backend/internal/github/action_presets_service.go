package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// GetActionPresets returns the stored preset lists for a workspace, falling
// back to the built-in defaults for any list that hasn't been customised yet.
func (s *Service) GetActionPresets(ctx context.Context, workspaceID string) (*ActionPresets, error) {
	stored, err := s.store.GetActionPresets(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	result := &ActionPresets{
		WorkspaceID: workspaceID,
		PR:          DefaultPRActionPresets(),
		Issue:       DefaultIssueActionPresets(),
	}
	if stored == nil {
		return result, nil
	}
	if len(stored.PR) > 0 {
		result.PR = stored.PR
	}
	if len(stored.Issue) > 0 {
		result.Issue = stored.Issue
	}
	return result, nil
}

// UpdateActionPresets replaces the PR or Issue preset lists for a workspace.
// Nil request fields leave that list untouched. For any kind not in the
// request, we persist whatever the store currently has (possibly empty)
// rather than the resolved defaults — otherwise future default improvements
// would never reach workspaces that have only customised one side.
func (s *Service) UpdateActionPresets(ctx context.Context, req *UpdateActionPresetsRequest) (*ActionPresets, error) {
	if req == nil || req.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	stored, err := s.store.GetActionPresets(ctx, req.WorkspaceID)
	if err != nil {
		return nil, err
	}
	next := &ActionPresets{WorkspaceID: req.WorkspaceID}
	if stored != nil {
		next.PR = stored.PR
		next.Issue = stored.Issue
	}
	if req.PR != nil {
		next.PR = normalisePresets(*req.PR)
	}
	if req.Issue != nil {
		next.Issue = normalisePresets(*req.Issue)
	}
	if err := s.store.UpsertActionPresets(ctx, next); err != nil {
		return nil, err
	}
	return s.GetActionPresets(ctx, req.WorkspaceID)
}

// ResetActionPresets drops any stored overrides so the defaults apply again.
func (s *Service) ResetActionPresets(ctx context.Context, workspaceID string) (*ActionPresets, error) {
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if err := s.store.DeleteActionPresets(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.GetActionPresets(ctx, workspaceID)
}

func normalisePresets(presets []ActionPreset) []ActionPreset {
	out := make([]ActionPreset, 0, len(presets))
	for _, p := range presets {
		id := strings.TrimSpace(p.ID)
		label := strings.TrimSpace(p.Label)
		if label == "" {
			continue
		}
		if id == "" {
			id = uuid.New().String()
		}
		out = append(out, ActionPreset{
			ID:             id,
			Label:          label,
			Hint:           strings.TrimSpace(p.Hint),
			Icon:           strings.TrimSpace(p.Icon),
			PromptTemplate: p.PromptTemplate,
		})
	}
	return out
}
