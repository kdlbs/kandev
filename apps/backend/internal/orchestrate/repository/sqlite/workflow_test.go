package sqlite_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

func TestEnsureOrchestrateWorkflow(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Initialize task repo (creates workspaces, workflows tables + migrations)
	repo, err := taskrepo.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("init task repo: %v", err)
	}

	// Initialize workflow repo (creates workflow_steps table)
	if _, err := workflowrepo.NewWithDB(db, db); err != nil {
		t.Fatalf("init workflow repo: %v", err)
	}

	// Get the default workspace
	ctx := context.Background()
	var wsID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM workspaces LIMIT 1`).Scan(&wsID); err != nil {
		t.Fatalf("get workspace: %v", err)
	}

	// Create orchestrate workflow
	workflowID, err := repo.EnsureOrchestrateWorkflow(ctx, wsID)
	if err != nil {
		t.Fatalf("ensure workflow: %v", err)
	}
	if workflowID == "" {
		t.Fatal("expected non-empty workflow ID")
	}

	// Verify workflow was created
	var wfName string
	var isSystem int
	err = db.QueryRowContext(ctx,
		`SELECT name, is_system FROM workflows WHERE id = ?`, workflowID).Scan(&wfName, &isSystem)
	if err != nil {
		t.Fatalf("query workflow: %v", err)
	}
	if wfName != "Orchestrate" {
		t.Errorf("workflow name = %q, want %q", wfName, "Orchestrate")
	}
	if isSystem != 1 {
		t.Errorf("is_system = %d, want 1", isSystem)
	}

	// Verify 7 steps were created
	var stepCount int
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workflow_steps WHERE workflow_id = ?`, workflowID).Scan(&stepCount)
	if err != nil {
		t.Fatalf("count steps: %v", err)
	}
	if stepCount != 7 {
		t.Errorf("step count = %d, want 7", stepCount)
	}

	// Verify start step is "Todo"
	var startName string
	err = db.QueryRowContext(ctx,
		`SELECT name FROM workflow_steps WHERE workflow_id = ? AND is_start_step = 1`,
		workflowID).Scan(&startName)
	if err != nil {
		t.Fatalf("query start step: %v", err)
	}
	if startName != "Todo" {
		t.Errorf("start step name = %q, want %q", startName, "Todo")
	}

	// Verify workspace was updated
	var orchWorkflowID string
	err = db.QueryRowContext(ctx,
		`SELECT orchestrate_workflow_id FROM workspaces WHERE id = ?`, wsID).Scan(&orchWorkflowID)
	if err != nil {
		t.Fatalf("query workspace: %v", err)
	}
	if orchWorkflowID != workflowID {
		t.Errorf("workspace.orchestrate_workflow_id = %q, want %q", orchWorkflowID, workflowID)
	}

	// Idempotent: calling again should return the same ID
	sameID, err := repo.EnsureOrchestrateWorkflow(ctx, wsID)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if sameID != workflowID {
		t.Errorf("idempotent call returned %q, want %q", sameID, workflowID)
	}
}
