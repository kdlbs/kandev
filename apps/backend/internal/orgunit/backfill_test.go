package orgunit

import (
	"context"
	"testing"
)

type stubAccounts struct{ users []UserRef }

func (s stubAccounts) ListUnitUsers(context.Context) ([]UserRef, error) { return s.users, nil }

type stubPlacer struct {
	pending []WorkspaceRef
	placed  map[string]string
}

func (s *stubPlacer) UnplacedWorkspaces(context.Context) ([]WorkspaceRef, error) {
	return s.pending, nil
}

func (s *stubPlacer) PlaceWorkspace(_ context.Context, workspaceID, unitID string) error {
	if s.placed == nil {
		s.placed = map[string]string{}
	}
	s.placed[workspaceID] = unitID
	return nil
}

// The rule an upgrade lives or dies by: a workspace that only its owner could
// reach must land somewhere only its owner reaches. Landing it under the root
// would hand the whole organization a board that was private a moment earlier,
// and nothing in the product would report that it had happened.
func TestBackfillDoesNotWidenReach(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	accounts := stubAccounts{users: []UserRef{
		{ID: "ada", OrgID: "org-1", DisplayName: "Ada Lovelace"},
		{ID: "grace", OrgID: "org-1", DisplayName: "Grace Hopper"},
	}}
	placer := &stubPlacer{pending: []WorkspaceRef{
		{ID: "ws-ada", OrgID: "org-1", OwnerID: "ada"},
		{ID: "ws-grace", OrgID: "org-1", OwnerID: "grace"},
		{ID: "ws-unowned", OrgID: "org-1", OwnerID: ""},
	}}

	result, err := svc.Backfill(ctx, accounts, placer)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if result.Placed != 3 {
		t.Fatalf("placed = %d, want 3", result.Placed)
	}

	adaUnit, err := svc.Store().Personal(ctx, "ada")
	if err != nil {
		t.Fatalf("ada personal unit: %v", err)
	}
	graceUnit, err := svc.Store().Personal(ctx, "grace")
	if err != nil {
		t.Fatalf("grace personal unit: %v", err)
	}
	root, err := svc.Store().Root(ctx, "org-1")
	if err != nil {
		t.Fatalf("root: %v", err)
	}

	if placer.placed["ws-ada"] != adaUnit.ID {
		t.Fatalf("an owned workspace landed in %q, want the owner's personal unit", placer.placed["ws-ada"])
	}
	if placer.placed["ws-grace"] != graceUnit.ID {
		t.Fatalf("an owned workspace landed in %q, want the owner's personal unit", placer.placed["ws-grace"])
	}
	// A workspace with no owner predates authentication and everyone reaches it
	// already, so the root changes nothing for it.
	if placer.placed["ws-unowned"] != root.ID {
		t.Fatalf("an unowned workspace landed in %q, want the root", placer.placed["ws-unowned"])
	}
	if placer.placed["ws-ada"] == root.ID || placer.placed["ws-grace"] == root.ID {
		t.Fatal("an owned workspace was placed under the root, which widens reach to the whole organization")
	}
}

// An owner whose account is gone must not hand their workspace to the
// organization. A dangling id still gets a unit of its own.
func TestBackfillIsolatesWorkspacesOfMissingOwners(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	placer := &stubPlacer{pending: []WorkspaceRef{
		{ID: "ws-orphan", OrgID: "org-1", OwnerID: "deleted-user"},
	}}
	if _, err := svc.Backfill(ctx, stubAccounts{}, placer); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	root, err := svc.Store().Root(ctx, "org-1")
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	if placer.placed["ws-orphan"] == root.ID {
		t.Fatal("a workspace owned by a missing account was placed under the root")
	}
	if _, err := svc.Store().Personal(ctx, "deleted-user"); err != nil {
		t.Fatalf("expected a personal unit for the dangling owner: %v", err)
	}
}

// Boot runs this every time, so a second pass must not create a second root, a
// second personal unit, or move a workspace that already has a home.
func TestBackfillIsIdempotent(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	accounts := stubAccounts{users: []UserRef{{ID: "ada", OrgID: "org-1"}}}

	first := &stubPlacer{pending: []WorkspaceRef{{ID: "ws-ada", OrgID: "org-1", OwnerID: "ada"}}}
	if _, err := svc.Backfill(ctx, accounts, first); err != nil {
		t.Fatalf("first backfill: %v", err)
	}

	// Second pass: the workspace now has a unit, so it is no longer pending.
	second := &stubPlacer{pending: nil}
	result, err := svc.Backfill(ctx, accounts, second)
	if err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if result.Placed != 0 {
		t.Fatalf("second pass placed %d workspaces, want 0", result.Placed)
	}

	units, err := svc.Store().ListByOrg(ctx, "org-1")
	if err != nil {
		t.Fatalf("list units: %v", err)
	}
	if len(units) != 2 {
		t.Fatalf("units = %d, want a root and one personal unit", len(units))
	}
}

// With organizations switched off there are no org rows and every id is empty,
// but the tree is still the reach model, so the implicit organization needs a
// root like any other.
func TestBackfillCreatesRootWithTenancyOff(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	placer := &stubPlacer{pending: []WorkspaceRef{{ID: "ws-1", OwnerID: "ada"}}}
	if _, err := svc.Backfill(ctx, stubAccounts{users: []UserRef{{ID: "ada"}}}, placer); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if _, err := svc.Store().Root(ctx, ""); err != nil {
		t.Fatalf("no root for the implicit organization: %v", err)
	}
	if placer.placed["ws-1"] == "" {
		t.Fatal("workspace was left unplaced")
	}
}
