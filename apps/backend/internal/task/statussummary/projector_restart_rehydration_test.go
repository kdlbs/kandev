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

func TestProjectorRehydratesPullRequestObservationsAfterRestartWithoutAggregate(t *testing.T) {
	store := newProjectorTestStore()
	store.rows["task-pr-restart-empty"] = &StoredTaskStatusSummary{
		TaskID:      "task-pr-restart-empty",
		WorkspaceID: "workspace-1",
		Summary:     TaskStatusSummary{Revision: 1},
	}
	loaderCalls := 0
	projector := NewProjector(ProjectorConfig{
		Store: store,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		LoadPullRequests: func(context.Context, string) ([]PullRequestInput, error) {
			loaderCalls++
			return []PullRequestInput{
				{Key: "repo-a#41", State: prStateOpen, Number: 41, URL: "https://example.test/41"},
				{Key: "repo-b#42", State: prStateOpen, Number: 42, URL: "https://example.test/42"},
			}, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 16, 0, 30, 0, 0, time.UTC) },
	})

	err := projector.HandleEvent(context.Background(), bus.NewEvent(events.GitHubTaskPRUpdated, "test", map[string]interface{}{
		"task_id":       "task-pr-restart-empty",
		"workspace_id":  "workspace-1",
		"repository_id": "repo-a",
		"state":         prStateOpen,
		"pr_number":     41,
		"pr_url":        "https://example.test/41",
		"checks_state":  prStateFailure,
	}))
	if err != nil {
		t.Fatalf("replay PR event: %v", err)
	}

	got := store.summary("task-pr-restart-empty")
	if got == nil || got.PullRequest == nil || got.PullRequest.Count != 2 ||
		got.PullRequest.OpenCount != 2 || got.PullRequest.AggregateState != prStateFailure {
		t.Fatalf("PR summary after empty-baseline restart = %+v, want two open PRs with failure", got)
	}
	if loaderCalls != 1 {
		t.Fatalf("PR loader calls = %d, want 1", loaderCalls)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.10
func TestProjectorRefreshesAutomationFlagsAfterCIOptionsUpdate(t *testing.T) {
	store := newProjectorTestStore()
	store.rows["task-pr-options-refresh"] = &StoredTaskStatusSummary{
		TaskID:      "task-pr-options-refresh",
		WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{
			Revision: 1,
			PullRequest: &PullRequestSummary{
				Count: 1, OpenCount: 1, State: prStateOpen, Number: 41,
				AutoFixEnabled: true,
			},
		},
	}
	loaderCalls := 0
	projector := NewProjector(ProjectorConfig{
		Store: store,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		LoadPullRequests: func(context.Context, string) ([]PullRequestInput, error) {
			loaderCalls++
			return []PullRequestInput{{
				Key: "repo-a#41", State: prStateOpen, Number: 41,
				AutoFixEnabled: loaderCalls == 1,
			}}, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 16, 0, 30, 0, 0, time.UTC) },
	})

	if err := projector.HandleEvent(context.Background(), bus.NewEvent(events.GitHubTaskCIOptionsUpdated, "test", map[string]interface{}{
		"task_id": "task-pr-options-refresh", "workspace_id": "workspace-1",
	})); err != nil {
		t.Fatalf("replay CI options event: %v", err)
	}
	updated := store.summary("task-pr-options-refresh")
	if updated == nil || updated.PullRequest == nil || updated.PullRequest.AutoFixEnabled {
		t.Fatalf("updated summary = %+v, want auto-fix disabled", updated)
	}
	if loaderCalls != 2 {
		t.Fatalf("PR loader calls = %d, want initial load plus options refresh", loaderCalls)
	}
}

