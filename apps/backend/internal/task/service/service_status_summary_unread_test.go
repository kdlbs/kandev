package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/statussummary"
)

const (
	unreadRepairSessionID = "session-unread"
	unreadRepairTurnID    = "turn-unread"
)

type unreadRepairFixture struct {
	svc      *Service
	repo     *sqliterepo.Repository
	eventBus *MockEventBus
	task     *models.Task
	clock    time.Time
}

func newUnreadRepairFixture(t *testing.T) *unreadRepairFixture {
	t.Helper()
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	task, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: unreadRepairSessionID, TaskID: task.ID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	setupTestTurn(t, repo, unreadRepairSessionID, task.ID, unreadRepairTurnID)
	svc.statusSummaries = repo
	return &unreadRepairFixture{svc: svc, repo: repo, eventBus: eventBus, task: task, clock: time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)}
}

func (f *unreadRepairFixture) addMessage(t *testing.T, id string) {
	t.Helper()
	f.clock = f.clock.Add(time.Second)
	if err := f.repo.CreateMessage(context.Background(), &models.Message{
		ID: id, TaskSessionID: unreadRepairSessionID, TaskID: f.task.ID, TurnID: unreadRepairTurnID,
		AuthorType: models.MessageAuthorAgent, Content: "output", CreatedAt: f.clock,
	}); err != nil {
		t.Fatalf("insert message %s: %v", id, err)
	}
}

func (f *unreadRepairFixture) markRead(t *testing.T, messageID string) {
	t.Helper()
	if err := f.repo.UpdateTaskSessionLastReadMessageID(context.Background(), unreadRepairSessionID, messageID); err != nil {
		t.Fatalf("mark read %s: %v", messageID, err)
	}
}

func (f *unreadRepairFixture) storeSummary(t *testing.T, summary statussummary.TaskStatusSummary) *statussummary.TaskStatusSummary {
	t.Helper()
	accepted, err := f.repo.CompareAndUpdateTaskStatusSummary(context.Background(), &statussummary.StoredTaskStatusSummary{
		TaskID: f.task.ID, WorkspaceID: f.task.WorkspaceID, Summary: summary,
	})
	if err != nil || !accepted {
		t.Fatalf("seed summary: accepted=%v err=%v", accepted, err)
	}
	return &summary
}

