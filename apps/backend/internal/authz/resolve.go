package authz

// Subject is the caller, as far as authorization is concerned.
type Subject struct {
	UserID  string
	OrgRole OrgRole
	// OrgID is the caller's tenant, empty when organizations are off.
	OrgID string
	// TenancyEnforced mirrors features.multiTenancy. When it is on, a subject
	// with no org is DENIED rather than treated as org-less: an account that
	// somehow escaped the migration must not become a key to every tenant.
	TenancyEnforced bool
	// Unscoped marks a caller that predates or bypasses per-user scoping: an
	// internal caller (event bus, pollers, schedulers) or the synthetic
	// identity injected while authentication is disabled. Such a caller gets
	// everything, which is exactly today's behavior.
	Unscoped bool
}

// WorkspaceRef is the workspace state authorization needs.
type WorkspaceRef struct {
	OwnerID string
	// OrgID is the workspace's tenant, empty when organizations are off.
	OrgID string
	// InheritedRole is the highest role the caller holds on any unit that is
	// an ancestor of the workspace's unit, including that unit itself. It is
	// WorkspaceRoleNone when they are a member of none of them.
	InheritedRole WorkspaceRole
	// MemberRole is a direct grant recorded against this workspace alone. It
	// can only raise the result, never lower it.
	MemberRole WorkspaceRole
}

// Decision is the resolved access for one (subject, workspace) pair.
type Decision struct {
	Role   WorkspaceRole
	Scopes Set
}

// CanRead reports whether the caller reaches the workspace at all. A false
// result must surface as 404, never 403: existence is not disclosed.
func (d Decision) CanRead() bool { return d.Scopes.Has(ScopeWorkspaceRead) }

// Has reports whether the decision grants a scope.
func (d Decision) Has(scope Scope) bool { return d.Scopes.Has(scope) }

// ResolveWorkspace is the only place workspace permissions are derived.
//
// Roles combine by maximum. Nothing subtracts, so no grant can lower what a
// unit already gives: narrowing is done by placing a workspace in a unit with
// fewer members, which keeps the rule to one sentence and keeps a permission
// question answerable by reading the tree.
func ResolveWorkspace(subject Subject, ref WorkspaceRef) Decision {
	// The tenant boundary is checked first and is absolute. It is not a
	// permission level that a role can outrank: a workspace in another org is
	// not visible to anybody, including an org admin or an instance operator.
	if crossOrg(subject, ref) {
		return decide(WorkspaceRoleNone)
	}

	switch {
	case subject.Unscoped:
		return decide(WorkspaceRoleOwner)

	// A workspace created before authentication was enabled has no owner and
	// stays visible to everyone until the setup wizard claims it. Preserving
	// this keeps single-user upgrades byte-identical.
	case ref.OwnerID == "":
		return decide(WorkspaceRoleOwner)

	case subject.UserID != "" && ref.OwnerID == subject.UserID:
		return decide(WorkspaceRoleOwner)

	default:
		return decide(highestRole(ref.InheritedRole, ref.MemberRole))
	}
}

// highestRole returns the strongest of the roles it is given. It is the whole
// of the combining rule.
func highestRole(roles ...WorkspaceRole) WorkspaceRole {
	best := WorkspaceRoleNone
	for _, role := range roles {
		if workspaceRoleRank(role) > workspaceRoleRank(best) {
			best = role
		}
	}
	return best
}

// Denied is the fail-closed decision. Resolution errors return this rather
// than falling back to the org default role, which is the branch that would
// silently widen access under a transient database failure.
func Denied() Decision { return decide(WorkspaceRoleNone) }

func decide(role WorkspaceRole) Decision {
	return Decision{Role: role, Scopes: WorkspaceScopes(role)}
}

// SubjectOrgScopes returns the org scopes a subject holds. An unscoped caller
// holds every org scope.
func SubjectOrgScopes(subject Subject) Set {
	if subject.Unscoped {
		return orgScopeSet()
	}
	return OrgScopes(subject.OrgRole)
}

func orgScopeSet() Set {
	out := NewSet()
	for _, def := range registry {
		if def.Kind == KindOrg {
			out[def.Scope] = struct{}{}
		}
	}
	return out
}

// crossOrg reports whether the subject and the workspace belong to different
// tenants. Both sides must carry an org for the check to apply, so a
// single-org instance (organizations off) is unaffected.
func crossOrg(subject Subject, ref WorkspaceRef) bool {
	if subject.Unscoped {
		return false
	}
	// With tenancy on, a caller carrying no org reaches nothing. Treating them
	// as org-less would make an unmigrated account a key to every tenant,
	// because an empty org matches everything.
	if subject.TenancyEnforced && subject.OrgID == "" {
		return true
	}
	if subject.OrgID == "" || ref.OrgID == "" {
		return false
	}
	return subject.OrgID != ref.OrgID
}
