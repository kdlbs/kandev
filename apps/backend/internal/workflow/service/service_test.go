package service

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
)

func setupTestService(t *testing.T) (*Service, *sqlx.DB) {
	rawDB, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	// Create workflows table (normally created by task repo)
	_, err = sqlxDB.Exec(`CREATE TABLE IF NOT EXISTS workflows (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
		workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL,
		description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
	)`)
	require.NoError(t, err)

	repo, err := repository.NewWithDB(sqlxDB, sqlxDB)
	require.NoError(t, err)

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	return NewService(repo, log), sqlxDB
}

func insertWorkflow(t *testing.T, db *sqlx.DB, id, name string) {
	now := time.Now().UTC()
	_, err := db.Exec("INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES (?, '', ?, ?, ?)", id, name, now, now)
	require.NoError(t, err)
}

// mockWorkflowProvider implements WorkflowProvider with in-memory state for tests.
type mockWorkflowProvider struct {
	workflows []*taskmodels.Workflow
}

func (m *mockWorkflowProvider) ListWorkflows(_ context.Context, workspaceID string) ([]*taskmodels.Workflow, error) {
	var result []*taskmodels.Workflow
	for _, wf := range m.workflows {
		if wf.WorkspaceID == workspaceID {
			result = append(result, wf)
		}
	}
	return result, nil
}

func (m *mockWorkflowProvider) GetWorkflow(_ context.Context, id string) (*taskmodels.Workflow, error) {
	for _, wf := range m.workflows {
		if wf.ID == id {
			return wf, nil
		}
	}
	return nil, fmt.Errorf("workflow %s not found", id)
}

func (m *mockWorkflowProvider) CreateWorkflow(_ context.Context, workspaceID, name, description string) (*taskmodels.Workflow, error) {
	now := time.Now().UTC()
	wf := &taskmodels.Workflow{
		ID:          "imported-" + name,
		WorkspaceID: workspaceID,
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	m.workflows = append(m.workflows, wf)
	return wf, nil
}

func (m *mockWorkflowProvider) addWorkflow(id, workspaceID, name string) {
	now := time.Now().UTC()
	m.workflows = append(m.workflows, &taskmodels.Workflow{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        name,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

func createStep(t *testing.T, svc *Service, step *models.WorkflowStep) {
	err := svc.CreateStep(context.Background(), step)
	require.NoError(t, err)
}

func TestGetNextStepByPosition(t *testing.T) {
	t.Run("middle step returns next step", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-1", "Test Workflow")

		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Todo", Position: 0})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "In Progress", Position: 1})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Done", Position: 2})

		next, err := svc.GetNextStepByPosition(ctx, "wf-1", 0)
		require.NoError(t, err)
		assert.NotNil(t, next)
		assert.Equal(t, "In Progress", next.Name)
		assert.Equal(t, 1, next.Position)
	})

	t.Run("last step returns nil", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-1", "Test Workflow")

		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Todo", Position: 0})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "In Progress", Position: 1})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Done", Position: 2})

		next, err := svc.GetNextStepByPosition(ctx, "wf-1", 2)
		require.NoError(t, err)
		assert.Nil(t, next)
	})

	t.Run("gaps in positions still finds next", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-1", "Test Workflow")

		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "First", Position: 0})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Second", Position: 2})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Third", Position: 5})

		next, err := svc.GetNextStepByPosition(ctx, "wf-1", 0)
		require.NoError(t, err)
		assert.NotNil(t, next)
		assert.Equal(t, "Second", next.Name)
		assert.Equal(t, 2, next.Position)

		next, err = svc.GetNextStepByPosition(ctx, "wf-1", 2)
		require.NoError(t, err)
		assert.NotNil(t, next)
		assert.Equal(t, "Third", next.Name)
		assert.Equal(t, 5, next.Position)
	})

	t.Run("single step returns nil", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-1", "Test Workflow")

		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Only Step", Position: 0})

		next, err := svc.GetNextStepByPosition(ctx, "wf-1", 0)
		require.NoError(t, err)
		assert.Nil(t, next)
	})
}

