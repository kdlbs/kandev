package service

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type moveTaskUpdateHookRepository struct {
	*sqliterepo.Repository
	beforeUpdate func()
	updateOnce   sync.Once
}

func (r *moveTaskUpdateHookRepository) UpdateTask(ctx context.Context, task *models.Task) error {
	r.updateOnce.Do(func() {
		if r.beforeUpdate != nil {
			r.beforeUpdate()
		}
	})
	return r.Repository.UpdateTask(ctx, task)
}

func (r *moveTaskUpdateHookRepository) UpdateTaskIfWorkflowStepMatches(
	ctx context.Context,
	task *models.Task,
	expectedStepID, expectedWorkflowID string,
) error {
	r.updateOnce.Do(func() {
		if r.beforeUpdate != nil {
			r.beforeUpdate()
		}
	})
	return r.Repository.UpdateTaskIfWorkflowStepMatches(ctx, task, expectedStepID, expectedWorkflowID)
}

// TestService_MoveTaskEventsUseTransactionalSourceWorkflow covers a stale
// pre-read that turns a same-step request into a real cross-workflow write.
// The event payload must use the source workflow read by that write
// transaction, not the workflow from the earlier service snapshot.
func TestService_MoveTaskSameStepRequestRejectsConcurrentWorkflowRoute(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-event-race", "wf-source", "step-source", nil)
	eventBus.ClearEvents()

	var injectedErr error
	svc.tasks = &moveTaskUpdateHookRepository{
		Repository: repo,
		beforeUpdate: func() {
			_, injectedErr = svc.MoveTaskWithOptions(
				ctx, "task-event-race", "wf-target", "step-target", 0, MoveTaskOptions{},
			)
		},
	}

	_, err := svc.MoveTaskWithOptions(
		pluginMoveContext("plugin:acme"), "task-event-race", "wf-source", "step-source", 0,
		pluginMoveOptions(),
	)
	require.NoError(t, injectedErr)
	require.Error(t, err)
	stored, loadErr := repo.GetTask(ctx, "task-event-race")
	require.NoError(t, loadErr)
	require.Equal(t, "wf-target", stored.WorkflowID)
	require.Equal(t, "step-target", stored.WorkflowStepID)
}
