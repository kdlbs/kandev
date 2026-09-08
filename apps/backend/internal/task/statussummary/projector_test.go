package statussummary

//revive:disable:file-length-limit // Projector regression coverage is intentionally scenario-heavy.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

type projectorTestStore struct {
	mu   sync.Mutex
	rows map[string]*StoredTaskStatusSummary
}

func newProjectorTestStore() *projectorTestStore {
	return &projectorTestStore{rows: make(map[string]*StoredTaskStatusSummary)}
}

func (s *projectorTestStore) LoadTaskStatusSummaries(_ context.Context, taskIDs []string) (map[string]*TaskStatusSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := make(map[string]*TaskStatusSummary, len(taskIDs))
	for _, taskID := range taskIDs {
		if row := s.rows[taskID]; row != nil {
			rows[taskID] = cloneSummary(&row.Summary)
		}
	}
	return rows, nil
}

func (s *projectorTestStore) CompareAndUpdateTaskStatusSummary(_ context.Context, stored *StoredTaskStatusSummary) (bool, error) {
	if stored == nil {
		return false, fmt.Errorf("nil summary")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous := s.rows[stored.TaskID]; previous != nil && previous.Summary.Revision >= stored.Summary.Revision {
		return false, nil
	}
	copy := *stored
	copy.Summary = *cloneSummary(&stored.Summary)
	s.rows[stored.TaskID] = &copy
	return true, nil
}

func (s *projectorTestStore) summary(taskID string) *TaskStatusSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.rows[taskID]
	if row == nil {
		return nil
	}
	return cloneSummary(&row.Summary)
}

func newProjectorTest(t *testing.T) (*Projector, *projectorTestStore, *bus.MemoryEventBus, *atomic.Int64, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	store := newProjectorTestStore()
	eventBus := bus.NewMemoryEventBus(logger.Default())
	updates := new(atomic.Int64)
	if _, err := eventBus.Subscribe(events.TaskStatusSummaryUpdated, func(_ context.Context, event *bus.Event) error {
		updates.Add(1)
		if _, ok := event.Data.(SummaryUpdated); !ok {
			t.Errorf("summary event data type = %T, want SummaryUpdated", event.Data)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	projector := NewProjector(ProjectorConfig{
		Store:    store,
		EventBus: eventBus,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 1, 18, 0, 0, 0, time.UTC) },
	})
	if err := projector.Start(ctx); err != nil {
		cancel()
		eventBus.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		projector.Close()
		eventBus.Close()
	})
	return projector, store, eventBus, updates, cancel
}

func publishProjectorEvent(t *testing.T, eventBus *bus.MemoryEventBus, eventType, subject string, data map[string]interface{}) {
	t.Helper()
	if err := eventBus.Publish(context.Background(), subject, bus.NewEvent(eventType, "test", data)); err != nil {
		t.Fatal(err)
	}
}

func publishSessionState(t *testing.T, eventBus *bus.MemoryEventBus, taskID, sessionID string, extra map[string]interface{}) {
	t.Helper()
	data := map[string]interface{}{
		"task_id":               taskID,
		"session_id":            sessionID,
		"workspace_id":          "workspace-1",
		"new_state":             "RUNNING",
		"is_primary":            true,
		"foreground_activity":   "generating",
		"active_subagent_count": 2,
	}
	for key, value := range extra {
		data[key] = value
	}
	publishProjectorEvent(t, eventBus, events.TaskSessionStateChanged, events.TaskSessionStateChanged, data)
}

func TestProjectorDoesNotSubscribeToRawStreamsOrStreamingMessageAppends(t *testing.T) {
	_, store, eventBus, updates, _ := newProjectorTest(t)
	ctx := context.Background()
	const taskID = "task-stream-boundary"

	publishProjectorEvent(t, eventBus, events.AgentStream, events.BuildAgentStreamSubject("session-1"), map[string]interface{}{
		"task_id":    taskID,
		"session_id": "session-1",
		"delta":      "a very large stream frame",
	})
	publishProjectorEvent(t, eventBus, events.MessageUpdated, events.MessageUpdated, map[string]interface{}{
		"task_id":     taskID,
		"session_id":  "session-1",
		"author_type": "agent",
		"type":        "text",
		"content":     "partial assistant output",
	})
	if got := store.summary(taskID); got != nil {
		t.Fatalf("unrelated streaming events created summary: %+v", got)
	}
	if updates.Load() != 0 {
		t.Fatalf("summary updates after unrelated streaming events = %d, want 0", updates.Load())
	}

	publishSessionState(t, eventBus, taskID, "session-1", nil)
	got := store.summary(taskID)
	if got == nil || got.PrimarySession == nil {
		t.Fatalf("summary after authoritative session state = %+v", got)
	}
	if got.PrimarySession.ID != "session-1" || got.PrimarySession.State != "RUNNING" {
		t.Fatalf("primary session = %+v", got.PrimarySession)
	}
	if got.ForegroundActivity != "generating" || got.ActiveSubagentCount != 2 {
		t.Fatalf("activity summary = %+v", got)
	}
	if updates.Load() != 1 {
		t.Fatalf("summary updates after authoritative state = %d, want 1", updates.Load())
	}

	// Replayed lifecycle state and assistant append updates are both no-ops at
	// the semantic projection boundary.
	publishSessionState(t, eventBus, taskID, "session-1", nil)
	publishProjectorEvent(t, eventBus, events.MessageUpdated, events.MessageUpdated, map[string]interface{}{
		"task_id":     taskID,
		"session_id":  "session-1",
		"author_type": "agent",
		"type":        "text",
		"content":     "more partial output",
	})
	if updates.Load() != 1 {
		t.Fatalf("summary updates after replay/append = %d, want 1", updates.Load())
	}
	_ = ctx
}

