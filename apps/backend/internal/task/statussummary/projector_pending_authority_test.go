package statussummary

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

type rejectingPendingProjectorStore struct {
	*projectorTestStore
	mu        sync.Mutex
	competing StoredTaskStatusSummary
	rejected  bool
}

func (s *rejectingPendingProjectorStore) CompareAndUpdateTaskStatusSummary(
	ctx context.Context,
	stored *StoredTaskStatusSummary,
) (bool, error) {
	s.mu.Lock()
	if !s.rejected {
		s.rejected = true
		s.mu.Unlock()
		if _, err := s.projectorTestStore.CompareAndUpdateTaskStatusSummary(ctx, &s.competing); err != nil {
			return false, err
		}
		return false, nil
	}
	s.mu.Unlock()
	return s.projectorTestStore.CompareAndUpdateTaskStatusSummary(ctx, stored)
}

func TestProjectorRefreshesPendingFromAuthorityOnRestore(t *testing.T) {
	store := newProjectorTestStore()
	storedAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	store.rows["task-stale"] = &StoredTaskStatusSummary{
		TaskID:      "task-stale",
		WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{
			Revision:      4,
			UpdatedAt:     storedAt,
			PendingAction: pendingClarification,
			Git:           &GitSummary{ChangedFiles: 3},
		},
	}
	projector := NewProjector(ProjectorConfig{
		Store: store,
		LoadPendingActions: func(context.Context, string) (map[string]string, error) {
			return map[string]string{}, nil
		},
		Now: func() time.Time { return storedAt.Add(time.Minute) },
	})

	err := projector.HandleEvent(context.Background(), bus.NewEvent(events.TaskUpdated, "test", map[string]interface{}{
		"task_id":      "task-stale",
		"workspace_id": "workspace-1",
	}))
	if err != nil {
		t.Fatalf("restore stale summary: %v", err)
	}

	got := store.summary("task-stale")
	if got == nil || got.PendingAction != "" {
		t.Fatalf("pending action after restore = %+v, want cleared", got)
	}
	if got.Revision != 5 || got.Git == nil || got.Git.ChangedFiles != 3 {
		t.Fatalf("repaired summary = %+v, want revision 5 with Git preserved", got)
	}
}

func TestProjectorRefreshesPendingAfterEveryPendingSensitiveMessage(t *testing.T) {
	store := newProjectorTestStore()
	actions := map[string]string{"session-1": pendingClarification}
	projector := NewProjector(ProjectorConfig{
		Store: store,
		LoadPendingActions: func(context.Context, string) (map[string]string, error) {
			copy := make(map[string]string, len(actions))
			for sessionID, action := range actions {
				copy[sessionID] = action
			}
			return copy, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC) },
	})

	message := func(eventType, messageType string) *bus.Event {
		return bus.NewEvent(eventType, "test", map[string]interface{}{
			"task_id":      "task-current",
			"workspace_id": "workspace-1",
			"session_id":   "session-1",
			"author_type":  "user",
			"type":         messageType,
		})
	}
	if err := projector.HandleEvent(context.Background(), message(events.MessageAdded, messageTypeClarificationRequest)); err != nil {
		t.Fatalf("arm current clarification: %v", err)
	}
	if got := store.summary("task-current"); got == nil || got.PendingAction != pendingClarification {
		t.Fatalf("armed summary = %+v", got)
	}

	for _, event := range []struct {
		eventType   string
		messageType string
	}{
		{eventType: events.MessageAdded, messageType: "text"},
		{eventType: events.MessageDeleted, messageType: messageTypeClarificationRequest},
	} {
		actions = map[string]string{}
		if err := projector.HandleEvent(context.Background(), message(event.eventType, event.messageType)); err != nil {
			t.Fatalf("refresh after %s: %v", event.eventType, err)
		}
		if got := store.summary("task-current"); got == nil || got.PendingAction != "" {
			t.Fatalf("pending after %s = %+v, want cleared", event.eventType, got)
		}

		actions = map[string]string{"session-1": pendingClarification}
		if err := projector.HandleEvent(context.Background(), message(events.MessageAdded, messageTypeClarificationRequest)); err != nil {
			t.Fatalf("re-arm after %s: %v", event.eventType, err)
		}
	}
}

func TestProjectorSkipsPendingRefreshForStreamingMessageUpdates(t *testing.T) {
	store := newProjectorTestStore()
	actions := map[string]string{}
	loaderCalls := 0
	projector := NewProjector(ProjectorConfig{
		Store: store,
		LoadPendingActions: func(context.Context, string) (map[string]string, error) {
			loaderCalls++
			return actions, nil
		},
	})
	ctx := context.Background()
	event := func(messageType string) *bus.Event {
		return bus.NewEvent(events.MessageUpdated, "test", map[string]interface{}{
			"task_id":      "task-streaming",
			"workspace_id": "workspace-1",
			"session_id":   "session-1",
			"author_type":  "agent",
			"type":         messageType,
		})
	}
	if err := projector.HandleEvent(ctx, bus.NewEvent(events.TaskUpdated, "test", map[string]interface{}{
		"task_id": "task-streaming", "workspace_id": "workspace-1",
	})); err != nil {
		t.Fatalf("initialize projection: %v", err)
	}
	loaderCalls = 0

	if err := projector.HandleEvent(ctx, event("message")); err != nil {
		t.Fatalf("streaming message update: %v", err)
	}
	if loaderCalls != 0 {
		t.Fatalf("pending loader calls after streaming update = %d, want 0", loaderCalls)
	}

	actions = map[string]string{"session-1": pendingClarification}
	if err := projector.HandleEvent(ctx, event(messageTypeClarificationRequest)); err != nil {
		t.Fatalf("clarification update: %v", err)
	}
	if loaderCalls != 1 {
		t.Fatalf("pending loader calls after clarification update = %d, want 1", loaderCalls)
	}
}

