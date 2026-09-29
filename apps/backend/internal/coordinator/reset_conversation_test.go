package coordinator

import (
	"context"
	"testing"
	"time"
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
