package authz

import "testing"

const (
	userAna   = "user-ana"
	userBruno = "user-bruno"
)

func member(role WorkspaceRole) WorkspaceRole { return role }

func TestResolveWorkspaceReachTable(t *testing.T) {
	cases := []struct {
		name     string
		subject  Subject
		ref      WorkspaceRef
		wantRole WorkspaceRole
		wantRead bool
	}{
		{
			name:     "owner reaches own private workspace",
			subject:  Subject{UserID: userAna, OrgRole: OrgRoleMember},
			ref:      WorkspaceRef{OwnerID: userAna, Visibility: VisibilityPrivate},
			wantRole: WorkspaceRoleOwner,
			wantRead: true,
		},
		{
			name:     "non-member cannot reach a private workspace",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleMember},
			ref:      WorkspaceRef{OwnerID: userAna, Visibility: VisibilityPrivate},
			wantRole: WorkspaceRoleNone,
			wantRead: false,
		},
		{
			name:     "member reaches an org-visible workspace with no row",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleMember},
			ref:      WorkspaceRef{OwnerID: userAna, Visibility: VisibilityOrg},
			wantRole: WorkspaceRoleCollaborator,
			wantRead: true,
		},
		{
			name:     "admin reaches an org-visible workspace as a plain collaborator",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleAdmin},
			ref:      WorkspaceRef{OwnerID: userAna, Visibility: VisibilityOrg},
			wantRole: WorkspaceRoleCollaborator,
			wantRead: true,
		},
		{
			name:     "admin does NOT reach a private workspace it is not in",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleAdmin},
			ref:      WorkspaceRef{OwnerID: userAna, Visibility: VisibilityPrivate},
			wantRole: WorkspaceRoleNone,
			wantRead: false,
		},
		{
			name:     "guest does NOT reach an org-visible workspace",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleGuest},
			ref:      WorkspaceRef{OwnerID: userAna, Visibility: VisibilityOrg},
			wantRole: WorkspaceRoleNone,
			wantRead: false,
		},
		{
			name:     "guest reaches a workspace it holds a row on",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleGuest},
			ref:      WorkspaceRef{OwnerID: userAna, Visibility: VisibilityPrivate, MemberRole: member(WorkspaceRoleCollaborator)},
			wantRole: WorkspaceRoleCollaborator,
			wantRead: true,
		},
		{
			name:     "explicit viewer row narrows a member on an org-visible workspace",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleMember},
			ref:      WorkspaceRef{OwnerID: userAna, Visibility: VisibilityOrg, MemberRole: member(WorkspaceRoleViewer)},
			wantRole: WorkspaceRoleViewer,
			wantRead: true,
		},
		{
			name:     "unscoped internal caller reaches everything",
			subject:  Subject{Unscoped: true},
			ref:      WorkspaceRef{OwnerID: userAna, Visibility: VisibilityPrivate},
			wantRole: WorkspaceRoleOwner,
			wantRead: true,
		},
		{
			name:     "pre-auth unowned workspace stays visible to everyone",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleGuest},
			ref:      WorkspaceRef{OwnerID: "", Visibility: VisibilityPrivate},
			wantRole: WorkspaceRoleOwner,
			wantRead: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveWorkspace(tc.subject, tc.ref)
			if got.Role != tc.wantRole {
				t.Errorf("role = %q, want %q", got.Role, tc.wantRole)
			}
			if got.CanRead() != tc.wantRead {
				t.Errorf("CanRead() = %v, want %v", got.CanRead(), tc.wantRead)
			}
		})
	}
}

// A viewer may read a transcript; a shell in the worktree is a different
// question, and collapsing the two is the mistake this test exists to catch.
func TestViewerHasNoExecOrWrite(t *testing.T) {
	viewer := ResolveWorkspace(
		Subject{UserID: userBruno, OrgRole: OrgRoleMember},
		WorkspaceRef{OwnerID: userAna, Visibility: VisibilityOrg, MemberRole: WorkspaceRoleViewer},
	)
	if !viewer.Has(ScopeWorkspaceRead) {
		t.Error("viewer should hold workspace.read")
	}
	for _, denied := range []Scope{ScopeSessionExec, ScopeSessionPrompt, ScopeTaskWrite, ScopeSessionControl} {
		if viewer.Has(denied) {
			t.Errorf("viewer must not hold %q", denied)
		}
	}
}

