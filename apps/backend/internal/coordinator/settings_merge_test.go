package coordinator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

type fakePrincipals struct{ ok map[string]bool }

func (f fakePrincipals) ManagerAuthorized(_ context.Context, _, userID string) (bool, error) {
	return f.ok[userID], nil
}

func mergeEnv(t *testing.T) (*Store, *Coordinator, *Service) {
	t.Helper()
	store, c, _, svc := phase2ApproveFixture(t)
	svc.merge = true
	mustExec(t, store, fmt.Sprintf(`CREATE TABLE repositories (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT NOT NULL, deleted_at %s);
		INSERT INTO repositories (id, workspace_id, name) VALUES ('r1','%s','beta'),('r2','%s','alpha'),('rx','other','foreign');`,
		tsType(store), c.WorkspaceID, c.WorkspaceID))
	svc.SetPrincipalValidator(fakePrincipals{ok: map[string]bool{}})
	return store, c, svc
}

func saveMerge(svc *Service, c *Coordinator, rev int, entries string) (*CoordinatorPhase2, error) {
	return svc.SaveSettings(context.Background(), c.WorkspaceID, c.ID,
		[]byte(fmt.Sprintf(`{"policy_revision":%d,"merge":{"repositories":[%s]}}`, rev, entries)))
}

