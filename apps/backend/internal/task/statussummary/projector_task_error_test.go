package statussummary

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func taskErrorProjector(t *testing.T, store *projectorTestStore, loader TaskLaunchErrorLoader) *Projector {
	t.Helper()
	return NewProjector(ProjectorConfig{
		Store: store,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		LoadTaskLaunchError: loader,
		Now: func() time.Time {
			return time.Date(2026, 8, 19, 20, 0, 0, 0, time.UTC)
		},
	})
}

func TestProjectorProjectsTaskOwnedLaunchErrorWithoutSession(t *testing.T) {
	store := newProjectorTestStore()
	projector := taskErrorProjector(t, store, func(context.Context, string) (TaskLaunchErrorObservation, error) {
		return TaskLaunchErrorObservation{
			Observed: true,
			Error: &ActiveErrorSummary{
				TaskRepositoryID: "task-repository-1",
				Stamp:            "task-error-1",
				OccurredAt:       time.Date(2026, 8, 19, 19, 0, 0, 0, time.UTC),
				Preview:          "The linked pull request is already closed or merged.",
				Category:         "pr_already_closed",
				RecoveryActions:  []string{"mark_review_done", "unknown", "mark_review_done"},
			},
		}, nil
	})

	err := projector.HandleEvent(context.Background(), bus.NewEvent(events.TaskUpdated, "test", map[string]interface{}{
		"task_id":      "task-1",
		"workspace_id": "workspace-1",
	}))
	if err != nil {
		t.Fatalf("project task error: %v", err)
	}
	got := store.summary("task-1")
	if got == nil || got.ActiveError == nil {
		t.Fatalf("summary = %+v, want task-owned active error", got)
	}
	if got.ActiveError.SessionID != "" || got.ActiveError.TaskRepositoryID != "task-repository-1" {
		t.Fatalf("active error identity = %+v", got.ActiveError)
	}
	if len(got.ActiveError.RecoveryActions) != 1 || got.ActiveError.RecoveryActions[0] != "mark_review_done" {
		t.Fatalf("active error actions = %#v", got.ActiveError.RecoveryActions)
	}
}

func TestProjectorMalformedTaskLaunchErrorDoesNotEraseStoredError(t *testing.T) {
	store := newProjectorTestStore()
	store.rows["task-1"] = &StoredTaskStatusSummary{
		TaskID:      "task-1",
		WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{
			Revision: 1,
			ActiveError: &ActiveErrorSummary{
				Stamp:      "stored-task-error",
				OccurredAt: time.Date(2026, 8, 19, 19, 0, 0, 0, time.UTC),
				Preview:    "stored task error",
			},
		},
	}
	projector := taskErrorProjector(t, store, func(context.Context, string) (TaskLaunchErrorObservation, error) {
		return TaskLaunchErrorObservation{}, nil
	})

	err := projector.HandleEvent(context.Background(), bus.NewEvent(events.TaskUpdated, "test", map[string]interface{}{
		"task_id":      "task-1",
		"workspace_id": "workspace-1",
	}))
	if err != nil {
		t.Fatalf("project malformed task error: %v", err)
	}
	got := store.summary("task-1")
	if got == nil || got.ActiveError == nil || got.ActiveError.Stamp != "stored-task-error" {
		t.Fatalf("summary = %+v, want stored error preserved", got)
	}
}

func TestProjectorRefreshesTaskLaunchErrorRemoval(t *testing.T) {
	store := newProjectorTestStore()
	active := true
	projector := taskErrorProjector(t, store, func(context.Context, string) (TaskLaunchErrorObservation, error) {
		if !active {
			return TaskLaunchErrorObservation{Observed: true}, nil
		}
		return TaskLaunchErrorObservation{
			Observed: true,
			Error: &ActiveErrorSummary{
				Stamp:      "task-error-1",
				OccurredAt: time.Date(2026, 8, 19, 19, 0, 0, 0, time.UTC),
				Preview:    "task error",
			},
		}, nil
	})

	event := func() *bus.Event {
		return bus.NewEvent(events.TaskUpdated, "test", map[string]interface{}{
			"task_id":      "task-1",
			"workspace_id": "workspace-1",
		})
	}
	if err := projector.HandleEvent(context.Background(), event()); err != nil {
		t.Fatalf("project initial task error: %v", err)
	}
	active = false
	if err := projector.HandleEvent(context.Background(), event()); err != nil {
		t.Fatalf("project cleared task error: %v", err)
	}
	if got := store.summary("task-1"); got == nil || got.ActiveError != nil {
		t.Fatalf("summary after task error removal = %+v, want no active error", got)
	}
}
