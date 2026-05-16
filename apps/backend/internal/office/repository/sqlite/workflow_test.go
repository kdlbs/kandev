package sqlite_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// TestEnsureOfficeWorkflow asserts that EnsureOfficeWorkflow materialises
// the YAML-templated office-default workflow, stamps it onto
// workspaces.office_workflow_id, and is idempotent. The legacy hardcoded
// 7-step Office workflow has been retired.
func TestEnsureOfficeWorkflow(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Initialize task repo (creates workspaces, workflows tables + migrations)
	repo, err := taskrepo.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init task repo: %v", err)
	}

	// Initialize workflow repo (creates workflow_steps table)
	if _, err := workflowrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init workflow repo: %v", err)
	}

	// Get the default workspace
	ctx := context.Background()
	var wsID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM workspaces LIMIT 1`).Scan(&wsID); err != nil {
		t.Fatalf("get workspace: %v", err)
	}

	// Create office workflow
	workflowID, err := repo.EnsureOfficeWorkflow(ctx, wsID)
	if err != nil {
		t.Fatalf("ensure workflow: %v", err)
	}
	if workflowID == "" {
		t.Fatal("expected non-empty workflow ID")
	}

	// Verify the office-default YAML-templated workflow was materialised.
	var wfName, templateID string
	var isSystem int
	err = db.QueryRowContext(ctx,
		`SELECT name, is_system, COALESCE(workflow_template_id, '') FROM workflows WHERE id = ?`,
		workflowID).Scan(&wfName, &isSystem, &templateID)
	if err != nil {
		t.Fatalf("query workflow: %v", err)
	}
	if wfName != "Office Default" {
		t.Errorf("workflow name = %q, want %q", wfName, "Office Default")
	}
	if isSystem != 1 {
		t.Errorf("is_system = %d, want 1", isSystem)
	}
	if templateID != "office-default" {
		t.Errorf("workflow_template_id = %q, want office-default", templateID)
	}

	// Verify the 5 YAML steps were created.
	var stepCount int
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workflow_steps WHERE workflow_id = ?`, workflowID).Scan(&stepCount)
	if err != nil {
		t.Fatalf("count steps: %v", err)
	}
	if stepCount != 5 {
		t.Errorf("step count = %d, want 5", stepCount)
	}

	// Verify start step is "Work" per office-default.yml.
	var startName string
	err = db.QueryRowContext(ctx,
		`SELECT name FROM workflow_steps WHERE workflow_id = ? AND is_start_step = 1`,
		workflowID).Scan(&startName)
	if err != nil {
		t.Fatalf("query start step: %v", err)
	}
	if startName != "Work" {
		t.Errorf("start step name = %q, want %q", startName, "Work")
	}

	// Verify workspace.office_workflow_id was stamped to this workflow.
	var orchWorkflowID string
	err = db.QueryRowContext(ctx,
		`SELECT office_workflow_id FROM workspaces WHERE id = ?`, wsID).Scan(&orchWorkflowID)
	if err != nil {
		t.Fatalf("query workspace: %v", err)
	}
	if orchWorkflowID != workflowID {
		t.Errorf("workspace.office_workflow_id = %q, want %q", orchWorkflowID, workflowID)
	}

	// Idempotent: calling again should return the same ID and not create
	// a second workflow row.
	sameID, err := repo.EnsureOfficeWorkflow(ctx, wsID)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if sameID != workflowID {
		t.Errorf("idempotent call returned %q, want %q", sameID, workflowID)
	}
}
