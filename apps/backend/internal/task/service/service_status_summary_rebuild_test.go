package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type statusSummaryRebuildPRReader struct {
	calls int
}

func (r *statusSummaryRebuildPRReader) ListTaskStatusSummaryPullRequests(
	context.Context,
	[]string,
) (map[string][]statussummary.PullRequestInput, error) {
	r.calls++
	return map[string][]statussummary.PullRequestInput{
		"task-1": {{
			Key:         "repo-pr-1",
			State:       "open",
			Number:      42,
			URL:         "https://github.test/pr/42",
			ReviewState: "approved",
			ChecksState: "success",
		}},
	}, nil
}

type statusSummaryRebuildActivityProvider struct{}

func (statusSummaryRebuildActivityProvider) ForegroundActivity(string) v1.ForegroundActivity {
	return v1.ForegroundActivityGenerating
}

func (statusSummaryRebuildActivityProvider) ActiveSubagentCount(string) int { return 3 }

func TestHydrateMissingTaskStatusSummariesRepairsExistingTaskOnce(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)

	now := time.Date(2026, 8, 1, 18, 0, 0, 0, time.UTC)
	session := &models.TaskSession{
		ID:        "session-1",
		TaskID:    "task-1",
		State:     models.TaskSessionStateRunning,
		IsPrimary: true,
		Metadata: map[string]interface{}{
			models.SessionMetaKeyLastAgentError: map[string]interface{}{
				"message":     "agent failed to complete the turn",
				"occurred_at": now.Add(-time.Minute).Format(time.RFC3339Nano),
			},
		},
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID:           "snapshot-1",
		SessionID:    session.ID,
		SnapshotType: models.SnapshotTypeStatusUpdate,
		Ahead:        2,
		Behind:       1,
		Files:        map[string]interface{}{"a.go": map[string]interface{}{}, "b.go": map[string]interface{}{}},
		TriggeredBy:  "agent_completed",
		Metadata: map[string]interface{}{
			"repository_name":  "kandev",
			"branch_additions": 5,
			"branch_deletions": 2,
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateGitSnapshot: %v", err)
	}

	// createTestService predates the status-summary repository field, so wire
	// it explicitly here while keeping the shared fixture unchanged.
	svc.statusSummaries = repo
	svc.SetForegroundActivityProvider(statusSummaryRebuildActivityProvider{})
	prReader := &statusSummaryRebuildPRReader{}
	svc.SetTaskStatusSummaryPRReader(prReader)
	task := &models.Task{ID: "task-1", WorkspaceID: "ws-1"}
	sessions := map[string][]*models.TaskSession{task.ID: {session}}
	pending := map[string]models.TaskPendingAction{session.ID: models.TaskPendingActionPermission}

	got, err := svc.HydrateMissingTaskStatusSummaries(
		ctx, []*models.Task{task}, sessions, pending, map[string]*statussummary.TaskStatusSummary{},
	)
	if err != nil {
		t.Fatalf("HydrateMissingTaskStatusSummaries: %v", err)
	}
	summary := got[task.ID]
	if summary == nil {
		t.Fatal("repaired summary missing from returned map")
	}
	if summary.Revision != 1 || summary.PrimarySession == nil || summary.PrimarySession.ID != session.ID {
		t.Fatalf("repaired summary identity = %+v", summary)
	}
	if summary.PendingAction != string(models.TaskPendingActionPermission) {
		t.Fatalf("pending action = %q", summary.PendingAction)
	}
	if summary.ForegroundActivity != "generating" || summary.ActiveSubagentCount != 3 {
		t.Fatalf("activity summary = %+v", summary)
	}
	if summary.ActiveError == nil || summary.ActiveError.Preview != "agent failed to complete the turn" {
		t.Fatalf("active error = %+v", summary.ActiveError)
	}
	if summary.Git == nil || summary.Git.Additions != 5 || summary.Git.Deletions != 2 || summary.Git.ChangedFiles != 2 {
		t.Fatalf("Git summary = %+v", summary.Git)
	}
	if summary.PullRequest == nil || summary.PullRequest.Number != 42 || summary.PullRequest.AggregateState != "ready" {
		t.Fatalf("pull request summary = %+v", summary.PullRequest)
	}
	if prReader.calls != 1 {
		t.Fatalf("PR reader calls = %d, want one batch read", prReader.calls)
	}

	persisted, err := repo.LoadTaskStatusSummaries(ctx, []string{task.ID})
	if err != nil {
		t.Fatalf("LoadTaskStatusSummaries: %v", err)
	}
	if persisted[task.ID] == nil || persisted[task.ID].Revision != 1 {
		t.Fatalf("persisted summary = %+v", persisted[task.ID])
	}

	if _, err := svc.HydrateMissingTaskStatusSummaries(ctx, []*models.Task{task}, sessions, pending, got); err != nil {
		t.Fatalf("second HydrateMissingTaskStatusSummaries: %v", err)
	}
	if prReader.calls != 1 {
		t.Fatalf("PR reader calls after existing summary = %d, want one", prReader.calls)
	}
}
