package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCreateCoordinator_Roundtrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := &Coordinator{
		WorkspaceID:       "ws-1",
		Name:              "Ops",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "executor-1",
		Context:           "standing context",
	}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	if c.ID == "" {
		t.Fatal("CreateCoordinator did not assign an id")
	}
	if c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() {
		t.Fatal("CreateCoordinator did not stamp timestamps")
	}

	got, err := store.GetCoordinator(ctx, "ws-1", c.ID)
	if err != nil {
		t.Fatalf("GetCoordinator: %v", err)
	}
	if got.Name != "Ops" || got.AgentProfileID != "agent-1" || got.ExecutorProfileID != "executor-1" || got.Context != "standing context" {
		t.Fatalf("GetCoordinator returned %+v", got)
	}
	if got.ConversationTaskID != nil {
		t.Fatalf("ConversationTaskID = %v, want nil", got.ConversationTaskID)
	}
}

func TestGetCoordinator_WrongWorkspaceIsNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}

	if _, err := store.GetCoordinator(ctx, "ws-2", c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetCoordinator across workspaces: err = %v, want ErrNotFound", err)
	}
}

func TestGetCoordinator_UnknownIDIsNotFound(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.GetCoordinator(context.Background(), "ws-1", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetCoordinator(missing): err = %v, want ErrNotFound", err)
	}
}

func TestCoordinatorForConversationTask_MatchAndNoMatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	taskID := "task-1"
	c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e", ConversationTaskID: &taskID}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}

	gotID, ok, err := store.CoordinatorForConversationTask(ctx, taskID)
	if err != nil {
		t.Fatalf("CoordinatorForConversationTask: %v", err)
	}
	if !ok || gotID != c.ID {
		t.Fatalf("CoordinatorForConversationTask(%q) = (%q, %v), want (%q, true)", taskID, gotID, ok, c.ID)
	}

	if _, ok, err := store.CoordinatorForConversationTask(ctx, "no-such-task"); err != nil || ok {
		t.Fatalf("CoordinatorForConversationTask(no-such-task) = (_, %v, %v), want (_, false, nil)", ok, err)
	}
}

func TestGetCoordinatorByID_NotFoundHasNoWorkspaceScope(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}

	got, err := store.GetCoordinatorByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetCoordinatorByID: %v", err)
	}
	if got.Name != "Ops" {
		t.Fatalf("GetCoordinatorByID returned %+v", got)
	}

	if _, err := store.GetCoordinatorByID(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetCoordinatorByID(missing): err = %v, want ErrNotFound", err)
	}
}

func TestListCoordinators_OrderedByCreatedAtThenID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	var ids []string
	for i := 0; i < 3; i++ {
		c := &Coordinator{WorkspaceID: "ws-1", Name: "c", AgentProfileID: "a", ExecutorProfileID: "e"}
		if err := store.CreateCoordinator(ctx, c); err != nil {
			t.Fatalf("CreateCoordinator: %v", err)
		}
		ids = append(ids, c.ID)
	}
	// A coordinator in a different workspace must not appear.
	other := &Coordinator{WorkspaceID: "ws-2", Name: "other", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := store.CreateCoordinator(ctx, other); err != nil {
		t.Fatalf("CreateCoordinator other: %v", err)
	}

	list, err := store.ListCoordinators(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListCoordinators: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("ListCoordinators returned %d rows, want 3", len(list))
	}
	for i, c := range list {
		if c.ID != ids[i] {
			t.Fatalf("ListCoordinators[%d].ID = %q, want %q (order mismatch)", i, c.ID, ids[i])
		}
	}
}

func TestListCoordinators_EmptyIsEmptySliceNotNil(t *testing.T) {
	store := newTestStore(t)
	list, err := store.ListCoordinators(context.Background(), "ws-none")
	if err != nil {
		t.Fatalf("ListCoordinators: %v", err)
	}
	if list == nil {
		t.Fatal("ListCoordinators returned nil, want empty slice")
	}
	if len(list) != 0 {
		t.Fatalf("ListCoordinators returned %d rows, want 0", len(list))
	}
}

