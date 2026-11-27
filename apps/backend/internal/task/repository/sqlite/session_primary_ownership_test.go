package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDelayedSessionUpdatePreservesExplicitPrimarySelection(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := t.Context()
	insertTask(t, repo.db, "primary-owner")
	for _, id := range []string{"older", "newer"} {
		insertSession(t, repo, id, "primary-owner", string(models.TaskSessionStateWaitingForInput))
	}
	if err := repo.SetSessionPrimary(ctx, "newer"); err != nil {
		t.Fatal(err)
	}
	stale, err := repo.GetTaskSession(ctx, "newer")
	if err != nil || !stale.IsPrimary {
		t.Fatalf("initial primary not persisted: %+v, %v", stale, err)
	}
	if err := repo.SetSessionPrimary(ctx, "older"); err != nil {
		t.Fatal(err)
	}
	stale.ErrorMessage = "delayed lifecycle observation"
	if err := repo.UpdateTaskSession(ctx, stale); err != nil {
		t.Fatal(err)
	}
	sessions, err := repo.ListTaskSessions(ctx, "primary-owner")
	if err != nil {
		t.Fatal(err)
	}
	var primary []string
	for _, session := range sessions {
		if session.IsPrimary {
			primary = append(primary, session.ID)
		}
		if session.ID == "newer" && session.ErrorMessage != stale.ErrorMessage {
			t.Fatal("ordinary lifecycle fields were not persisted")
		}
	}
	if len(primary) != 1 || primary[0] != "older" || stale.IsPrimary {
		t.Fatalf("delayed update reclaimed primary: persisted=%v returned=%v", primary, stale.IsPrimary)
	}
}

func TestSnapshotRollbackCannotReleaseAChangedPrimarySelection(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := t.Context()
	insertTask(t, repo.db, "guarded-primary-owner")
	for _, id := range []string{"captured", "selected"} {
		insertSession(t, repo, id, "guarded-primary-owner", string(models.TaskSessionStateWaitingForInput))
	}
	if err := repo.SetSessionPrimary(ctx, "captured"); err != nil {
		t.Fatal(err)
	}
	stale, err := repo.GetTaskSession(ctx, "captured")
	if err != nil {
		t.Fatal(err)
	}
	revision := stale.UpdatedAt
	stale.IsPrimary = false
	if err := repo.SetSessionPrimary(ctx, "selected"); err != nil {
		t.Fatal(err)
	}
	changed, err := repo.UpdateTaskSessionIfCurrentSnapshot(ctx, stale, stale.State, revision, stale.Metadata)
	if err != nil || changed {
		t.Fatalf("changed primary selection did not fence snapshot rollback: %v %v", changed, err)
	}
	selected, err := repo.GetTaskSession(ctx, "selected")
	if err != nil || !selected.IsPrimary {
		t.Fatalf("selected primary changed: %+v %v", selected, err)
	}
	ownedRevision := selected.UpdatedAt
	selected.IsPrimary = false
	changed, err = repo.UpdateTaskSessionIfCurrentSnapshot(ctx, selected, selected.State, ownedRevision, selected.Metadata)
	if err != nil || !changed || selected.IsPrimary {
		t.Fatalf("owned snapshot could not release its flag: %v %v primary=%v", changed, err, selected.IsPrimary)
	}
	sessions, err := repo.ListTaskSessions(ctx, "guarded-primary-owner")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sessions {
		if s.IsPrimary {
			t.Fatal("owned release left another primary")
		}
	}
	if err := repo.SetSessionPrimary(ctx, "captured"); err != nil {
		t.Fatal(err)
	}
	promoted, err := repo.GetTaskSession(ctx, "captured")
	if err != nil || !promoted.IsPrimary {
		t.Fatalf("explicit primary promotion failed after release: %+v %v", promoted, err)
	}
}