func TestGetPreviousStepByPosition(t *testing.T) {
	t.Run("middle step returns previous step", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-1", "Test Workflow")

		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Todo", Position: 0})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "In Progress", Position: 1})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Done", Position: 2})

		prev, err := svc.GetPreviousStepByPosition(ctx, "wf-1", 2)
		require.NoError(t, err)
		assert.NotNil(t, prev)
		assert.Equal(t, "In Progress", prev.Name)
		assert.Equal(t, 1, prev.Position)
	})

	t.Run("first step returns nil", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-1", "Test Workflow")

		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Todo", Position: 0})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "In Progress", Position: 1})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Done", Position: 2})

		prev, err := svc.GetPreviousStepByPosition(ctx, "wf-1", 0)
		require.NoError(t, err)
		assert.Nil(t, prev)
	})

	t.Run("gaps in positions still finds previous", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-1", "Test Workflow")

		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "First", Position: 0})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Second", Position: 2})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Third", Position: 5})

		prev, err := svc.GetPreviousStepByPosition(ctx, "wf-1", 5)
		require.NoError(t, err)
		assert.NotNil(t, prev)
		assert.Equal(t, "Second", prev.Name)
		assert.Equal(t, 2, prev.Position)

		prev, err = svc.GetPreviousStepByPosition(ctx, "wf-1", 2)
		require.NoError(t, err)
		assert.NotNil(t, prev)
		assert.Equal(t, "First", prev.Name)
		assert.Equal(t, 0, prev.Position)
	})

	t.Run("single step returns nil", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-1", "Test Workflow")

		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Only Step", Position: 0})

		prev, err := svc.GetPreviousStepByPosition(ctx, "wf-1", 0)
		require.NoError(t, err)
		assert.Nil(t, prev)
	})
}

func TestResolveStartStep(t *testing.T) {
	t.Run("explicit is_start_step returns that step", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-1", "Test Workflow")

		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Todo", Position: 0})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Start Here", Position: 1, IsStartStep: true})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Done", Position: 2})

		start, err := svc.ResolveStartStep(ctx, "wf-1")
		require.NoError(t, err)
		assert.NotNil(t, start)
		assert.Equal(t, "Start Here", start.Name)
		assert.True(t, start.IsStartStep)
	})

	t.Run("fallback to first step with auto_start_agent", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-1", "Test Workflow")

		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Todo", Position: 0})
		createStep(t, svc, &models.WorkflowStep{
			WorkflowID: "wf-1",
			Name:       "Auto Start",
			Position:   1,
			Events: models.StepEvents{
				OnEnter: []models.OnEnterAction{
					{Type: models.OnEnterAutoStartAgent},
				},
			},
		})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Done", Position: 2})

		start, err := svc.ResolveStartStep(ctx, "wf-1")
		require.NoError(t, err)
		assert.NotNil(t, start)
		assert.Equal(t, "Auto Start", start.Name)
	})

	t.Run("fallback to first step by position", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-1", "Test Workflow")

		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "First", Position: 0})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Second", Position: 1})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Third", Position: 2})

		start, err := svc.ResolveStartStep(ctx, "wf-1")
		require.NoError(t, err)
		assert.NotNil(t, start)
		assert.Equal(t, "First", start.Name)
		assert.Equal(t, 0, start.Position)
	})

	t.Run("empty workflow returns error", func(t *testing.T) {
		svc, db := setupTestService(t)
		ctx := context.Background()

		insertWorkflow(t, db, "wf-empty", "Empty Workflow")

		start, err := svc.ResolveStartStep(ctx, "wf-empty")
		assert.Error(t, err)
		assert.Nil(t, start)
		assert.Contains(t, err.Error(), "has no steps")
	})
}

