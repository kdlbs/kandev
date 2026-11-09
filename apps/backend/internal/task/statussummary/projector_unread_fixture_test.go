package statussummary

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

const (
	unreadTestTaskID      = "task-unread"
	unreadTestWorkspaceID = "workspace-unread"
)

var errUnreadReadFailed = errors.New("unread read failed")

type unreadFakeMessage struct {
	id          string
	authorType  string
	messageType string
	content     string
	metadata    map[string]interface{}
	createdAt   time.Time
}

// unreadFakeDB models the durable inputs: the task's sessions, their read
// cursors, and their messages. Its unread answers use the same predicate and
// rank order as the repository.
type unreadFakeDB struct {
	mu         sync.Mutex
	sessions   []string
	cursors    map[string]string
	messages   map[string][]unreadFakeMessage
	failUnread bool
	unreadIDs  [][]string
	clock      time.Time
}

func newUnreadFakeDB(sessions ...string) *unreadFakeDB {
	return &unreadFakeDB{
		sessions: sessions,
		cursors:  make(map[string]string),
		messages: make(map[string][]unreadFakeMessage),
		clock:    time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC),
	}
}

func (db *unreadFakeDB) setFailUnread(fail bool) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.failUnread = fail
}

// add stores an agent message and returns its MessageAdded payload. The payload
// keeps nanosecond precision, as newMessageEvent does.
func (db *unreadFakeDB) add(sessionID, id, messageType, content string, metadata map[string]interface{}) map[string]interface{} {
	return db.addAuthored(sessionID, id, messageTypeAgent, messageType, content, metadata)
}

func (db *unreadFakeDB) addAuthored(sessionID, id, authorType, messageType, content string, metadata map[string]interface{}) map[string]interface{} {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.clock = db.clock.Add(time.Second + 789*time.Nanosecond)
	message := unreadFakeMessage{id: id, authorType: authorType, messageType: messageType, content: content, metadata: metadata, createdAt: db.clock}
	db.messages[sessionID] = append(db.messages[sessionID], message)
	data := map[string]interface{}{
		"task_id":     unreadTestTaskID,
		"session_id":  sessionID,
		"message_id":  id,
		"type":        messageType,
		"content":     content,
		"author_type": authorType,
		"created_at":  message.createdAt.Format(time.RFC3339Nano),
	}
	if metadata != nil {
		data["metadata"] = metadata
	}
	return data
}

func (db *unreadFakeDB) addSession(sessionID string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.sessions = append(db.sessions, sessionID)
}

// setCursor writes the cursor and returns the cursor event payload.
func (db *unreadFakeDB) setCursor(sessionID, messageID string, reset bool) map[string]interface{} {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.cursors[sessionID] = messageID
	created := db.messageLocked(sessionID, messageID).createdAt
	return map[string]interface{}{
		"task_id":              unreadTestTaskID,
		"session_id":           sessionID,
		"last_read_message_id": messageID,
		"last_read_created_at": created.Format(time.RFC3339Nano),
		"reset":                reset,
	}
}

func (db *unreadFakeDB) remove(sessionID string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	kept := db.sessions[:0]
	for _, id := range db.sessions {
		if id != sessionID {
			kept = append(kept, id)
		}
	}
	db.sessions = kept
	delete(db.cursors, sessionID)
	delete(db.messages, sessionID)
}

func (db *unreadFakeDB) messageLocked(sessionID, messageID string) unreadFakeMessage {
	for _, message := range db.messages[sessionID] {
		if message.id == messageID {
			return message
		}
	}
	return unreadFakeMessage{}
}

// stateLocked mirrors the repository: a cursorless session reads from a zero
// rank, so any qualifying message makes it unread.
func (db *unreadFakeDB) stateLocked(sessionID string) SessionUnreadState {
	var read UnreadRank
	if cursorID, ok := db.cursors[sessionID]; ok {
		read = NewUnreadRank(cursorID, db.messageLocked(sessionID, cursorID).createdAt)
	}
	newest := read
	for _, message := range db.messages[sessionID] {
		rank := NewUnreadRank(message.id, message.createdAt)
		if rank.After(read) && rank.After(newest) &&
			IsUnreadQualifyingMessage(message.authorType, message.messageType, message.content, message.metadata) {
			newest = rank
		}
	}
	return SessionUnreadState{Read: read, Newest: newest}
}

