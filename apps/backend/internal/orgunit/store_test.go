package orgunit

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "units.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	store, err := NewStore(db.NewPool(sqlxDB, sqlxDB))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func mustInsert(t *testing.T, s *Store, unit *Unit) *Unit {
	t.Helper()
	out, err := s.Insert(context.Background(), unit)
	if err != nil {
		t.Fatalf("insert %s: %v", unit.Name, err)
	}
	return out
}

// A path is the whole ancestry, which is what lets reach be one query rather
// than a walk. Getting the separators wrong makes a prefix match either miss a
// descendant or catch a sibling whose id merely starts with the same bytes.
func TestInsertBuildsMaterializedPath(t *testing.T) {
	s := newTestStore(t)
	root := mustInsert(t, s, &Unit{OrgID: "org-1", Kind: KindRoot, Name: "Acme"})
	dept := mustInsert(t, s, &Unit{OrgID: "org-1", ParentID: root.ID, Name: "Platform"})
	team := mustInsert(t, s, &Unit{OrgID: "org-1", ParentID: dept.ID, Name: "Runtime"})

	if root.Path != "/"+root.ID+"/" {
		t.Fatalf("root path = %q", root.Path)
	}
	if want := "/" + root.ID + "/" + dept.ID + "/"; dept.Path != want {
		t.Fatalf("dept path = %q, want %q", dept.Path, want)
	}
	if want := "/" + root.ID + "/" + dept.ID + "/" + team.ID + "/"; team.Path != want {
		t.Fatalf("team path = %q, want %q", team.Path, want)
	}
	ids := AncestorIDs(team.Path)
	if len(ids) != 3 || ids[0] != root.ID || ids[2] != team.ID {
		t.Fatalf("ancestors = %v", ids)
	}
}

// Reparenting has to carry the subtree. A move that rewrites only the moved
// row leaves its descendants claiming an ancestry they no longer have, and
// every reach answer below that point silently becomes wrong.
func TestReparentRewritesDescendantPaths(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	root := mustInsert(t, s, &Unit{OrgID: "org-1", Kind: KindRoot, Name: "Acme"})
	oldDept := mustInsert(t, s, &Unit{OrgID: "org-1", ParentID: root.ID, Name: "Platform"})
	newDept := mustInsert(t, s, &Unit{OrgID: "org-1", ParentID: root.ID, Name: "Product"})
	team := mustInsert(t, s, &Unit{OrgID: "org-1", ParentID: oldDept.ID, Name: "Runtime"})
	squad := mustInsert(t, s, &Unit{OrgID: "org-1", ParentID: team.ID, Name: "Scheduler"})

	if err := s.Reparent(ctx, team, newDept); err != nil {
		t.Fatalf("reparent: %v", err)
	}

	movedTeam, err := s.Get(ctx, team.ID)
	if err != nil {
		t.Fatalf("get team: %v", err)
	}
	if want := newDept.Path + team.ID + "/"; movedTeam.Path != want {
		t.Fatalf("team path = %q, want %q", movedTeam.Path, want)
	}
	if movedTeam.ParentID != newDept.ID {
		t.Fatalf("team parent = %q, want %q", movedTeam.ParentID, newDept.ID)
	}

	movedSquad, err := s.Get(ctx, squad.ID)
	if err != nil {
		t.Fatalf("get squad: %v", err)
	}
	if want := movedTeam.Path + squad.ID + "/"; movedSquad.Path != want {
		t.Fatalf("descendant path = %q, want %q", movedSquad.Path, want)
	}
	if got := AncestorIDs(movedSquad.Path); len(got) != 4 || got[1] != newDept.ID {
		t.Fatalf("descendant ancestry = %v, want it to run through the new parent", got)
	}
}