func TestDeleteCoordinator_RemovesRowAndProposals(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	// Insert a proposal row directly: the proposal store methods land in a
	// later TDD unit, but the delete contract (coordinators.md#routes) must
	// already remove any proposal row scoped to this coordinator.
	now := time.Now().UTC()
	if _, err := store.db.ExecContext(ctx, store.db.Rebind(`
		INSERT INTO coordinator_proposals (id, coordinator_id, workspace_id, status, spec_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`),
		"proposal-1", c.ID, "ws-1", "pending", "{}", now, now); err != nil {
		t.Fatalf("seed proposal: %v", err)
	}

	if err := store.DeleteCoordinator(ctx, "ws-1", c.ID); err != nil {
		t.Fatalf("DeleteCoordinator: %v", err)
	}

	if _, err := store.GetCoordinator(ctx, "ws-1", c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetCoordinator after delete: err = %v, want ErrNotFound", err)
	}
	var remaining int
	if err := store.ro.GetContext(ctx, &remaining, store.ro.Rebind(
		`SELECT COUNT(*) FROM coordinator_proposals WHERE coordinator_id = ?`), c.ID); err != nil {
		t.Fatalf("count proposals after delete: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("coordinator_proposals rows after delete = %d, want 0", remaining)
	}
}

// TestSetConversationTaskID_CAS proves the compare-and-swap only replaces
// conversation_task_id when the column currently holds exactly staleTaskID
// (store.go's own doc comment), including the empty-vs-NULL case, and never
// matches a NULL column against a non-empty staleTaskID it did not read
// (docs/specs/coordinator/system-design/copilot.md#conversation-task step 4).
func TestSetConversationTaskID_CAS(t *testing.T) {
	ctx := context.Background()

	t.Run("empty staleTaskID matches a NULL column", func(t *testing.T) {
		store := newTestStore(t)
		c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
		if err := store.CreateCoordinator(ctx, c); err != nil {
			t.Fatalf("CreateCoordinator: %v", err)
		}

		ok, err := store.SetConversationTaskID(ctx, c.ID, "task-new", "")
		if err != nil {
			t.Fatalf("SetConversationTaskID: %v", err)
		}
		if !ok {
			t.Fatal("SetConversationTaskID(staleTaskID=\"\") against a NULL column = false, want true")
		}
	})

	t.Run("matching non-empty staleTaskID wins", func(t *testing.T) {
		store := newTestStore(t)
		taskID := "task-current"
		c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e", ConversationTaskID: &taskID}
		if err := store.CreateCoordinator(ctx, c); err != nil {
			t.Fatalf("CreateCoordinator: %v", err)
		}

		ok, err := store.SetConversationTaskID(ctx, c.ID, "task-new", taskID)
		if err != nil {
			t.Fatalf("SetConversationTaskID: %v", err)
		}
		if !ok {
			t.Fatal("SetConversationTaskID with a matching non-empty staleTaskID = false, want true")
		}
	})

	t.Run("non-empty staleTaskID never matches a concurrently cleared NULL column", func(t *testing.T) {
		store := newTestStore(t)
		taskID := "task-current"
		c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e", ConversationTaskID: &taskID}
		if err := store.CreateCoordinator(ctx, c); err != nil {
			t.Fatalf("CreateCoordinator: %v", err)
		}
		// Simulate a concurrent PatchCoordinator context/profile-change clear
		// landing between the caller's stale read (staleTaskID="task-current")
		// and this CAS call.
		if _, err := store.db.ExecContext(ctx, store.db.Rebind(
			`UPDATE coordinators SET conversation_task_id = NULL WHERE id = ?`), c.ID); err != nil {
			t.Fatalf("simulate concurrent clear: %v", err)
		}

		ok, err := store.SetConversationTaskID(ctx, c.ID, "task-new", taskID)
		if err != nil {
			t.Fatalf("SetConversationTaskID: %v", err)
		}
		if ok {
			t.Fatal("SetConversationTaskID with a non-empty staleTaskID against a concurrently cleared NULL column = true, want false")
		}

		got, err := store.GetCoordinator(ctx, "ws-1", c.ID)
		if err != nil {
			t.Fatalf("GetCoordinator: %v", err)
		}
		if got.ConversationTaskID != nil {
			t.Fatalf("ConversationTaskID after refused CAS = %v, want nil (still cleared, not overwritten)", *got.ConversationTaskID)
		}
	})

	t.Run("non-empty staleTaskID never matches a mismatched non-empty column", func(t *testing.T) {
		store := newTestStore(t)
		taskID := "task-current"
		c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e", ConversationTaskID: &taskID}
		if err := store.CreateCoordinator(ctx, c); err != nil {
			t.Fatalf("CreateCoordinator: %v", err)
		}

		ok, err := store.SetConversationTaskID(ctx, c.ID, "task-new", "task-other")
		if err != nil {
			t.Fatalf("SetConversationTaskID: %v", err)
		}
		if ok {
			t.Fatal("SetConversationTaskID with a mismatched non-empty staleTaskID = true, want false")
		}
	})
}

func TestDeleteCoordinator_UnknownOrRepeatedIsNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.DeleteCoordinator(ctx, "ws-1", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteCoordinator(missing): err = %v, want ErrNotFound", err)
	}

	c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	if err := store.DeleteCoordinator(ctx, "ws-1", c.ID); err != nil {
		t.Fatalf("first DeleteCoordinator: %v", err)
	}
	if err := store.DeleteCoordinator(ctx, "ws-1", c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("repeated DeleteCoordinator: err = %v, want ErrNotFound", err)
	}
}
