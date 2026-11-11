package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/completion"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type refusingCompletionPolicy struct{ refusal *completion.RefusedError }

func (p refusingCompletionPolicy) AdmitCompletion(
	context.Context,
	completion.Querier,
	*models.Task,
	*models.Task,
	completion.Admission,
) error {
	return p.refusal
}

func TestPolicyRefusalLeavesTaskUnchanged(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "completion-admission-ws", Name: "Office"}))
	workflowID, err := repo.EnsureOfficeWorkflow(ctx, "completion-admission-ws")
	require.NoError(t, err)
	var stepID string
	require.NoError(t, repo.db.QueryRowContext(ctx, repo.db.Rebind(`
		SELECT id FROM workflow_steps WHERE workflow_id = ? AND complete_task_on_enter = 1 ORDER BY position LIMIT 1
	`), workflowID).Scan(&stepID))

	task := &models.Task{
		ID: "completion-admission-task", WorkspaceID: "completion-admission-ws",
		WorkflowID: workflowID, WorkflowStepID: stepID, Title: "Completion admission",
		State: v1.TaskStateInProgress,
	}
	require.NoError(t, repo.CreateTask(ctx, task))
	current, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	candidate := *current
	candidate.State = v1.TaskStateCompleted

	refusal := &completion.RefusedError{Code: "test_refusal", Subjects: []string{"subject-1"}}
	repo.SetCompletionPolicies(refusingCompletionPolicy{refusal: refusal})
	admission := &completion.Admission{
		TaskID: task.ID, Actor: wfmodels.StepTransitionActorHuman, Kind: completion.SetState,
		Expected: completion.Scope{
			WorkspaceID: current.WorkspaceID, WorkflowID: current.WorkflowID,
			StepID: current.WorkflowStepID, TaskState: string(current.State),
		},
	}
	err = repo.UpdateTaskWithCompletionAdmission(ctx, &candidate, admission)
	require.Error(t, err)
	var refused *completion.RefusedError
	require.True(t, errors.As(err, &refused))
	require.Equal(t, refusal.Code, refused.Code)
	require.Equal(t, refusal.Subjects, refused.Subjects)

	stored, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, current.WorkflowStepID, stored.WorkflowStepID)
	require.Equal(t, current.State, stored.State)
}