func TestProjectorPreservesAutomationFlagsAcrossPRRefresh(t *testing.T) {
	store := newProjectorTestStore()
	loaderCalls := 0
	projector := NewProjector(ProjectorConfig{
		Store: store,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		LoadPullRequests: func(context.Context, string) ([]PullRequestInput, error) {
			loaderCalls++
			return []PullRequestInput{{
				Key: "repo-a#41", State: prStateOpen, Number: 41,
				AutoFixEnabled: loaderCalls > 1,
			}}, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 16, 0, 30, 0, 0, time.UTC) },
	})

	prEvent := func() *bus.Event {
		return bus.NewEvent(events.GitHubTaskPRUpdated, "test", map[string]interface{}{
			"task_id": "task-pr-options-preserve", "workspace_id": "workspace-1",
			"repository_id": "repo-a", "state": prStateOpen, "pr_number": 41,
			"pr_url": "https://example.test/41",
		})
	}
	if err := projector.HandleEvent(context.Background(), prEvent()); err != nil {
		t.Fatalf("initial PR refresh: %v", err)
	}
	if err := projector.HandleEvent(context.Background(), bus.NewEvent(events.GitHubTaskCIOptionsUpdated, "test", map[string]interface{}{
		"task_id": "task-pr-options-preserve", "workspace_id": "workspace-1",
	})); err != nil {
		t.Fatalf("CI options refresh: %v", err)
	}
	if err := projector.HandleEvent(context.Background(), prEvent()); err != nil {
		t.Fatalf("subsequent PR refresh: %v", err)
	}

	got := store.summary("task-pr-options-preserve")
	if got == nil || got.PullRequest == nil || !got.PullRequest.AutoFixEnabled {
		t.Fatalf("PR summary after refresh = %+v, want auto-fix preserved", got)
	}
	if loaderCalls != 2 {
		t.Fatalf("PR loader calls = %d, want initial load plus options refresh", loaderCalls)
	}
}

func TestProjectorRehydratesSourceObservationsWithoutPersistedSummary(t *testing.T) {
	store := newProjectorTestStore()
	sessionLoaderCalls := 0
	gitLoaderCalls := 0
	prLoaderCalls := 0
	projector := NewProjector(ProjectorConfig{
		Store: store,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		LoadSessionObservations: func(context.Context, string) (SessionObservationSnapshot, error) {
			sessionLoaderCalls++
			return SessionObservationSnapshot{Sessions: []RebuildSession{{
				ID: "session-primary", State: sessionStateRunning, IsPrimary: true,
			}}}, nil
		},
		LoadGitObservations: func(context.Context, string) ([]GitObservation, error) {
			gitLoaderCalls++
			return []GitObservation{
				{Repository: "repo-a", Summary: GitSummary{Additions: 5, ChangedFiles: 2}},
				{Repository: "repo-b", Summary: GitSummary{Additions: 2, ChangedFiles: 1}},
			}, nil
		},
		LoadPullRequests: func(context.Context, string) ([]PullRequestInput, error) {
			prLoaderCalls++
			return []PullRequestInput{
				{Key: "repo-a#41", State: prStateOpen, Number: 41, URL: "https://example.test/41"},
				{Key: "repo-b#42", State: prStateOpen, Number: 42, URL: "https://example.test/42"},
			}, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 16, 1, 0, 0, 0, time.UTC) },
	})

	err := projector.HandleEvent(context.Background(), bus.NewEvent(events.GitEvent, "test", map[string]interface{}{
		"task_id":      "task-without-summary",
		"workspace_id": "workspace-1",
		"session_id":   "session-primary",
		"type":         "status_update",
		"status": map[string]interface{}{
			"repository_name":  "repo-a",
			"branch_additions": 6,
			"changed_files":    2,
		},
	}))
	if err != nil {
		t.Fatalf("project first source event: %v", err)
	}

	got := store.summary("task-without-summary")
	if got == nil || got.PrimarySession == nil || got.PrimarySession.ID != "session-primary" ||
		got.Git == nil || got.Git.Additions != 8 || got.Git.ChangedFiles != 3 ||
		got.PullRequest == nil || got.PullRequest.Count != 2 {
		t.Fatalf("summary after missing-row hydration = %+v, want all keyed source siblings", got)
	}
	if sessionLoaderCalls != 1 || gitLoaderCalls != 1 || prLoaderCalls != 1 {
		t.Fatalf("loader calls = session:%d git:%d pr:%d, want one each",
			sessionLoaderCalls, gitLoaderCalls, prLoaderCalls)
	}
}