func TestCreateStepsFromTemplate_RemapsStepIDs(t *testing.T) {
	svc, db := setupTestService(t)
	ctx := context.Background()

	insertWorkflow(t, db, "wf-1", "Test Workflow")

	// Use the "simple" (Kanban) template which has move_to_step references
	err := svc.CreateStepsFromTemplate(ctx, "wf-1", "simple")
	require.NoError(t, err)

	steps, err := svc.repo.ListStepsByWorkflow(ctx, "wf-1")
	require.NoError(t, err)
	require.Len(t, steps, 4)

	// Build a map of step name → ID for verification
	nameToID := make(map[string]string, len(steps))
	for _, s := range steps {
		nameToID[s.Name] = s.ID
	}

	// Backlog's OnTurnComplete should reference the real Review step UUID
	backlog := findStepByName(steps, "Backlog")
	require.NotNil(t, backlog)
	require.Len(t, backlog.Events.OnTurnComplete, 1)
	assert.Equal(t, models.OnTurnCompleteMoveToStep, backlog.Events.OnTurnComplete[0].Type)
	assert.Equal(t, nameToID["Review"], backlog.Events.OnTurnComplete[0].Config["step_id"])

	// In Progress's OnTurnComplete should reference the real Review step UUID
	inProgress := findStepByName(steps, "In Progress")
	require.NotNil(t, inProgress)
	require.Len(t, inProgress.Events.OnTurnComplete, 1)
	assert.Equal(t, nameToID["Review"], inProgress.Events.OnTurnComplete[0].Config["step_id"])

	// Done's OnTurnStart should reference the real In Progress step UUID
	done := findStepByName(steps, "Done")
	require.NotNil(t, done)
	require.Len(t, done.Events.OnTurnStart, 1)
	assert.Equal(t, models.OnTurnStartMoveToStep, done.Events.OnTurnStart[0].Type)
	assert.Equal(t, nameToID["In Progress"], done.Events.OnTurnStart[0].Config["step_id"])
}

func findStepByName(steps []*models.WorkflowStep, name string) *models.WorkflowStep {
	for _, s := range steps {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func setupTestServiceWithProvider(t *testing.T) (*Service, *sqlx.DB, *mockWorkflowProvider) {
	svc, db := setupTestService(t)
	mock := &mockWorkflowProvider{}
	svc.SetWorkflowProvider(mock)
	return svc, db, mock
}

func TestExportWorkflow(t *testing.T) {
	t.Run("exports single workflow with steps", func(t *testing.T) {
		svc, _, mock := setupTestServiceWithProvider(t)
		ctx := context.Background()

		mock.addWorkflow("wf-1", "", "My Pipeline")
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Todo", Position: 0, Color: "gray"})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Done", Position: 1, Color: "green"})

		export, err := svc.ExportWorkflow(ctx, "wf-1")
		require.NoError(t, err)
		assert.Equal(t, models.ExportVersion, export.Version)
		assert.Equal(t, models.ExportType, export.Type)
		require.Len(t, export.Workflows, 1)
		assert.Equal(t, "My Pipeline", export.Workflows[0].Name)
		require.Len(t, export.Workflows[0].Steps, 2)
		assert.Equal(t, "Todo", export.Workflows[0].Steps[0].Name)
		assert.Equal(t, "Done", export.Workflows[0].Steps[1].Name)
	})

	t.Run("converts step_id refs to positions", func(t *testing.T) {
		svc, _, mock := setupTestServiceWithProvider(t)
		ctx := context.Background()

		mock.addWorkflow("wf-1", "", "Pipeline")
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "A", Position: 0, Color: "gray"})

		// Get the created step's ID so we can reference it.
		steps, err := svc.repo.ListStepsByWorkflow(ctx, "wf-1")
		require.NoError(t, err)
		stepAID := steps[0].ID

		createStep(t, svc, &models.WorkflowStep{
			WorkflowID: "wf-1", Name: "B", Position: 1, Color: "blue",
			Events: models.StepEvents{
				OnTurnComplete: []models.OnTurnCompleteAction{
					{Type: models.OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": stepAID}},
				},
			},
		})

		export, err := svc.ExportWorkflow(ctx, "wf-1")
		require.NoError(t, err)

		stepB := export.Workflows[0].Steps[1]
		require.Len(t, stepB.Events.OnTurnComplete, 1)
		assert.Equal(t, 0, stepB.Events.OnTurnComplete[0].Config["step_position"])
		assert.Nil(t, stepB.Events.OnTurnComplete[0].Config["step_id"])
	})
}

func TestExportWorkflows(t *testing.T) {
	t.Run("exports all workflows for workspace", func(t *testing.T) {
		svc, _, mock := setupTestServiceWithProvider(t)
		ctx := context.Background()

		mock.addWorkflow("wf-1", "ws-1", "Alpha")
		mock.addWorkflow("wf-2", "ws-1", "Beta")
		mock.addWorkflow("wf-3", "ws-2", "Other Workspace")
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-1", Name: "Step1", Position: 0})
		createStep(t, svc, &models.WorkflowStep{WorkflowID: "wf-2", Name: "Step2", Position: 0})

		export, err := svc.ExportWorkflows(ctx, "ws-1")
		require.NoError(t, err)
		require.Len(t, export.Workflows, 2)

		names := []string{export.Workflows[0].Name, export.Workflows[1].Name}
		assert.Contains(t, names, "Alpha")
		assert.Contains(t, names, "Beta")
	})
}