func TestProjectorRepairsStoredRunningSummaryAfterStartupRecovery(t *testing.T) {
	store := newProjectorTestStore()
	storedAt := time.Date(2026, 8, 1, 17, 59, 0, 0, time.UTC)
	store.rows["task-startup-recovery"] = &StoredTaskStatusSummary{
		TaskID:      "task-startup-recovery",
		WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{
			Revision:            7,
			UpdatedAt:           storedAt,
			PrimarySession:      &PrimarySessionSummary{ID: "session-startup-recovery", State: "RUNNING"},
			ForegroundActivity:  "generating",
			ActiveSubagentCount: 2,
		},
	}

	projector := NewProjector(ProjectorConfig{
		Store: store,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		Now: func() time.Time { return storedAt.Add(time.Minute) },
	})

	if err := projector.HandleEvent(context.Background(), bus.NewEvent(events.TaskSessionStateChanged, "test", map[string]interface{}{
		"task_id":               "task-startup-recovery",
		"workspace_id":          "workspace-1",
		"session_id":            "session-startup-recovery",
		"new_state":             "WAITING_FOR_INPUT",
		"is_primary":            true,
		"foreground_activity":   nil,
		"active_subagent_count": 0,
	})); err != nil {
		t.Fatalf("apply startup recovery event: %v", err)
	}

	got := store.summary("task-startup-recovery")
	if got == nil || got.PrimarySession == nil {
		t.Fatalf("repaired summary = %+v, want primary session", got)
	}
	if got.Revision <= 7 {
		t.Fatalf("repaired summary revision = %d, want newer than 7", got.Revision)
	}
	if got.PrimarySession.State != "WAITING_FOR_INPUT" {
		t.Errorf("primary session state = %q, want WAITING_FOR_INPUT", got.PrimarySession.State)
	}
	if got.ForegroundActivity != "" {
		t.Errorf("foreground activity = %q, want empty", got.ForegroundActivity)
	}
	if got.ActiveSubagentCount != 0 {
		t.Errorf("active subagent count = %d, want 0", got.ActiveSubagentCount)
	}
}

func TestProjectorDerivesBoundedStatusAcrossSources(t *testing.T) {
	_, store, eventBus, updates, _ := newProjectorTest(t)
	const taskID = "task-status-sources"
	const sessionID = "session-status"

	publishSessionState(t, eventBus, taskID, sessionID, nil)
	occurredAt := "2026-08-01T18:01:00.000000000Z"
	longError := strings.Repeat("ошибка ", 200)
	publishProjectorEvent(t, eventBus, events.TaskSessionErrorChanged, events.TaskSessionErrorChanged, map[string]interface{}{
		"task_id":     taskID,
		"session_id":  sessionID,
		"active":      true,
		"message":     longError,
		"occurred_at": occurredAt,
		"stamp":       "error-stamp-1",
	})
	publishProjectorEvent(t, eventBus, events.MessageAdded, events.MessageAdded, map[string]interface{}{
		"task_id":        taskID,
		"session_id":     sessionID,
		"author_type":    "user",
		"type":           "clarification_request",
		"requests_input": true,
	})
	publishProjectorEvent(t, eventBus, events.GitEvent, events.BuildGitEventSubject(sessionID), map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"type":       "status_update",
		"status": map[string]interface{}{
			"repository_name":  "repo-a",
			"branch_additions": 4,
			"branch_deletions": 1,
			"modified":         []interface{}{"a.go"},
			"added":            []interface{}{"b.go"},
			"ahead":            2,
			"behind":           1,
		},
	})
	publishProjectorEvent(t, eventBus, events.GitEvent, events.BuildGitEventSubject(sessionID), map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"type":       "status_update",
		"status": map[string]interface{}{
			"repository_name":  "repo-b",
			"changed_files":    3,
			"branch_additions": 6,
			"branch_deletions": 2,
			"ahead":            1,
			"behind":           4,
		},
	})
	publishProjectorEvent(t, eventBus, events.GitHubTaskPRUpdated, events.GitHubTaskPRUpdated, map[string]interface{}{
		"task_id":              taskID,
		"workspace_id":         "workspace-1",
		"repository_id":        "repo-a",
		"state":                "open",
		"pr_number":            42,
		"pr_url":               "https://example.test/pr/42",
		"review_state":         "changes_requested",
		"checks_state":         "success",
		"required_reviews":     1,
		"pending_review_count": 0,
	})

	got := store.summary(taskID)
	if got == nil {
		t.Fatal("missing projected summary")
	}
	if got.ActiveError == nil || got.ActiveError.Preview == "" {
		t.Fatalf("active error = %+v", got.ActiveError)
	}
	if len(got.ActiveError.Preview) > MaxActiveErrorPreviewBytes || !utf8.ValidString(got.ActiveError.Preview) {
		t.Fatalf("error preview is not bounded valid UTF-8: bytes=%d", len(got.ActiveError.Preview))
	}
	if got.PendingAction != "clarification" {
		t.Fatalf("pending action = %q, want clarification", got.PendingAction)
	}
	if got.Git == nil || got.Git.Additions != 10 || got.Git.Deletions != 3 || got.Git.ChangedFiles != 5 || got.Git.Ahead != 3 || got.Git.Behind != 5 {
		t.Fatalf("git summary = %+v", got.Git)
	}
	if got.PullRequest == nil || got.PullRequest.Count != 1 || !got.PullRequest.Attention || got.PullRequest.Number != 42 || got.PullRequest.AggregateState != "failure" {
		t.Fatalf("pull request summary = %+v", got.PullRequest)
	}

	publishProjectorEvent(t, eventBus, events.ClarificationAnswered, events.ClarificationAnswered, map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
	})
	publishProjectorEvent(t, eventBus, events.MessageAdded, events.MessageAdded, map[string]interface{}{
		"task_id":     taskID,
		"session_id":  sessionID,
		"author_type": "agent",
		"type":        "text",
		"content":     "recovered",
	})
	got = store.summary(taskID)
	if got.PendingAction != "" || got.ActiveError != nil {
		t.Fatalf("cleared status = %+v", got)
	}
	if updates.Load() < 7 {
		t.Fatalf("summary updates = %d, want one for each semantic source change", updates.Load())
	}
}

