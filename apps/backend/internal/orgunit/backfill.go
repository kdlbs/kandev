package orgunit

import (
	"context"

	"go.uber.org/zap"
)

// UserRef is the little a backfill needs to know about an account.
type UserRef struct {
	ID          string
	OrgID       string
	DisplayName string
}

// WorkspaceRef is a workspace awaiting placement.
type WorkspaceRef struct {
	ID      string
	OrgID   string
	OwnerID string
}

// AccountLister supplies the accounts that need a personal unit.
type AccountLister interface {
	ListUnitUsers(ctx context.Context) ([]UserRef, error)
}

// WorkspacePlacer supplies and records workspace placement.
type WorkspacePlacer interface {
	UnplacedWorkspaces(ctx context.Context) ([]WorkspaceRef, error)
	PlaceWorkspace(ctx context.Context, workspaceID, unitID string) error
}

// BackfillResult reports what the one-shot placement did.
type BackfillResult struct {
	Roots     int
	Personals int
	Placed    int
}

// Backfill gives every organization a root unit, every user a personal unit,
// and every unplaced workspace a home.
//
// The placement rule is the one that keeps an upgrade from widening access: a
// workspace with an owner lands in that owner's personal unit, where only they
// reach it, and never under the root, where everyone would. A workspace with no
// owner predates authentication and is reachable by everyone already, so the
// root is where it belongs and nothing changes for it.
//
// It is idempotent: units are ensured rather than created, and only workspaces
// with no unit are considered.
func (s *Service) Backfill(ctx context.Context, accounts AccountLister, placer WorkspacePlacer) (BackfillResult, error) {
	var result BackfillResult

	users, err := accounts.ListUnitUsers(ctx)
	if err != nil {
		return result, err
	}
	workspaces, err := placer.UnplacedWorkspaces(ctx)
	if err != nil {
		return result, err
	}

	roots, err := s.ensureRoots(ctx, orgIDsIn(users, workspaces))
	if err != nil {
		return result, err
	}
	result.Roots = len(roots)

	personals, err := s.ensurePersonals(ctx, users)
	if err != nil {
		return result, err
	}
	result.Personals = len(personals)

	placed, err := s.placeAll(ctx, placer, workspaces, roots, personals)
	if err != nil {
		return result, err
	}
	result.Placed = placed

	if s.log != nil && (result.Placed > 0 || result.Personals > 0) {
		s.log.Info("organization unit backfill applied",
			zap.Int("roots", result.Roots),
			zap.Int("personal_units", result.Personals),
			zap.Int("workspaces_placed", result.Placed))
	}
	return result, nil
}

// orgIDsIn collects every organization the backfill has to cover. An instance
// with organizations switched off has one implicit organization whose id is
// empty, and it needs a root exactly like a named one: the tree is the reach
// model whether or not tenancy is on.
func orgIDsIn(users []UserRef, workspaces []WorkspaceRef) []string {
	seen := map[string]struct{}{}
	for _, u := range users {
		seen[u.OrgID] = struct{}{}
	}
	for _, w := range workspaces {
		seen[w.OrgID] = struct{}{}
	}
	if len(seen) == 0 {
		seen[""] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

func (s *Service) ensureRoots(ctx context.Context, orgIDs []string) (map[string]string, error) {
	roots := make(map[string]string, len(orgIDs))
	for _, orgID := range orgIDs {
		root, err := s.EnsureRoot(ctx, orgID, "")
		if err != nil {
			return nil, err
		}
		roots[orgID] = root.ID
	}
	return roots, nil
}

func (s *Service) ensurePersonals(ctx context.Context, users []UserRef) (map[string]string, error) {
	personals := make(map[string]string, len(users))
	for _, u := range users {
		unit, err := s.EnsurePersonal(ctx, u.OrgID, u.ID, u.DisplayName)
		if err != nil {
			return nil, err
		}
		personals[u.ID] = unit.ID
	}
	return personals, nil
}

// placeAll gives every unplaced workspace a home.
//
// The rule is the one that keeps an upgrade from widening access: a workspace
// with an owner lands in that owner's personal unit, where only they reach it,
// and never under the root, where everyone would. A workspace with no owner
// predates authentication and is reachable by everyone already, so the root is
// where it belongs and nothing changes for it.
func (s *Service) placeAll(
	ctx context.Context,
	placer WorkspacePlacer,
	workspaces []WorkspaceRef,
	roots, personals map[string]string,
) (int, error) {
	placed := 0
	for _, w := range workspaces {
		target, err := s.targetUnit(ctx, w, roots, personals)
		if err != nil {
			return placed, err
		}
		if target == "" {
			continue
		}
		if err := placer.PlaceWorkspace(ctx, w.ID, target); err != nil {
			return placed, err
		}
		placed++
	}
	return placed, nil
}

func (s *Service) targetUnit(
	ctx context.Context,
	w WorkspaceRef,
	roots, personals map[string]string,
) (string, error) {
	if w.OwnerID == "" {
		return roots[w.OrgID], nil
	}
	if unit, ok := personals[w.OwnerID]; ok {
		return unit, nil
	}
	// An owner with no account left. Their workspace must not fall back to the
	// root, where the whole organization would reach it, so it gets a personal
	// unit keyed to the dangling id.
	unit, err := s.EnsurePersonal(ctx, w.OrgID, w.OwnerID, "")
	if err != nil {
		return "", err
	}
	personals[w.OwnerID] = unit.ID
	return unit.ID, nil
}
