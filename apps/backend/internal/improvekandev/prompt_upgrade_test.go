package improvekandev

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	workflowcfg "github.com/kandev/kandev/config/workflows"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

func loadPreviousImprovePrompts(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "workflow", "repository", "testdata", "improve-kandev-before-lifecycle.yml"))
	if err != nil {
		t.Fatal(err)
	}
	template, err := workflowcfg.LoadTemplateYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	prompts := make(map[string]string, len(template.Steps))
	for _, step := range template.Steps {
		prompts[step.ID] = step.Prompt
	}
	return prompts
}

func openImproveBootstrapStore(t *testing.T, path string) (*sqlx.DB, *tasksqlite.Repository, *workflowrepository.Repository) {
	t.Helper()
	writer, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	database := sqlx.NewDb(writer, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	taskRepo, err := tasksqlite.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatal(err)
	}
	workflowRepo, err := workflowrepository.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatal(err)
	}
	return database, taskRepo, workflowRepo
}

func improvePromptIDForStep(name string) string {
	switch name {
	case "Improve":
		return "improve"
	case "Test":
		return "test"
	case "PR":
		return "pr"
	default:
		return ""
	}
}

func TestImproveBootstrapUsesUpgradedInstructions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "improve-bootstrap.db")
	database, taskRepo, workflowRepo := openImproveBootstrapStore(t, path)
	previous := loadPreviousImprovePrompts(t)
	currentTemplates, err := workflowcfg.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var current map[string]string
	for _, template := range currentTemplates {
		if template.ID != improveTemplateID {
			continue
		}
		current = make(map[string]string, len(template.Steps))
		for _, step := range template.Steps {
			current[step.ID] = step.Prompt
		}
	}
	if len(current) == 0 {
		t.Fatal("embedded Improve Kandev template is missing")
	}

	if err := taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-existing", Name: "Existing workspace"}); err != nil {
		t.Fatal(err)
	}
	templateID := improveTemplateID
	existingWorkflow := &models.Workflow{
		ID:                 "wf-existing",
		WorkspaceID:        "ws-existing",
		Name:               improveWorkflowName,
		Description:        improveWorkflowDesc,
		WorkflowTemplateID: &templateID,
		Hidden:             true,
	}
	if err := taskRepo.CreateWorkflow(ctx, existingWorkflow); err != nil {
		t.Fatal(err)
	}
	workflowSvc := workflowservice.NewService(workflowRepo, logger.Default())
	t.Cleanup(func() {
		if err := workflowSvc.Close(); err != nil {
			t.Errorf("close initial workflow service: %v", err)
		}
	})
	if err := workflowSvc.CreateStepsFromTemplate(ctx, existingWorkflow.ID, improveTemplateID); err != nil {
		t.Fatal(err)
	}
	template, err := workflowRepo.GetTemplate(ctx, improveTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range template.Steps {
		if prompt, ok := previous[template.Steps[i].ID]; ok {
			template.Steps[i].Prompt = prompt
		}
	}
	stepsJSON, err := json.Marshal(template.Steps)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE workflow_templates SET steps = ? WHERE id = ?`, string(stepsJSON), improveTemplateID); err != nil {
		t.Fatal(err)
	}
	steps, err := workflowRepo.ListStepsByWorkflow(ctx, existingWorkflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range steps {
		promptID := improvePromptIDForStep(step.Name)
		if prompt, ok := previous[promptID]; ok {
			if _, err := database.ExecContext(ctx, `UPDATE workflow_steps SET prompt = ? WHERE id = ?`, prompt, step.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	stepIDs := make([]string, 0, len(steps))
	for _, step := range steps {
		stepIDs = append(stepIDs, step.ID)
	}

	if err := workflowSvc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	database, taskRepo, workflowRepo = openImproveBootstrapStore(t, path)
	log := logger.Default()
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	taskSvc := taskservice.NewService(taskservice.Repos{Workflows: taskRepo}, eventBus, log, taskservice.RepositoryDiscoveryConfig{})
	reopenedWorkflowSvc := workflowservice.NewService(workflowRepo, log)
	t.Cleanup(func() {
		if err := reopenedWorkflowSvc.Close(); err != nil {
			t.Errorf("close reopened workflow service: %v", err)
		}
	})
	taskSvc.SetWorkflowStepGetter(reopenedWorkflowSvc)
	workflows, err := taskSvc.ListWorkflows(ctx, "ws-existing", true)
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{taskSvc: taskSvc}
	reused, err := handler.ensureWorkflow(ctx, "ws-existing", workflows, improveTemplateID, improveWorkflowName, improveWorkflowDesc)
	if err != nil {
		t.Fatal(err)
	}
	if reused.ID != existingWorkflow.ID {
		t.Fatalf("reused workflow ID = %q, want %q", reused.ID, existingWorkflow.ID)
	}
	steps, err = workflowRepo.ListStepsByWorkflow(ctx, reused.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != len(stepIDs) {
		t.Fatalf("reused workflow step count = %d, want %d", len(steps), len(stepIDs))
	}
	for i, step := range steps {
		if step.ID != stepIDs[i] {
			t.Fatalf("reused step %d ID = %q, want %q", i, step.ID, stepIDs[i])
		}
		promptID := improvePromptIDForStep(step.Name)
		if step.Prompt != current[promptID] {
			t.Fatalf("reused %s prompt does not match the current instruction", step.Name)
		}
	}

	if err := taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-new", Name: "New workspace"}); err != nil {
		t.Fatal(err)
	}
	newWorkflow := &models.Workflow{ID: "wf-new", WorkspaceID: "ws-new", Name: improveWorkflowName, WorkflowTemplateID: &templateID, Hidden: true}
	if err := taskRepo.CreateWorkflow(ctx, newWorkflow); err != nil {
		t.Fatal(err)
	}
	newWorkflowSvc := workflowservice.NewService(workflowRepo, log)
	t.Cleanup(func() {
		if err := newWorkflowSvc.Close(); err != nil {
			t.Errorf("close reopened workflow service: %v", err)
		}
	})
	if err := newWorkflowSvc.CreateStepsFromTemplate(ctx, newWorkflow.ID, improveTemplateID); err != nil {
		t.Fatal(err)
	}
	newSteps, err := workflowRepo.ListStepsByWorkflow(ctx, newWorkflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range newSteps {
		promptID := improvePromptIDForStep(step.Name)
		if step.Prompt != current[promptID] {
			t.Fatalf("new %s prompt does not match the current instruction", step.Name)
		}
	}
	var workflowCount int
	if err := database.Get(&workflowCount, `SELECT COUNT(*) FROM workflows WHERE workflow_template_id = ?`, improveTemplateID); err != nil {
		t.Fatal(err)
	}
	if workflowCount != 2 {
		t.Fatalf("Improve workflow rows = %d, want existing and new workflows", workflowCount)
	}
	if err := newWorkflowSvc.Close(); err != nil {
		t.Fatal(err)
	}
	eventBus.Close()
	if err := reopenedWorkflowSvc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
}
