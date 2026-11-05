package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
	"github.com/kandev/kandev/internal/testutil"
)

type unreadSeed struct {
	repo   *Repository
	db     *sqlx.DB
	taskID string
	clock  time.Time
}

func newUnreadSeed(t *testing.T, repo *Repository, db *sqlx.DB, taskID string, sessionIDs ...string) *unreadSeed {
	t.Helper()
	seedTaskForStatusSummary(t, db, taskID, "workspace-"+taskID)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, sessionID := range sessionIDs {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
		}); err != nil {
			t.Fatalf("create session %s: %v", sessionID, err)
		}
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`), "turn-"+sessionID, sessionID, taskID, now, now, now); err != nil {
			t.Fatalf("seed turn for %s: %v", sessionID, err)
		}
	}
	// Nanosecond precision exercises the microsecond normalization.
	return &unreadSeed{repo: repo, db: db, taskID: taskID, clock: time.Date(2026, 10, 3, 9, 0, 0, 123456789, time.UTC)}
}

func (s *unreadSeed) add(t *testing.T, sessionID, id, messageType, content string, metadata map[string]interface{}) time.Time {
	t.Helper()
	return s.addAuthored(t, sessionID, id, "agent", messageType, content, metadata)
}

func (s *unreadSeed) addAuthored(t *testing.T, sessionID, id, authorType, messageType, content string, metadata map[string]interface{}) time.Time {
	t.Helper()
	s.clock = s.clock.Add(time.Second)
	metadataJSON := []byte("{}")
	if metadata != nil {
		var err error
		if metadataJSON, err = json.Marshal(metadata); err != nil {
			t.Fatalf("encode metadata: %v", err)
		}
	}
	if _, err := s.db.Exec(s.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, '', ?, 0, ?, ?, ?)
	`), id, sessionID, s.taskID, "turn-"+sessionID, authorType, content, messageType, string(metadataJSON), s.clock); err != nil {
		t.Fatalf("insert message %s: %v", id, err)
	}
	return s.clock
}

func (s *unreadSeed) markRead(t *testing.T, sessionID, messageID string) {
	t.Helper()
	if err := s.repo.UpdateTaskSessionLastReadMessageID(context.Background(), sessionID, messageID); err != nil {
		t.Fatalf("mark %s read at %s: %v", sessionID, messageID, err)
	}
}

func listUnread(t *testing.T, repo *Repository, sessionIDs ...string) map[string]statussummary.SessionUnreadState {
	t.Helper()
	states, err := repo.ListSessionUnreadState(context.Background(), sessionIDs)
	if err != nil {
		t.Fatalf("ListSessionUnreadState: %v", err)
	}
	return states
}