func (db *unreadFakeDB) loadUnread(_ context.Context, sessionIDs []string) (map[string]SessionUnreadState, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	requested := append([]string(nil), sessionIDs...)
	sort.Strings(requested)
	db.unreadIDs = append(db.unreadIDs, requested)
	if db.failUnread {
		return nil, errUnreadReadFailed
	}
	result := make(map[string]SessionUnreadState)
	for _, sessionID := range sessionIDs {
		if db.hasSessionLocked(sessionID) {
			result[sessionID] = db.stateLocked(sessionID)
		}
	}
	return result, nil
}

func (db *unreadFakeDB) hasSessionLocked(sessionID string) bool {
	for _, id := range db.sessions {
		if id == sessionID {
			return true
		}
	}
	return false
}

func (db *unreadFakeDB) snapshot(context.Context, string) (SessionObservationSnapshot, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	snapshot := SessionObservationSnapshot{UnreadObserved: !db.failUnread}
	for _, sessionID := range db.sessions {
		session := RebuildSession{ID: sessionID, State: sessionStateWaitingForInput}
		if !db.failUnread {
			state := db.stateLocked(sessionID)
			session.Unread = &state
		}
		snapshot.Sessions = append(snapshot.Sessions, session)
	}
	return snapshot, nil
}

func (db *unreadFakeDB) lastUnreadRequest() []string {
	db.mu.Lock()
	defer db.mu.Unlock()
	if len(db.unreadIDs) == 0 {
		return nil
	}
	return db.unreadIDs[len(db.unreadIDs)-1]
}

func newUnreadProjector(store SummaryStore, db *unreadFakeDB) *Projector {
	return NewProjector(ProjectorConfig{
		Store:                   store,
		LoadSessionObservations: db.snapshot,
		LoadSessionUnreadState:  db.loadUnread,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return unreadTestWorkspaceID, nil
		},
		Logger:       logger.Default(),
		Now:          func() time.Time { return time.Date(2026, 10, 3, 18, 0, 0, 0, time.UTC) },
		RetryBackoff: func(context.Context, int) error { return nil },
	})
}

func handleUnreadEvent(t *testing.T, projector *Projector, eventType string, data map[string]interface{}) {
	t.Helper()
	if err := projector.HandleEvent(context.Background(), bus.NewEvent(eventType, "test", data)); err != nil {
		t.Fatalf("handle %s: %v", eventType, err)
	}
}

// touchSession publishes a session-state event, which always persists, so a
// cold projector restores and writes its derived summary.
func touchSession(t *testing.T, projector *Projector, sessionID string) {
	t.Helper()
	handleUnreadEvent(t, projector, events.TaskSessionStateChanged, map[string]interface{}{
		"task_id":    unreadTestTaskID,
		"session_id": sessionID,
		"new_state":  sessionStateWaitingForInput,
	})
}

func assertUnread(t *testing.T, store *projectorTestStore, want bool) {
	t.Helper()
	summary := store.summary(unreadTestTaskID)
	if summary == nil {
		t.Fatalf("no summary stored, want unread=%v", want)
	}
	if got := len(summary.UnreadSessionIDs) > 0; got != want {
		t.Fatalf("unread_session_ids = %v, want unread=%v", summary.UnreadSessionIDs, want)
	}
}

func assertUnreadSessions(t *testing.T, store *projectorTestStore, want ...string) {
	t.Helper()
	summary := store.summary(unreadTestTaskID)
	if summary == nil {
		t.Fatalf("no summary stored, want unread sessions %v", want)
	}
	if len(want) == 0 {
		want = nil
	}
	if !reflect.DeepEqual(summary.UnreadSessionIDs, want) {
		t.Fatalf("unread_session_ids = %v, want %v", summary.UnreadSessionIDs, want)
	}
}
