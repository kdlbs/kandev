package orgunit

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// WorkspaceCounter reports how many workspaces sit in a unit.
//
// Workspaces belong to the task repository, so this package asks rather than
// reads. Without it a unit could be deleted out from under the workspaces it
// holds, which would leave them unreachable with no way to say why.
type WorkspaceCounter interface {
	CountWorkspacesInUnit(ctx context.Context, unitID string) (int, error)
}

// Service owns the tree's invariants.
type Service struct {
	store      *Store
	workspaces WorkspaceCounter
	log        *logger.Logger
}

// NewService builds the service. The workspace counter is wired separately
// because the task repository is constructed after this package.
func NewService(store *Store, log *logger.Logger) *Service {
	return &Service{store: store, log: log}
}

// SetWorkspaceCounter wires the workspace-occupancy seam. Until it is set, a
// delete is refused rather than allowed unchecked: an unwired dependency must
// not read as "no workspaces here".
func (s *Service) SetWorkspaceCounter(c WorkspaceCounter) { s.workspaces = c }

// Store exposes the store for read paths that do not need the invariants.
func (s *Service) Store() *Store { return s.store }

// EnsureRoot returns the organization's root unit, creating it when absent.
// It is idempotent so that boot, migration, and organization creation can all
// call it without coordinating.
func (s *Service) EnsureRoot(ctx context.Context, orgID, orgName string) (*Unit, error) {
	if existing, err := s.store.Root(ctx, orgID); err == nil {
		return existing, nil
	} else if err != ErrUnitNotFound {
		return nil, err
	}
	name := strings.TrimSpace(orgName)
	if name == "" {
		name = "Organization"
	}
	unit, err := s.store.Insert(ctx, &Unit{OrgID: orgID, Kind: KindRoot, Name: name})
	if err != nil {
		// Lost a race with a concurrent caller. The unique index kept the
		// database right; re-reading is what keeps the method idempotent, as
		// its name promises.
		if existing, readErr := s.store.Root(ctx, orgID); readErr == nil {
			return existing, nil
		}
		return nil, err
	}
	return unit, nil
}

// EnsurePersonal returns a user's personal unit, creating it when absent.
//
// It hangs off the root rather than standing outside the tree, so that one
// walk answers reach for every workspace. Its emptiness of members, not its
// position, is what keeps it private.
func (s *Service) EnsurePersonal(ctx context.Context, orgID, userID, displayName string) (*Unit, error) {
	if existing, err := s.store.Personal(ctx, userID); err == nil {
		return existing, nil
	} else if err != ErrUnitNotFound {
		return nil, err
	}
	root, err := s.EnsureRoot(ctx, orgID, "")
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = "Personal"
	}
	unit, err := s.store.Insert(ctx, &Unit{
		OrgID:       orgID,
		ParentID:    root.ID,
		Kind:        KindPersonal,
		OwnerUserID: userID,
		Name:        name,
	})
	if err != nil {
		// Same race as the root: two first-time requests for one account can
		// both pass the lookup above. Returning the row the winner created is
		// the idempotent answer, and matters more here because the caller's
		// fallback for an error used to be the organization root.
		if existing, readErr := s.store.Personal(ctx, userID); readErr == nil {
			return existing, nil
		}
		return nil, err
	}
	return unit, nil
}

// Create adds a standard unit under a parent.
func (s *Service) Create(ctx context.Context, parentID, name string) (*Unit, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ErrNameRequired
	}
	if parentID == "" {
		return nil, ErrParentRequired
	}
	parent, err := s.store.Get(ctx, parentID)
	if err != nil {
		return nil, err
	}
	unit, err := s.store.Insert(ctx, &Unit{
		OrgID:    parent.OrgID,
		ParentID: parent.ID,
		Kind:     KindStandard,
		Name:     strings.TrimSpace(name),
	})
	if err != nil {
		return nil, err
	}
	s.logInfo("unit created", unit)
	return unit, nil
}

// Rename changes a unit's display name, including a protected one: naming is
// not structural.
func (s *Service) Rename(ctx context.Context, id, name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrNameRequired
	}
	return s.store.Rename(ctx, id, strings.TrimSpace(name))
}

// Move reparents a unit.
func (s *Service) Move(ctx context.Context, id, newParentID string) error {
	unit, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if unit.IsProtected() {
		return ErrProtectedUnit
	}
	parent, err := s.store.Get(ctx, newParentID)
	if err != nil {
		return err
	}
	if parent.OrgID != unit.OrgID {
		return ErrCrossOrgParent
	}
	// A unit cannot be moved beneath itself. The destination's path carries
	// its whole ancestry, so containment is a prefix test rather than a walk.
	if strings.HasPrefix(parent.Path, unit.Path) {
		return ErrCycle
	}
	if err := s.store.Reparent(ctx, unit, parent); err != nil {
		return err
	}
	s.logInfo("unit moved", unit)
	return nil
}

