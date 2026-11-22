package statussummary

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
)

func storeUnreadBaseline(store *projectorTestStore, unread bool) {
	var sessions []string
	if unread {
		sessions = []string{"session-a"}
	}
	store.rows[unreadTestTaskID] = &StoredTaskStatusSummary{
		TaskID: unreadTestTaskID, WorkspaceID: unreadTestWorkspaceID,
		Summary: TaskStatusSummary{Revision: 1, UnreadSessionIDs: sessions},
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.8
func TestUnreadColdRestoreKeepsStoredAggregateWhenReadFails(t *testing.T) {
	db := newUnreadFakeDB("session-a")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	db.setFailUnread(true)
	store := newProjectorTestStore()
	storeUnreadBaseline(store, true)
	projector := newUnreadProjector(store, db)

	touchSession(t, projector, "session-a")
	assertUnread(t, store, true)
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.8
func TestUnreadColdRestoreDerivesFromStoredMessages(t *testing.T) {
	db := newUnreadFakeDB("session-a")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	db.add("session-a", "a-2", "thinking", "unread before restart", nil)
	store := newProjectorTestStore()
	storeUnreadBaseline(store, false)
	projector := newUnreadProjector(store, db)

	touchSession(t, projector, "session-a")
	assertUnread(t, store, true)
}

func TestUnreadNilLoaderPreservesStoredAggregate(t *testing.T) {
	store := newProjectorTestStore()
	storeUnreadBaseline(store, true)
	projector := NewProjector(ProjectorConfig{
		Store: store,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return unreadTestWorkspaceID, nil
		},
		Now: func() time.Time { return time.Date(2026, 10, 3, 18, 0, 0, 0, time.UTC) },
	})
	touchSession(t, projector, "session-a")
	assertUnread(t, store, true)
}

// rejectOnceStore lets a competing writer win the first compare-and-set.
type rejectOnceStore struct {
	*projectorTestStore
	rejected bool
	winner   func()
}

func (s *rejectOnceStore) CompareAndUpdateTaskStatusSummary(ctx context.Context, stored *StoredTaskStatusSummary) (bool, error) {
	if !s.rejected {
		s.rejected = true
		s.winner()
		return false, nil
	}
	return s.projectorTestStore.CompareAndUpdateTaskStatusSummary(ctx, stored)
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.8
func TestUnreadRebaseKeepsWinnerAggregateWhenReadFails(t *testing.T) {
	db := newUnreadFakeDB("session-a")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	db.add("session-a", "a-2", "message", "unread", nil)
	base := newProjectorTestStore()
	store := &rejectOnceStore{projectorTestStore: base, rejected: true}
	projector := newUnreadProjector(store, db)
	touchSession(t, projector, "session-a")
	assertUnread(t, base, true)

	store.rejected = false
	store.winner = func() {
		winner := base.summary(unreadTestTaskID)
		winner.Revision++
		winner.QueuedPromptCount = 2
		base.rows[unreadTestTaskID] = &StoredTaskStatusSummary{
			TaskID: unreadTestTaskID, WorkspaceID: unreadTestWorkspaceID, Summary: *winner,
		}
	}
	db.setFailUnread(true)
	handleUnreadEvent(t, projector, events.TaskSessionStateChanged, map[string]interface{}{
		"task_id": unreadTestTaskID, "session_id": "session-a", "new_state": sessionStateRunning, "is_primary": true,
	})
	if summary := base.summary(unreadTestTaskID); summary.QueuedPromptCount != 2 || summary.PrimarySession == nil {
		t.Fatalf("rebase lost the winner: %+v", summary)
	}
	assertUnread(t, base, true)
}
