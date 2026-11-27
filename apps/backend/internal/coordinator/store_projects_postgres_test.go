package coordinator

import (
	"context"
	"testing"
)

func TestProjectScope_Postgres_WriteAndReadBackThroughTheToggle(t *testing.T) {
	store := newMultiConnStorePostgres(t)
	c := newTestCoordinator(t, store, "ws-1")
	ctx := context.Background()
	for _, noRepo := range []bool{true, false} {
		next := projectState{scope: watchScopeSelected, entries: []ProjectEntry{{Kind: projectKindRepository, ID: "repo-a"}, {Kind: projectKindSet, ID: "set-1"}}, includeNoRepo: noRepo}
		err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
			return store.writeProjectState(ctx, tx, c.WorkspaceID, c.ID, next)
		})
		if err != nil {
			t.Fatalf("write selected scope (toggle %t): %v", noRepo, err)
		}
		got, err := store.loadProjectState(ctx, store.ro, c.ID)
		if err != nil {
			t.Fatalf("read back (toggle %t): %v", noRepo, err)
		}
		if got.scope != watchScopeSelected || got.includeNoRepo != noRepo || len(got.entries) != 2 {
			t.Fatalf("read back %+v, want selected with toggle %t and 2 entries", got, noRepo)
		}
		set, err := store.LoadWatchSet(ctx, store.ro, c.ID)
		if err != nil {
			t.Fatalf("load watch set: %v", err)
		}
		if !set.Projects.Selected || set.Projects.IncludeNoRepo != noRepo {
			t.Fatalf("watch set projects = %+v, want selected with toggle %t", set.Projects, noRepo)
		}
	}
}

func TestProjectScope_Postgres_DeletedEntryIsRemoved(t *testing.T) {
	store := newMultiConnStorePostgres(t)
	c := newTestCoordinator(t, store, "ws-1")
	ctx := context.Background()
	err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
		return store.writeProjectState(ctx, tx, c.WorkspaceID, c.ID, projectState{scope: watchScopeSelected, entries: []ProjectEntry{{Kind: projectKindSet, ID: "set-1"}, {Kind: projectKindRepository, ID: "repo-a"}}})
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{store: store}
	if err := svc.ProjectDeleted(ctx, projectKindSet, "set-1"); err != nil {
		t.Fatalf("project deleted: %v", err)
	}
	got, err := store.loadProjectState(ctx, store.ro, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.entries) != 1 || got.entries[0].ID != "repo-a" {
		t.Fatalf("entries after delete = %+v, want only repo-a", got.entries)
	}
}