func TestProjectorLastActivityTracksBoundedSourcesMonotonically(t *testing.T) {
	_, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-last-activity-projector"
	const sessionID = "session-last-activity-projector"

	publishProjectorEvent(t, eventBus, events.TaskCreated, events.TaskCreated, map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": "workspace-1",
		"created_at":   "2026-08-01T18:01:00Z",
		"updated_at":   "2026-08-01T18:01:00Z",
	})
	assertProjectedLastActivity(t, store.summary(taskID), "2026-08-01T18:01:00Z")

	// Focus and subscription changes are operational state, not task activity.
	publishProjectorEvent(t, eventBus, events.TaskSessionActivityChanged, events.TaskSessionActivityChanged, map[string]interface{}{
		"task_id":             taskID,
		"workspace_id":        "workspace-1",
		"session_id":          sessionID,
		"foreground_activity": "background",
		"updated_at":          "2026-08-01T18:02:00Z",
	})
	assertProjectedLastActivity(t, store.summary(taskID), "2026-08-01T18:01:00Z")

	// Provider polling can refresh a PR summary without making the task active.
	publishProjectorEvent(t, eventBus, events.GitHubTaskPRUpdated, events.GitHubTaskPRUpdated, map[string]interface{}{
		"task_id":       taskID,
		"workspace_id":  "workspace-1",
		"repository_id": "repo-last-activity",
		"state":         "open",
		"pr_number":     12,
		"pr_url":        "https://example.test/pr/12",
		"updated_at":    "2026-08-01T18:03:00Z",
	})
	assertProjectedLastActivity(t, store.summary(taskID), "2026-08-01T18:01:00Z")

	// Agent-authored output and streamed/message-update bookkeeping do not move
	// activity. The user-authored message below is the first message source that
	// should move it.
	publishProjectorEvent(t, eventBus, events.MessageAdded, events.MessageAdded, map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": "workspace-1",
		"session_id":   sessionID,
		"author_type":  "agent",
		"type":         "text",
		"created_at":   "2026-08-01T18:04:00Z",
	})
	publishProjectorEvent(t, eventBus, events.MessageUpdated, events.MessageUpdated, map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": "workspace-1",
		"session_id":   sessionID,
		"author_type":  "agent",
		"type":         "text",
		"updated_at":   "2026-08-01T18:04:30Z",
	})
	assertProjectedLastActivity(t, store.summary(taskID), "2026-08-01T18:01:00Z")

	publishProjectorEvent(t, eventBus, events.MessageAdded, events.MessageAdded, map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": "workspace-1",
		"session_id":   sessionID,
		"author_type":  "user",
		"type":         "text",
		"created_at":   "2026-08-01T18:05:00Z",
	})
	assertProjectedLastActivity(t, store.summary(taskID), "2026-08-01T18:05:00Z")

	publishProjectorEvent(t, eventBus, events.TurnStarted, events.TurnStarted, map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": "workspace-1",
		"session_id":   sessionID,
		"started_at":   "2026-08-01T18:06:00Z",
	})
	assertProjectedLastActivity(t, store.summary(taskID), "2026-08-01T18:06:00Z")

	publishProjectorEvent(t, eventBus, events.TurnCompleted, events.TurnCompleted, map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": "workspace-1",
		"session_id":   sessionID,
		"completed_at": "2026-08-01T18:07:00Z",
	})
	assertProjectedLastActivity(t, store.summary(taskID), "2026-08-01T18:07:00Z")

	// State-only transitions are activity even when they carry no other
	// bounded-summary field.
	publishProjectorEvent(t, eventBus, events.TaskStateChanged, events.TaskStateChanged, map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": "workspace-1",
		"new_state":    "in_progress",
		"updated_at":   "2026-08-01T18:08:00Z",
	})
	assertProjectedLastActivity(t, store.summary(taskID), "2026-08-01T18:08:00Z")

	// Queue bookkeeping is deliberately ignored, and an older persisted update
	// cannot move the monotonic maximum backwards.
	publishProjectorEvent(t, eventBus, events.MessageQueueStatusChanged, events.MessageQueueStatusChanged, map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": "workspace-1",
		"updated_at":   "2026-08-01T18:09:00Z",
	})
	publishProjectorEvent(t, eventBus, events.TaskUpdated, events.TaskUpdated, map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": "workspace-1",
		"updated_at":   "2026-08-01T18:07:30Z",
	})
	assertProjectedLastActivity(t, store.summary(taskID), "2026-08-01T18:08:00Z")

	publishProjectorEvent(t, eventBus, events.TaskUpdated, events.TaskUpdated, map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": "workspace-1",
		"updated_at":   "2026-08-01T18:10:00Z",
	})
	assertProjectedLastActivity(t, store.summary(taskID), "2026-08-01T18:10:00Z")
}

// @covers AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.10
func TestProjectorLastActivityIgnoresLifecycleTurns(t *testing.T) {
	_, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-last-activity-lifecycle"
	const sessionID = "session-last-activity-lifecycle"

	publishProjectorEvent(t, eventBus, events.TaskCreated, events.TaskCreated, map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": "workspace-1",
		"created_at":   "2026-08-01T18:00:00Z",
		"updated_at":   "2026-08-01T18:00:00Z",
	})

	publishProjectorEvent(t, eventBus, events.TurnStarted, events.TurnStarted, map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"started_at": "2026-08-01T18:01:00Z",
		"metadata":   map[string]interface{}{models.TurnMetaKeyLifecycleOnly: true},
	})
	publishProjectorEvent(t, eventBus, events.TurnCompleted, events.TurnCompleted, map[string]interface{}{
		"task_id":      taskID,
		"session_id":   sessionID,
		"completed_at": "2026-08-01T18:02:00Z",
		"metadata":     map[string]interface{}{models.TurnMetaKeyLifecycleOnly: true},
	})
	assertProjectedLastActivity(t, store.summary(taskID), "2026-08-01T18:00:00Z")

	publishProjectorEvent(t, eventBus, events.TurnStarted, events.TurnStarted, map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"started_at": "2026-08-01T18:03:00Z",
	})
	assertProjectedLastActivity(t, store.summary(taskID), "2026-08-01T18:03:00Z")

	publishProjectorEvent(t, eventBus, events.TurnCompleted, events.TurnCompleted, map[string]interface{}{
		"task_id":      taskID,
		"session_id":   sessionID,
		"completed_at": "2026-08-01T18:04:00Z",
	})
	assertProjectedLastActivity(t, store.summary(taskID), "2026-08-01T18:04:00Z")
}

