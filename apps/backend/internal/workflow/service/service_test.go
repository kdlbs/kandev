package service

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
)

func setupTestService(t *testing.T) (*Service, *sql.DB) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// Create workflows table (normally created by task repo)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS workflows (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
		workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL,
		description TEXT DEFAULT '', created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
	)`)
	require.NoError(t, err)

	repo, err := repository.NewWithDB(db)
	require.NoError(t, err)

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	return NewService(repo, log), db
}

func insertWorkflow(t *testing.T, db *sql.DB, id, name string) {
	_, err := db.Exec("INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES (?, '', ?, datetime('now'), datetime('now'))", id, name)
	require.NoError(t, err)
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
