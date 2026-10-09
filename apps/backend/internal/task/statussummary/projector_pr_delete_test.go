package statussummary

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestProjectorRefreshesPRSummaryAfterAssociationDelete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newProjectorTestStore()
	eventBus := bus.NewMemoryEventBus(logger.Default())
	defer eventBus.Close()
	prs := []PullRequestInput{{Key: "repo-1#42", State: "open", Number: 42}}
	newProjector := func() *Projector {
		return NewProjector(ProjectorConfig{
			Store:    store,
			EventBus: eventBus,
			ResolveWorkspace: func(context.Context, string) (string, error) {
				return "workspace-1", nil
			},
			LoadPullRequests: func(context.Context, string) ([]PullRequestInput, error) {
				return append([]PullRequestInput(nil), prs...), nil
			},
			Now: func() time.Time { return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC) },
		})
	}
	projector := newProjector()
	if err := projector.Start(ctx); err != nil {
		t.Fatalf("start projector: %v", err)
	}
	defer projector.Close()

	const taskID = "task-pr-delete"
	publishProjectorEvent(t, eventBus, events.TaskCreated, events.TaskCreated, map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": "workspace-1",
		"created_at":   "2026-09-26T11:00:00Z",
		"updated_at":   "2026-09-26T11:00:00Z",
	})
	if summary := store.summary(taskID); summary == nil || summary.PullRequest == nil || summary.PullRequest.Count != 1 {
		t.Fatalf("initial pull request summary = %+v, want one linked PR", summary)
	}

	projector.Close()
	prs = nil
	projector = newProjector()
	if err := projector.Start(ctx); err != nil {
		t.Fatalf("restart projector: %v", err)
	}
	defer projector.Close()
	publishProjectorEvent(t, eventBus, events.GitHubTaskPRDeleted, events.GitHubTaskPRDeleted, map[string]interface{}{
		"task_id":        taskID,
		"workspace_id":   "workspace-1",
		"association_id": "association-1",
	})

	if summary := store.summary(taskID); summary == nil || summary.PullRequest != nil {
		t.Fatalf("pull request summary after final unlink = %+v, want no PR summary", summary)
	}
}