func TestProjectorLastActivityRehydratesFromDurableLoader(t *testing.T) {
	const taskID = "task-last-activity-rehydrate"
	want := time.Date(2026, 8, 1, 18, 11, 0, 0, time.UTC)
	store := newProjectorTestStore()
	store.rows[taskID] = &StoredTaskStatusSummary{
		TaskID:      taskID,
		WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{
			Revision:  4,
			UpdatedAt: want.Add(-time.Minute),
		},
	}
	var calls atomic.Int64
	projector := NewProjector(ProjectorConfig{
		Store: store,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		LoadTaskActivity: func(context.Context, string) (*time.Time, error) {
			calls.Add(1)
			return &want, nil
		},
	})

	state, err := projector.ensureState(context.Background(), taskID)
	if err != nil {
		t.Fatalf("restore projector state: %v", err)
	}
	if state.lastActivityAt == nil || !state.lastActivityAt.Equal(want) {
		t.Fatalf("rehydrated last activity = %v, want %v", state.lastActivityAt, want)
	}
	if calls.Load() != 1 {
		t.Fatalf("durable activity loader calls = %d, want 1", calls.Load())
	}
}

func assertProjectedLastActivity(t *testing.T, summary *TaskStatusSummary, want string) {
	t.Helper()
	if summary == nil || summary.LastActivityAt == nil {
		t.Fatalf("projected last activity = %+v, want %s", summary, want)
	}
	expected, err := time.Parse(time.RFC3339, want)
	if err != nil {
		t.Fatalf("parse expected activity %q: %v", want, err)
	}
	if !summary.LastActivityAt.Equal(expected) {
		t.Fatalf("projected last activity = %s, want %s", summary.LastActivityAt.Format(time.RFC3339), want)
	}
}

// Session events carry a `session_metadata` snapshot taken when the publisher
// read the session, so one taken before the error was cleared must not re-arm an
// error this projection already cleared.
func TestProjectorDoesNotResurrectClearedErrorFromSessionMetadata(t *testing.T) {
	_, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-error-replay"
	const sessionID = "session-error-replay"

	occurredAt := "2026-08-01T18:01:00.000000000Z"
	breadcrumb := map[string]interface{}{
		"last_agent_error": map[string]interface{}{
			"message":     "agent crashed",
			"occurred_at": occurredAt,
		},
	}

	publishSessionState(t, eventBus, taskID, sessionID, map[string]interface{}{
		"session_metadata": breadcrumb,
	})
	if got := store.summary(taskID); got == nil || got.ActiveError == nil {
		t.Fatalf("active error after failure = %+v", got)
	}

	// The agent recovers and posts an ordinary message.
	publishProjectorEvent(t, eventBus, events.MessageAdded, events.MessageAdded, map[string]interface{}{
		"task_id":     taskID,
		"session_id":  sessionID,
		"author_type": "agent",
		"type":        "text",
		"content":     "recovered",
	})
	if got := store.summary(taskID); got.ActiveError != nil {
		t.Fatalf("active error after recovery = %+v, want cleared", got.ActiveError)
	}

	// Turn end republishes the session, metadata breadcrumb and all.
	publishSessionState(t, eventBus, taskID, sessionID, map[string]interface{}{
		"new_state":        "WAITING_FOR_INPUT",
		"session_metadata": breadcrumb,
	})
	if got := store.summary(taskID); got.ActiveError != nil {
		t.Fatalf("active error after metadata replay = %+v, want it to stay cleared", got.ActiveError)
	}

	// A genuinely new failure is still authoritative.
	publishProjectorEvent(t, eventBus, events.TaskSessionErrorChanged, events.TaskSessionErrorChanged, map[string]interface{}{
		"task_id":     taskID,
		"session_id":  sessionID,
		"active":      true,
		"message":     "agent crashed again",
		"occurred_at": "2026-08-01T18:05:00.000000000Z",
		"stamp":       "error-stamp-2",
	})
	got := store.summary(taskID)
	if got.ActiveError == nil || got.ActiveError.Stamp != "error-stamp-2" {
		t.Fatalf("active error after a new failure = %+v, want the new error", got.ActiveError)
	}
}

// A retired record is an authoritative "no active error" for the session, so it
// must clear an error the projection restored from its persisted row rather than
// being read as an absence of information.
func TestProjectorClearsErrorWhenSessionMetadataIsRetired(t *testing.T) {
	store := newProjectorTestStore()
	storedAt := time.Date(2026, 8, 1, 17, 59, 0, 0, time.UTC)
	store.rows["task-superseded"] = &StoredTaskStatusSummary{
		TaskID:      "task-superseded",
		WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{
			Revision:       3,
			UpdatedAt:      storedAt,
			PrimarySession: &PrimarySessionSummary{ID: "session-superseded", State: "RUNNING"},
			ActiveError: &ActiveErrorSummary{
				SessionID:  "session-superseded",
				Stamp:      "error-stored",
				OccurredAt: storedAt,
				Preview:    "agent crashed",
			},
		},
	}

	projector := NewProjector(ProjectorConfig{
		Store: store,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		Now: func() time.Time { return storedAt.Add(time.Minute) },
	})

	if err := projector.HandleEvent(context.Background(), bus.NewEvent(events.TaskSessionStateChanged, "test", map[string]interface{}{
		"task_id":      "task-superseded",
		"workspace_id": "workspace-1",
		"session_id":   "session-superseded",
		"new_state":    "WAITING_FOR_INPUT",
		"is_primary":   true,
		"session_metadata": map[string]interface{}{
			"last_agent_error": map[string]interface{}{
				"message":      "agent crashed",
				"occurred_at":  storedAt.Format(time.RFC3339Nano),
				"dismissed_at": storedAt.Add(30 * time.Second).Format(time.RFC3339Nano),
			},
		},
	})); err != nil {
		t.Fatalf("replay session event: %v", err)
	}

	got := store.summary("task-superseded")
	if got == nil {
		t.Fatal("summary disappeared")
	}
	if got.ActiveError != nil {
		t.Fatalf("active error after retired metadata = %+v, want cleared", got.ActiveError)
	}
}

