package repository

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/workflow/models"
)

type workspaceStepCopier interface {
	CopyWorkspaceStepsTx(context.Context, *sqlx.Tx, map[string]string) ([]*models.WorkflowStep, error)
}

func TestWorkspaceCloneSteps(t *testing.T) {
	repo, database := setupTestRepoWithDB(t)
	ctx := t.Context()
	_, err := database.Exec(`DELETE FROM workflow_steps WHERE workflow_id = 'wf-test'`)
	require.NoError(t, err)
	first := &models.WorkflowStep{ID: "first", WorkflowID: "wf-test", Name: "First", Position: 0, IsStartStep: true, Prompt: "plan", Color: "blue", AllowManualMove: true, ShowInCommandPanel: true}
	last := &models.WorkflowStep{ID: "last", WorkflowID: "wf-test", Name: "Last", Position: 1, Prompt: "finish", AgentProfileID: "profile", WIPLimit: 4, PullFromStepID: first.ID, SessionTarget: &models.WorkflowSessionTarget{Kind: models.WorkflowSessionTargetStep, StepID: first.ID}, StageType: models.StageTypeReview, AutoArchiveAfterHours: 24, AutoAdvanceRequiresSignal: true, CancelTriggersTurnComplete: true, CompleteTaskOnEnter: true, DisableUnclassifiedFallback: true, Events: models.StepEvents{OnTurnComplete: []models.OnTurnCompleteAction{{Type: models.OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": first.ID}}}, OnComment: []models.GenericAction{{Type: models.GenericActionQueueRun, Config: map[string]any{"task_id": "this"}}}}}
	require.NoError(t, repo.CreateStep(ctx, first))
	require.NoError(t, repo.CreateStep(ctx, last))
	_, err = database.Exec(`INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES ('wf-copy', '', 'Copy', datetime('now'), datetime('now'))`)
	require.NoError(t, err)
	copier, ok := any(repo).(workspaceStepCopier)
	require.True(t, ok, "copied steps must remap references within the cloned graph")
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	steps, err := copier.CopyWorkspaceStepsTx(ctx, tx, map[string]string{"wf-test": "wf-copy"})
	require.NoError(t, err)
	require.Len(t, steps, 2)
	require.NoError(t, tx.Commit())
	stored, err := repo.ListStepsByWorkflow(ctx, "wf-copy")
	require.NoError(t, err)
	require.Len(t, stored, 2)
	require.NotEqual(t, first.ID, stored[0].ID)
	require.NotEqual(t, last.ID, stored[1].ID)
	require.Equal(t, stored[0].ID, stored[1].PullFromStepID)
	require.Equal(t, stored[0].ID, stored[1].SessionTarget.StepID)
	require.Equal(t, stored[0].ID, stored[1].Events.OnTurnComplete[0].Config["step_id"])
	require.Equal(t, "this", stored[1].Events.OnComment[0].Config["task_id"])
	require.Equal(t, last.Prompt, stored[1].Prompt)
	require.Equal(t, last.AgentProfileID, stored[1].AgentProfileID)
	require.Equal(t, last.AutoArchiveAfterHours, stored[1].AutoArchiveAfterHours)
	require.Equal(t, last.WIPLimit, stored[1].WIPLimit)
	require.Equal(t, last.StageType, stored[1].StageType)
	require.True(t, stored[1].AutoAdvanceRequiresSignal)
	require.True(t, stored[1].CancelTriggersTurnComplete)
	require.True(t, stored[1].CompleteTaskOnEnter)
	require.True(t, stored[1].DisableUnclassifiedFallback)
	unchanged, err := repo.GetStep(ctx, last.ID)
	require.NoError(t, err)
	require.Equal(t, first.ID, unchanged.PullFromStepID)
}

func TestWorkspaceCloneStepsRejectExternalReferences(t *testing.T) {
	for _, events := range []models.StepEvents{
		{OnTurnComplete: []models.OnTurnCompleteAction{{Type: models.OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": "excluded-step"}}}},
		{OnEnter: []models.OnEnterAction{{Type: models.OnEnterQueueRun, Config: map[string]any{"task_id": "source-task"}}}},
	} {
		t.Run("external reference", func(t *testing.T) {
			repo, database := setupTestRepoWithDB(t)
			ctx := t.Context()
			require.NoError(t, repo.CreateStep(ctx, &models.WorkflowStep{ID: "external", WorkflowID: "wf-test", Name: "External", Events: events}))
			_, err := database.Exec(`INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES ('wf-copy', '', 'Copy', datetime('now'), datetime('now'))`)
			require.NoError(t, err)
			copier, ok := any(repo).(workspaceStepCopier)
			require.True(t, ok)
			tx, err := database.BeginTxx(ctx, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = tx.Rollback() })
			_, err = copier.CopyWorkspaceStepsTx(ctx, tx, map[string]string{"wf-test": "wf-copy"})
			require.Error(t, err)
			require.NoError(t, tx.Rollback())
			steps, err := repo.ListStepsByWorkflow(ctx, "wf-copy")
			require.NoError(t, err)
			require.Empty(t, steps)
		})
	}
}
