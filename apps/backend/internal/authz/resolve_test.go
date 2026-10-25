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
			ref:      WorkspaceRef{OwnerID: userAna},
			wantRole: WorkspaceRoleOwner,
			wantRead: true,
		},
		{
			name:     "non-member cannot reach a private workspace",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleMember},
			ref:      WorkspaceRef{OwnerID: userAna},
			wantRole: WorkspaceRoleNone,
			wantRead: false,
		},
		{
			name:     "member reaches a workspace through a unit they belong to",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleMember},
			ref:      WorkspaceRef{OwnerID: userAna, InheritedRole: WorkspaceRoleCollaborator},
			wantRole: WorkspaceRoleCollaborator,
			wantRead: true,
		},
		{
			name:     "admin inherits the same role as any other member, not more",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleAdmin},
			ref:      WorkspaceRef{OwnerID: userAna, InheritedRole: WorkspaceRoleCollaborator},
			wantRole: WorkspaceRoleCollaborator,
			wantRead: true,
		},
		{
			name:     "admin does NOT reach a workspace in a unit they are not in",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleAdmin},
			ref:      WorkspaceRef{OwnerID: userAna},
			wantRole: WorkspaceRoleNone,
			wantRead: false,
		},
		{
			name:     "guest inherits nothing, so a shared unit does not reach them",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleGuest},
			ref:      WorkspaceRef{OwnerID: userAna},
			wantRole: WorkspaceRoleNone,
			wantRead: false,
		},
		{
			name:     "guest reaches a workspace it holds a row on",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleGuest},
			ref:      WorkspaceRef{OwnerID: userAna, MemberRole: member(WorkspaceRoleCollaborator)},
			wantRole: WorkspaceRoleCollaborator,
			wantRead: true,
		},
		{
			name:     "a direct viewer grant cannot lower what a unit already gives",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleMember},
			ref:      WorkspaceRef{OwnerID: userAna, InheritedRole: WorkspaceRoleCollaborator, MemberRole: member(WorkspaceRoleViewer)},
			wantRole: WorkspaceRoleCollaborator,
			wantRead: true,
		},
		{
			name:     "unscoped internal caller reaches everything",
			subject:  Subject{Unscoped: true},
			ref:      WorkspaceRef{OwnerID: userAna},
			wantRole: WorkspaceRoleOwner,
			wantRead: true,
		},
		{
			name:     "pre-auth unowned workspace stays visible to everyone",
			subject:  Subject{UserID: userBruno, OrgRole: OrgRoleGuest},
			ref:      WorkspaceRef{OwnerID: ""},
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
		WorkspaceRef{OwnerID: userAna, MemberRole: WorkspaceRoleViewer},
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
	workspaceInB := WorkspaceRef{OwnerID: userAna, OrgID: "org-b"}

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
	owned := WorkspaceRef{OwnerID: userBruno, OrgID: "org-b"}
	if ResolveWorkspace(inOrgA, owned).CanRead() {
		t.Error("a workspace in another org must not be readable even by its owner_id")
	}

	// Same org resolves normally: the tenant check is a gate in front of the
	// tree, not a replacement for it, so an inherited role still applies.
	sameOrg := WorkspaceRef{OwnerID: userAna, OrgID: "org-a", InheritedRole: WorkspaceRoleCollaborator}
	if !ResolveWorkspace(inOrgA, sameOrg).CanRead() {
		t.Error("a same-org workspace inherited through a unit should be readable")
	}
}

// With organizations off nothing carries an org, and behavior is unchanged.
func TestSingleOrgInstanceUnaffectedByTenancyCheck(t *testing.T) {
	subject := Subject{UserID: userBruno, OrgRole: OrgRoleMember}
	ref := WorkspaceRef{OwnerID: userAna, InheritedRole: WorkspaceRoleCollaborator}
	if !ResolveWorkspace(subject, ref).CanRead() {
		t.Error("org-less instance must resolve through the tree like any other")
	}
}

// A workspace with no org (created before the migration) stays reachable by
// its own org's users rather than becoming invisible to everyone.
func TestWorkspaceWithoutOrgStaysReachable(t *testing.T) {
	subject := Subject{UserID: userAna, OrgRole: OrgRoleMember, OrgID: "org-a"}
	ref := WorkspaceRef{OwnerID: userAna}
	if !ResolveWorkspace(subject, ref).CanRead() {
		t.Error("an unmigrated workspace must stay reachable by its owner")
	}
}