// The orchestrator clears a recovered failure by writing JSON null, which
// round-trips as a nil value under an existing key. Reading that as "no
// information" instead of "no active error" would leave a restored summary's
// error armed forever — the restart path this whole fix is about.
func TestProjectorClearsErrorWhenSessionMetadataIsNull(t *testing.T) {
	store := newProjectorTestStore()
	storedAt := time.Date(2026, 8, 1, 17, 59, 0, 0, time.UTC)
	store.rows["task-null"] = &StoredTaskStatusSummary{
		TaskID:      "task-null",
		WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{
			Revision:       3,
			UpdatedAt:      storedAt,
			PrimarySession: &PrimarySessionSummary{ID: "session-null", State: "RUNNING"},
			ActiveError: &ActiveErrorSummary{
				SessionID:  "session-null",
				Stamp:      "error-stored",
				OccurredAt: storedAt,
				Preview:    "agent crashed",
			},
		},
	}

	projector := NewProjector(ProjectorConfig{
		Store: store,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		Now: func() time.Time { return storedAt.Add(time.Minute) },
	})

	if err := projector.HandleEvent(context.Background(), bus.NewEvent(events.TaskSessionStateChanged, "test", map[string]interface{}{
		"task_id":      "task-null",
		"workspace_id": "workspace-1",
		"session_id":   "session-null",
		"new_state":    "WAITING_FOR_INPUT",
		"is_primary":   true,
		"session_metadata": map[string]interface{}{
			"last_agent_error": nil,
		},
	})); err != nil {
		t.Fatalf("replay session event: %v", err)
	}

	got := store.summary("task-null")
	if got == nil {
		t.Fatal("summary disappeared")
	}
	if got.ActiveError != nil {
		t.Fatalf("active error after null metadata = %+v, want cleared", got.ActiveError)
	}
}

func TestProjectorConvergesConcurrentGitObservations(t *testing.T) {
	projector, store, _, _, _ := newProjectorTest(t)
	const taskID = "task-concurrent-git"
	const sessionID = "session-concurrent-git"

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			data := map[string]interface{}{
				"task_id":      taskID,
				"session_id":   sessionID,
				"workspace_id": "workspace-1",
				"type":         "status_update",
				"status": map[string]interface{}{
					"repository_name":  fmt.Sprintf("repo-%d", index),
					"changed_files":    1,
					"branch_additions": 2,
				},
			}
			if err := projector.HandleEvent(context.Background(), bus.NewEvent(events.GitEvent, "test", data)); err != nil {
				t.Errorf("project concurrent Git event %d: %v", index, err)
			}
		}(i)
	}
	wg.Wait()

	got := store.summary(taskID)
	if got == nil || got.Git == nil {
		t.Fatalf("concurrent summary = %+v", got)
	}
	if got.Git.ChangedFiles != 8 || got.Git.Additions != 16 {
		t.Fatalf("concurrent Git aggregate = %+v, want 8 files and 16 additions", got.Git)
	}
	if got.Revision != 8 {
		t.Fatalf("concurrent revision = %d, want 8", got.Revision)
	}
}

func TestProjectorRetainsStoredDomainsUntilTheirFirstObservation(t *testing.T) {
	store := newProjectorTestStore()
	storedAt := time.Date(2026, 8, 1, 17, 59, 0, 0, time.UTC)
	store.rows["task-restart"] = &StoredTaskStatusSummary{
		TaskID:      "task-restart",
		WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{
			Revision:            8,
			UpdatedAt:           storedAt,
			PrimarySession:      &PrimarySessionSummary{ID: "primary-restart", State: "RUNNING"},
			ForegroundActivity:  "generating",
			ActiveSubagentCount: 3,
			PendingAction:       "permission",
			ActiveError: &ActiveErrorSummary{
				SessionID:  "primary-restart",
				Stamp:      "error-restart",
				OccurredAt: storedAt,
				Preview:    "stored error",
			},
			Git:         &GitSummary{Additions: 4, Deletions: 2, ChangedFiles: 3, Ahead: 1, Behind: 2},
			PullRequest: &PullRequestSummary{Count: 1, OpenCount: 1, Attention: true, State: "open", Number: 12, URL: "https://example.test/12"},
		},
	}

	projector := NewProjector(ProjectorConfig{
		Store: store,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		Now: func() time.Time { return storedAt.Add(time.Minute) },
	})

	if err := projector.HandleEvent(context.Background(), bus.NewEvent(events.TaskSessionStateChanged, "test", map[string]interface{}{
		"task_id":      "task-restart",
		"workspace_id": "workspace-1",
		"session_id":   "background-restart",
		"new_state":    "WAITING_FOR_INPUT",
		"is_primary":   false,
	})); err != nil {
		t.Fatalf("replay lifecycle event: %v", err)
	}

	got := store.summary("task-restart")
	if got == nil {
		t.Fatal("stored summary disappeared after projector restart")
	}
	if got.Revision != 8 {
		t.Fatalf("revision after unrelated replay = %d, want 8", got.Revision)
	}
	if got.PrimarySession == nil || got.PrimarySession.ID != "primary-restart" || got.PrimarySession.State != "RUNNING" {
		t.Fatalf("primary session after replay = %+v", got.PrimarySession)
	}
	if got.ForegroundActivity != "generating" || got.ActiveSubagentCount != 3 || got.PendingAction != "permission" {
		t.Fatalf("stored task status after replay = %+v", got)
	}
	if got.ActiveError == nil || got.ActiveError.Stamp != "error-restart" {
		t.Fatalf("stored error after replay = %+v", got.ActiveError)
	}
	if got.Git == nil || got.Git.ChangedFiles != 3 || got.PullRequest == nil || got.PullRequest.Number != 12 {
		t.Fatalf("stored source domains after replay = git=%+v pr=%+v", got.Git, got.PullRequest)
	}
}