func runSessionUnreadStateContract(t *testing.T, repo *Repository, db *sqlx.DB) {
	seed := newUnreadSeed(t, repo, db, "task-unread-contract",
		"s-caught-up", "s-unread", "s-cursorless", "s-own-prompt", "s-paged", "s-deleted-cursor")

	readAt := seed.add(t, "s-caught-up", "cu-1", "message", "seen", nil)
	seed.markRead(t, "s-caught-up", "cu-1")
	seed.addAuthored(t, "s-caught-up", "cu-2", "user", "message", "my follow-up", nil)

	seed.add(t, "s-unread", "u-1", "message", "seen", nil)
	seed.markRead(t, "s-unread", "u-1")
	qualifyingAt := seed.add(t, "s-unread", "u-2", "tool_call", "", nil)
	seed.add(t, "s-unread", "u-3", "script_execution", "", map[string]interface{}{"script_type": "setup"})
	seed.add(t, "s-unread", "u-4", "status", "Session resumed", nil)
	seed.add(t, "s-unread", "u-5", "error", "git failed", map[string]interface{}{"actions": []interface{}{map[string]interface{}{"type": "ws_request"}}})

	cursorlessAt := seed.add(t, "s-cursorless", "c-1", "message", "never opened", nil)
	seed.addAuthored(t, "s-cursorless", "c-2", "user", "message", "my later prompt", nil)

	seed.addAuthored(t, "s-own-prompt", "o-1", "user", "message", "only my prompt", nil)

	seed.add(t, "s-paged", "p-0", "message", "seen", nil)
	seed.markRead(t, "s-paged", "p-0")
	pagedAt := seed.add(t, "s-paged", "p-1", "message", "below a page of hidden rows", nil)
	for i := range unreadWalkPageSize + 3 {
		seed.add(t, "s-paged", fmt.Sprintf("p-hidden-%03d", i), "permission_request", "", nil)
	}

	seed.add(t, "s-deleted-cursor", "d-1", "message", "read then deleted", nil)
	seed.markRead(t, "s-deleted-cursor", "d-1")
	deletedNewestAt := seed.add(t, "s-deleted-cursor", "d-2", "message", "after", nil)
	if _, err := db.Exec(db.Rebind(`DELETE FROM task_session_messages WHERE id = ?`), "d-1"); err != nil {
		t.Fatalf("delete cursor message: %v", err)
	}

	states := listUnread(t, repo, "s-caught-up", "s-unread", "s-cursorless", "s-own-prompt", "s-paged", "s-deleted-cursor", "s-missing")
	if _, ok := states["s-missing"]; ok {
		t.Fatal("a missing session must be absent")
	}
	if got := states["s-cursorless"]; got.Read != (statussummary.UnreadRank{}) ||
		got.Newest != statussummary.NewUnreadRank("c-1", cursorlessAt) || !got.Unread() {
		t.Fatalf("cursorless state = %+v, want unread from a zero cursor to c-1", got)
	}
	if got, ok := states["s-own-prompt"]; !ok || got.Unread() {
		t.Fatalf("own-prompt state = %+v (present %v), want present and read", got, ok)
	}
	caughtUp := states["s-caught-up"]
	if want := statussummary.NewUnreadRank("cu-1", readAt); caughtUp.Read != want || caughtUp.Newest != want || caughtUp.Unread() {
		t.Fatalf("caught-up state = %+v, want read and newest %+v", caughtUp, want)
	}
	if got := states["s-unread"]; got.Newest != statussummary.NewUnreadRank("u-2", qualifyingAt) || !got.Unread() {
		t.Fatalf("unread state = %+v, want newest qualifying u-2", got)
	}
	if got := states["s-paged"]; got.Newest != statussummary.NewUnreadRank("p-1", pagedAt) || !got.Unread() {
		t.Fatalf("paged state = %+v, want newest qualifying p-1", got)
	}
	deleted := states["s-deleted-cursor"]
	if deleted.Read.ID != "d-1" || !deleted.Read.CreatedAt.IsZero() ||
		deleted.Newest != statussummary.NewUnreadRank("d-2", deletedNewestAt) || !deleted.Unread() {
		t.Fatalf("deleted-cursor state = %+v, want unbounded walk to d-2", deleted)
	}

	seed.markRead(t, "s-unread", "u-5")
	if got := listUnread(t, repo, "s-unread")["s-unread"]; got.Unread() {
		t.Fatalf("state after reading the tail = %+v, want read", got)
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.2
// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
// @covers AC-TASKS-TASK-UNREAD-DOT-001.10
func TestListSessionUnreadState(t *testing.T) {
	repo, db := newTaskStatusSummaryTestRepo(t)
	runSessionUnreadStateContract(t, repo, db)
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.2
// @covers AC-TASKS-TASK-UNREAD-DOT-001.18
func TestListSessionUnreadStateFindsAgentRowBelowManyUserRows(t *testing.T) {
	repo, db := newTaskStatusSummaryTestRepo(t)
	seed := newUnreadSeed(t, repo, db, "task-unread-buried", "s-buried", "s-only-user")
	// An untyped agent row is a message; more than two pages of user rows
	// and hidden types follow it in a never-opened session.
	buriedAt := seed.addAuthored(t, "s-buried", "b-agent", "agent", "", "reply", nil)
	for i := range 3 * unreadWalkPageSize {
		seed.addAuthored(t, "s-buried", fmt.Sprintf("b-user-%03d", i), "user", "message", "prompt", nil)
		seed.add(t, "s-only-user", fmt.Sprintf("u-perm-%03d", i), "permission_request", "", nil)
	}
	states := listUnread(t, repo, "s-buried", "s-only-user")
	if got := states["s-buried"]; got.Newest != statussummary.NewUnreadRank("b-agent", buriedAt) || !got.Unread() {
		t.Fatalf("buried state = %+v, want unread at the untyped agent row", got)
	}
	if got, ok := states["s-only-user"]; !ok || got.Unread() {
		t.Fatalf("hidden-only state = %+v (present %v), want present and read", got, ok)
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.10
func TestListSessionUnreadStateOrdersSameMicrosecondByID(t *testing.T) {
	repo, db := newTaskStatusSummaryTestRepo(t)
	seed := newUnreadSeed(t, repo, db, "task-unread-micro", "s-micro")
	insert := func(id string, at time.Time) {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO task_session_messages
				(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
			VALUES (?, 's-micro', 'task-unread-micro', 'turn-s-micro', 'agent', '', 'seen', 0, 'message', '{}', ?)
		`), id, at); err != nil {
			t.Fatalf("insert message %s: %v", id, err)
		}
	}
	// Both timestamps fall in the same microsecond; the id decides the order.
	base := time.Date(2026, 10, 3, 9, 0, 0, 123456100, time.UTC)
	insert("z-cur", base)
	seed.markRead(t, "s-micro", "z-cur")
	insert("a-same", base.Add(400*time.Nanosecond))
	got := listUnread(t, repo, "s-micro")["s-micro"]
	if got.Read.ID != "z-cur" || got.Newest != got.Read || got.Unread() {
		t.Fatalf("same-microsecond state = %+v, want read cursor z-cur with no later qualifying row", got)
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.10
func TestMarkReadRejectsSameMicrosecondRegressionByID(t *testing.T) {
	repo, db := newTaskStatusSummaryTestRepo(t)
	seed := newUnreadSeed(t, repo, db, "task-unread-micro-cursor", "s-micro-cursor")
	insert := func(id string, at time.Time) {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO task_session_messages
				(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
			VALUES (?, 's-micro-cursor', 'task-unread-micro-cursor', 'turn-s-micro-cursor', 'agent', '', 'seen', 0, 'message', '{}', ?)
		`), id, at); err != nil {
			t.Fatalf("insert message %s: %v", id, err)
		}
	}
	// Same microsecond. The id orders them: "a-same" < "z-cur", so z-cur is newest.
	// Raw nanoseconds order them the other way: a-same is later by 300ns.
	base := time.Date(2026, 10, 3, 9, 0, 0, 123456100, time.UTC)
	insert("z-cur", base)
	insert("a-same", base.Add(400*time.Nanosecond))
	seed.markRead(t, "s-micro-cursor", "z-cur")
	// Stale out-of-order mark-read for the older (by id) message must be rejected.
	seed.markRead(t, "s-micro-cursor", "a-same")
	got := listUnread(t, repo, "s-micro-cursor")["s-micro-cursor"]
	if got.Read.ID != "z-cur" || got.Unread() {
		t.Fatalf("cursor after stale same-microsecond mark = %+v, want z-cur read", got)
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.10
func TestListSessionUnreadStateAcrossTailChunks(t *testing.T) {
	repo, db := newTaskStatusSummaryTestRepo(t)
	// More live cursors than one tail chunk, so the UNION ALL spans chunks.
	count := 2*(sqliteMaxHostParams/unreadTailParams) + 3
	ids := make([]string, 0, count)
	for i := range count {
		ids = append(ids, fmt.Sprintf("s-chunk-%03d", i))
	}
	seed := newUnreadSeed(t, repo, db, "task-unread-chunk", ids...)
	wantRead := make(map[string]statussummary.UnreadRank, count)
	wantNewest := make(map[string]statussummary.UnreadRank, count)
	for i, sessionID := range ids {
		readID := sessionID + "-read"
		readAt := seed.add(t, sessionID, readID, "message", "seen", nil)
		seed.markRead(t, sessionID, readID)
		wantRead[sessionID] = statussummary.NewUnreadRank(readID, readAt)
		wantNewest[sessionID] = wantRead[sessionID]
		if i%3 == 0 {
			continue // caught up: nothing after the cursor
		}
		if i%2 == 0 {
			seed.add(t, sessionID, sessionID+"-hidden", "permission_request", "", nil)
		}
		qualID := sessionID + "-qual"
		qualAt := seed.add(t, sessionID, qualID, "tool_call", "", nil)
		wantNewest[sessionID] = statussummary.NewUnreadRank(qualID, qualAt)
	}
	states := listUnread(t, repo, ids...)
	if len(states) != count {
		t.Fatalf("got %d session states, want %d", len(states), count)
	}
	for _, sessionID := range ids {
		got := states[sessionID]
		if got.Read != wantRead[sessionID] || got.Newest != wantNewest[sessionID] {
			t.Fatalf("session %s state = %+v, want read %+v newest %+v",
				sessionID, got, wantRead[sessionID], wantNewest[sessionID])
		}
	}
}

func TestPostgresListSessionUnreadState(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	runSessionUnreadStateContract(t, repo, db)
}

// runLiveUnreadOrdering drives a real projector with the repository loader:
// a live MessageAdded (nanosecond payload) followed by the persisted cursor
// event must end read.
func runLiveUnreadOrdering(t *testing.T, repo *Repository, db *sqlx.DB) {
	const taskID = "task-unread-live"
	seed := newUnreadSeed(t, repo, db, taskID, "s-live")
	seed.add(t, "s-live", "l-1", "message", "seen", nil)
	seed.markRead(t, "s-live", "l-1")
	eventBus := bus.NewMemoryEventBus(logger.Default())
	t.Cleanup(func() { eventBus.Close() })
	projector := statussummary.NewProjector(statussummary.ProjectorConfig{
		Store:    repo,
		EventBus: eventBus,
		LoadSessionObservations: func(ctx context.Context, _ string) (statussummary.SessionObservationSnapshot, error) {
			snapshot := statussummary.SessionObservationSnapshot{UnreadObserved: true}
			states, err := repo.ListSessionUnreadState(ctx, []string{"s-live"})
			if err != nil {
				return snapshot, err
			}
			state := states["s-live"]
			snapshot.Sessions = []statussummary.RebuildSession{{ID: "s-live", State: "WAITING_FOR_INPUT", Unread: &state}}
			return snapshot, nil
		},
		LoadSessionUnreadState: repo.ListSessionUnreadState,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-" + taskID, nil
		},
	})
	handle := func(eventType string, data map[string]interface{}) {
		t.Helper()
		data["task_id"] = taskID
		data["session_id"] = "s-live"
		if err := projector.HandleEvent(context.Background(), bus.NewEvent(eventType, "test", data)); err != nil {
			t.Fatalf("handle %s: %v", eventType, err)
		}
	}
	unread := func() bool {
		t.Helper()
		rows, err := repo.LoadTaskStatusSummaries(context.Background(), []string{taskID})
		if err != nil || rows[taskID] == nil {
			t.Fatalf("load summary: %v (%v)", rows, err)
		}
		return len(rows[taskID].UnreadSessionIDs) > 0
	}

	createdAt := seed.add(t, "s-live", "l-2", "message", "live", nil)
	handle(events.MessageAdded, map[string]interface{}{
		"message_id": "l-2", "type": "message", "author_type": "agent",
		"created_at": createdAt.Format(time.RFC3339Nano),
	})
	if !unread() {
		t.Fatal("live message did not make the task unread")
	}
	seed.markRead(t, "s-live", "l-2")
	stored, err := repo.GetMessage(context.Background(), "l-2")
	if err != nil {
		t.Fatalf("read cursor message: %v", err)
	}
	handle(events.TaskSessionReadCursorChanged, map[string]interface{}{
		"last_read_message_id": "l-2",
		"last_read_created_at": stored.CreatedAt.Format(time.RFC3339Nano),
	})
	if unread() {
		t.Fatal("reading the live message left the task unread")
	}
	// A late replay of the same message must not re-arm the task.
	handle(events.MessageAdded, map[string]interface{}{
		"message_id": "l-2", "type": "message", "author_type": "agent",
		"created_at": createdAt.Format(time.RFC3339Nano),
	})
	if unread() {
		t.Fatal("replayed message re-armed a read task")
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
func TestTaskStatusSummaryUnreadLiveOrdering(t *testing.T) {
	repo, db := newTaskStatusSummaryTestRepo(t)
	runLiveUnreadOrdering(t, repo, db)
}

func TestPostgresTaskStatusSummaryUnreadLiveOrdering(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	runLiveUnreadOrdering(t, repo, db)
}
