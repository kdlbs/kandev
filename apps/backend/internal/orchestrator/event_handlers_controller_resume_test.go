package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestHandleTaskStateChanged_QueuesResumeForOptedInParentOnReview(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "")

	parent, err := repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent: %v", err)
	}
	parent.Metadata = map[string]interface{}{"controller_resume_enabled": true}
	if err := repo.UpdateTask(ctx, parent); err != nil {
		t.Fatalf("enable controller resume: %v", err)
	}

	now := time.Now().UTC()
	requireCreateTask(t, repo, &models.Task{
		ID:          "child",
		WorkspaceID: "ws1",
		Title:       "Reviewable child",
		State:       v1.TaskStateInProgress,
		ParentID:    "parent",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	oldState := v1.TaskStateInProgress
	newState := v1.TaskStateReview
	svc.handleTaskStateChanged(ctx, watcher.TaskEventData{
		TaskID:   "child",
		OldState: &oldState,
		NewState: &newState,
	})

	if got := svc.messageQueue.GetStatus(ctx, "parent-session").Count; got != 1 {
		t.Fatalf("queued controller resume wakes = %d, want 1", got)
	}
}

func TestHandleTaskStateChanged_DoesNotResumeParentWithoutOptIn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "")

	now := time.Now().UTC()
	requireCreateTask(t, repo, &models.Task{
		ID:          "child",
		WorkspaceID: "ws1",
		Title:       "Reviewable child",
		State:       v1.TaskStateInProgress,
		ParentID:    "parent",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	oldState := v1.TaskStateInProgress
	newState := v1.TaskStateReview
	svc.handleTaskStateChanged(ctx, watcher.TaskEventData{TaskID: "child", OldState: &oldState, NewState: &newState})

	if got := svc.messageQueue.GetStatus(ctx, "parent-session").Count; got != 0 {
		t.Fatalf("queued controller resume wakes = %d, want 0 without opt-in", got)
	}
}

func TestHandleTaskStateChanged_CoalescesControllerResumeAcrossChildren(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "")

	parent, err := repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent: %v", err)
	}
	parent.Metadata = map[string]interface{}{"controller_resume_enabled": true}
	if err := repo.UpdateTask(ctx, parent); err != nil {
		t.Fatalf("enable controller resume: %v", err)
	}

	now := time.Now().UTC()
	for _, childID := range []string{"child-1", "child-2"} {
		requireCreateTask(t, repo, &models.Task{
			ID:          childID,
			WorkspaceID: "ws1",
			Title:       childID,
			State:       v1.TaskStateInProgress,
			ParentID:    "parent",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	oldState := v1.TaskStateInProgress
	newState := v1.TaskStateReview
	for _, childID := range []string{"child-1", "child-2"} {
		svc.handleTaskStateChanged(ctx, watcher.TaskEventData{TaskID: childID, OldState: &oldState, NewState: &newState})
	}

	status := svc.messageQueue.GetStatus(ctx, "parent-session")
	if status.Count != 1 {
		t.Fatalf("queued controller resume wakes = %d, want one coalesced wake", status.Count)
	}
	if got := status.Entries[0].Metadata["child_task_id"]; got != "child-2" {
		t.Fatalf("coalesced wake child_task_id = %v, want child-2", got)
	}
}
