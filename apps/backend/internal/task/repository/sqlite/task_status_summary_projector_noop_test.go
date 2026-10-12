package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

// @covers AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.2
// @covers AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.3
func TestTaskStatusSummaryProjectorPRNoopWithRetainedError(t *testing.T) {
	repo, db := newTaskStatusSummaryTestRepo(t)
	seedTaskForStatusSummary(t, db, "task-summary-pr-noop", "workspace-summary-pr-noop")
	eventBus := bus.NewMemoryEventBus(logger.Default())
	t.Cleanup(func() { eventBus.Close() })
	publicationCount := 0
	if _, err := eventBus.Subscribe(events.TaskStatusSummaryUpdated, func(context.Context, *bus.Event) error {
		publicationCount++
		return nil
	}); err != nil {
		t.Fatalf("subscribe to summary updates: %v", err)
	}

	loader := func(context.Context, string) (statussummary.SessionObservationSnapshot, error) {
		return statussummary.SessionObservationSnapshot{
			ErrorsObserved: true,
			Sessions: []statussummary.RebuildSession{{
				ID: "session-pr-noop", State: "WAITING_FOR_INPUT", IsPrimary: true,
				ActiveError: &statussummary.ActiveErrorSummary{
					Scope: models.ErrorScopeSession, SessionID: "session-pr-noop",
					Stamp: "retained-error", OccurredAt: time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC),
					Preview: "agent failed",
				},
			}},
		}, nil
	}
	newProjector := func() *statussummary.Projector {
		return statussummary.NewProjector(statussummary.ProjectorConfig{
			Store: repo, EventBus: eventBus, LoadSessionObservations: loader,
			Now: func() time.Time { return time.Date(2026, 9, 17, 12, 1, 0, 0, time.UTC) },
		})
	}
	prEvent := func(checksState string, pendingReviewCount int) *bus.Event {
		return bus.NewEvent(events.GitHubTaskPRUpdated, "test", map[string]interface{}{
			"task_id": "task-summary-pr-noop", "workspace_id": "workspace-summary-pr-noop",
			"repository_id": "repo-a", "state": "open", "pr_number": 41,
			"pr_url": "https://example.test/41", "checks_state": checksState,
			"pending_review_count": pendingReviewCount,
		})
	}

	projector := newProjector()
	if err := projector.HandleEvent(context.Background(), prEvent("failure", 1)); err != nil {
		t.Fatalf("initial PR projection: %v", err)
	}
	first, err := repo.LoadTaskStatusSummaries(context.Background(), []string{"task-summary-pr-noop"})
	if err != nil {
		t.Fatalf("load initial summary: %v", err)
	}
	if err := projector.HandleEvent(context.Background(), prEvent("failure", 2)); err != nil {
		t.Fatalf("aggregate-preserving PR update: %v", err)
	}
	second, err := repo.LoadTaskStatusSummaries(context.Background(), []string{"task-summary-pr-noop"})
	if err != nil {
		t.Fatalf("load no-op summary: %v", err)
	}
	if second["task-summary-pr-noop"].Revision != first["task-summary-pr-noop"].Revision {
		t.Fatalf("no-op changed revision from %d to %d", first["task-summary-pr-noop"].Revision, second["task-summary-pr-noop"].Revision)
	}
	if publicationCount != 1 {
		t.Fatalf("no-op publication count = %d, want initial publication only", publicationCount)
	}

	recreated := newProjector()
	if err := recreated.HandleEvent(context.Background(), prEvent("failure", 3)); err != nil {
		t.Fatalf("recreated projector no-op: %v", err)
	}
	third, err := repo.LoadTaskStatusSummaries(context.Background(), []string{"task-summary-pr-noop"})
	if err != nil {
		t.Fatalf("load recreated no-op summary: %v", err)
	}
	if third["task-summary-pr-noop"].Revision != first["task-summary-pr-noop"].Revision {
		t.Fatalf("recreated no-op changed revision to %d", third["task-summary-pr-noop"].Revision)
	}
	if publicationCount != 1 {
		t.Fatalf("recreated no-op publication count = %d, want 1", publicationCount)
	}

	if err := recreated.HandleEvent(context.Background(), prEvent("success", 0)); err != nil {
		t.Fatalf("real PR aggregate change: %v", err)
	}
	fourth, err := repo.LoadTaskStatusSummaries(context.Background(), []string{"task-summary-pr-noop"})
	if err != nil {
		t.Fatalf("load changed summary: %v", err)
	}
	if fourth["task-summary-pr-noop"].Revision != first["task-summary-pr-noop"].Revision+1 {
		t.Fatalf("real change revision = %d, want %d", fourth["task-summary-pr-noop"].Revision, first["task-summary-pr-noop"].Revision+1)
	}
	if fourth["task-summary-pr-noop"].PullRequest == nil || fourth["task-summary-pr-noop"].PullRequest.AggregateState != "passing" {
		t.Fatalf("real change PR summary = %+v, want passing aggregate", fourth["task-summary-pr-noop"].PullRequest)
	}
	if fourth["task-summary-pr-noop"].ActiveError == nil || fourth["task-summary-pr-noop"].ActiveError.Stamp != "retained-error" {
		t.Fatalf("real change dropped retained error: %+v", fourth["task-summary-pr-noop"].ActiveError)
	}
	if publicationCount != 2 {
		t.Fatalf("real change publication count = %d, want 2", publicationCount)
	}
	if err := recreated.HandleEvent(context.Background(), prEvent("success", 0)); err != nil {
		t.Fatalf("replayed real PR state: %v", err)
	}
	if publicationCount != 2 {
		t.Fatalf("replayed real change publication count = %d, want 2", publicationCount)
	}
}
