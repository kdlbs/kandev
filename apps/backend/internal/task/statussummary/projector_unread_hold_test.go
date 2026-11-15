package statussummary

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
)

// manualFlushes captures scheduled unread flushes so a test decides when the
// hold window ends.
type manualFlushes struct {
	mu      sync.Mutex
	pending []func()
}

func (m *manualFlushes) schedule(_ time.Duration, flush func()) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending = append(m.pending, flush)
	return func() {}
}

func (m *manualFlushes) runAll() int {
	m.mu.Lock()
	pending := m.pending
	m.pending = nil
	m.mu.Unlock()
	for _, flush := range pending {
		flush()
	}
	return len(pending)
}

func newHeldUnreadProjector(store SummaryStore, db *unreadFakeDB, flushes *manualFlushes) *Projector {
	return NewProjector(ProjectorConfig{
		Store:                   store,
		LoadSessionObservations: db.snapshot,
		LoadSessionUnreadState:  db.loadUnread,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return unreadTestWorkspaceID, nil
		},
		Logger:              logger.Default(),
		Now:                 func() time.Time { return time.Date(2026, 10, 3, 18, 0, 0, 0, time.UTC) },
		RetryBackoff:        func(context.Context, int) error { return nil },
		UnreadRiseHold:      time.Second,
		ScheduleUnreadFlush: flushes.schedule,
	})
}

func storedRevision(t *testing.T, store *projectorTestStore) uint64 {
	t.Helper()
	summary := store.summary(unreadTestTaskID)
	if summary == nil {
		t.Fatal("no summary stored")
	}
	return summary.Revision
}

func TestUnreadRiseIsPersistedWhenTheHoldEnds(t *testing.T) {
	db := newUnreadFakeDB("session-a")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	store := newProjectorTestStore()
	flushes := &manualFlushes{}
	projector := newHeldUnreadProjector(store, db, flushes)
	touchSession(t, projector, "session-a")
	revision := storedRevision(t, store)

	handleUnreadEvent(t, projector, events.MessageAdded, db.add("session-a", "a-2", "message", "background", nil))
	handleUnreadEvent(t, projector, events.MessageAdded, db.add("session-a", "a-3", "message", "more", nil))
	if got := storedRevision(t, store); got != revision {
		t.Fatalf("held rise wrote revision %d, want %d", got, revision)
	}
	if ran := flushes.runAll(); ran != 1 {
		t.Fatalf("scheduled %d flushes, want one per hold window", ran)
	}
	assertUnreadSessions(t, store, "session-a")
	if got := storedRevision(t, store); got != revision+1 {
		t.Fatalf("flush wrote revision %d, want %d", got, revision+1)
	}
}

func TestUnreadRiseReadDuringTheHoldWritesNothing(t *testing.T) {
	db := newUnreadFakeDB("session-a")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	store := newProjectorTestStore()
	flushes := &manualFlushes{}
	projector := newHeldUnreadProjector(store, db, flushes)
	touchSession(t, projector, "session-a")
	revision := storedRevision(t, store)

	handleUnreadEvent(t, projector, events.MessageAdded, db.add("session-a", "a-2", "tool_call", "", nil))
	handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, db.setCursor("session-a", "a-2", false))
	flushes.runAll()
	assertUnreadSessions(t, store)
	if got := storedRevision(t, store); got != revision {
		t.Fatalf("a rise read within the hold wrote revision %d, want %d", got, revision)
	}
}

func TestUnreadRiseIsNotHeldWhenOtherFieldsChange(t *testing.T) {
	db := newUnreadFakeDB("session-a")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	store := newProjectorTestStore()
	flushes := &manualFlushes{}
	projector := newHeldUnreadProjector(store, db, flushes)
	touchSession(t, projector, "session-a")

	question := db.add("session-a", "a-2", "message", "Which branch?", nil)
	question["requests_input"] = true
	handleUnreadEvent(t, projector, events.MessageAdded, question)
	summary := store.summary(unreadTestTaskID)
	if summary.PendingAction != pendingClarification || len(summary.UnreadSessionIDs) != 1 {
		t.Fatalf("summary = %+v, want the rise written with the pending question", summary)
	}
}

func TestUnreadFallIsNotHeld(t *testing.T) {
	db := newUnreadFakeDB("session-a")
	db.add("session-a", "a-1", "message", "seen", nil)
	db.setCursor("session-a", "a-1", false)
	db.add("session-a", "a-2", "message", "unread before restore", nil)
	store := newProjectorTestStore()
	flushes := &manualFlushes{}
	projector := newHeldUnreadProjector(store, db, flushes)
	touchSession(t, projector, "session-a")
	assertUnreadSessions(t, store, "session-a")

	handleUnreadEvent(t, projector, events.TaskSessionReadCursorChanged, db.setCursor("session-a", "a-2", false))
	assertUnreadSessions(t, store)
}