func storedRevision(t *testing.T, store *Store, id string) int {
	t.Helper()
	var n int
	if err := store.db.QueryRow(store.db.Rebind(`SELECT policy_revision FROM coordinators WHERE id = ?`), id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMergeSaveBumpsRevisionAndAudits(t *testing.T) {
	store, c, svc := mergeEnv(t)
	before := storedRevision(t, store, c.ID)
	out, err := saveMerge(svc, c, before, `{"repository_id":"r1","policy":"requires_approval"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out.PolicyRevision != before+1 || storedRevision(t, store, c.ID) != before+1 {
		t.Fatalf("revision = %d", out.PolicyRevision)
	}
	var n int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM coordinator_merge_audit WHERE kind = 'policy_changed' AND subject = 'r1' AND revision = ?`, before+1).Scan(&n)
	if n != 1 {
		t.Fatalf("audit rows = %d", n)
	}
	// The same save again changes nothing: no revision, no audit.
	if _, err := saveMerge(svc, c, 0, `{"repository_id":"r1","policy":"requires_approval"}`); err != nil {
		t.Fatalf("unchanged save needs no revision: %v", err)
	}
	if storedRevision(t, store, c.ID) != before+1 {
		t.Fatal("unchanged save bumped the revision")
	}
}

func TestMergeSaveStaleOrMissingRevisionStoresNothing(t *testing.T) {
	store, c, svc := mergeEnv(t)
	rev := storedRevision(t, store, c.ID)
	_, err := saveMerge(svc, c, rev+5, `{"repository_id":"r1","policy":"requires_approval"},{"repository_id":"r2","policy":"requires_approval"}`)
	var conflict *MergeConflictError
	if !errors.As(err, &conflict) || conflict.Code != codeRevisionConflict {
		t.Fatalf("err = %v", err)
	}
	_, err = svc.SaveSettings(context.Background(), c.WorkspaceID, c.ID, []byte(`{"merge":{"repositories":[{"repository_id":"r1","policy":"requires_approval"}]}}`))
	var se *SettingsError
	if !errors.As(err, &se) || se.Code != codeRevisionRequired {
		t.Fatalf("err = %v", err)
	}
	var n int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM coordinator_merge_repos`).Scan(&n)
	if n != 0 {
		t.Fatalf("rows stored by a refused save: %d", n)
	}
}

func TestMergeSaveRefusesUnknownAndForeignRepository(t *testing.T) {
	store, c, svc := mergeEnv(t)
	rev := storedRevision(t, store, c.ID)
	for _, id := range []string{"rx", "nope"} {
		_, err := saveMerge(svc, c, rev, fmt.Sprintf(`{"repository_id":%q,"policy":"automatic"}`, id))
		var se *SettingsError
		if !errors.As(err, &se) || se.Code != codeRepositoryUnknown {
			t.Fatalf("%s: err = %v", id, err)
		}
	}
}

func TestMergePrincipalReplacementOverHTTPShape(t *testing.T) {
	store, c, svc := mergeEnv(t)
	ctx := managerCtx("u1")
	rev := storedRevision(t, store, c.ID)
	if _, err := svc.SaveSettings(ctx, c.WorkspaceID, c.ID, []byte(fmt.Sprintf(`{"policy_revision":%d,"merge":{"repositories":[{"repository_id":"r1","policy":"automatic"}]}}`, rev))); err != nil {
		t.Fatal(err)
	}
	rev = storedRevision(t, store, c.ID)
	rows, _ := store.loadRepoPolicies(context.Background(), store.db, c.ID)
	if rows["r1"].SelectedBy != "u1" || rows["r1"].AutomaticSinceRevision != rev {
		t.Fatalf("row = %+v", rows["r1"])
	}
	body := []byte(fmt.Sprintf(`{"policy_revision":%d,"merge":{"repositories":[{"repository_id":"r1","policy":"automatic","replace_principal":true}]}}`, rev))
	svc.SetPrincipalValidator(fakePrincipals{ok: map[string]bool{"u1": true}})
	_, err := svc.SaveSettings(managerCtx("u2"), c.WorkspaceID, c.ID, body)
	var conflict *MergeConflictError
	if !errors.As(err, &conflict) || conflict.Code != codePrincipalValid {
		t.Fatalf("err = %v", err)
	}
	svc.SetPrincipalValidator(fakePrincipals{ok: map[string]bool{}})
	if _, err := svc.SaveSettings(managerCtx("u2"), c.WorkspaceID, c.ID, body); err != nil {
		t.Fatal(err)
	}
	rows, _ = store.loadRepoPolicies(context.Background(), store.db, c.ID)
	if rows["r1"].SelectedBy != "u2" || rows["r1"].AutomaticSinceRevision != rev {
		t.Fatalf("row after replace = %+v", rows["r1"])
	}
}

func TestMergeSaveFlagOffRefused(t *testing.T) {
	store, c, svc := mergeEnv(t)
	svc.merge = false
	_, err := saveMerge(svc, c, storedRevision(t, store, c.ID), `{"repository_id":"r1","policy":"requires_approval"}`)
	var se *SettingsError
	if !errors.As(err, &se) || se.Code != codeMergeUnavailable {
		t.Fatalf("err = %v", err)
	}
}

func TestMergeSaveRequiresManageScope(t *testing.T) {
	store, c, svc := mergeEnv(t)
	svc.authz = &fakeWorkspaceAuthorizer{err: taskservice.ErrForbidden}
	_, err := saveMerge(svc, c, storedRevision(t, store, c.ID), `{"repository_id":"r1","policy":"requires_approval"}`)
	if !errors.Is(err, taskservice.ErrForbidden) {
		t.Fatalf("err = %v", err)
	}
}

func TestMergeSaveLeavesActionsAndEligibilityAlone(t *testing.T) {
	store, c, svc := mergeEnv(t)
	mustSave(t, svc, c.WorkspaceID, c.ID, policyBody(map[string]string{"create_task": "requires_approval"}))
	before, _ := svc.GetSettings(context.Background(), c.WorkspaceID, c.ID)
	if _, err := saveMerge(svc, c, before.PolicyRevision, `{"repository_id":"r1","policy":"requires_approval"}`); err != nil {
		t.Fatal(err)
	}
	after, _ := svc.GetSettings(context.Background(), c.WorkspaceID, c.ID)
	if after.Policy.Actions["create_task"] != before.Policy.Actions["create_task"] || after.Policy.Eligibility != before.Policy.Eligibility {
		t.Fatalf("policy changed: %+v -> %+v", before.Policy, after.Policy)
	}
	_ = store
}

func tsType(s *Store) string {
	if s.db.DriverName() != "sqlite3" {
		return "TIMESTAMP"
	}
	return "DATETIME"
}

func TestMergeViewListsRepositoriesByNameAndMarksInactivePrincipal(t *testing.T) {
	store, c, svc := mergeEnv(t)
	rev := storedRevision(t, store, c.ID)
	if _, err := svc.SaveSettings(managerCtx("u1"), c.WorkspaceID, c.ID, []byte(fmt.Sprintf(`{"policy_revision":%d,"merge":{"repositories":[{"repository_id":"r1","policy":"automatic"}]}}`, rev))); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetSettings(context.Background(), c.WorkspaceID, c.ID)
	if err != nil || got.Merge == nil || len(got.Merge.Repositories) != 2 {
		t.Fatalf("merge = %+v err = %v", got.Merge, err)
	}
	a, b := got.Merge.Repositories[0], got.Merge.Repositories[1]
	if a.Name != "alpha" || a.Policy != RepoDenied || b.Name != "beta" || b.Policy != RepoAutomatic || b.AutomaticActive {
		t.Fatalf("repos = %+v %+v", a, b)
	}
	svc.SetPrincipalValidator(fakePrincipals{ok: map[string]bool{"u1": true}})
	got, _ = svc.GetSettings(context.Background(), c.WorkspaceID, c.ID)
	if !got.Merge.Repositories[1].AutomaticActive {
		t.Fatal("valid principal shown inactive")
	}
	svc.merge = false
	got, _ = svc.GetSettings(context.Background(), c.WorkspaceID, c.ID)
	if got.Merge != nil {
		t.Fatal("merge settings shown with the flag off")
	}
}
