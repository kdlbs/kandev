package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
)

// UnitReachResolver answers what a user inherits from a workspace's ancestry.
type UnitReachResolver interface {
	// InheritedRole returns the highest role the user holds on any unit that
	// is an ancestor of the given unit, including that unit itself.
	InheritedRole(ctx context.Context, userID, unitID string) (string, error)
	// InheritedRolesByUnit answers the same question for many units at once,
	// so filtering a workspace list stays two queries rather than one per row.
	InheritedRolesByUnit(ctx context.Context, userID string, unitIDs []string) (map[string]string, error)
	// UnitReaders returns everyone who reaches a workspace placed in a unit,
	// which is who a workspace-scoped event fans out to.
	UnitReaders(ctx context.Context, unitID string) ([]string, error)
}

// SetUnitReach wires the inherited-role seam.
func (s *Service) SetUnitReach(r UnitReachResolver) { s.unitReach = r }

// inheritedRole resolves what a workspace's placement gives the caller.
//
// It fails closed. A lookup error returning "no role" rather than an error
// would be indistinguishable from a genuine absence of membership, which is
// the shape of failure that quietly hands out access, so the caller treats a
// failure as denial instead.
func (s *Service) inheritedRole(ctx context.Context, userID string, workspace *models.Workspace) (authz.WorkspaceRole, bool) {
	if s.unitReach == nil || workspace == nil || workspace.UnitID == "" || userID == "" {
		return authz.WorkspaceRoleNone, true
	}
	role, err := s.unitReach.InheritedRole(ctx, userID, workspace.UnitID)
	if err != nil {
		s.logger.Warn("unit reach lookup failed; denying access",
			zap.String("workspace_id", workspace.ID), zap.Error(err))
		return authz.WorkspaceRoleNone, false
	}
	return authz.NormalizeWorkspaceRole(role), true
}

// inheritedRolesFor resolves the inherited role for a list of workspaces.
//
// It fails closed for the whole list: a partial answer would silently hide
// workspaces the caller can actually reach, which reads as data loss rather
// than as a permission error.
func (s *Service) inheritedRolesFor(ctx context.Context, userID string, workspaces []*models.Workspace) (map[string]authz.WorkspaceRole, bool) {
	out := map[string]authz.WorkspaceRole{}
	if s.unitReach == nil || userID == "" {
		return out, true
	}
	unitIDs := make([]string, 0, len(workspaces))
	seen := map[string]struct{}{}
	for _, w := range workspaces {
		if w == nil || w.UnitID == "" {
			continue
		}
		if _, ok := seen[w.UnitID]; ok {
			continue
		}
		seen[w.UnitID] = struct{}{}
		unitIDs = append(unitIDs, w.UnitID)
	}
	byUnit, err := s.unitReach.InheritedRolesByUnit(ctx, userID, unitIDs)
	if err != nil {
		s.logger.Warn("unit reach list failed; returning no workspaces", zap.Error(err))
		return nil, false
	}
	for _, w := range workspaces {
		if w == nil {
			continue
		}
		if role, ok := byUnit[w.UnitID]; ok {
			out[w.ID] = authz.NormalizeWorkspaceRole(role)
		}
	}
	return out, true
}
