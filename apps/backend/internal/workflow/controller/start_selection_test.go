package controller

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/internal/workflow/service"
)

type selectionWorkflowProvider struct {
	service.WorkflowProvider
	db     *sqlx.DB
	before func()
}

func (p *selectionWorkflowProvider) GetWorkflow(ctx context.Context, id string) (*taskmodels.Workflow, error) {
	if p.before != nil {
		before := p.before
		p.before = nil
		before()
	}
	var wf taskmodels.Workflow
	err := p.db.QueryRowContext(ctx, "SELECT id, workspace_id, name FROM workflows WHERE id = ?", id).Scan(&wf.ID, &wf.WorkspaceID, &wf.Name)
	return &wf, err
}

func setupSelectionController(t *testing.T, selected string) (*Controller, *service.Service, *repository.Repository, *selectionWorkflowProvider) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE workflows (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
		workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL, description TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`)
	require.NoError(t, err)
	repo, err := repository.NewWithDB(db, db, nil)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO workflows (id, name, created_at, updated_at) VALUES ('selection', 'Selection', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	reader := service.NewService(repo, logger.Default())
	writer := service.NewService(repo, logger.Default())
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	provider := &selectionWorkflowProvider{db: db}
	reader.SetWorkflowProvider(provider)
	for i, id := range []string{"edited", "other"} {
		require.NoError(t, writer.CreateStep(context.Background(), &models.WorkflowStep{
			ID: id, WorkflowID: "selection", Name: id, Position: i, IsStartStep: id == selected,
		}))
	}
	return NewController(reader), writer, repo, provider
}

// @covers AC-TASKS-WORKFLOW-START-SELECTION-001.1, AC-TASKS-WORKFLOW-START-SELECTION-001.3, AC-TASKS-WORKFLOW-START-SELECTION-001.5
func TestWorkflowStartSelectionControllerOmission(t *testing.T) {
	for _, initial := range []string{"edited", "other"} {
		t.Run(initial, func(t *testing.T) {
			ctrl, writer, repo, provider := setupSelectionController(t, initial)
			selected := "edited"
			if initial == selected {
				selected = "other"
			}
			provider.before = func() {
				step, err := repo.GetStep(context.Background(), selected)
				require.NoError(t, err)
				step.IsStartStep = true
				require.NoError(t, writer.UpdateStep(context.Background(), step))
			}
			name := "Renamed"
			resp, err := ctrl.UpdateStep(context.Background(), UpdateStepRequest{ID: "edited", Name: &name})
			require.NoError(t, err)
			assert.Empty(t, resp.DemotedStartSteps)
			assert.Equal(t, selected == "edited", resp.Step.IsStartStep)
			for _, id := range []string{"edited", "other"} {
				stored, err := repo.GetStep(context.Background(), id)
				require.NoError(t, err)
				assert.Equal(t, id == selected, stored.IsStartStep, id)
				if id == "edited" {
					assert.Equal(t, name, stored.Name)
				}
			}
			resolved, err := writer.ResolveStartStep(context.Background(), "selection")
			require.NoError(t, err)
			assert.Equal(t, selected, resolved.ID)
		})
	}
}

// @covers AC-TASKS-WORKFLOW-START-SELECTION-001.2, AC-TASKS-WORKFLOW-START-SELECTION-001.5
func TestWorkflowStartSelectionControllerControls(t *testing.T) {
	for _, initial := range []string{"edited", "other"} {
		t.Run(initial, func(t *testing.T) {
			ctrl, writer, repo, provider := setupSelectionController(t, initial)
			ctx := context.Background()
			name := "Renamed"
			response, err := ctrl.UpdateStep(ctx, UpdateStepRequest{ID: "edited", Name: &name})
			require.NoError(t, err)
			require.Equal(t, initial == "edited", response.Step.IsStartStep)
			require.Empty(t, response.DemotedStartSteps)
			// The supplied value remains explicit even when it equals the snapshot.
			intent := initial == "edited"
			provider.before = func() {
				step, getErr := repo.GetStep(ctx, "edited")
				require.NoError(t, getErr)
				step.IsStartStep = !intent
				require.NoError(t, writer.UpdateStep(ctx, step))
			}
			response, err = ctrl.UpdateStep(ctx, UpdateStepRequest{ID: "edited", IsStartStep: &intent})
			require.NoError(t, err)
			require.Equal(t, intent, response.Step.IsStartStep)
			promote := true
			response, err = ctrl.UpdateStep(ctx, UpdateStepRequest{ID: "other", IsStartStep: &promote})
			require.NoError(t, err)
			require.True(t, response.Step.IsStartStep)
			resolved, err := writer.ResolveStartStep(ctx, "selection")
			require.NoError(t, err)
			require.Equal(t, "other", resolved.ID)
			clear := false
			response, err = ctrl.UpdateStep(ctx, UpdateStepRequest{ID: "edited", IsStartStep: &clear})
			require.NoError(t, err)
			require.Empty(t, response.DemotedStartSteps)
			resolved, err = writer.ResolveStartStep(ctx, "selection")
			require.NoError(t, err)
			require.Equal(t, "other", resolved.ID)
			_, err = ctrl.UpdateStep(ctx, UpdateStepRequest{ID: "other", IsStartStep: &clear})
			require.NoError(t, err)
			resolved, err = writer.ResolveStartStep(ctx, "selection")
			require.NoError(t, err)
			require.Equal(t, "edited", resolved.ID)
		})
	}
}
