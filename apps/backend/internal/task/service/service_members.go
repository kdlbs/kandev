package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"go.uber.org/zap"
)

// Workspace membership errors. Each failure mode gets its own sentinel so the
// UI can say what actually went wrong instead of "bad request".
var (
	ErrMemberUserNotFound           = errors.New("user not found")
	ErrMemberUserDisabled           = errors.New("user account is disabled")
	ErrMemberIsOwner                = errors.New("the workspace owner cannot be removed; transfer ownership first")
	ErrMemberRoleInvalid            = errors.New("role must be collaborator or viewer")
	ErrMemberSelf                   = errors.New("you already own this workspace")
	ErrTransferTargetNotMember      = errors.New("add the user as a member before transferring ownership")
	ErrAssigneeCannotReachWorkspace = errors.New("that person cannot see this workspace")
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
	// ListDirectory returns the pickable accounts in one organization. An
	// empty orgID means the single implicit organization.
	ListDirectory(ctx context.Context, orgID string) ([]DirectoryUser, error)
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
	if err := s.requireAssignableWorkspaceUser(ctx, userID, workspace.OrgID); err != nil {
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
	if err := s.requireAssignableWorkspaceUser(ctx, toUserID, workspace.OrgID); err != nil {
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

// ListDirectoryUsers returns the reduced user list for a member picker.
//
// It is scoped to the caller's organization. The picker is the one place a
// person's name is shown to someone who has no other relationship with them,
// so an unscoped list would make every account on the instance enumerable from
// any tenant.
func (s *Service) ListDirectoryUsers(ctx context.Context) ([]DirectoryUser, error) {
	if s.userDirectory == nil {
		return []DirectoryUser{}, nil
	}
	return s.userDirectory.ListDirectory(ctx, callerOrgID(ctx))
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

// requireAssignableWorkspaceUser additionally enforces the tenant boundary.
// A mismatch is reported as not found so a crafted user ID cannot reveal an
// account in another organization.
func (s *Service) requireAssignableWorkspaceUser(ctx context.Context, userID, workspaceOrgID string) error {
	if err := s.requireAssignableUser(ctx, userID); err != nil {
		return err
	}
	if workspaceOrgID == "" {
		return nil
	}
	if s.userOrgs == nil {
		return ErrMemberUserNotFound
	}
	userOrgID, err := s.userOrgs(ctx, userID)
	if err != nil {
		return err
	}
	if userOrgID != workspaceOrgID {
		return ErrMemberUserNotFound
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

	inherited, ok := s.inheritedRolesFor(ctx, subject.UserID, workspaces)
	if !ok {
		for _, workspace := range workspaces {
			if workspace != nil {
				projection.Decisions[workspace.ID] = authz.Denied()
			}
		}
		return projection
	}

	for _, workspace := range workspaces {
		if workspace == nil {
			continue
		}
		projection.Decisions[workspace.ID] = authz.ResolveWorkspace(subject, authz.WorkspaceRef{
			OwnerID:       workspace.OwnerID,
			OrgID:         workspace.OrgID,
			MemberRole:    authz.NormalizeWorkspaceRole(memberRoles[workspace.ID]),
			InheritedRole: inherited[workspace.ID],
		})
	}
	return projection
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
	// Everyone the placement reaches, which is every member of the workspace's
	// unit and of its ancestors. A workspace shared with the whole
	// organization is one placed in a unit the whole organization is in.
	if s.unitReach != nil && workspace.UnitID != "" {
		readers, err := s.unitReach.UnitReaders(ctx, workspace.UnitID)
		if err != nil {
			return nil, err
		}
		for _, id := range readers {
			seen[id] = struct{}{}
		}
	}

	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out, nil
}

// errUnscopedWorkspace tells the gateway to fall back to its previous routing
// for a workspace that predates ownership.
var errUnscopedWorkspace = errors.New("workspace has no owner")

// SetUserOrgResolver wires the account-to-organization lookup used by
// workspace fan-out.
func (s *Service) SetUserOrgResolver(resolve func(ctx context.Context, userID string) (string, error)) {
	s.userOrgs = resolve
}

// SetHumanAssignee applies a human assignee to a task on behalf of the caller
// in ctx, so surfaces outside this package (today: the office PATCH handler)
// get the same authorization and validation as the task API instead of
// reimplementing either. Empty unassigns.
//
// It deliberately goes through UpdateTask rather than writing the column: the
// office route carries no `:wsId`, so it is not covered by the office
// workspace-scope middleware, and a direct write there would let any signed-in
// user assign a task in a workspace they cannot reach.
func (s *Service) SetHumanAssignee(ctx context.Context, taskID, userID string) error {
	_, err := s.UpdateTask(ctx, taskID, &UpdateTaskRequest{AssigneeUserID: &userID})
	return err
}

// resolveTaskAssignee validates a human assignee for a task.
//
// Assignment is advisory: it gates nothing, and any caller holding task.write
// may assign to anyone, including themselves. The one rule is that the
// assignee has to be able to reach the workspace, so a task cannot be parked
// on somebody who will never see it. An empty value unassigns.
func (s *Service) resolveTaskAssignee(ctx context.Context, task *models.Task, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", nil
	}
	if err := s.requireAssignableUser(ctx, userID); err != nil {
		return "", err
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil || workspace == nil {
		// A dangling workspace reference should not block assignment on a task
		// the caller can already edit.
		return userID, nil //nolint:nilerr // visibility fallback, not a failure
	}
	if !s.userReachesWorkspace(ctx, workspace, userID) {
		return "", ErrAssigneeCannotReachWorkspace
	}
	return userID, nil
}

// userReachesWorkspace answers the reach question for a user who is NOT the
// caller, so it resolves their membership and org role rather than reading the
// request identity.
func (s *Service) userReachesWorkspace(ctx context.Context, workspace *models.Workspace, userID string) bool {
	if workspace.OwnerID == "" || workspace.OwnerID == userID {
		return true
	}
	member, err := s.workspaces.GetWorkspaceMember(ctx, workspace.ID, userID)
	if err != nil {
		return false
	}
	ref := authz.WorkspaceRef{
		OwnerID: workspace.OwnerID,
		OrgID:   workspace.OrgID,
	}
	if member != nil {
		ref.MemberRole = authz.NormalizeWorkspaceRole(member.Role)
	}
	// Reach comes from the tree first: most people reach a workspace because
	// they are in a unit above it, not because they hold a row on it. Asking
	// only about the row is how this check came to refuse a colleague who can
	// plainly see the board.
	if inherited, ok := s.inheritedRole(ctx, userID, workspace); ok {
		ref.InheritedRole = inherited
	} else {
		return false
	}
	subject := authz.Subject{UserID: userID, TenancyEnforced: tenancyEnforced}
	if s.userDirectory != nil {
		if _, role, ok, lookupErr := s.userDirectory.LookupStatus(ctx, userID); lookupErr == nil && ok {
			subject.OrgRole = authz.NormalizeOrgRole(role)
		}
	}
	if s.userOrgs != nil {
		if orgID, orgErr := s.userOrgs(ctx, userID); orgErr == nil {
			subject.OrgID = orgID
		}
	}
	return authz.ResolveWorkspace(subject, ref).CanRead()
}
