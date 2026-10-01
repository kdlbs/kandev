package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/workflow/models"
)

func seedReorderSteps(t *testing.T, repo *Repository) []*models.WorkflowStep {
	t.Helper()
	_, err := repo.db.Exec(`DELETE FROM workflow_steps WHERE workflow_id = 'wf-test'`)
	require.NoError(t, err)
	steps := []*models.WorkflowStep{
		{ID: "a", WorkflowID: "wf-test", Name: "A", Position: 0, Prompt: "saved prompt", AgentProfileID: "saved-profile", CompleteTaskOnEnter: true},
		{ID: "b", WorkflowID: "wf-test", Name: "B", Position: 1},
		{ID: "c", WorkflowID: "wf-test", Name: "C", Position: 2},
	}
	for _, step := range steps {
		require.NoError(t, repo.CreateStep(context.Background(), step))
	}
	stored, err := repo.ListStepsByWorkflow(context.Background(), "wf-test")
	require.NoError(t, err)
	return stored
}

// @covers AC-TASKS-WORKFLOW-STEP-ORDERING-001.1, AC-TASKS-WORKFLOW-STEP-ORDERING-001.4
func TestReorderStepsValidatesCompleteMembership(t *testing.T) {
	runReorderMembershipTests(t, setupTestRepo)
}

func runReorderMembershipTests(t *testing.T, factory func(*testing.T) *Repository) {
	t.Helper()
	for name, ids := range map[string][]string{
		"complete": {"c", "a", "b"}, "duplicate": {"b", "a", "a"},
		"omitted": {"b", "a"}, "empty": {}, "missing": {"b", "missing", "a"},
		"foreign": {"b", "foreign", "a"},
	} {
		t.Run(name, func(t *testing.T) {
			repo := factory(t)
			before := seedReorderSteps(t, repo)
			ctx := context.Background()
			_, err := repo.db.ExecContext(ctx, `INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES ('other', '', 'Other', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
			require.NoError(t, err)
			foreign := &models.WorkflowStep{ID: "foreign", WorkflowID: "other", Name: "Foreign", Position: 7}
			require.NoError(t, repo.CreateStep(ctx, foreign))
			err = repo.ReorderSteps(ctx, "wf-test", ids)
			if name != "complete" {
				require.Error(t, err)
				if name == "missing" || name == "foreign" {
					require.ErrorIs(t, err, models.ErrWorkflowStepNotFound)
				}
				for _, original := range before {
					stored, err := repo.GetStep(ctx, original.ID)
					require.NoError(t, err)
					require.Equal(t, original.Position, stored.Position)
					require.True(t, original.UpdatedAt.Equal(stored.UpdatedAt))
				}
			} else {
				require.NoError(t, err)
				assertReorderPositions(t, repo, ids)
				stored, err := repo.GetStep(ctx, "a")
				require.NoError(t, err)
				require.Equal(t, "saved prompt", stored.Prompt)
				require.Equal(t, "saved-profile", stored.AgentProfileID)
				require.True(t, stored.CompleteTaskOnEnter)
			}
			storedForeign, err := repo.GetStep(ctx, foreign.ID)
			require.NoError(t, err)
			require.Equal(t, foreign.Position, storedForeign.Position)

		})
	}
	t.Run("empty workflow", func(t *testing.T) {
		repo := factory(t)
		_, err := repo.db.Exec(`DELETE FROM workflow_steps WHERE workflow_id = 'wf-test'`)
		require.NoError(t, err)
		require.NoError(t, repo.ReorderSteps(context.Background(), "wf-test", nil))
	})
}

func assertReorderPositions(t *testing.T, repo *Repository, ids []string) {
	t.Helper()
	steps, err := repo.ListStepsByWorkflow(context.Background(), "wf-test")
	require.NoError(t, err)
	require.Len(t, steps, len(ids))
	for i, step := range steps {
		require.Equal(t, ids[i], step.ID)
		require.Equal(t, i, step.Position)
		require.True(t, steps[0].UpdatedAt.Equal(step.UpdatedAt))
	}
}