// Delete removes an empty unit.
func (s *Service) Delete(ctx context.Context, id string) error {
	unit, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if unit.IsProtected() {
		return ErrProtectedUnit
	}
	children, err := s.store.ChildCount(ctx, id)
	if err != nil {
		return err
	}
	if children > 0 {
		return ErrNotEmpty
	}
	if s.workspaces == nil {
		return ErrNotEmpty
	}
	count, err := s.workspaces.CountWorkspacesInUnit(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrNotEmpty
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	s.logInfo("unit deleted", unit)
	return nil
}

// SetMember adds or re-roles a member.
func (s *Service) SetMember(ctx context.Context, unitID, userID, role, addedBy string) error {
	unit, err := s.store.Get(ctx, unitID)
	if err != nil {
		return err
	}
	// A personal unit is private because nobody else can be in it. Admitting a
	// member would make "private" a property some units have and others do
	// not, which is the flag this model removed.
	if unit.Kind == KindPersonal {
		return ErrPersonalNoMember
	}
	return s.store.SetMember(ctx, &Member{
		UnitID: unitID, UserID: userID, Role: role, AddedBy: addedBy,
	})
}

// RemoveMember drops a membership.
func (s *Service) RemoveMember(ctx context.Context, unitID, userID string) error {
	return s.store.RemoveMember(ctx, unitID, userID)
}

func (s *Service) logInfo(msg string, unit *Unit) {
	if s.log == nil {
		return
	}
	s.log.Info(msg,
		zap.String("unit_id", unit.ID),
		zap.String("org_id", unit.OrgID),
		zap.String("kind", string(unit.Kind)))
}

// PersonalUnitID returns a user's personal unit, creating it on demand. It
// satisfies the task service's placement seam.
func (s *Service) PersonalUnitID(ctx context.Context, orgID, userID, displayName string) (string, error) {
	unit, err := s.EnsurePersonal(ctx, orgID, userID, displayName)
	if err != nil {
		return "", err
	}
	return unit.ID, nil
}

// RootUnitID returns an organization's root unit, creating it on demand.
func (s *Service) RootUnitID(ctx context.Context, orgID string) (string, error) {
	unit, err := s.EnsureRoot(ctx, orgID, "")
	if err != nil {
		return "", err
	}
	return unit.ID, nil
}

// InheritedRole returns the highest role a user holds on a unit or any of its
// ancestors. It satisfies the task service's reach seam.
func (s *Service) InheritedRole(ctx context.Context, userID, unitID string) (string, error) {
	unit, err := s.store.Get(ctx, unitID)
	if err != nil {
		if err == ErrUnitNotFound {
			// A workspace pointing at a unit that is gone reaches nobody,
			// which is the safe reading: it is not an error the caller can act
			// on, and treating it as one would deny every request instead.
			return "", nil
		}
		return "", err
	}
	roles, err := s.store.AncestorRoles(ctx, userID, unit.Path)
	if err != nil {
		return "", err
	}
	return strongest(roles), nil
}

// InheritedRolesByUnit resolves inherited roles for many units at once, in two
// queries rather than one per unit.
func (s *Service) InheritedRolesByUnit(ctx context.Context, userID string, unitIDs []string) (map[string]string, error) {
	if userID == "" || len(unitIDs) == 0 {
		return map[string]string{}, nil
	}
	held, err := s.store.UserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(held) == 0 {
		return map[string]string{}, nil
	}
	paths, err := s.store.PathsByID(ctx, unitIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(paths))
	for unitID, path := range paths {
		var roles []string
		for _, ancestor := range AncestorIDs(path) {
			if role, ok := held[ancestor]; ok {
				roles = append(roles, role)
			}
		}
		if role := strongest(roles); role != "" {
			out[unitID] = role
		}
	}
	return out, nil
}

// strongest picks the highest role, which is the whole combining rule: nothing
// a person holds anywhere in the ancestry can take away what another grant
// gives them.
func strongest(roles []string) string {
	rank := map[string]int{"viewer": 1, "collaborator": 2, "owner": 3}
	best, bestRank := "", 0
	for _, role := range roles {
		if r := rank[role]; r > bestRank {
			best, bestRank = role, r
		}
	}
	return best
}

// UnitOrgID reports which organization a unit belongs to.
func (s *Service) UnitOrgID(ctx context.Context, unitID string) (string, error) {
	unit, err := s.store.Get(ctx, unitID)
	if err != nil {
		return "", err
	}
	return unit.OrgID, nil
}

// UnitReaders returns everyone who reaches a workspace placed in this unit,
// which is every member of the unit and of all its ancestors.
func (s *Service) UnitReaders(ctx context.Context, unitID string) ([]string, error) {
	unit, err := s.store.Get(ctx, unitID)
	if err != nil {
		if err == ErrUnitNotFound {
			return nil, nil
		}
		return nil, err
	}
	return s.store.AncestorMemberIDs(ctx, unit.Path)
}

// DeleteOrgUnits removes an organization's whole tree.
func (s *Service) DeleteOrgUnits(ctx context.Context, orgID string) error {
	return s.store.DeleteByOrg(ctx, orgID)
}
