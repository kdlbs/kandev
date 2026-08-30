package service

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
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

// TestService_MoveTaskEventsUseTransactionalSourceWorkflow covers a stale
// pre-read that turns a same-step request into a real cross-workflow write.
// The event payload must use the source workflow read by that write
// transaction, not the workflow from the earlier service snapshot.
func TestService_MoveTaskEventsUseTransactionalSourceWorkflow(t *testing.T) {
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
	require.NoError(t, err)

	var updatedData map[string]interface{}
	var movedData map[string]interface{}
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type == events.TaskUpdated {
			updatedData, _ = event.Data.(map[string]interface{})
		}
		if event.Type == events.TaskMoved {
			movedData, _ = event.Data.(map[string]interface{})
		}
	}
	require.Equal(t, "wf-target", updatedData["old_workflow_id"],
		"task.updated must identify the workflow observed by the committing write")
	require.Equal(t, "wf-target", movedData["from_workflow_id"],
		"task.moved must identify the workflow observed by the committing write")
}