// An account that escaped the migration must not become a key to every tenant:
// with tenancy enforced, no org means no access.
func TestOrglessSubjectDeniedWhenTenancyEnforced(t *testing.T) {
	orgless := Subject{UserID: userBruno, OrgRole: OrgRoleAdmin, TenancyEnforced: true}
	for _, ref := range []WorkspaceRef{
		{OwnerID: userAna, OrgID: "org-a"},
		{OwnerID: userAna},
		{OwnerID: userBruno, OrgID: "org-a"},
	} {
		if ResolveWorkspace(orgless, ref).CanRead() {
			t.Errorf("org-less subject reached %+v under enforced tenancy", ref)
		}
	}
	// Without tenancy enforced the same subject behaves as before.
	legacy := Subject{UserID: userBruno, OrgRole: OrgRoleMember}
	if !ResolveWorkspace(legacy, WorkspaceRef{OwnerID: userAna, InheritedRole: WorkspaceRoleCollaborator}).CanRead() {
		t.Error("org-less subject must be unaffected when tenancy is off")
	}
}

// An unrecognized stored role must resolve to the LEAST privileged role, not
// to member. Identity construction carries the stored value through unchanged
// precisely so this normalization is the one that decides.
func TestUnknownStoredRoleResolvesToGuest(t *testing.T) {
	unknown := Subject{UserID: userBruno, OrgRole: NormalizeOrgRole("superuser")}
	orgVisible := WorkspaceRef{OwnerID: userAna}
	if ResolveWorkspace(unknown, orgVisible).CanRead() {
		t.Error("an unknown stored role must not reach an org-visible workspace")
	}
	if got := NormalizeOrgRole("superuser"); got != OrgRoleGuest {
		t.Errorf("NormalizeOrgRole(unknown) = %q, want guest", got)
	}
}

// The combining rule in one test: what you inherit and what you were granted
// are unioned, and the strongest wins. A model where a grant could lower an
// inherited role would need a deny concept, which is the thing this design
// removed on purpose.
func TestRolesCombineByMaximum(t *testing.T) {
	cases := []struct {
		name      string
		inherited WorkspaceRole
		direct    WorkspaceRole
		want      WorkspaceRole
	}{
		{"a direct grant raises what a unit gives", WorkspaceRoleViewer, WorkspaceRoleCollaborator, WorkspaceRoleCollaborator},
		{"a direct grant cannot lower it", WorkspaceRoleCollaborator, WorkspaceRoleViewer, WorkspaceRoleCollaborator},
		{"inheritance alone is enough", WorkspaceRoleCollaborator, WorkspaceRoleNone, WorkspaceRoleCollaborator},
		{"a grant alone is enough", WorkspaceRoleNone, WorkspaceRoleViewer, WorkspaceRoleViewer},
		{"neither reaches nothing", WorkspaceRoleNone, WorkspaceRoleNone, WorkspaceRoleNone},
		{"owner outranks everything", WorkspaceRoleViewer, WorkspaceRoleOwner, WorkspaceRoleOwner},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveWorkspace(
				Subject{UserID: userBruno, OrgRole: OrgRoleMember},
				WorkspaceRef{OwnerID: userAna, InheritedRole: tc.inherited, MemberRole: tc.direct},
			)
			if got.Role != tc.want {
				t.Errorf("role = %q, want %q", got.Role, tc.want)
			}
		})
	}
}

// An unknown stored role must not outrank a real one. Ranking the zero value
// lowest is what makes a corrupt row fail closed rather than silently win.
func TestUnknownRoleDoesNotOutrank(t *testing.T) {
	got := ResolveWorkspace(
		Subject{UserID: userBruno, OrgRole: OrgRoleMember},
		WorkspaceRef{OwnerID: userAna, InheritedRole: WorkspaceRoleCollaborator, MemberRole: WorkspaceRole("superuser")},
	)
	if got.Role != WorkspaceRoleCollaborator {
		t.Errorf("role = %q, want collaborator: an unrecognized role must not win", got.Role)
	}
}
