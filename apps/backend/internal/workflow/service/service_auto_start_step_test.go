package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/workflow/models"
)

func TestResolveAutoStartStep(t *testing.T) {
	t.Run("returns first step by position carrying auto_start_agent", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-1", "Test Workflow")

		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Backlog", Position: 0, IsStartStep: true})
		createStep(t, svc, &models.WorkflowStep{
			WorkflowID: "wf-1",
			Name:       "In Progress",
			Position:   1,
			Events: models.StepEvents{
				OnEnter: []models.OnEnterAction{{Type: models.OnEnterAutoStartAgent}},
			},
		})
		createStep(t, svc, &models.WorkflowStep{
			WorkflowID: "wf-1",
			Name:       "Deploy",
			Position:   2,
			Events: models.StepEvents{
				OnEnter: []models.OnEnterAction{{Type: models.OnEnterAutoStartAgent}},
			},
		})

		step, err := svc.ResolveAutoStartStep(ctx, "wf-1")
		require.NoError(t, err)
		require.NotNil(t, step)
		assert.Equal(t, "In Progress", step.Name)
	})

	t.Run("falls back to the start step when no step auto-starts", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-1", "Test Workflow")

		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Todo", Position: 0})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Start Here", Position: 1, IsStartStep: true})

		step, err := svc.ResolveAutoStartStep(ctx, "wf-1")
		require.NoError(t, err)
		require.NotNil(t, step)
		assert.Equal(t, "Start Here", step.Name)
	})

	t.Run("empty workflow returns error", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-empty", "Empty Workflow")

		step, err := svc.ResolveAutoStartStep(ctx, "wf-empty")
		assert.Error(t, err)
		assert.Nil(t, step)
		assert.Contains(t, err.Error(), "has no steps")
	})
}