func TestProjectorStaleDismissedClearsRestoredPendingAction(t *testing.T) {
	store := newProjectorTestStore()
	storedAt := time.Date(2026, 8, 1, 17, 59, 0, 0, time.UTC)
	store.rows["task-stale-dismiss-restart"] = &StoredTaskStatusSummary{
		TaskID:      "task-stale-dismiss-restart",
		WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{
			Revision:      8,
			UpdatedAt:     storedAt,
			PendingAction: pendingClarification,
		},
	}
	projector := NewProjector(ProjectorConfig{
		Store: store,
		Now:   func() time.Time { return storedAt.Add(time.Minute) },
	})

	err := projector.HandleEvent(context.Background(), bus.NewEvent(
		events.ClarificationStaleDismissed,
		"test",
		map[string]interface{}{
			"task_id":      "task-stale-dismiss-restart",
			"workspace_id": "workspace-1",
			"session_id":   "session-stale-dismiss-restart",
		},
	))
	if err != nil {
		t.Fatalf("project stale dismissal after restart: %v", err)
	}

	got := store.summary("task-stale-dismiss-restart")
	if got == nil || got.PendingAction != "" || got.Revision != 9 {
		t.Fatalf("summary after stale dismissal = %+v, want empty pending action at revision 9", got)
	}
}

func TestProjectorStaleDismissedKeepsDifferentPendingClarification(t *testing.T) {
	_, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-stale-dismiss-current"
	const sessionID = "session-stale-dismiss-current"

	publishSessionState(t, eventBus, taskID, sessionID, nil)
	publishProjectorEvent(t, eventBus, events.MessageAdded, events.MessageAdded, map[string]interface{}{
		"task_id":        taskID,
		"workspace_id":   "workspace-1",
		"session_id":     sessionID,
		"type":           messageTypeClarificationRequest,
		"requests_input": true,
		"metadata": map[string]interface{}{
			"status":     statusPending,
			"pending_id": "pending-current",
		},
	})
	publishProjectorEvent(t, eventBus, events.ClarificationStaleDismissed, events.ClarificationStaleDismissed, map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": "workspace-1",
		"session_id":   sessionID,
		"pending_id":   "pending-older",
	})

	got := store.summary(taskID)
	if got == nil || got.PendingAction != pendingClarification {
		t.Fatalf("pending action after different stale dismissal = %+v, want clarification", got)
	}
}

func TestProjectorKeepsPRsWithTheSameRepositoryDistinct(t *testing.T) {
	_, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-pr-identity"

	for _, number := range []int{41, 42} {
		publishProjectorEvent(t, eventBus, events.GitHubTaskPRUpdated, events.GitHubTaskPRUpdated, map[string]interface{}{
			"task_id":       taskID,
			"workspace_id":  "workspace-1",
			"repository_id": "repo-a",
			"state":         "open",
			"pr_number":     number,
			"pr_url":        fmt.Sprintf("https://example.test/pr/%d", number),
		})
	}

	got := store.summary(taskID)
	if got == nil || got.PullRequest == nil {
		t.Fatalf("PR summary = %+v", got)
	}
	if got.PullRequest.Count != 2 {
		t.Fatalf("PR count = %d, want 2 for two PR numbers in one repository", got.PullRequest.Count)
	}
}

func TestProjectorRehydratesSiblingGitObservationsAfterRestart(t *testing.T) {
	store := newProjectorTestStore()
	store.rows["task-git-restart"] = &StoredTaskStatusSummary{
		TaskID:      "task-git-restart",
		WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{
			Revision: 1,
			Git:      &GitSummary{Additions: 7, ChangedFiles: 3},
		},
	}
	projector := NewProjector(ProjectorConfig{
		Store: store,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		LoadGitObservations: func(context.Context, string) ([]GitObservation, error) {
			return []GitObservation{
				{Repository: "repo-a", Summary: GitSummary{Additions: 5, ChangedFiles: 2}},
				{Repository: "repo-b", Summary: GitSummary{Additions: 2, ChangedFiles: 1}},
			}, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 1, 18, 0, 0, 0, time.UTC) },
	})

	if err := projector.HandleEvent(context.Background(), bus.NewEvent(events.GitEvent, "test", map[string]interface{}{
		"task_id":      "task-git-restart",
		"workspace_id": "workspace-1",
		"session_id":   "session-1",
		"type":         "status_update",
		"status": map[string]interface{}{
			"repository_name":  "repo-a",
			"branch_additions": 6,
			"changed_files":    2,
		},
	})); err != nil {
		t.Fatalf("replay Git event: %v", err)
	}

	got := store.summary("task-git-restart")
	if got == nil || got.Git == nil {
		t.Fatalf("Git summary = %+v", got)
	}
	if got.Git.Additions != 8 || got.Git.ChangedFiles != 3 {
		t.Fatalf("Git summary after sibling rehydration = %+v, want additions=8 changed_files=3", got.Git)
	}
}

