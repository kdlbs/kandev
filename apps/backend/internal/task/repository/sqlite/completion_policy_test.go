package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/completion"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
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

func TestWorkflowMoveRechecksCompletingStepAtCommit(t *testing.T) {
	for _, tc := range []struct {
		name               string
		databaseCompleting bool
		candidateState     v1.TaskState
	}{
		{name: "step became completing after preflight", databaseCompleting: true, candidateState: v1.TaskStateInProgress},
		{name: "step stopped completing after preflight", databaseCompleting: false, candidateState: v1.TaskStateCompleted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepoForBuiltinWorkflowTests(t)
			ctx := context.Background()
			require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "completion-move-ws", Name: "Office"}))
			workflowID, err := repo.EnsureOfficeWorkflow(ctx, "completion-move-ws")
			require.NoError(t, err)

			var sourceStepID, targetStepID string
			require.NoError(t, repo.db.QueryRowContext(ctx, repo.db.Rebind(`
				SELECT id FROM workflow_steps WHERE workflow_id = ? ORDER BY position ASC LIMIT 1
			`), workflowID).Scan(&sourceStepID))
			require.NoError(t, repo.db.QueryRowContext(ctx, repo.db.Rebind(`
				SELECT id FROM workflow_steps WHERE workflow_id = ? ORDER BY position DESC LIMIT 1
			`), workflowID).Scan(&targetStepID))
			completeOnEnter := 0
			if tc.databaseCompleting {
				completeOnEnter = 1
			}
			_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
				UPDATE workflow_steps SET complete_task_on_enter = ? WHERE id = ?
			`), completeOnEnter, targetStepID)
			require.NoError(t, err)

			current := &models.Task{
				ID: "completion-move-task", WorkspaceID: "completion-move-ws", WorkflowID: workflowID,
				WorkflowStepID: sourceStepID, Title: "Move admission", State: v1.TaskStateInProgress,
			}
			require.NoError(t, repo.CreateTask(ctx, current))
			current, err = repo.GetTask(ctx, current.ID)
			require.NoError(t, err)
			candidate := *current
			candidate.WorkflowStepID = targetStepID
			candidate.State = tc.candidateState
			admission := &completion.Admission{
				TaskID: current.ID, Actor: wfmodels.StepTransitionActorHuman, Kind: completion.Move,
				Expected: completion.Scope{
					WorkspaceID: current.WorkspaceID, WorkflowID: current.WorkflowID,
					StepID: current.WorkflowStepID, TaskState: string(current.State),
				},
			}
			state := tc.candidateState
			_, err = repo.UpdateTaskWithWorkflowStepAdmissionAndStateWithCompletionAdmission(
				ctx, &candidate, sourceStepID, targetStepID, 0, &state, false, workflowID, admission,
			)
			require.ErrorIs(t, err, repoerrors.ErrWorkflowChangeConflict)

			stored, readErr := repo.GetTask(ctx, current.ID)
			require.NoError(t, readErr)
			require.Equal(t, sourceStepID, stored.WorkflowStepID)
			require.Equal(t, current.State, stored.State)
		})
	}
}

func TestFinalStepDoneRunsRequiredAssignmentGuard(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "final-done-ws", Name: "Office"}))
	workflowID, err := repo.EnsureOfficeWorkflow(ctx, "final-done-ws")
	require.NoError(t, err)
	var finalStepID string
	require.NoError(t, repo.db.QueryRowContext(ctx, repo.db.Rebind(`
		SELECT id FROM workflow_steps WHERE workflow_id = ? ORDER BY position DESC LIMIT 1
	`), workflowID).Scan(&finalStepID))
	task := &models.Task{
		ID: "final-done-task", WorkspaceID: "final-done-ws", WorkflowID: workflowID,
		WorkflowStepID: finalStepID, Title: "Pending child assignment", State: v1.TaskStateInProgress,
		Metadata: map[string]interface{}{models.MetaKeyRequiredSubtaskCount: 1},
	}
	require.NoError(t, repo.CreateTask(ctx, task))
	current, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	admission := &completion.Admission{
		TaskID: current.ID, Actor: wfmodels.StepTransitionActorHuman, Kind: completion.FinalStepDone,
		Expected: completion.Scope{
			WorkspaceID: current.WorkspaceID, WorkflowID: current.WorkflowID,
			StepID: current.WorkflowStepID, TaskState: string(current.State),
		},
	}
	_, _, _, err = repo.CompleteTaskOnFinalStep(ctx, current.ID, admission)
	require.ErrorIs(t, err, repoerrors.ErrTaskCompletionGateBlocked)

	stored, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateInProgress, stored.State)
}
