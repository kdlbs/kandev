package statussummary

import (
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/events"
)

// @covers AC-TASKS-TASK-UNREAD-DOT-001.1
// @covers AC-TASKS-TASK-UNREAD-DOT-001.2
// @covers AC-TASKS-TASK-UNREAD-DOT-001.9
func TestUnreadSessionsIncludeNeverOpenedSessions(t *testing.T) {
	db := newUnreadFakeDB("session-a", "session-b", "session-c")
	db.add("session-a", "a-1", "message", "done", nil)
	db.setCursor("session-a", "a-1", false)
	db.add("session-b", "b-1", "message", "first", nil)
	db.setCursor("session-b", "b-1", false)
	db.add("session-b", "b-2", "message", "second", nil)
	db.add("session-c", "c-1", "message", "never opened", nil)
	store := newProjectorTestStore()
	projector := newUnreadProjector(store, db)

	touchSession(t, projector, "session-a")
	assertUnreadSessions(t, store, "session-b", "session-c")

	handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, db.setCursor("session-b", "b-2", false))
	assertUnreadSessions(t, store, "session-c")

	handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, db.setCursor("session-c", "c-1", false))
	assertUnreadSessions(t, store)

	handleUnreadEvent(t, projector, events.MessageAdded, db.add("session-a", "a-2", "message", "new", nil))
	assertUnreadSessions(t, store, "session-a")
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.18
func TestUnreadIgnoresUserWrittenMessages(t *testing.T) {
	db := newUnreadFakeDB("session-read", "session-new")
	db.add("session-read", "r-1", "message", "seen", nil)
	db.setCursor("session-read", "r-1", false)
	db.addAuthored("session-new", "n-1", messageTypeUser, "message", "my prompt", nil)
	store := newProjectorTestStore()
	projector := newUnreadProjector(store, db)

	touchSession(t, projector, "session-read")
	assertUnreadSessions(t, store)

	handleUnreadEvent(t, projector, events.MessageAdded,
		db.addAuthored("session-read", "r-2", messageTypeUser, "message", "follow-up", nil))
	assertUnreadSessions(t, store)

	handleUnreadEvent(t, projector, events.MessageAdded, db.add("session-new", "n-2", "message", "agent reply", nil))
	assertUnreadSessions(t, store, "session-new")
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.2
func TestUnreadLoadsSessionCreatedAfterRestore(t *testing.T) {
	db := newUnreadFakeDB("session-a")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	store := newProjectorTestStore()
	projector := newUnreadProjector(store, db)
	touchSession(t, projector, "session-a")
	assertUnreadSessions(t, store)

	db.addSession("session-late")
	handleUnreadEvent(t, projector, events.MessageAdded, db.add("session-late", "l-1", "message", "background output", nil))
	assertUnreadSessions(t, store, "session-late")
	if got := db.lastUnreadRequest(); !reflect.DeepEqual(got, []string{"session-late"}) {
		t.Fatalf("late session load requested %v, want only the new session", got)
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
// @covers AC-TASKS-TASK-UNREAD-DOT-001.7
func TestUnreadConvergesRegardlessOfCursorAndMessageOrder(t *testing.T) {
	for _, cursorFirst := range []bool{true, false} {
		db := newUnreadFakeDB("session-a")
		db.add("session-a", "a-1", "message", "seen", nil)
		db.setCursor("session-a", "a-1", false)
		store := newProjectorTestStore()
		projector := newUnreadProjector(store, db)
		touchSession(t, projector, "session-a")
		assertUnread(t, store, false)

		added := db.add("session-a", "a-2", "message", "new", nil)
		cursor := db.setCursor("session-a", "a-2", false)
		if cursorFirst {
			handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, cursor)
			handleUnreadEvent(t, projector, events.MessageAdded, added)
		} else {
			handleUnreadEvent(t, projector, events.MessageAdded, added)
			assertUnread(t, store, true)
			handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, cursor)
		}
		assertUnread(t, store, false)

		handleUnreadEvent(t, projector, events.MessageAdded, db.add("session-a", "a-3", "message", "newer", nil))
		assertUnread(t, store, true)
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.10
func TestUnreadIgnoresNonQualifyingMessages(t *testing.T) {
	db := newUnreadFakeDB("session-a")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	store := newProjectorTestStore()
	projector := newUnreadProjector(store, db)
	touchSession(t, projector, "session-a")

	handleUnreadEvent(t, projector, events.MessageAdded,
		db.add("session-a", "a-2", "script_execution", "", map[string]interface{}{"script_type": "setup"}))
	handleUnreadEvent(t, projector, events.MessageAdded,
		db.add("session-a", "a-3", "status", "Session resumed", nil))
	handleUnreadEvent(t, projector, events.MessageAdded,
		db.add("session-a", "a-4", "error", "git failed", map[string]interface{}{"actions": []map[string]interface{}{{"type": "ws_request"}}}))
	assertUnread(t, store, false)
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.7
// @covers AC-TASKS-TASK-UNREAD-DOT-001.8
func TestUnreadResetCursorReloadsRanksAndRearms(t *testing.T) {
	db := newUnreadFakeDB("session-a")
	db.add("session-a", "a-1", "message", "one", nil)
	db.add("session-a", "a-2", "message", "two", nil)
	db.setCursor("session-a", "a-2", false)
	store := newProjectorTestStore()
	projector := newUnreadProjector(store, db)
	touchSession(t, projector, "session-a")
	assertUnread(t, store, false)

	// A plain cursor event cannot lower the cursor.
	handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, db.setCursor("session-a", "a-1", false))
	assertUnread(t, store, false)

	handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, db.setCursor("session-a", "a-1", true))
	assertUnread(t, store, true)
	if got := db.lastUnreadRequest(); !reflect.DeepEqual(got, []string{"session-a"}) {
		t.Fatalf("reset reload requested %v, want the affected session", got)
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.8
func TestUnreadFailedObservationRecoversTaskWideOnBackgroundMessage(t *testing.T) {
	db := newUnreadFakeDB("session-a", "session-b")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	db.add("session-b", "b-1", "message", "seen", nil)
	db.setCursor("session-b", "b-1", false)
	db.add("session-b", "b-2", "message", "unread sibling", nil)
	db.setFailUnread(true)
	store := newProjectorTestStore()
	projector := newUnreadProjector(store, db)
	touchSession(t, projector, "session-a")
	assertUnread(t, store, false)

	// Recovery still failing: the baseline stays.
	handleUnreadEvent(t, projector, events.MessageAdded, db.add("session-a", "a-0", "message", "older", nil))
	assertUnread(t, store, false)

	db.setFailUnread(false)
	handleUnreadEvent(t, projector, events.MessageAdded, db.add("session-a", "a-2", "status", "Session resumed", nil))
	assertUnread(t, store, false)
	handleUnreadEvent(t, projector, events.MessageAdded, db.add("session-a", "a-3", "message", "background", nil))
	assertUnread(t, store, true)
	if got := db.lastUnreadRequest(); !reflect.DeepEqual(got, []string{"session-a", "session-b"}) {
		t.Fatalf("recovery requested %v, want every session", got)
	}

	// Once observed, reading session-a leaves the unread sibling in charge.
	handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, db.setCursor("session-a", "a-3", false))
	assertUnread(t, store, true)
	handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, db.setCursor("session-b", "b-2", false))
	assertUnread(t, store, false)
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.8
func TestUnreadFailedObservationRecoversTaskWideOnCursorEvent(t *testing.T) {
	db := newUnreadFakeDB("session-a", "session-b")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.add("session-a", "a-2", "message", "seen too", nil)
	db.setCursor("session-a", "a-1", false)
	db.add("session-b", "b-1", "message", "seen", nil)
	db.setCursor("session-b", "b-1", false)
	db.add("session-b", "b-2", "message", "unread sibling", nil)
	db.setFailUnread(true)
	store := newProjectorTestStore()
	store.rows[unreadTestTaskID] = &StoredTaskStatusSummary{
		TaskID: unreadTestTaskID, WorkspaceID: unreadTestWorkspaceID,
		Summary: TaskStatusSummary{Revision: 1},
	}
	projector := newUnreadProjector(store, db)

	cursor := db.setCursor("session-a", "a-2", false)
	handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, cursor)
	if summary := store.summary(unreadTestTaskID); len(summary.UnreadSessionIDs) > 0 {
		t.Fatal("failed recovery changed the stored baseline")
	}

	db.setFailUnread(false)
	handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, cursor)
	assertUnread(t, store, true)
	if got := db.lastUnreadRequest(); !reflect.DeepEqual(got, []string{"session-a", "session-b"}) {
		t.Fatalf("recovery requested %v, want every session", got)
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.8
func TestUnreadFailedSessionReloadRecoversOnNextQualifyingMessage(t *testing.T) {
	db := newUnreadFakeDB("session-a", "session-b")
	db.add("session-a", "a-1", "message", "first", nil)
	db.setCursor("session-a", "a-1", false)
	db.add("session-b", "b-1", "message", "seen", nil)
	db.setCursor("session-b", "b-1", false)
	store := newProjectorTestStore()
	projector := newUnreadProjector(store, db)
	touchSession(t, projector, "session-a")
	assertUnread(t, store, false)

	db.setFailUnread(true)
	handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, db.setCursor("session-a", "a-1", true))
	assertUnread(t, store, false)

	db.setFailUnread(false)
	handleUnreadEvent(t, projector, events.MessageAdded, db.add("session-a", "a-2", "message", "later", nil))
	assertUnreadSessions(t, store, "session-a")
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
func TestUnreadSessionRemovedNeedsSuccessfulReload(t *testing.T) {
	db := newUnreadFakeDB("session-a", "session-b")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	db.add("session-b", "b-1", "message", "seen", nil)
	db.setCursor("session-b", "b-1", false)
	db.add("session-b", "b-2", "message", "only unread contributor", nil)
	store := newProjectorTestStore()
	projector := newUnreadProjector(store, db)
	touchSession(t, projector, "session-a")
	assertUnread(t, store, true)

	removed := map[string]interface{}{"task_id": unreadTestTaskID, "session_id": "session-b"}
	db.remove("session-b")
	db.setFailUnread(true)
	handleUnreadEvent(t, projector, events.SessionRemoved, removed)
	assertUnread(t, store, true)

	db.setFailUnread(false)
	handleUnreadEvent(t, projector, events.MessageAdded, db.add("session-a", "a-0", "status", "New session started", nil))
	assertUnread(t, store, true)
	handleUnreadEvent(t, projector, events.SessionRemoved, removed)
	assertUnread(t, store, false)
	if got := db.lastUnreadRequest(); !reflect.DeepEqual(got, []string{"session-a"}) {
		t.Fatalf("removal reload requested %v, want the remaining sessions", got)
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
func TestUnreadSessionRemovedClearsOnSuccessfulReload(t *testing.T) {
	db := newUnreadFakeDB("session-a")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	db.add("session-a", "a-2", "message", "unread", nil)
	store := newProjectorTestStore()
	projector := newUnreadProjector(store, db)
	touchSession(t, projector, "session-a")
	assertUnread(t, store, true)

	db.remove("session-a")
	handleUnreadEvent(t, projector, events.SessionRemoved, map[string]interface{}{
		"task_id": unreadTestTaskID, "session_id": "session-a",
	})
	assertUnread(t, store, false)
}

func TestUnreadMessageDeletedKeepsPendingClearWhenReloadFails(t *testing.T) {
	db := newUnreadFakeDB("session-a")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	store := newProjectorTestStore()
	projector := newUnreadProjector(store, db)
	touchSession(t, projector, "session-a")

	question := db.add("session-a", "a-2", "message", "Which branch?", nil)
	question["requests_input"] = true
	handleUnreadEvent(t, projector, events.MessageAdded, question)
	summary := store.summary(unreadTestTaskID)
	if len(summary.UnreadSessionIDs) == 0 || summary.PendingAction != pendingClarification {
		t.Fatalf("summary after question = %+v, want unread clarification", summary)
	}

	db.setFailUnread(true)
	handleUnreadEvent(t, projector, events.MessageDeleted, question)
	summary = store.summary(unreadTestTaskID)
	if summary.PendingAction != "" {
		t.Fatalf("pending action = %q after delete, want cleared", summary.PendingAction)
	}
	if len(summary.UnreadSessionIDs) == 0 {
		t.Fatal("failed reload changed the unread aggregate")
	}
}

func TestUnreadFlipsAreCounted(t *testing.T) {
	db := newUnreadFakeDB("session-a")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	store := newProjectorTestStore()
	projector := newUnreadProjector(store, db)
	touchSession(t, projector, "session-a")
	before := unreadFlipsTotal.Value()
	handleUnreadEvent(t, projector, events.MessageAdded, db.add("session-a", "a-2", "message", "new", nil))
	handleUnreadEvent(t, projector, events.MessageAdded, db.add("session-a", "a-3", "message", "newer", nil))
	handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, db.setCursor("session-a", "a-3", false))
	if delta := unreadFlipsTotal.Value() - before; delta != 2 {
		t.Fatalf("unread flip delta = %d, want 2", delta)
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
func TestUnreadCursorEventAfterRemovalDoesNotRecreateSession(t *testing.T) {
	db := newUnreadFakeDB("session-a", "session-b")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	db.add("session-b", "b-1", "message", "seen", nil)
	sessionBCursor := db.setCursor("session-b", "b-1", false)
	store := newProjectorTestStore()
	projector := newUnreadProjector(store, db)
	touchSession(t, projector, "session-a")
	touchSession(t, projector, "session-b")

	db.remove("session-b")
	handleUnreadEvent(t, projector, events.SessionRemoved, map[string]interface{}{
		"task_id": unreadTestTaskID, "session_id": "session-b",
	})
	// A late cursor event for the removed session must not re-create its observation.
	handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, sessionBCursor)

	projector.mu.Lock()
	_, ghost := projector.state[unreadTestTaskID].sessions["session-b"]
	projector.mu.Unlock()
	if ghost {
		t.Fatal("removed session was re-created by a late cursor event")
	}
	assertUnread(t, store, false)
}
