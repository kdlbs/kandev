package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"
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

func TestWorkflowSessionBindingAllowsNewerSessionWithinSameEntry(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-workflow-binding-same-entry"
	const targetKey = "step:implement"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Same entry"}))
	first := &models.TaskSession{ID: "session-same-entry-first", TaskID: taskID, AgentProfileID: "profile-a", State: models.TaskSessionStateRunning}
	second := &models.TaskSession{ID: "session-same-entry-second", TaskID: taskID, AgentProfileID: "profile-a", State: models.TaskSessionStateRunning}
	require.NoError(t, repo.CreateTaskSession(ctx, first))
	require.NoError(t, repo.CreateTaskSession(ctx, second))

	entryOperation := "workflow-step-entry-v2:entry:00000000000000000021"
	firstStamp := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	secondStamp := firstStamp.Add(time.Second)
	accepted, err := repo.UpsertWorkflowSessionBinding(ctx, &models.WorkflowSessionBinding{
		TaskID: taskID, TargetKey: targetKey, WorkflowID: "workflow-same-entry", AgentProfileID: "profile-a",
		SessionID: first.ID, OperationID: entryOperation, UpdatedAt: firstStamp,
	})
	require.NoError(t, err)
	require.True(t, accepted)
	accepted, err = repo.UpsertWorkflowSessionBinding(ctx, &models.WorkflowSessionBinding{
		TaskID: taskID, TargetKey: targetKey, WorkflowID: "workflow-same-entry", AgentProfileID: "profile-a",
		SessionID: second.ID, OperationID: entryOperation, UpdatedAt: secondStamp,
	})
	require.NoError(t, err)
	require.True(t, accepted)

	binding, err := repo.GetWorkflowSessionBinding(ctx, taskID, targetKey)
	require.NoError(t, err)
	require.Equal(t, second.ID, binding.SessionID)
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

func TestDeleteWorkflowRemovesOnlyItsStepsAndBindings(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const (
		workspaceID = "ws-workflow-step-delete"
		taskID      = "task-workflow-step-delete"
	)
	seedWorkspace(t, repo, workspaceID)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Step delete"}))
	for _, workflow := range []*models.Workflow{
		{ID: "wf-delete-steps-a", WorkspaceID: workspaceID, Name: "A"},
		{ID: "wf-delete-steps-b", WorkspaceID: workspaceID, Name: "B"},
	} {
		require.NoError(t, repo.CreateWorkflow(ctx, workflow))
	}
	now := time.Now().UTC()
	for _, binding := range []*models.WorkflowSessionBinding{
		{TaskID: taskID, TargetKey: "step:a", WorkflowID: "wf-delete-steps-a", AgentProfileID: "profile", OperationID: "op-a", UpdatedAt: now},
		{TaskID: taskID, TargetKey: "step:b", WorkflowID: "wf-delete-steps-b", AgentProfileID: "profile", OperationID: "op-b", UpdatedAt: now},
	} {
		accepted, err := repo.UpsertWorkflowSessionBinding(ctx, binding)
		require.NoError(t, err)
		require.True(t, accepted)
	}
	insertWorkflowDeleteTestStep(t, repo, "step-delete-a1", "wf-delete-steps-a")
	insertWorkflowDeleteTestStep(t, repo, "step-delete-a2", "wf-delete-steps-a")
	insertWorkflowDeleteTestStep(t, repo, "step-delete-b1", "wf-delete-steps-b")

	require.NoError(t, repo.DeleteWorkflow(ctx, "wf-delete-steps-a"))
	require.Equal(t, 0, countWorkflowDeleteTestSteps(t, repo, "wf-delete-steps-a"))
	require.Equal(t, 1, countWorkflowDeleteTestSteps(t, repo, "wf-delete-steps-b"))
	deletedBinding, err := repo.GetWorkflowSessionBinding(ctx, taskID, "step:a")
	require.NoError(t, err)
	require.Nil(t, deletedBinding)
	retainedBinding, err := repo.GetWorkflowSessionBinding(ctx, taskID, "step:b")
	require.NoError(t, err)
	require.NotNil(t, retainedBinding)
	_, err = repo.GetWorkflow(ctx, "wf-delete-steps-b")
	require.NoError(t, err)
}

func TestDeleteWorkflowMissingRollsBackStepAndBindingDeletes(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const (
		workspaceID = "ws-workflow-step-delete-rollback"
		taskID      = "task-workflow-step-delete-rollback"
		workflowID  = "wf-delete-missing"
	)
	seedWorkspace(t, repo, workspaceID)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Rollback"}))
	insertWorkflowDeleteTestStep(t, repo, "step-delete-orphan", workflowID)
	accepted, err := repo.UpsertWorkflowSessionBinding(ctx, &models.WorkflowSessionBinding{
		TaskID: taskID, TargetKey: "step:missing", WorkflowID: workflowID,
		AgentProfileID: "profile", OperationID: "op-missing", UpdatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	require.True(t, accepted)

	require.Error(t, repo.DeleteWorkflow(ctx, workflowID))
	require.Equal(t, 1, countWorkflowDeleteTestSteps(t, repo, workflowID))
	binding, err := repo.GetWorkflowSessionBinding(ctx, taskID, "step:missing")
	require.NoError(t, err)
	require.NotNil(t, binding)
}

func TestDeleteWorkflowsByWorkspaceRemovesOnlySelectedSteps(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const (
		doomedWorkspace = "ws-workflow-bulk-delete"
		otherWorkspace  = "ws-workflow-bulk-keep"
	)
	seedWorkspace(t, repo, doomedWorkspace)
	seedWorkspace(t, repo, otherWorkspace)
	for _, workflow := range []*models.Workflow{
		{ID: "wf-bulk-delete-a", WorkspaceID: doomedWorkspace, Name: "A"},
		{ID: "wf-bulk-delete-b", WorkspaceID: doomedWorkspace, Name: "B"},
		{ID: "wf-bulk-excluded", WorkspaceID: doomedWorkspace, Name: "Excluded"},
		{ID: "wf-bulk-other-workspace", WorkspaceID: otherWorkspace, Name: "Other"},
	} {
		require.NoError(t, repo.CreateWorkflow(ctx, workflow))
		insertWorkflowDeleteTestStep(t, repo, "step-"+workflow.ID, workflow.ID)
	}

	deleted, err := repo.DeleteWorkflowsByWorkspace(ctx, doomedWorkspace, []string{"wf-bulk-excluded"})
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted)
	for _, workflowID := range []string{"wf-bulk-delete-a", "wf-bulk-delete-b"} {
		require.Equal(t, 0, countWorkflowDeleteTestSteps(t, repo, workflowID))
	}
	for _, workflowID := range []string{"wf-bulk-excluded", "wf-bulk-other-workspace"} {
		require.Equal(t, 1, countWorkflowDeleteTestSteps(t, repo, workflowID))
	}
}

func insertWorkflowDeleteTestStep(t *testing.T, repo *Repository, stepID, workflowID string) {
	t.Helper()
	_, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(`
		INSERT INTO workflow_steps (id, workflow_id, name, position)
		VALUES (?, ?, ?, 0)
	`), stepID, workflowID, stepID)
	require.NoError(t, err)
}

func countWorkflowDeleteTestSteps(t *testing.T, repo *Repository, workflowID string) int {
	t.Helper()
	var count int
	require.NoError(t, repo.db.Get(&count, repo.db.Rebind(`
		SELECT COUNT(*) FROM workflow_steps WHERE workflow_id = ?
	`), workflowID))
	return count
}

func TestCreateStepForMissingWorkflowIsRejected(t *testing.T) {
	repo := newRepoForEntityTests(t)
	workflowRepo, err := workflowrepository.NewWithDB(repo.db, repo.db, nil)
	require.NoError(t, err)

	err = workflowRepo.CreateStep(context.Background(), &workflowmodels.WorkflowStep{
		ID: "step-missing-workflow", WorkflowID: "wf-missing", Name: "Missing", Position: 0,
	})
	require.ErrorIs(t, err, repoerrors.ErrWorkflowNotFound)
}