func TestImportWorkflows(t *testing.T) {
	t.Run("imports new workflows", func(t *testing.T) {
		svc, _, _ := setupTestServiceWithProvider(t)
		ctx := context.Background()

		export := &models.WorkflowExport{
			Version: models.ExportVersion,
			Type:    models.ExportType,
			Workflows: []models.WorkflowPortable{
				{
					Name: "Imported WF",
					Steps: []models.StepPortable{
						{Name: "Todo", Position: 0, Color: "gray"},
						{Name: "Done", Position: 1, Color: "green"},
					},
				},
			},
		}

		result, err := svc.ImportWorkflows(ctx, "ws-1", export)
		require.NoError(t, err)
		assert.Equal(t, []string{"Imported WF"}, result.Created)
		assert.Empty(t, result.Skipped)

		// Verify steps were created.
		steps, err := svc.repo.ListStepsByWorkflow(ctx, "imported-Imported WF")
		require.NoError(t, err)
		assert.Len(t, steps, 2)
	})

	t.Run("skips workflows that already exist by name", func(t *testing.T) {
		svc, _, mock := setupTestServiceWithProvider(t)
		ctx := context.Background()

		mock.addWorkflow("wf-existing", "ws-1", "Existing")

		export := &models.WorkflowExport{
			Version: models.ExportVersion,
			Type:    models.ExportType,
			Workflows: []models.WorkflowPortable{
				{Name: "Existing", Steps: []models.StepPortable{{Name: "S", Position: 0, Color: "blue"}}},
				{Name: "Brand New", Steps: []models.StepPortable{{Name: "S", Position: 0, Color: "red"}}},
			},
		}

		result, err := svc.ImportWorkflows(ctx, "ws-1", export)
		require.NoError(t, err)
		assert.Equal(t, []string{"Brand New"}, result.Created)
		assert.Equal(t, []string{"Existing"}, result.Skipped)
	})

	t.Run("remaps step_position to step_id on import", func(t *testing.T) {
		svc, _, _ := setupTestServiceWithProvider(t)
		ctx := context.Background()

		export := &models.WorkflowExport{
			Version: models.ExportVersion,
			Type:    models.ExportType,
			Workflows: []models.WorkflowPortable{
				{
					Name: "WithRefs",
					Steps: []models.StepPortable{
						{Name: "A", Position: 0, Color: "gray"},
						{
							Name: "B", Position: 1, Color: "blue",
							Events: models.StepEvents{
								OnTurnComplete: []models.OnTurnCompleteAction{
									{Type: models.OnTurnCompleteMoveToStep, Config: map[string]any{"step_position": 0}},
								},
							},
						},
					},
				},
			},
		}

		result, err := svc.ImportWorkflows(ctx, "ws-1", export)
		require.NoError(t, err)
		require.Len(t, result.Created, 1)

		// Verify the imported step B references step A's new ID.
		steps, err := svc.repo.ListStepsByWorkflow(ctx, "imported-WithRefs")
		require.NoError(t, err)
		require.Len(t, steps, 2)

		nameToID := map[string]string{}
		for _, s := range steps {
			nameToID[s.Name] = s.ID
		}

		stepB := findStepByName(steps, "B")
		require.NotNil(t, stepB)
		require.Len(t, stepB.Events.OnTurnComplete, 1)
		assert.Equal(t, nameToID["A"], stepB.Events.OnTurnComplete[0].Config["step_id"])
		assert.Nil(t, stepB.Events.OnTurnComplete[0].Config["step_position"])
	})

	t.Run("rejects invalid export data", func(t *testing.T) {
		svc, _, _ := setupTestServiceWithProvider(t)
		ctx := context.Background()

		export := &models.WorkflowExport{Version: 99, Type: "wrong"}
		_, err := svc.ImportWorkflows(ctx, "ws-1", export)
		assert.ErrorContains(t, err, "invalid export data")
	})
}
