package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestUpdateSessionStepLink_SkipsStaleWrite guards against the rapid
// A -> B -> C move race: on-enter processing runs in its own goroutine (see
// handleTaskMovedWithSession/processStepExitAndEnter), so a delayed goroutine
// for an intermediate step can finish writing after a later step's goroutine.
// updateSessionStepLink must re-check the task's current step and no-op when
// the task has already moved past the step it was asked to persist.
func TestUpdateSessionStepLink_SkipsStaleWrite(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("skips write when task has already moved to a different step", func(t *testing.T) {
		repo := setupTestRepo(t)
		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		require.NoError(t, repo.CreateWorkspace(ctx, ws))
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		require.NoError(t, repo.CreateWorkflow(ctx, wf))
		task := &models.Task{
			ID: "t1", WorkflowID: "wf1", WorkflowStepID: "stepC",
			Title: "Test", Description: "desc", State: v1.TaskStateInProgress,
			CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreateTask(ctx, task))
		session := &models.TaskSession{
			ID: "s1", TaskID: "t1", State: models.TaskSessionStateRunning,
			WorkflowStepID: "stepA", StartedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreateTaskSession(ctx, session))

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		// A delayed goroutine for the A->B move arrives after the task has
		// already advanced (via a separate, faster B->C move) to stepC.
		svc.updateSessionStepLink(ctx, "t1", session, "stepB")

		require.Equal(t, "stepA", session.WorkflowStepID, "in-memory session must not be rewound to the stale step")
		persisted, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		require.Equal(t, "stepA", persisted.WorkflowStepID, "persisted session must not be rewound to the stale step")
	})

	t.Run("writes when the task is still on the target step", func(t *testing.T) {
		repo := setupTestRepo(t)
		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		require.NoError(t, repo.CreateWorkspace(ctx, ws))
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		require.NoError(t, repo.CreateWorkflow(ctx, wf))
		task := &models.Task{
			ID: "t1", WorkflowID: "wf1", WorkflowStepID: "stepC",
			Title: "Test", Description: "desc", State: v1.TaskStateInProgress,
			CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreateTask(ctx, task))
		session := &models.TaskSession{
			ID: "s1", TaskID: "t1", State: models.TaskSessionStateRunning,
			WorkflowStepID: "stepA", StartedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.CreateTaskSession(ctx, session))

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		svc.updateSessionStepLink(ctx, "t1", session, "stepC")

		require.Equal(t, "stepC", session.WorkflowStepID)
		persisted, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		require.Equal(t, "stepC", persisted.WorkflowStepID)
	})
}