func (f *unreadRepairFixture) reconcile(t *testing.T, summaries map[string]*statussummary.TaskStatusSummary) *statussummary.TaskStatusSummary {
	t.Helper()
	ctx := context.Background()
	sessions, err := f.repo.ListTaskSessions(ctx, f.task.ID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	got, err := f.svc.ReconcileTaskStatusSummaries(ctx, []*models.Task{f.task},
		map[string][]*models.TaskSession{f.task.ID: sessions}, map[string]models.TaskPendingAction{}, summaries)
	if err != nil {
		t.Fatalf("ReconcileTaskStatusSummaries: %v", err)
	}
	if got[f.task.ID] == nil {
		t.Fatal("reconciled summary is missing")
	}
	return got[f.task.ID]
}

func (f *unreadRepairFixture) persistedUnread(t *testing.T) bool {
	t.Helper()
	rows, err := f.repo.LoadTaskStatusSummaries(context.Background(), []string{f.task.ID})
	if err != nil || rows[f.task.ID] == nil {
		t.Fatalf("load persisted summary: %v (%v)", rows, err)
	}
	return len(rows[f.task.ID].UnreadSessionIDs) > 0
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.8
func TestReconcileRebuildsMissingSummaryAsUnread(t *testing.T) {
	f := newUnreadRepairFixture(t)
	f.addMessage(t, "m-1")
	f.markRead(t, "m-1")
	f.addMessage(t, "m-2")

	if got := f.reconcile(t, nil); len(got.UnreadSessionIDs) == 0 {
		t.Fatalf("rebuilt summary = %+v, want unread", got)
	}
	if !f.persistedUnread(t) {
		t.Fatal("rebuilt summary was not persisted as unread")
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.8
func TestReconcileRepairsExistingUnreadAggregate(t *testing.T) {
	f := newUnreadRepairFixture(t)
	f.addMessage(t, "m-1")
	f.markRead(t, "m-1")
	f.addMessage(t, "m-2")
	stored := f.storeSummary(t, statussummary.TaskStatusSummary{Revision: 3})

	repaired := f.reconcile(t, map[string]*statussummary.TaskStatusSummary{f.task.ID: stored})
	if len(repaired.UnreadSessionIDs) == 0 || repaired.Revision != 4 || !f.persistedUnread(t) {
		t.Fatalf("stale false row repaired to %+v, want unread at revision 4", repaired)
	}

	f.markRead(t, "m-2")
	repaired = f.reconcile(t, map[string]*statussummary.TaskStatusSummary{f.task.ID: repaired})
	if len(repaired.UnreadSessionIDs) > 0 || repaired.Revision != 5 || f.persistedUnread(t) {
		t.Fatalf("stale true row repaired to %+v, want read at revision 5", repaired)
	}
}

// markReadOnCompareStore simulates a projector that advances the cursor and
// commits false after the repair read but before the repair's write.
type markReadOnCompareStore struct {
	repository.TaskStatusSummaryRepository
	fixture  *unreadRepairFixture
	t        *testing.T
	competed bool
}

func (s *markReadOnCompareStore) CompareAndUpdateTaskStatusSummary(
	ctx context.Context,
	stored *statussummary.StoredTaskStatusSummary,
) (bool, error) {
	if !s.competed {
		s.competed = true
		s.fixture.markRead(s.t, "m-2")
		winner := stored.Summary
		winner.UnreadSessionIDs = nil
		winner.QueuedPromptCount = 1
		accepted, err := s.TaskStatusSummaryRepository.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
			TaskID: stored.TaskID, WorkspaceID: stored.WorkspaceID, Summary: winner,
		})
		if err != nil || !accepted {
			s.t.Fatalf("competing write: accepted=%v err=%v", accepted, err)
		}
		return false, nil
	}
	return s.TaskStatusSummaryRepository.CompareAndUpdateTaskStatusSummary(ctx, stored)
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.8
func TestReconcileRejectedRepairKeepsRacingMarkRead(t *testing.T) {
	f := newUnreadRepairFixture(t)
	f.addMessage(t, "m-1")
	f.markRead(t, "m-1")
	f.addMessage(t, "m-2")
	stored := f.storeSummary(t, statussummary.TaskStatusSummary{Revision: 3})
	f.svc.statusSummaries = &markReadOnCompareStore{TaskStatusSummaryRepository: f.repo, fixture: f, t: t}

	repaired := f.reconcile(t, map[string]*statussummary.TaskStatusSummary{f.task.ID: stored})
	if len(repaired.UnreadSessionIDs) > 0 || f.persistedUnread(t) {
		t.Fatalf("repair after a racing mark-read = %+v, want read", repaired)
	}
}

type countingUnreadSessions struct {
	repository.SessionRepository
	reader repository.SessionUnreadStateRepository
	calls  int
}

func (r *countingUnreadSessions) ListSessionUnreadState(
	ctx context.Context,
	sessionIDs []string,
) (map[string]statussummary.SessionUnreadState, error) {
	r.calls++
	return r.reader.ListSessionUnreadState(ctx, sessionIDs)
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.2
func TestReconcileRebuildsNeverOpenedSessionAsUnread(t *testing.T) {
	f := newUnreadRepairFixture(t)
	f.addMessage(t, "m-1")
	counting := &countingUnreadSessions{SessionRepository: f.repo, reader: f.repo}
	f.svc.sessions = counting

	got := f.reconcile(t, nil)
	if len(got.UnreadSessionIDs) != 1 || got.UnreadSessionIDs[0] != unreadRepairSessionID {
		t.Fatalf("never-opened task rebuilt as %+v, want its session unread", got)
	}
	if counting.calls != 1 {
		t.Fatalf("unread reads = %d, want one batch read", counting.calls)
	}
	f.markRead(t, "m-1")
	repaired := f.reconcile(t, map[string]*statussummary.TaskStatusSummary{f.task.ID: got})
	if len(repaired.UnreadSessionIDs) > 0 {
		t.Fatalf("read task repaired to %+v, want no unread sessions", repaired)
	}
}

func (f *unreadRepairFixture) cursorEvents() []map[string]interface{} {
	f.eventBus.mu.Lock()
	defer f.eventBus.mu.Unlock()
	var out []map[string]interface{}
	for _, event := range f.eventBus.publishedEvents {
		if event.Type == events.TaskSessionReadCursorChanged {
			out = append(out, event.Data.(map[string]interface{}))
		}
	}
	return out
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
func TestMarkSessionReadPublishesPersistedCursor(t *testing.T) {
	f := newUnreadRepairFixture(t)
	f.addMessage(t, "m-1")
	f.addMessage(t, "m-2")
	ctx := context.Background()
	if _, err := f.svc.MarkSessionRead(ctx, unreadRepairSessionID, "m-2"); err != nil {
		t.Fatalf("mark m-2 read: %v", err)
	}
	// A stale request republishes the persisted (newer) cursor, never its own.
	if _, err := f.svc.MarkSessionRead(ctx, unreadRepairSessionID, "m-1"); err != nil {
		t.Fatalf("stale mark-read: %v", err)
	}
	published := f.cursorEvents()
	if len(published) != 2 {
		t.Fatalf("cursor events = %d, want 2", len(published))
	}
	want := f.clock.Format(time.RFC3339Nano)
	for _, data := range published {
		if data["task_id"] != f.task.ID || data["session_id"] != unreadRepairSessionID ||
			data["last_read_message_id"] != "m-2" || data["last_read_created_at"] != want || data["reset"] != false {
			t.Fatalf("cursor event = %+v, want persisted m-2 at %s", data, want)
		}
	}
}
