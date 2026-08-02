package statussummary

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
