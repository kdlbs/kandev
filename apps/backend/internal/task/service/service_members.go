package service

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"go.uber.org/zap"
)

// Workspace membership errors. Each failure mode gets its own sentinel so the
// UI can say what actually went wrong instead of "bad request".
var (
	ErrMemberUserNotFound      = errors.New("user not found")
	ErrMemberUserDisabled      = errors.New("user account is disabled")
	ErrMemberIsOwner           = errors.New("the workspace owner cannot be removed; transfer ownership first")
	ErrMemberRoleInvalid       = errors.New("role must be collaborator or viewer")
	ErrMemberSelf              = errors.New("you already own this workspace")
	ErrTransferTargetNotMember = errors.New("add the user as a member before transferring ownership")
	ErrVisibilityOwnerIsGuest  = errors.New("a guest-owned workspace cannot be shared with the organization")
)

// DirectoryUser is the reduced user record exposed to a member picker: an ID
// and a display name, never an email, role, or status. Reaching a colleague's
// name is what adding a member needs; anything more is a directory leak to
// every authenticated user.
type DirectoryUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// UserDirectory resolves users for membership operations.
type UserDirectory interface {
	ListDirectory(ctx context.Context) ([]DirectoryUser, error)
	// LookupStatus returns the user's status ("active"/"disabled") and role,
	// or ok=false when no such user exists.
	LookupStatus(ctx context.Context, userID string) (status string, role string, ok bool, err error)
}

// SetUserDirectory wires the account lookup used by membership operations.
func (s *Service) SetUserDirectory(directory UserDirectory) { s.userDirectory = directory }

// ListWorkspaceMembers returns the workspace's membership. Any caller who can
// read the workspace can see who else is in it.
func (s *Service) ListWorkspaceMembers(ctx context.Context, workspaceID string) ([]*models.WorkspaceMember, error) {
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.workspaces.ListWorkspaceMembers(ctx, workspaceID)
}