// Managing a workspace, its members, repositories and secrets belongs to the
// owner. A collaborator contributes; it does not administer.
func TestCollaboratorCannotAdminister(t *testing.T) {
	collab := WorkspaceScopes(WorkspaceRoleCollaborator)
	for _, denied := range []Scope{ScopeWorkspaceManage, ScopeMemberManage, ScopeSecretManage, ScopeRepositoryManage} {
		if collab.Has(denied) {
			t.Errorf("collaborator must not hold %q", denied)
		}
	}
	owner := WorkspaceScopes(WorkspaceRoleOwner)
	for _, granted := range []Scope{ScopeWorkspaceManage, ScopeMemberManage, ScopeSecretManage, ScopeRepositoryManage} {
		if !owner.Has(granted) {
			t.Errorf("owner should hold %q", granted)
		}
	}
}

func TestDeniedGrantsNothing(t *testing.T) {
	if Denied().CanRead() {
		t.Error("Denied() must not grant workspace.read")
	}
	if len(Denied().Scopes) != 0 {
		t.Errorf("Denied() granted %v", Denied().Scopes.List())
	}
}

func TestOrgScopes(t *testing.T) {
	admin := SubjectOrgScopes(Subject{OrgRole: OrgRoleAdmin})
	if !admin.Has(ScopeOrgMembersManage) || !admin.Has(ScopeOrgConfigManage) {
		t.Errorf("admin org scopes = %v", admin.List())
	}
	for _, role := range []OrgRole{OrgRoleMember, OrgRoleGuest} {
		if scopes := SubjectOrgScopes(Subject{OrgRole: role}); len(scopes) != 0 {
			t.Errorf("%s should hold no org scopes, got %v", role, scopes.List())
		}
	}
	if unscoped := SubjectOrgScopes(Subject{Unscoped: true}); !unscoped.Has(ScopeOrgMembersManage) {
		t.Error("unscoped caller should hold every org scope")
	}
}

// Unknown stored values must fail closed rather than widen access.
func TestNormalizersFailClosed(t *testing.T) {
	if got := NormalizeOrgRole("superuser"); got != OrgRoleGuest {
		t.Errorf("NormalizeOrgRole(unknown) = %q, want guest", got)
	}
	if got := NormalizeOrgRole(""); got != OrgRoleGuest {
		t.Errorf("NormalizeOrgRole(empty) = %q, want guest", got)
	}
	if got := NormalizeWorkspaceRole("superuser"); got != WorkspaceRoleNone {
		t.Errorf("NormalizeWorkspaceRole(unknown) = %q, want none", got)
	}
	if got := NormalizeVisibility("public"); got != VisibilityPrivate {
		t.Errorf("NormalizeVisibility(unknown) = %q, want private", got)
	}
	if got := NormalizeVisibility(""); got != VisibilityPrivate {
		t.Errorf("NormalizeVisibility(empty) = %q, want private", got)
	}
}

func TestIsAssignableWorkspaceRole(t *testing.T) {
	if IsAssignableWorkspaceRole(WorkspaceRoleOwner) {
		t.Error("owner must be reached by transfer, not assignment")
	}
	if !IsAssignableWorkspaceRole(WorkspaceRoleCollaborator) || !IsAssignableWorkspaceRole(WorkspaceRoleViewer) {
		t.Error("collaborator and viewer must be assignable")
	}
}

