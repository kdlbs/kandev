package sandbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"

	"github.com/kandev/kandev/internal/auth/authn"
)

func (s *Service) GetBinding(ctx context.Context, scope, targetID string) (*Binding, error) {
	if !s.enabled {
		return nil, ErrDisabled
	}
	id, ok := authn.IdentityFromContext(ctx)
	if !ok || id.Synthetic || id.Instance || id.UserID == "" || s.resolveTarget == nil {
		return nil, ErrNotFound
	}
	if !slices.Contains([]string{ScopeExecutor, ScopeAgent, ScopeWorkflowStep, ScopeTask}, scope) {
		return nil, ErrNotFound
	}
	target, err := s.resolveTarget(ctx, scope, targetID)
	if err != nil || target.OrgID != id.OrgID {
		return nil, ErrNotFound
	}
	if err = s.workspaceAccess(ctx, target.WorkspaceID); err != nil {
		return nil, ErrNotFound
	}
	var raw string
	err = s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT content_json FROM sandbox_policy_bindings WHERE org_id=? AND scope=? AND target_id=?`), id.OrgID, scope, targetID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return &Binding{Scope: scope, TargetID: targetID, WorkspaceID: target.WorkspaceID}, nil
	}
	if err != nil {
		return nil, err
	}
	var binding Binding
	if err = json.Unmarshal([]byte(raw), &binding); err != nil {
		return nil, err
	}
	if binding.WorkspaceID != target.WorkspaceID {
		return nil, ErrNotFound
	}
	return &binding, nil
}

// HasBindings is an admission check for operations that lack executor context.
// It stays available when the feature is disabled so saved requirements cannot disappear.
func (s *Service) HasBindings(ctx context.Context, orgID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT EXISTS (SELECT 1 FROM sandbox_policy_bindings WHERE org_id=?)`), orgID).Scan(&exists)
	return exists, err
}

// HasAnyBindings prevents operations without a tenant from bypassing saved requirements.
func (s *Service) HasAnyBindings(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM sandbox_policy_bindings)`).Scan(&exists)
	return exists, err
}
