package controller

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/internal/workflow/service"
)

// @covers AC-TASKS-WORKFLOW-STEP-ORDERING-001.5
func TestReorderStepsPreservesSessionTargetOrderGuard(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	database.SetMaxOpenConns(1)
	_, err = database.Exec(`CREATE TABLE workflows (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL, description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`)
	require.NoError(t, err)
	repo, err := repository.NewWithDB(database, database, nil)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO workflows (id, name, created_at, updated_at) VALUES ('wf', 'Test', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	svc := service.NewService(repo, log)
	t.Cleanup(func() { _ = svc.Close() })
	ctx := context.Background()
	for _, step := range []*models.WorkflowStep{
		{ID: "source", WorkflowID: "wf", Name: "Source", Position: 0, AgentProfileID: "profile"},
		{ID: "target", WorkflowID: "wf", Name: "Target", Position: 1, SessionTarget: &models.WorkflowSessionTarget{Kind: models.WorkflowSessionTargetStep, StepID: "source"}},
		{ID: "other", WorkflowID: "wf", Name: "Other", Position: 2},
	} {
		require.NoError(t, svc.CreateStep(ctx, step))
	}
	before, err := repo.ListStepsByWorkflow(ctx, "wf")
	require.NoError(t, err)
	controller := NewController(svc)
	err = controller.ReorderSteps(ctx, ReorderStepsRequest{WorkflowID: "wf", StepIDs: []string{"target", "source", "other"}})
	require.ErrorContains(t, err, "must remain earlier")
	after, err := repo.ListStepsByWorkflow(ctx, "wf")
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoError(t, controller.ReorderSteps(ctx, ReorderStepsRequest{WorkflowID: "wf", StepIDs: []string{"other", "source", "target"}}))
	after, err = repo.ListStepsByWorkflow(ctx, "wf")
	require.NoError(t, err)
	require.Equal(t, []string{"other", "source", "target"}, []string{after[0].ID, after[1].ID, after[2].ID})
}
