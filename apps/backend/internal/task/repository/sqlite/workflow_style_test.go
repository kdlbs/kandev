package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

// newRepoForWorkflowStyleTests builds a fresh repo against an on-disk SQLite
// file, exercising the schema migrations end-to-end so the workflows.style
// ALTER actually runs.
func newRepoForWorkflowStyleTests(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "wf-style.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo
}

func TestWorkflowStyle_RoundTrip(t *testing.T) {
	repo := newRepoForWorkflowStyleTests(t)
	ctx := context.Background()

	wf := &models.Workflow{
		WorkspaceID: "ws-1",
		Name:        "Office",
		Description: "office workflow",
		Style:       "office",
	}
	if err := repo.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetWorkflow(ctx, wf.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Style != "office" {
		t.Fatalf("expected style 'office', got %q", got.Style)
	}

	got.Style = "custom"
	if err := repo.UpdateWorkflow(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, _ := repo.GetWorkflow(ctx, wf.ID)
	if got2.Style != "custom" {
		t.Fatalf("expected style 'custom' after update, got %q", got2.Style)
	}
}

func TestWorkflowStyle_DefaultsToKanbanWhenUnset(t *testing.T) {
	repo := newRepoForWorkflowStyleTests(t)
	ctx := context.Background()

	wf := &models.Workflow{
		WorkspaceID: "ws-1",
		Name:        "Default style",
	}
	if err := repo.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetWorkflow(ctx, wf.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Style != "kanban" {
		t.Fatalf("expected default style 'kanban', got %q", got.Style)
	}
}

func TestWorkflowStyle_NormalizesUnknownInput(t *testing.T) {
	repo := newRepoForWorkflowStyleTests(t)
	ctx := context.Background()

	wf := &models.Workflow{
		WorkspaceID: "ws-1",
		Name:        "Invalid style",
		Style:       "agile", // not in the allowed set; collapses to "kanban"
	}
	if err := repo.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.GetWorkflow(ctx, wf.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Style != "kanban" {
		t.Fatalf("expected unknown style to normalize to 'kanban', got %q", got.Style)
	}
}

func TestWorkflowStyle_AppearsInList(t *testing.T) {
	repo := newRepoForWorkflowStyleTests(t)
	ctx := context.Background()

	if err := repo.CreateWorkflow(ctx, &models.Workflow{WorkspaceID: "ws-list", Name: "Office one", Style: "office"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{WorkspaceID: "ws-list", Name: "Plain"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	rows, err := repo.ListWorkflows(ctx, "ws-list", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 workflows, got %d", len(rows))
	}
	styles := map[string]int{}
	for _, w := range rows {
		styles[w.Style]++
	}
	if styles["office"] != 1 || styles["kanban"] != 1 {
		t.Fatalf("unexpected style distribution: %v", styles)
	}
}
