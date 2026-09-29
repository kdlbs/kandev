package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestResetConversation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	now := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	var got string
	err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
		var err error
		got, err = store.resetConversation(ctx, tx, c.ID)
		return err
	})
	if err != nil || got != "" {
		t.Fatalf("no conversation: %q, %v", got, err)
	}

	if _, err := store.db.ExecContext(ctx, `UPDATE coordinators SET conversation_task_id = 'task-1' WHERE id = ?`, c.ID); err != nil {
		t.Fatal(err)
	}
	err = store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
		var err error
		got, err = store.resetConversation(ctx, tx, c.ID)
		return err
	})
	if err != nil || got != "task-1" {
		t.Fatalf("reset returned %q, %v", got, err)
	}
	after, err := store.GetCoordinatorByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ConversationTaskID != nil || !after.UpdatedAt.Equal(now) {
		t.Fatalf("after reset = %+v", after)
	}
}

func TestArchiveConversation_SkipsEmptyID(t *testing.T) {
	store := newTestStore(t)
	svc := newPhase2Service(t, store, true)
	// A nil conversationTasks would panic if an empty id reached the archive call.
	svc.archiveConversation(context.Background(), "c", "")
}

func setConversation(t *testing.T, store *Store, id, taskID string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), store.db.Rebind(`UPDATE coordinators SET conversation_task_id = ? WHERE id = ?`), taskID, id); err != nil {
		t.Fatal(err)
	}
}

func TestResetConversation_AlwaysIncrementsConfigRevision(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	svc := newPhase2Service(t, store, true)
	for want := int64(1); want <= 2; want++ {
		// The second pass has no conversation task and still increments.
		if want == 1 {
			setConversation(t, store, c.ID, "task-1")
		}
		err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
			_, err := svc.resetConversation(ctx, tx, c.ID)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		got, err := store.GetCoordinatorByID(ctx, c.ID)
		if err != nil || got.ConfigRevision != want || got.ConversationTaskID != nil {
			t.Fatalf("after reset %d: %+v err=%v", want, got, err)
		}
	}
}

func TestResetConversation_RollbackKeepsRevisionAndTask(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	setConversation(t, store, c.ID, "task-1")
	sentinel := errors.New("later step failed")
	err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
		if _, err := store.resetConversation(ctx, tx, c.ID); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
	got, _ := store.GetCoordinatorByID(ctx, c.ID)
	if got.ConfigRevision != 0 || got.ConversationTaskID == nil || *got.ConversationTaskID != "task-1" {
		t.Fatalf("rolled-back reset leaked: %+v", got)
	}
}

func TestResetConversation_MissingCoordinator(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.resetConversation(ctx, store.db, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestResetConversation_ArchivesOnceAfterCommit(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()
	c := deps.coordinator
	deps.tasks.tasks["task-1"] = &taskmodels.Task{ID: "task-1"}
	setConversation(t, deps.svc.store, c.ID, "task-1")
	var prev string
	err := deps.svc.store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
		var err error
		prev, err = deps.svc.resetConversation(ctx, tx, c.ID)
		return err
	})
	if err != nil || prev != "task-1" {
		t.Fatalf("prev = %q err=%v", prev, err)
	}
	if len(deps.tasks.archivedIDs) != 0 {
		t.Fatal("archive must not run inside the lock")
	}
	deps.svc.archiveConversation(ctx, c.ID, prev)
	deps.svc.archiveConversation(ctx, c.ID, prev)
	if len(deps.tasks.archivedIDs) != 1 {
		t.Fatalf("archived = %v, want exactly once", deps.tasks.archivedIDs)
	}
}

func TestPatchParityWithReset(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	fixture := func() (*Store, *Coordinator) {
		store := newTestStore(t)
		store.now = func() time.Time { return now }
		c := newTestCoordinator(t, store, "ws-1")
		setConversation(t, store, c.ID, "task-1")
		return store, c
	}
	pStore, pc := fixture()
	newContext := "changed"
	patched, _, err := pStore.PatchCoordinator(ctx, "ws-1", pc.ID, CoordinatorPatch{Context: &newContext}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rStore, rc := fixture()
	if err := rStore.withCoordinatorLock(ctx, rc.ID, func(tx coordinatorExec) error {
		_, err := rStore.resetConversation(ctx, tx, rc.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	reset, _ := rStore.GetCoordinatorByID(ctx, rc.ID)
	if patched.ConversationTaskID != nil || reset.ConversationTaskID != nil ||
		patched.ConfigRevision != 1 || reset.ConfigRevision != 1 ||
		!patched.UpdatedAt.Equal(now) || !reset.UpdatedAt.Equal(now) {
		t.Fatalf("patch %+v vs reset %+v", patched, reset)
	}
}

func TestPatch_NameOnlyDoesNotIncrementConfigRevision(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	name := "Renamed"
	got, _, err := store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Name: &name}, nil)
	if err != nil || got.ConfigRevision != 0 {
		t.Fatalf("name-only patch: %+v err=%v", got, err)
	}
}
