package service

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestBulkMovePreflightsCompletionGateEvenWhenStateStaysTerminal(t *testing.T) {
	ctx := t.Context()
	svc, _, repo := createTestService(t)
	seedMoveWorkflows(t, ctx, repo)
	seedBulkCompletingTarget(t, ctx, svc, repo, func(string) bool { return false })
	createMoveTask(t, ctx, repo, "task-a", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-b", "wf-source", "step-source", nil)
	require.NoError(t, repo.UpdateTaskState(ctx, "task-b", v1.TaskStateCancelled))
	blocked, err := repo.GetTask(ctx, "task-b")
	require.NoError(t, err)
	_, err = repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID: blocked.ID, WorkspaceID: blocked.WorkspaceID, ActorKind: "human", ActorID: "reviewer",
		Criteria: []models.TaskCompletionCriterion{{
			ID: "review", Description: "Review the task",
			EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceTaskRevision, ID: blocked.ID},
		}},
	})
	require.NoError(t, err)

	_, err = svc.BulkMoveSelectedTasks(ctx, []string{"task-a", blocked.ID}, "wf-target", "step-target", wfmodels.StepTransitionActorHuman)
	var refused *BulkMoveRefusedError
	require.ErrorAs(t, err, &refused)
	require.Len(t, refused.Refusals, 1)
	require.Equal(t, blocked.ID, refused.Refusals[0].TaskID)
	require.Equal(t, "task_completion_gate_blocked", refused.Refusals[0].Code)
	for _, id := range []string{"task-a", blocked.ID} {
		task, err := repo.GetTask(ctx, id)
		require.NoError(t, err)
		require.Equal(t, "step-source", task.WorkflowStepID)
	}
}