// UpsertWorkspaceMember adds a member or changes an existing member's role.
func (s *Service) UpsertWorkspaceMember(ctx context.Context, workspaceID, userID, role string) (*models.WorkspaceMember, error) {
	workspace, err := s.requireMemberManage(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	wantRole := authz.NormalizeWorkspaceRole(role)
	if !authz.IsAssignableWorkspaceRole(wantRole) {
		return nil, ErrMemberRoleInvalid
	}
	if userID == workspace.OwnerID {
		return nil, ErrMemberSelf
	}
	if err := s.requireAssignableUser(ctx, userID); err != nil {
		return nil, err
	}

	actor, _ := callerScope(ctx)
	member := &models.WorkspaceMember{
		WorkspaceID: workspaceID,
		UserID:      userID,
		Role:        string(wantRole),
		AddedBy:     actor,
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.workspaces.UpsertWorkspaceMember(ctx, member); err != nil {
		return nil, err
	}
	s.publishWorkspaceAccessChanged(ctx, workspace)
	s.logger.Info("workspace member upserted",
		zap.String("workspace_id", workspaceID), zap.String("user_id", userID), zap.String("role", string(wantRole)))
	return member, nil
}

// RemoveWorkspaceMember drops a membership row. The accountable owner's row
// cannot be removed: ownership is transferred, never vacated.
func (s *Service) RemoveWorkspaceMember(ctx context.Context, workspaceID, userID string) error {
	workspace, err := s.requireMemberManage(ctx, workspaceID)
	if err != nil {
		return err
	}
	if userID == workspace.OwnerID {
		return ErrMemberIsOwner
	}
	if err := s.workspaces.DeleteWorkspaceMember(ctx, workspaceID, userID); err != nil {
		return err
	}
	s.publishWorkspaceAccessChanged(ctx, workspace)
	s.logger.Info("workspace member removed",
		zap.String("workspace_id", workspaceID), zap.String("user_id", userID))
	return nil
}

// TransferWorkspaceOwnership moves the accountable owner to an existing
// member, demoting the previous owner to collaborator.
func (s *Service) TransferWorkspaceOwnership(ctx context.Context, workspaceID, toUserID string) error {
	workspace, err := s.requireMemberManage(ctx, workspaceID)
	if err != nil {
		return err
	}
	if toUserID == workspace.OwnerID {
		return ErrMemberSelf
	}
	if err := s.requireAssignableUser(ctx, toUserID); err != nil {
		return err
	}
	member, err := s.workspaces.GetWorkspaceMember(ctx, workspaceID, toUserID)
	if err != nil {
		return err
	}
	if member == nil {
		return ErrTransferTargetNotMember
	}
	if err := s.workspaces.TransferWorkspaceOwnership(ctx, workspaceID, workspace.OwnerID, toUserID); err != nil {
		return err
	}
	workspace.OwnerID = toUserID
	s.publishWorkspaceAccessChanged(ctx, workspace)
	s.logger.Info("workspace ownership transferred",
		zap.String("workspace_id", workspaceID), zap.String("to_user_id", toUserID))
	return nil
}

// SetWorkspaceVisibility switches a workspace between private and org-visible.
func (s *Service) SetWorkspaceVisibility(ctx context.Context, workspaceID, visibility string) (*models.Workspace, error) {
	workspace, err := s.workspaces.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if err := s.requireWorkspaceManage(ctx, workspace); err != nil {
		return nil, err
	}
	want := authz.NormalizeVisibility(visibility)
	// A guest reaches only workspaces they hold a row on, so a guest-owned
	// workspace marked org-visible would be published to an organization its
	// own owner cannot see. Refuse rather than create that asymmetry.
	if want == authz.VisibilityOrg && workspace.OwnerID != "" {
		_, role, ok, lookupErr := s.lookupUser(ctx, workspace.OwnerID)
		// Fail closed: publishing a workspace to the organization on the
		// strength of an account lookup that did not succeed is exactly the
		// wrong direction to guess in.
		if lookupErr != nil {
			return nil, lookupErr
		}
		if !ok || authz.NormalizeOrgRole(role) == authz.OrgRoleGuest {
			return nil, ErrVisibilityOwnerIsGuest
		}
	}
	workspace.Visibility = string(want)
	workspace.UpdatedAt = time.Now().UTC()
	if err := s.workspaces.UpdateWorkspace(ctx, workspace); err != nil {
		return nil, err
	}
	s.publishWorkspaceAccessChanged(ctx, workspace)
	s.logger.Info("workspace visibility changed",
		zap.String("workspace_id", workspaceID), zap.String("visibility", string(want)))
	return workspace, nil
}

// ListDirectoryUsers returns the reduced user list for a member picker.
func (s *Service) ListDirectoryUsers(ctx context.Context) ([]DirectoryUser, error) {
	if s.userDirectory == nil {
		return []DirectoryUser{}, nil
	}
	return s.userDirectory.ListDirectory(ctx)
}

// requireMemberManage resolves the workspace and enforces member.manage.
func (s *Service) requireMemberManage(ctx context.Context, workspaceID string) (*models.Workspace, error) {
	workspace, err := s.workspaces.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if _, scoped := callerScope(ctx); !scoped {
		return workspace, nil
	}
	decision := s.workspaceDecision(ctx, workspace)
	if !decision.CanRead() {
		return nil, repoerrors.ErrWorkspaceNotFound
	}
	if !decision.Has(authz.ScopeMemberManage) {
		return nil, ErrForbidden
	}
	return workspace, nil
}

// requireAssignableUser rejects unknown and disabled accounts before a write.
func (s *Service) requireAssignableUser(ctx context.Context, userID string) error {
	status, _, ok, err := s.lookupUser(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrMemberUserNotFound
	}
	if status == "disabled" {
		return ErrMemberUserDisabled
	}
	return nil
}

func (s *Service) lookupUser(ctx context.Context, userID string) (string, string, bool, error) {
	if s.userDirectory == nil {
		// No directory wired (pre-auth single-user installs): accept the ID
		// rather than blocking membership entirely.
		return "active", string(authz.OrgRoleMember), true, nil
	}
	return s.userDirectory.LookupStatus(ctx, userID)
}

// OrgSettings supplies instance-wide defaults that membership depends on.
type OrgSettings interface {
	DefaultWorkspaceVisibility(ctx context.Context) authz.Visibility
	SetDefaultWorkspaceVisibility(ctx context.Context, visibility authz.Visibility) error
}

// SetOrgSettings wires the org-level defaults provider.
func (s *Service) SetOrgSettings(settings OrgSettings) { s.orgSettings = settings }

// defaultWorkspaceVisibility resolves the visibility a new workspace starts
// with. A team install sets this to org once and never invites anyone; an
// install that is several individuals sharing a box leaves it private and
// behaves exactly as it does today. Unwired means private.
func (s *Service) defaultWorkspaceVisibility(ctx context.Context) authz.Visibility {
	if s.orgSettings == nil {
		return authz.VisibilityPrivate
	}
	return s.orgSettings.DefaultWorkspaceVisibility(ctx)
}

// seedWorkspaceOwnerMember mirrors workspaces.owner_id into the membership
// table so the two never disagree. Failure is logged rather than fatal: the
// owner still reaches the workspace through owner_id, and the backfill
// migration repairs a missing row on the next boot.
func (s *Service) seedWorkspaceOwnerMember(ctx context.Context, workspace *models.Workspace) error {
	if workspace == nil || workspace.OwnerID == "" {
		return nil
	}
	member := &models.WorkspaceMember{
		WorkspaceID: workspace.ID,
		UserID:      workspace.OwnerID,
		Role:        string(authz.WorkspaceRoleOwner),
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.workspaces.UpsertWorkspaceMember(ctx, member); err != nil {
		// The owner row is not decoration: membership, counts and ownership
		// transfer all read it, so a workspace without one is inconsistent.
		// Report the failure rather than publishing a created event for it.
		s.logger.Error("failed to seed workspace owner membership",
			zap.String("workspace_id", workspace.ID), zap.Error(err))
		return err
	}
	return nil
}

// WorkspaceAccessProjection carries the resolved access for a set of
// workspaces so handlers can build DTOs without re-resolving per row.
type WorkspaceAccessProjection struct {
	Decisions    map[string]authz.Decision
	MemberCounts map[string]int
}

// Decision returns the resolved access for one workspace, defaulting to the
// unscoped view when the projection was built for an internal caller.
func (p WorkspaceAccessProjection) Decision(workspaceID string) authz.Decision {
	if decision, ok := p.Decisions[workspaceID]; ok {
		return decision
	}
	return authz.Denied()
}

// ProjectWorkspaceAccess resolves roles and scopes for a list of workspaces
// using one membership query and one count query, regardless of list length.
func (s *Service) ProjectWorkspaceAccess(ctx context.Context, workspaces []*models.Workspace) WorkspaceAccessProjection {
	projection := WorkspaceAccessProjection{
		Decisions:    make(map[string]authz.Decision, len(workspaces)),
		MemberCounts: map[string]int{},
	}
	if counts, err := s.workspaces.CountWorkspaceMembers(ctx); err == nil {
		projection.MemberCounts = counts
	}

	subject := callerSubject(ctx)
	memberRoles := map[string]string{}
	if !subject.Unscoped {
		roles, err := s.workspaces.ListWorkspaceIDsForMember(ctx, subject.UserID)
		if err != nil {
			// Fail closed: every workspace resolves to Denied rather than
			// silently falling through to the org default role.
			s.logger.Warn("membership projection failed; denying scopes")
			for _, workspace := range workspaces {
				if workspace != nil {
					projection.Decisions[workspace.ID] = authz.Denied()
				}
			}
			return projection
		}
		memberRoles = roles
	}

	for _, workspace := range workspaces {
		if workspace == nil {
			continue
		}
		projection.Decisions[workspace.ID] = authz.ResolveWorkspace(subject, authz.WorkspaceRef{
			OwnerID:    workspace.OwnerID,
			OrgID:      workspace.OrgID,
			Visibility: authz.NormalizeVisibility(workspace.Visibility),
			MemberRole: authz.NormalizeWorkspaceRole(memberRoles[workspace.ID]),
		})
	}
	return projection
}

// DefaultWorkspaceVisibility reports the visibility new workspaces start with.
func (s *Service) DefaultWorkspaceVisibility(ctx context.Context) authz.Visibility {
	return s.defaultWorkspaceVisibility(ctx)
}

// SetDefaultWorkspaceVisibility changes the install-wide default for new
// workspaces. It never touches existing workspaces: turning the default on
// must not retroactively publish work that was private a moment ago.
func (s *Service) SetDefaultWorkspaceVisibility(ctx context.Context, visibility string) (authz.Visibility, error) {
	if !authz.SubjectOrgScopes(callerSubject(ctx)).Has(authz.ScopeOrgSettingsManage) {
		return "", ErrForbidden
	}
	if s.orgSettings == nil {
		return authz.VisibilityPrivate, nil
	}
	want := authz.NormalizeVisibility(visibility)
	if err := s.orgSettings.SetDefaultWorkspaceVisibility(ctx, want); err != nil {
		return "", err
	}
	s.logger.Info("default workspace visibility changed", zap.String("visibility", string(want)))
	return want, nil
}

// WorkspaceReaderIDs returns every user that may currently read a workspace:
// its owner, everyone holding a membership row, and, when the workspace is
// visible to the organization, that organization's non-guest users.
//
// It backs WebSocket fan-out. Returning an empty slice means "nobody" and is a
// real answer: the gateway must not turn it into a global broadcast.
func (s *Service) WorkspaceReaderIDs(ctx context.Context, workspaceID string) ([]string, error) {
	workspace, err := s.workspaces.GetWorkspace(ctx, workspaceID)
	if err != nil || workspace == nil {
		return nil, err
	}
	// A pre-auth workspace has no owner and stays visible to everyone, so it
	// keeps the previous global routing rather than resolving to nobody.
	if workspace.OwnerID == "" {
		return nil, errUnscopedWorkspace
	}

	seen := map[string]struct{}{workspace.OwnerID: {}}
	members, err := s.workspaces.ListWorkspaceMembers(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, member := range members {
		if authz.NormalizeWorkspaceRole(member.Role) != authz.WorkspaceRoleNone {
			seen[member.UserID] = struct{}{}
		}
	}
	if authz.NormalizeVisibility(workspace.Visibility) == authz.VisibilityOrg {
		if err := s.addOrgReaders(ctx, workspace.OrgID, seen); err != nil {
			return nil, err
		}
	}

	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out, nil
}

// addOrgReaders adds the organization's non-guest, active users to the set.
// Guests reach only workspaces they hold an explicit row on, which the caller
// has already collected.
func (s *Service) addOrgReaders(ctx context.Context, orgID string, seen map[string]struct{}) error {
	if s.userDirectory == nil {
		return nil
	}
	users, err := s.userDirectory.ListDirectory(ctx)
	if err != nil {
		return err
	}
	for _, user := range users {
		if s.orgReaderEligible(ctx, orgID, user.ID) {
			seen[user.ID] = struct{}{}
		}
	}
	return nil
}

func (s *Service) orgReaderEligible(ctx context.Context, orgID, userID string) bool {
	status, role, ok, err := s.userDirectory.LookupStatus(ctx, userID)
	if err != nil || !ok || status == "disabled" {
		return false
	}
	if authz.NormalizeOrgRole(role) == authz.OrgRoleGuest {
		return false
	}
	return s.sameOrg(ctx, orgID, userID)
}

// errUnscopedWorkspace tells the gateway to fall back to its previous routing
// for a workspace that predates ownership.
var errUnscopedWorkspace = errors.New("workspace has no owner")

// sameOrg reports whether a user belongs to the workspace's organization.
// With organizations off both sides are empty and every user matches.
func (s *Service) sameOrg(ctx context.Context, workspaceOrgID, userID string) bool {
	if workspaceOrgID == "" || s.userOrgs == nil {
		return true
	}
	orgID, err := s.userOrgs(ctx, userID)
	return err == nil && orgID == workspaceOrgID
}

// SetUserOrgResolver wires the account-to-organization lookup used by
// workspace fan-out.
func (s *Service) SetUserOrgResolver(resolve func(ctx context.Context, userID string) (string, error)) {
	s.userOrgs = resolve
}