func TestProjectorPendingLoaderFailureRetainsStoredState(t *testing.T) {
	store := newProjectorTestStore()
	storedAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	store.rows["task-load-error"] = &StoredTaskStatusSummary{
		TaskID:      "task-load-error",
		WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{
			Revision:      2,
			UpdatedAt:     storedAt,
			PendingAction: pendingPermission,
		},
	}
	projector := NewProjector(ProjectorConfig{
		Store: store,
		LoadPendingActions: func(context.Context, string) (map[string]string, error) {
			return nil, errors.New("pending lookup failed")
		},
		Now: func() time.Time { return storedAt.Add(time.Minute) },
	})

	err := projector.HandleEvent(context.Background(), bus.NewEvent(events.MessageAdded, "test", map[string]interface{}{
		"task_id":      "task-load-error",
		"workspace_id": "workspace-1",
		"session_id":   "session-1",
		"author_type":  "user",
		"type":         "text",
	}))
	if err == nil {
		t.Fatal("pending loader error = nil, want surfaced error")
	}
	got := store.summary("task-load-error")
	if got == nil || got.Revision != 2 || got.PendingAction != pendingPermission {
		t.Fatalf("stored summary after loader failure = %+v", got)
	}
}

func TestProjectorPendingRefreshRetriesAfterCASRejection(t *testing.T) {
	base := newProjectorTestStore()
	storedAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	base.rows["task-cas"] = &StoredTaskStatusSummary{
		TaskID:      "task-cas",
		WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{
			Revision:      5,
			UpdatedAt:     storedAt,
			PendingAction: pendingClarification,
			Git:           &GitSummary{ChangedFiles: 1},
		},
	}
	store := &rejectingPendingProjectorStore{
		projectorTestStore: base,
		competing: StoredTaskStatusSummary{
			TaskID:      "task-cas",
			WorkspaceID: "workspace-1",
			Summary: TaskStatusSummary{
				Revision:          6,
				UpdatedAt:         storedAt.Add(time.Second),
				PendingAction:     pendingClarification,
				QueuedPromptCount: 7,
				Git:               &GitSummary{ChangedFiles: 2},
			},
		},
	}
	loaderCalls := 0
	gitLoaderCalls := 0
	projector := NewProjector(ProjectorConfig{
		Store: store,
		LoadPendingActions: func(context.Context, string) (map[string]string, error) {
			loaderCalls++
			return map[string]string{}, nil
		},
		LoadGitObservations: func(context.Context, string) ([]GitObservation, error) {
			gitLoaderCalls++
			return []GitObservation{
				{Repository: "repo-a", Summary: GitSummary{ChangedFiles: 1}},
				{Repository: "repo-b", Summary: GitSummary{ChangedFiles: 1}},
			}, nil
		},
		Now: func() time.Time { return storedAt.Add(2 * time.Second) },
	})

	err := projector.HandleEvent(context.Background(), bus.NewEvent(events.TaskUpdated, "test", map[string]interface{}{
		"task_id":               "task-cas",
		"workspace_id":          "workspace-1",
		"primary_session_id":    "session-current",
		"primary_session_state": sessionStateRunning,
	}))
	if err != nil {
		t.Fatalf("refresh after CAS rejection: %v", err)
	}
	got := base.summary("task-cas")
	if got == nil || got.Revision != 7 || got.PendingAction != "" || got.QueuedPromptCount != 7 ||
		got.Git == nil || got.Git.ChangedFiles != 2 || got.PrimarySession == nil ||
		got.PrimarySession.ID != "session-current" {
		t.Fatalf("summary after CAS retry = %+v", got)
	}
	if loaderCalls != 2 {
		t.Fatalf("pending loader calls = %d, want reload after rejection", loaderCalls)
	}
	if gitLoaderCalls != 2 {
		t.Fatalf("Git loader calls = %d, want reload after rejection", gitLoaderCalls)
	}

	err = projector.HandleEvent(context.Background(), bus.NewEvent(events.GitEvent, "test", map[string]interface{}{
		"task_id":      "task-cas",
		"workspace_id": "workspace-1",
		"session_id":   "session-current",
		"type":         "status_update",
		"status": map[string]interface{}{
			"repository_name": "repo-a",
			"changed_files":   3,
		},
	}))
	if err != nil {
		t.Fatalf("Git replay after CAS rejection: %v", err)
	}
	got = base.summary("task-cas")
	if got == nil || got.Git == nil || got.Git.ChangedFiles != 4 {
		t.Fatalf("Git summary after CAS replay = %+v, want both repositories (4 changed files)", got)
	}
}
