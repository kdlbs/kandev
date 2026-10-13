package coordinator

import "testing"

func runWakeOwnershipWorkspaceBoundary(t *testing.T, store *Store) {
	t.Helper()
	f := newWakeFixture(t, store)
	f.addOwn(t, "same-workspace", "wf-a")
	f.addOwn(t, "moved-workspace", "wf-a")
	mustExec(t, store, `UPDATE tasks SET workspace_id = 'ws-other' WHERE id = 'moved-workspace'`)
	if ids := ownTaskIDs(t, store, f.c.ID); !sameStrings(ids, []string{"same-workspace"}) {
		t.Fatalf("coordinator's own-task read crosses workspace: %v", ids)
	}
	owners, err := store.OwnersOfTask(t.Context(), "moved-workspace")
	if err != nil || len(owners) != 0 {
		t.Fatalf("moved task has old workspace owner: %+v, err %v", owners, err)
	}
	got, err := store.RecordWake(t.Context(), f.c.ID, "moved-workspace", WakeKindQuestion, "question-1")
	if err != nil || got.Outcome != WakeNotOwn || wakeRows(t, store, f.c.ID) != 0 {
		t.Fatalf("foreign-workspace wake accepted: %+v, err %v", got, err)
	}
	positive, err := store.RecordWake(t.Context(), f.c.ID, "same-workspace", WakeKindQuestion, "question-2")
	if err != nil || positive.Outcome != WakeInserted || wakeRows(t, store, f.c.ID) != 1 {
		t.Fatalf("same-workspace positive control refused: %+v, err %v", positive, err)
	}
}

func TestWakeOwnershipStaysInCoordinatorWorkspaceSQLite(t *testing.T) {
	runWakeOwnershipWorkspaceBoundary(t, newTestStore(t))
}

func TestWakeOwnershipStaysInCoordinatorWorkspacePostgres(t *testing.T) {
	runWakeOwnershipWorkspaceBoundary(t, newTestStorePostgres(t))
}