// The reach query asks one question: which of these ancestors does this user
// hold a role on. A membership on a sibling branch must not answer it.
func TestAncestorRolesIgnoresOtherBranches(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	root := mustInsert(t, s, &Unit{OrgID: "org-1", Kind: KindRoot, Name: "Acme"})
	platform := mustInsert(t, s, &Unit{OrgID: "org-1", ParentID: root.ID, Name: "Platform"})
	product := mustInsert(t, s, &Unit{OrgID: "org-1", ParentID: root.ID, Name: "Product"})
	runtime := mustInsert(t, s, &Unit{OrgID: "org-1", ParentID: platform.ID, Name: "Runtime"})

	if err := s.SetMember(ctx, &Member{UnitID: platform.ID, UserID: "ada", Role: "collaborator"}); err != nil {
		t.Fatalf("set member: %v", err)
	}
	if err := s.SetMember(ctx, &Member{UnitID: product.ID, UserID: "grace", Role: "owner"}); err != nil {
		t.Fatalf("set member: %v", err)
	}

	roles, err := s.AncestorRoles(ctx, "ada", runtime.Path)
	if err != nil {
		t.Fatalf("ancestor roles: %v", err)
	}
	if len(roles) != 1 || roles[0] != "collaborator" {
		t.Fatalf("ada on runtime = %v, want one collaborator inherited from the department", roles)
	}

	roles, err = s.AncestorRoles(ctx, "grace", runtime.Path)
	if err != nil {
		t.Fatalf("ancestor roles: %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("grace on runtime = %v, want none: her membership is on another branch", roles)
	}
}

// Re-roling is an update, not a second row, or a demotion would leave the old
// role in place and the maximum rule would keep returning it.
func TestSetMemberReRoles(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	root := mustInsert(t, s, &Unit{OrgID: "org-1", Kind: KindRoot, Name: "Acme"})

	for _, role := range []string{"viewer", "owner"} {
		if err := s.SetMember(ctx, &Member{UnitID: root.ID, UserID: "ada", Role: role}); err != nil {
			t.Fatalf("set member %s: %v", role, err)
		}
	}
	members, err := s.ListMembers(ctx, root.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 1 || members[0].Role != "owner" {
		t.Fatalf("members = %+v, want a single owner row", members)
	}
}

// One root per organization and one personal unit per user are database
// constraints, because a second of either would give a user two ancestries and
// make reach depend on which one a query happened to find.
func TestUniqueRootAndPersonalUnits(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustInsert(t, s, &Unit{OrgID: "org-1", Kind: KindRoot, Name: "Acme"})

	if _, err := s.Insert(ctx, &Unit{OrgID: "org-1", Kind: KindRoot, Name: "Acme again"}); err == nil {
		t.Fatal("a second root unit was accepted")
	}

	root, err := s.Root(ctx, "org-1")
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	mustInsert(t, s, &Unit{OrgID: "org-1", ParentID: root.ID, Kind: KindPersonal, OwnerUserID: "ada", Name: "Ada"})
	if _, err := s.Insert(ctx, &Unit{
		OrgID: "org-1", ParentID: root.ID, Kind: KindPersonal, OwnerUserID: "ada", Name: "Ada again",
	}); err == nil {
		t.Fatal("a second personal unit was accepted for one user")
	}
}

// Deleting an organization has to take its tree with it. Units are the one
// thing no other owner removes: workspaces and accounts are deleted by their
// own paths, so an orphaned tree would accumulate silently.
func TestDeleteByOrgRemovesUnitsAndMemberships(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	doomed := mustInsert(t, s, &Unit{OrgID: "org-doomed", Kind: KindRoot, Name: "Initech"})
	child := mustInsert(t, s, &Unit{OrgID: "org-doomed", ParentID: doomed.ID, Name: "Sales"})
	keeper := mustInsert(t, s, &Unit{OrgID: "org-keep", Kind: KindRoot, Name: "Acme"})
	for _, unit := range []*Unit{doomed, child, keeper} {
		if err := s.SetMember(ctx, &Member{UnitID: unit.ID, UserID: "ada", Role: "owner"}); err != nil {
			t.Fatalf("seed member: %v", err)
		}
	}

	if err := s.DeleteByOrg(ctx, "org-doomed"); err != nil {
		t.Fatalf("delete by org: %v", err)
	}

	if units, err := s.ListByOrg(ctx, "org-doomed"); err != nil || len(units) != 0 {
		t.Fatalf("units after delete = %v (err %v), want none", units, err)
	}
	for _, id := range []string{doomed.ID, child.ID} {
		if members, err := s.ListMembers(ctx, id); err != nil || len(members) != 0 {
			t.Fatalf("memberships survived for %s: %v (err %v)", id, members, err)
		}
	}
	// The other organization is untouched.
	if units, err := s.ListByOrg(ctx, "org-keep"); err != nil || len(units) != 1 {
		t.Fatalf("other org units = %v (err %v), want its root intact", units, err)
	}
	if members, err := s.ListMembers(ctx, keeper.ID); err != nil || len(members) != 1 {
		t.Fatalf("other org membership = %v (err %v), want it intact", members, err)
	}
}
