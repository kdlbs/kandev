package service

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
)

// UnitPlacer answers where a new workspace belongs.
//
// Placement is resolved lazily rather than through a user-creation hook: the
// personal unit is ensured on demand, so an account created by any path, at any
// time, gets one the first time it needs one. There is no ordering to get wrong
// and no account that can slip past a hook that was not wired.
type UnitPlacer interface {
	// PersonalUnitID returns the caller's personal unit, creating it if needed.
	PersonalUnitID(ctx context.Context, orgID, userID, displayName string) (string, error)
	// RootUnitID returns the organization's root unit, creating it if needed.
	RootUnitID(ctx context.Context, orgID string) (string, error)
	// UnitOrgID reports which organization a unit belongs to, so a move can
	// be refused before it crosses a tenant boundary.
	UnitOrgID(ctx context.Context, unitID string) (string, error)
}

// SetUnitPlacer wires the placement seam.
func (s *Service) SetUnitPlacer(p UnitPlacer) { s.unitPlacer = p }

// placementFor decides the unit a new workspace goes in.
//
// A workspace created by a signed-in caller lands in their personal unit,
// which is where "only I can see this" lives now that there is no private
// flag. One created by an internal or pre-authentication caller has no person
// to belong to and lands at the root, which reproduces the
// everyone-reaches-it behaviour those workspaces already had.
//
// A failure to resolve an owner's personal unit is an error rather than a
// fallback. Falling back to the root would take a workspace that was meant to
// be private and hand it to everyone in the organization, which is the one
// outcome a placement bug must never produce quietly.
func (s *Service) placementFor(ctx context.Context, ownerID, orgID string) (string, error) {
	if s.unitPlacer == nil {
		return "", nil
	}
	if ownerID != "" {
		unitID, err := s.unitPlacer.PersonalUnitID(ctx, orgID, ownerID, "")
		if err != nil {
			s.logger.Error("personal unit lookup failed; refusing to create the workspace",
				zap.String("user_id", ownerID), zap.Error(err))
			return "", err
		}
		return unitID, nil
	}
	unitID, err := s.unitPlacer.RootUnitID(ctx, orgID)
	if err != nil {
		s.logger.Warn("root unit lookup failed; workspace is left unplaced", zap.Error(err))
		return "", nil
	}
	return unitID, nil
}

// ErrUnitNotInWorkspaceOrg reports a destination in another tenant.
var ErrUnitNotInWorkspaceOrg = errors.New("that unit belongs to another organization")

// moveWorkspaceToUnit changes a workspace's placement, which is the only way to
// change who reaches it.
//
// The caller already holds workspace.manage. They must also hold unit.manage,
// because moving a board into a unit hands it to everyone in that unit, and
// deciding who is in a unit is exactly what that scope governs.
func (s *Service) moveWorkspaceToUnit(ctx context.Context, workspace *models.Workspace, unitID string) error {
	if unitID == "" || unitID == workspace.UnitID {
		return nil
	}
	subject := callerSubject(ctx)
	if !subject.Unscoped && !authz.SubjectOrgScopes(subject).Has(authz.ScopeUnitManage) {
		return ErrForbidden
	}
	if s.unitPlacer == nil {
		return ErrUnitNotInWorkspaceOrg
	}
	orgID, err := s.unitPlacer.UnitOrgID(ctx, unitID)
	if err != nil {
		return err
	}
	if orgID != workspace.OrgID {
		return ErrUnitNotInWorkspaceOrg
	}
	workspace.UnitID = unitID
	return nil
}