// A resolved permission/clarification request must clear the task-list pending
// affordance. The resolution arrives as a message.updated on the request row
// itself, so the projector has to read the terminal status off that row instead
// of treating every request-typed message as evidence of a live prompt.
func TestProjectorClearsPendingWhenRequestMessageResolves(t *testing.T) {
	cases := []struct {
		name        string
		messageType string
		arm         func(t *testing.T, eventBus *bus.MemoryEventBus, taskID, sessionID string)
		wantArmed   string
		status      string
	}{
		{
			name:        "permission approved",
			messageType: messageTypePermissionRequest,
			arm: func(t *testing.T, eventBus *bus.MemoryEventBus, taskID, sessionID string) {
				publishProjectorEvent(t, eventBus, events.PermissionRequestReceived, events.BuildPermissionRequestSubject(sessionID), map[string]interface{}{
					"task_id":    taskID,
					"session_id": sessionID,
					"pending_id": "pending-1",
				})
			},
			wantArmed: "permission",
			status:    "approved",
		},
		{
			name:        "permission expired",
			messageType: messageTypePermissionRequest,
			arm: func(t *testing.T, eventBus *bus.MemoryEventBus, taskID, sessionID string) {
				publishProjectorEvent(t, eventBus, events.PermissionRequestReceived, events.BuildPermissionRequestSubject(sessionID), map[string]interface{}{
					"task_id":    taskID,
					"session_id": sessionID,
					"pending_id": "pending-1",
				})
			},
			wantArmed: "permission",
			status:    "expired",
		},
		{
			name:        "clarification answered",
			messageType: "clarification_request",
			arm: func(t *testing.T, eventBus *bus.MemoryEventBus, taskID, sessionID string) {
				publishProjectorEvent(t, eventBus, events.MessageAdded, events.MessageAdded, map[string]interface{}{
					"task_id":        taskID,
					"session_id":     sessionID,
					"author_type":    "user",
					"type":           "clarification_request",
					"requests_input": true,
					"metadata":       map[string]interface{}{"status": "pending", "pending_id": "pending-1"},
				})
			},
			wantArmed: "clarification",
			status:    "answered",
		},
		{
			name:        "permission rejected",
			messageType: messageTypePermissionRequest,
			arm: func(t *testing.T, eventBus *bus.MemoryEventBus, taskID, sessionID string) {
				publishProjectorEvent(t, eventBus, events.PermissionRequestReceived, events.BuildPermissionRequestSubject(sessionID), map[string]interface{}{
					"task_id":    taskID,
					"session_id": sessionID,
					"pending_id": "pending-1",
				})
			},
			wantArmed: "permission",
			status:    "rejected",
		},
		{
			name:        "clarification cancelled",
			messageType: "clarification_request",
			arm: func(t *testing.T, eventBus *bus.MemoryEventBus, taskID, sessionID string) {
				publishProjectorEvent(t, eventBus, events.MessageAdded, events.MessageAdded, map[string]interface{}{
					"task_id":        taskID,
					"session_id":     sessionID,
					"author_type":    "user",
					"type":           "clarification_request",
					"requests_input": true,
					"metadata":       map[string]interface{}{"status": "pending", "pending_id": "pending-1"},
				})
			},
			wantArmed: "clarification",
			status:    "cancelled",
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, store, eventBus, _, _ := newProjectorTest(t)
			taskID := fmt.Sprintf("task-pending-resolve-%d", i)
			sessionID := fmt.Sprintf("session-pending-resolve-%d", i)

			publishSessionState(t, eventBus, taskID, sessionID, nil)
			tc.arm(t, eventBus, taskID, sessionID)

			if got := store.summary(taskID); got == nil || got.PendingAction != tc.wantArmed {
				t.Fatalf("pending action before resolution = %+v, want %q", got, tc.wantArmed)
			}

			publishProjectorEvent(t, eventBus, events.MessageUpdated, events.MessageUpdated, map[string]interface{}{
				"task_id":        taskID,
				"session_id":     sessionID,
				"author_type":    "user",
				"type":           tc.messageType,
				"requests_input": tc.messageType == "clarification_request",
				"metadata":       map[string]interface{}{"status": tc.status, "pending_id": "pending-1"},
			})

			got := store.summary(taskID)
			if got == nil {
				t.Fatal("missing projected summary")
			}
			if got.PendingAction != "" {
				t.Fatalf("pending action after %q resolution = %q, want empty", tc.status, got.PendingAction)
			}
		})
	}
}

func TestProjectorKeepsPendingWhenUnrelatedMessageResolves(t *testing.T) {
	_, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-pending-unrelated"
	const sessionID = "session-pending-unrelated"

	publishSessionState(t, eventBus, taskID, sessionID, nil)
	publishProjectorEvent(t, eventBus, events.PermissionRequestReceived, events.BuildPermissionRequestSubject(sessionID), map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"pending_id": "permission-a",
	})

	publishProjectorEvent(t, eventBus, events.MessageUpdated, events.MessageUpdated, map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"type":       "tool_execute",
		"metadata":   map[string]interface{}{"status": "completed", "pending_id": "tool-1"},
	})

	got := store.summary(taskID)
	if got == nil || got.PendingAction != "permission" {
		t.Fatalf("pending action after unrelated resolution = %+v, want permission", got)
	}

	publishProjectorEvent(t, eventBus, events.MessageUpdated, events.MessageUpdated, map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"type":       messageTypePermissionRequest,
		"metadata":   map[string]interface{}{"status": "expired", "pending_id": "permission-b"},
	})

	got = store.summary(taskID)
	if got == nil || got.PendingAction != "permission" {
		t.Fatalf("pending action after different request resolution = %+v, want permission", got)
	}

	publishProjectorEvent(t, eventBus, events.MessageDeleted, events.MessageDeleted, map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"type":       messageTypePermissionRequest,
		"metadata":   map[string]interface{}{"pending_id": "permission-b"},
	})

	got = store.summary(taskID)
	if got == nil || got.PendingAction != "permission" {
		t.Fatalf("pending action after deleting a different request = %+v, want permission", got)
	}

	publishProjectorEvent(t, eventBus, events.MessageUpdated, events.MessageUpdated, map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"type":       messageTypePermissionRequest,
		"metadata":   map[string]interface{}{"status": "approved", "pending_id": "permission-a"},
	})

	got = store.summary(taskID)
	if got == nil || got.PendingAction != "" {
		t.Fatalf("pending action after request resolution = %+v, want empty", got)
	}
}

