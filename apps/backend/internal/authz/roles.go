package authz

// OrgRole is the instance-wide role carried on the user record. It grants org
// scopes outright and supplies the default workspace role used on workspaces
// that are visible to the whole organization.
//
// The `owner` role and the `org.delete` scope described in
// docs/specs/auth/requirements/roles-and-scopes.md arrive with multi-tenancy: without an
// org entity there is nothing to own or delete, and shipping an unreachable
// role would only clutter the role picker.
type OrgRole string

const (
	// OrgRoleAdmin manages users, org settings, and shared configuration. It
	// is NOT a reach role: an admin reaches a workspace the same way anyone
	// else does, through the unit tree, and never reaches one sitting in
	// someone's personal unit.
	OrgRoleAdmin OrgRole = "admin"
	// OrgRoleMember is a regular contributor.
	OrgRoleMember OrgRole = "member"
	// OrgRoleGuest holds no org scopes and reaches only workspaces they are an
	// explicit member of, never one reached through the unit tree.
	OrgRoleGuest OrgRole = "guest"
)

// WorkspaceRole is the per-workspace role, held either by an explicit
// workspace_members row or inherited from membership of a unit above the one
// the workspace sits in. Where both apply the stronger of the two wins.
type WorkspaceRole string

const (
	WorkspaceRoleOwner        WorkspaceRole = "owner"
	WorkspaceRoleCollaborator WorkspaceRole = "collaborator"
	WorkspaceRoleViewer       WorkspaceRole = "viewer"
	// WorkspaceRoleNone means the workspace is unreachable.
	WorkspaceRoleNone WorkspaceRole = ""
)

// NormalizeOrgRole coerces an unknown or empty stored value to the least
// privileged real role. Unknown input must never widen access.
func NormalizeOrgRole(value string) OrgRole {
	switch OrgRole(value) {
	case OrgRoleAdmin:
		return OrgRoleAdmin
	case OrgRoleMember:
		return OrgRoleMember
	case OrgRoleGuest:
		return OrgRoleGuest
	default:
		return OrgRoleGuest
	}
}

// NormalizeWorkspaceRole coerces an unknown stored value to no access.
func NormalizeWorkspaceRole(value string) WorkspaceRole {
	switch WorkspaceRole(value) {
	case WorkspaceRoleOwner:
		return WorkspaceRoleOwner
	case WorkspaceRoleCollaborator:
		return WorkspaceRoleCollaborator
	case WorkspaceRoleViewer:
		return WorkspaceRoleViewer
	default:
		return WorkspaceRoleNone
	}
}

// IsAssignableWorkspaceRole reports whether a role may be set through the
// member API. Owner is reached by transfer, not by assignment.
func IsAssignableWorkspaceRole(role WorkspaceRole) bool {
	return role == WorkspaceRoleCollaborator || role == WorkspaceRoleViewer
}

// orgRoleScopes maps an org role to the org scopes it grants.
var orgRoleScopes = map[OrgRole]Set{
	OrgRoleAdmin:  NewSet(ScopeOrgMembersManage, ScopeOrgSettingsManage, ScopeOrgConfigManage, ScopeUnitManage),
	OrgRoleMember: NewSet(),
	OrgRoleGuest:  NewSet(),
}

// workspaceRoleScopes maps a workspace role to the workspace scopes it grants.
//
// session.exec is deliberately absent from viewer and present on collaborator:
// prompting an agent is bounded by that agent's own permissions, a shell in the
// worktree is not, so reading a transcript must never imply a shell.
var workspaceRoleScopes = map[WorkspaceRole]Set{
	WorkspaceRoleOwner: NewSet(
		ScopeWorkspaceRead, ScopeWorkspaceManage, ScopeTaskWrite,
		ScopeSessionPrompt, ScopeSessionControl, ScopeSessionExec,
		ScopeRepositoryManage, ScopeSecretManage, ScopeMemberManage,
	),
	WorkspaceRoleCollaborator: NewSet(
		ScopeWorkspaceRead, ScopeTaskWrite,
		ScopeSessionPrompt, ScopeSessionControl, ScopeSessionExec,
	),
	WorkspaceRoleViewer: NewSet(ScopeWorkspaceRead),
	WorkspaceRoleNone:   NewSet(),
}

// OrgScopes returns the org scopes granted by an org role.
func OrgScopes(role OrgRole) Set { return copySet(orgRoleScopes[role]) }

// WorkspaceScopes returns the workspace scopes granted by a workspace role.
func WorkspaceScopes(role WorkspaceRole) Set { return copySet(workspaceRoleScopes[role]) }

// AllScopes returns every scope both role tables can grant. Used by the
// synthetic (auth-disabled) identity, which must behave exactly as the
// pre-auth single user did.
func AllScopes() Set {
	all := NewSet()
	for _, def := range registry {
		all[def.Scope] = struct{}{}
	}
	return all
}

func copySet(src Set) Set {
	out := make(Set, len(src))
	for scope := range src {
		out[scope] = struct{}{}
	}
	return out
}

// workspaceRoleRank orders the workspace roles so they can be combined by
// maximum. The zero value is the weakest, so an unknown role never outranks a
// real one.
func workspaceRoleRank(role WorkspaceRole) int {
	switch role {
	case WorkspaceRoleOwner:
		return 3
	case WorkspaceRoleCollaborator:
		return 2
	case WorkspaceRoleViewer:
		return 1
	default:
		return 0
	}
}
