package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestWorkflowSessionBindingRejectsStaleOperationAndRetainsProfileAfterSessionDelete(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID     = "task-workflow-binding"
		workflowID = "workflow-workflow-binding"
		targetKey  = "step:implement"
	)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Binding"}))
	session := &models.TaskSession{
		ID:             "workflow-binding-session",
		TaskID:         taskID,
		AgentProfileID: "profile-implement",
		State:          models.TaskSessionStateWaitingForInput,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, session))

	newer := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	accepted, err := repo.UpsertWorkflowSessionBinding(ctx, &models.WorkflowSessionBinding{
		TaskID:         taskID,
		TargetKey:      targetKey,
		WorkflowID:     workflowID,
		AgentProfileID: session.AgentProfileID,
		SessionID:      session.ID,
		OperationID:    "operation-new",
		UpdatedAt:      newer,
	})
	require.NoError(t, err)
	require.True(t, accepted)

	accepted, err = repo.UpsertWorkflowSessionBinding(ctx, &models.WorkflowSessionBinding{
		TaskID:         taskID,
		TargetKey:      targetKey,
		WorkflowID:     workflowID,
		AgentProfileID: session.AgentProfileID,
		SessionID:      "stale-session",
		OperationID:    "operation-stale",
		UpdatedAt:      newer.Add(-time.Second),
	})
	require.NoError(t, err)
	require.False(t, accepted)

	binding, err := repo.GetWorkflowSessionBinding(ctx, taskID, targetKey)
	require.NoError(t, err)
	require.Equal(t, session.ID, binding.SessionID)
	require.Equal(t, "operation-new", binding.OperationID)

	require.NoError(t, repo.DeleteTaskSession(ctx, session))
	binding, err = repo.GetWorkflowSessionBinding(ctx, taskID, targetKey)
	require.NoError(t, err)
	require.NotNil(t, binding)
	require.Empty(t, binding.SessionID)
	require.Equal(t, session.AgentProfileID, binding.AgentProfileID)
}

func TestWorkflowSessionBindingRejectsEqualTimestampReplacement(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID     = "task-workflow-binding-equal"
		workflowID = "workflow-workflow-binding-equal"
		targetKey  = "step:review"
	)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Equal timestamp"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-first", TaskID: taskID, AgentProfileID: "profile-review",
		State: models.TaskSessionStateWaitingForInput,
	}))
	stamp := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

	accepted, err := repo.UpsertWorkflowSessionBinding(ctx, &models.WorkflowSessionBinding{
		TaskID:         taskID,
		TargetKey:      targetKey,
		WorkflowID:     workflowID,
		AgentProfileID: "profile-review",
		SessionID:      "session-first",
		OperationID:    "operation-first",
		UpdatedAt:      stamp,
	})
	require.NoError(t, err)
	require.True(t, accepted)

	accepted, err = repo.UpsertWorkflowSessionBinding(ctx, &models.WorkflowSessionBinding{
		TaskID:         taskID,
		TargetKey:      targetKey,
		WorkflowID:     workflowID,
		AgentProfileID: "profile-review",
		SessionID:      "session-equal",
		OperationID:    "operation-equal",
		UpdatedAt:      stamp,
	})
	require.NoError(t, err)
	require.False(t, accepted)

	binding, err := repo.GetWorkflowSessionBinding(ctx, taskID, targetKey)
	require.NoError(t, err)
	require.Equal(t, "session-first", binding.SessionID)
	require.Equal(t, "operation-first", binding.OperationID)
}

func TestWorkflowSessionBindingsCascadeWithTask(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-workflow-binding-cascade"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Binding cascade"}))
	accepted, err := repo.UpsertWorkflowSessionBinding(ctx, &models.WorkflowSessionBinding{
		TaskID:         taskID,
		TargetKey:      "step:implement",
		WorkflowID:     "workflow-cascade",
		AgentProfileID: "profile-implement",
		OperationID:    "operation-cascade",
	})
	require.NoError(t, err)
	require.True(t, accepted)

	require.NoError(t, repo.DeleteTask(ctx, taskID))
	binding, err := repo.GetWorkflowSessionBinding(ctx, taskID, "step:implement")
	require.NoError(t, err)
	require.Nil(t, binding)
}
