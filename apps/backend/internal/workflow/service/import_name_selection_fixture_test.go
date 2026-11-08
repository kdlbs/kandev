package service

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
)

type importNameTaskProvider struct{ *taskservice.Service }

func (p importNameTaskProvider) CreateWorkflow(ctx context.Context, workspaceID, name, description string) (*taskmodels.Workflow, error) {
	return p.Service.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: workspaceID, Name: name, Description: description})
}

func (p importNameTaskProvider) UpdateWorkflow(ctx context.Context, workflow *taskmodels.Workflow) error {
	_, err := p.Service.UpdateWorkflow(ctx, workflow.ID, &taskservice.UpdateWorkflowRequest{
		Name: &workflow.Name, Description: &workflow.Description, Prompt: &workflow.Prompt, AgentProfileID: &workflow.AgentProfileID,
	})
	return err
}

type importNameFixture struct {
	svc   *Service
	tasks *taskservice.Service
	db    *sqlx.DB
}

func newImportNameFixture(t *testing.T) *importNameFixture {
	t.Helper()
	raw, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "import.db")+"?_foreign_keys=on")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	raw.SetMaxOpenConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	var foreignKeys int
	require.NoError(t, db.Get(&foreignKeys, "PRAGMA foreign_keys"))
	require.Equal(t, 1, foreignKeys)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	tasks, err := taskrepo.NewWithDB(db, db, log)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, tasks.Close()) })
	workflows, err := repository.NewWithDB(db, db, log)
	require.NoError(t, err)
	svc := NewService(workflows, log)
	t.Cleanup(func() { require.NoError(t, svc.Close()) })
	taskSvc := taskservice.NewService(taskservice.Repos{Workspaces: tasks, Workflows: tasks, Tasks: tasks}, bus.NewMemoryEventBus(log), log, taskservice.RepositoryDiscoveryConfig{})
	svc.SetWorkflowProvider(importNameTaskProvider{taskSvc})
	for _, id := range []string{"ws-1", "ws-other"} {
		require.NoError(t, tasks.CreateWorkspace(context.Background(), &taskmodels.Workspace{ID: id, Name: id}))
	}
	f := &importNameFixture{svc: svc, tasks: taskSvc, db: db}
	f.installAudit(t)
	return f
}

func (f *importNameFixture) installAudit(t *testing.T) {
	t.Helper()
	_, err := f.db.Exec("CREATE TABLE import_write_audit (entry TEXT NOT NULL)")
	require.NoError(t, err)
	for _, table := range []string{"workflows", "workflow_steps"} {
		for _, operation := range []string{"INSERT", "UPDATE", "DELETE"} {
			row := "NEW"
			if operation == "DELETE" {
				row = "OLD"
			}
			_, err = f.db.Exec(fmt.Sprintf(`CREATE TRIGGER audit_%s_%s AFTER %s ON %s BEGIN
    INSERT INTO import_write_audit VALUES ('%s:%s:' || %s.name); END`, table, operation, operation, table, table, operation, row))
			require.NoError(t, err)
		}
	}
}

func (f *importNameFixture) clearAudit(t *testing.T) {
	t.Helper()
	_, err := f.db.Exec("DELETE FROM import_write_audit")
	require.NoError(t, err)
}

func (f *importNameFixture) audit(t *testing.T) []string {
	t.Helper()
	var entries []string
	require.NoError(t, f.db.Select(&entries, "SELECT entry FROM import_write_audit ORDER BY rowid"))
	return entries
}

func (f *importNameFixture) seed(t *testing.T, workspace, name string, hidden bool) *taskmodels.Workflow {
	t.Helper()
	workflow, err := f.tasks.CreateWorkflow(context.Background(), &taskservice.CreateWorkflowRequest{WorkspaceID: workspace, Name: name, Description: "existing content", Hidden: hidden})
	require.NoError(t, err)
	require.NoError(t, f.svc.CreateStep(context.Background(), &models.WorkflowStep{ID: uuid.NewString(), WorkflowID: workflow.ID, Name: "Existing lane", Position: 0, Color: "bg-blue-500"}))
	return workflow
}

func (f *importNameFixture) workflows(t *testing.T) []*taskmodels.Workflow {
	t.Helper()
	workflows, err := f.tasks.ListWorkflows(context.Background(), "ws-1", true)
	require.NoError(t, err)
	for _, workflow := range workflows {
		_, err = uuid.Parse(workflow.ID)
		require.NoError(t, err)
	}
	return workflows
}

func importNameDocument(version int, workflows ...models.WorkflowPortable) *models.WorkflowExport {
	return &models.WorkflowExport{Version: version, Type: models.ExportType, Workflows: workflows}
}

func importNameWorkflow(name, lane string) models.WorkflowPortable {
	return models.WorkflowPortable{Name: name, Description: lane, Steps: []models.StepPortable{{Name: lane, Position: 0, Color: "bg-blue-500"}}}
}

func (f *importNameFixture) submit(mode string, document *models.WorkflowExport, bindings []ImportProfileBinding) (*ImportResult, error) {
	if mode == "raw" {
		return f.svc.ImportWorkflows(context.Background(), "ws-1", document)
	}
	return f.svc.ImportWorkflowsWithBindings(context.Background(), "ws-1", document, bindings)
}