// The tenant boundary is absolute: it is checked before any role, so no role
// can outrank it. These cases are the whole point of organizations.
func TestCrossOrgIsAbsolute(t *testing.T) {
	inOrgA := Subject{UserID: userBruno, OrgRole: OrgRoleAdmin, OrgID: "org-a"}
	workspaceInB := WorkspaceRef{OwnerID: userAna, OrgID: "org-b", Visibility: VisibilityOrg}

	if ResolveWorkspace(inOrgA, workspaceInB).CanRead() {
		t.Error("an org-visible workspace in another org must not be readable")
	}

	// Even an explicit membership row cannot cross the boundary: the tenancy
	// migration drops such rows, and a stale one must not grant access.
	withRow := workspaceInB
	withRow.MemberRole = WorkspaceRoleCollaborator
	if ResolveWorkspace(inOrgA, withRow).CanRead() {
		t.Error("a stale cross-org membership row must not grant access")
	}

	// Owning it does not help either: owner_id and org_id disagreeing is
	// corrupt data, and the safe reading is no access.
	owned := WorkspaceRef{OwnerID: userBruno, OrgID: "org-b", Visibility: VisibilityPrivate}
	if ResolveWorkspace(inOrgA, owned).CanRead() {
		t.Error("a workspace in another org must not be readable even by its owner_id")
	}

	// Same org resolves normally.
	sameOrg := WorkspaceRef{OwnerID: userAna, OrgID: "org-a", Visibility: VisibilityOrg}
	if !ResolveWorkspace(inOrgA, sameOrg).CanRead() {
		t.Error("same-org org-visible workspace should be readable")
	}
}

// With organizations off nothing carries an org, and behavior is unchanged.
func TestSingleOrgInstanceUnaffectedByTenancyCheck(t *testing.T) {
	subject := Subject{UserID: userBruno, OrgRole: OrgRoleMember}
	ref := WorkspaceRef{OwnerID: userAna, Visibility: VisibilityOrg}
	if !ResolveWorkspace(subject, ref).CanRead() {
		t.Error("org-less instance must behave exactly as before")
	}
}

// A workspace with no org (created before the migration) stays reachable by
// its own org's users rather than becoming invisible to everyone.
func TestWorkspaceWithoutOrgStaysReachable(t *testing.T) {
	subject := Subject{UserID: userAna, OrgRole: OrgRoleMember, OrgID: "org-a"}
	ref := WorkspaceRef{OwnerID: userAna, Visibility: VisibilityPrivate}
	if !ResolveWorkspace(subject, ref).CanRead() {
		t.Error("an unmigrated workspace must stay reachable by its owner")
	}
}

// An account that escaped the migration must not become a key to every tenant:
// with tenancy enforced, no org means no access.
func TestOrglessSubjectDeniedWhenTenancyEnforced(t *testing.T) {
	orgless := Subject{UserID: userBruno, OrgRole: OrgRoleAdmin, TenancyEnforced: true}
	for _, ref := range []WorkspaceRef{
		{OwnerID: userAna, OrgID: "org-a", Visibility: VisibilityOrg},
		{OwnerID: userAna, Visibility: VisibilityOrg},
		{OwnerID: userBruno, OrgID: "org-a"},
	} {
		if ResolveWorkspace(orgless, ref).CanRead() {
			t.Errorf("org-less subject reached %+v under enforced tenancy", ref)
		}
	}
	// Without tenancy enforced the same subject behaves as before.
	legacy := Subject{UserID: userBruno, OrgRole: OrgRoleMember}
	if !ResolveWorkspace(legacy, WorkspaceRef{OwnerID: userAna, Visibility: VisibilityOrg}).CanRead() {
		t.Error("org-less subject must be unaffected when tenancy is off")
	}
}

// An unrecognized stored role must resolve to the LEAST privileged role, not
// to member. Identity construction carries the stored value through unchanged
// precisely so this normalization is the one that decides.
func TestUnknownStoredRoleResolvesToGuest(t *testing.T) {
	unknown := Subject{UserID: userBruno, OrgRole: NormalizeOrgRole("superuser")}
	orgVisible := WorkspaceRef{OwnerID: userAna, Visibility: VisibilityOrg}
	if ResolveWorkspace(unknown, orgVisible).CanRead() {
		t.Error("an unknown stored role must not reach an org-visible workspace")
	}
	if got := NormalizeOrgRole("superuser"); got != OrgRoleGuest {
		t.Errorf("NormalizeOrgRole(unknown) = %q, want guest", got)
	}
}