// A detached-but-still-answerable clarification keeps status=pending, so the
// task-list affordance must survive that update.
func TestProjectorKeepsPendingWhenRequestStaysAnswerable(t *testing.T) {
	_, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-pending-detached"
	const sessionID = "session-pending-detached"

	publishSessionState(t, eventBus, taskID, sessionID, nil)
	publishProjectorEvent(t, eventBus, events.MessageAdded, events.MessageAdded, map[string]interface{}{
		"task_id":        taskID,
		"session_id":     sessionID,
		"author_type":    "user",
		"type":           "clarification_request",
		"requests_input": true,
		"metadata":       map[string]interface{}{"status": "pending", "pending_id": "pending-1"},
	})
	publishProjectorEvent(t, eventBus, events.MessageUpdated, events.MessageUpdated, map[string]interface{}{
		"task_id":        taskID,
		"session_id":     sessionID,
		"author_type":    "user",
		"type":           "clarification_request",
		"requests_input": true,
		"metadata": map[string]interface{}{
			"status":             "pending",
			"pending_id":         "pending-1",
			"agent_disconnected": true,
		},
	})

	got := store.summary(taskID)
	if got == nil || got.PendingAction != "clarification" {
		t.Fatalf("pending action after detach = %+v, want clarification", got)
	}
}

// Every announced tool call is persisted with metadata.status="pending" (the raw
// ACP tool status) before its first tool_update arrives. That is ordinary agent
// work, not a prompt waiting on the user, so it must never arm the amber
// permission affordance on the task row.
func TestProjectorIgnoresPendingToolCallMessages(t *testing.T) {
	for _, messageType := range []string{"tool_call", "tool_execute", "tool_read", "tool_edit", "tool_search"} {
		t.Run(messageType, func(t *testing.T) {
			_, store, eventBus, _, _ := newProjectorTest(t)
			taskID := "task-pending-tool-" + messageType
			sessionID := "session-pending-tool-" + messageType

			publishSessionState(t, eventBus, taskID, sessionID, nil)
			publishProjectorEvent(t, eventBus, events.MessageAdded, events.MessageAdded, map[string]interface{}{
				"task_id":     taskID,
				"session_id":  sessionID,
				"author_type": "agent",
				"type":        messageType,
				"metadata": map[string]interface{}{
					"status":       statusPending,
					"tool_call_id": "tool-1",
					"title":        "Terminal",
				},
			})

			got := store.summary(taskID)
			if got == nil {
				t.Fatal("missing projected summary")
			}
			if got.PendingAction != "" {
				t.Fatalf("pending action for a pending %s = %q, want empty", messageType, got.PendingAction)
			}
		})
	}
}

// A message that merely carries a pending_id must not default to the permission
// affordance either: only a real permission_request does.
func TestProjectorDoesNotDefaultUnknownPendingMessagesToPermission(t *testing.T) {
	_, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-pending-unknown"
	const sessionID = "session-pending-unknown"

	publishSessionState(t, eventBus, taskID, sessionID, nil)
	publishProjectorEvent(t, eventBus, events.MessageAdded, events.MessageAdded, map[string]interface{}{
		"task_id":     taskID,
		"session_id":  sessionID,
		"author_type": "agent",
		"type":        "status",
		"metadata":    map[string]interface{}{"status": statusPending, "pending_id": "pending-1"},
	})

	got := store.summary(taskID)
	if got == nil {
		t.Fatal("missing projected summary")
	}
	if got.PendingAction != "" {
		t.Fatalf("pending action for an untyped pending message = %q, want empty", got.PendingAction)
	}
}

// A genuinely pending permission must survive unrelated agent traffic in the
// same session — a background tool call completing while the foreground turn is
// blocked on the prompt must not tear down the affordance the user still has to
// act on.
func TestProjectorKeepsPendingAcrossUnrelatedToolTraffic(t *testing.T) {
	_, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-pending-unrelated-tools"
	const sessionID = "session-pending-unrelated-tools"

	publishSessionState(t, eventBus, taskID, sessionID, nil)
	publishProjectorEvent(t, eventBus, events.PermissionRequestReceived, events.BuildPermissionRequestSubject(sessionID), map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
	})
	if got := store.summary(taskID); got == nil || got.PendingAction != pendingPermission {
		t.Fatalf("pending action before tool traffic = %+v, want permission", got)
	}

	for _, status := range []string{"complete", "error", "cancelled"} {
		publishProjectorEvent(t, eventBus, events.MessageUpdated, events.MessageUpdated, map[string]interface{}{
			"task_id":     taskID,
			"session_id":  sessionID,
			"author_type": "agent",
			"type":        "tool_execute",
			"metadata":    map[string]interface{}{"status": status, "tool_call_id": "background-1"},
		})
		got := store.summary(taskID)
		if got == nil || got.PendingAction != pendingPermission {
			t.Fatalf("pending action after a %q tool update = %+v, want permission", status, got)
		}
	}
}

// The exact request type wins over the generic requests_input flag, so a
// permission row is never classified as a clarification.
func TestPendingActionForMessagePrefersExactRequestType(t *testing.T) {
	cases := []struct {
		messageType   string
		requestsInput bool
		want          string
	}{
		{messageTypePermissionRequest, false, pendingPermission},
		{messageTypePermissionRequest, true, pendingPermission},
		{messageTypeClarificationRequest, false, pendingClarification},
		{messageTypeClarificationRequest, true, pendingClarification},
		{"message", true, pendingClarification},
		{"tool_execute", false, ""},
		{"tool_execute", true, pendingClarification},
		{"", false, ""},
	}
	for _, tc := range cases {
		got := pendingActionForMessage(tc.messageType, tc.requestsInput)
		if got != tc.want {
			t.Errorf("pendingActionForMessage(%q, %t) = %q, want %q",
				tc.messageType, tc.requestsInput, got, tc.want)
		}
	}
}
