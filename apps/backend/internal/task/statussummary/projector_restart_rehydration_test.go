package statussummary

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestProjectorRehydratesGitObservationsAfterRestartWithoutAggregate(t *testing.T) {
	store := newProjectorTestStore()
	store.rows["task-git-restart-empty"] = &StoredTaskStatusSummary{
		TaskID:      "task-git-restart-empty",
		WorkspaceID: "workspace-1",
		Summary:     TaskStatusSummary{Revision: 1},
	}
	loaderCalls := 0
	projector := NewProjector(ProjectorConfig{
		Store: store,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		LoadGitObservations: func(context.Context, string) ([]GitObservation, error) {
			loaderCalls++
			return []GitObservation{
				{Repository: "repo-a", Summary: GitSummary{Additions: 5, ChangedFiles: 2}},
				{Repository: "repo-b", Summary: GitSummary{Additions: 2, ChangedFiles: 1}},
			}, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 16, 0, 30, 0, 0, time.UTC) },
	})

	err := projector.HandleEvent(context.Background(), bus.NewEvent(events.GitEvent, "test", map[string]interface{}{
		"task_id":      "task-git-restart-empty",
		"workspace_id": "workspace-1",
		"session_id":   "session-1",
		"type":         "status_update",
		"status": map[string]interface{}{
			"repository_name":  "repo-a",
			"branch_additions": 6,
			"changed_files":    2,
		},
	}))
	if err != nil {
		t.Fatalf("replay Git event: %v", err)
	}

	got := store.summary("task-git-restart-empty")
	if got == nil || got.Git == nil || got.Git.Additions != 8 || got.Git.ChangedFiles != 3 {
		t.Fatalf("Git summary after empty-baseline restart = %+v, want additions=8 changed_files=3", got)
	}
	if loaderCalls != 1 {
		t.Fatalf("Git loader calls = %d, want 1", loaderCalls)
	}
}
