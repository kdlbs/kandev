package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func newRepoForWorkflowSourceTests(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "workflow-source-test.db")
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

// TestWorkflowSource_RoundTrip verifies the workflow-sync provenance columns
// survive create, get, list, and update.
func TestWorkflowSource_RoundTrip(t *testing.T) {
	repo := newRepoForWorkflowSourceTests(t)
	ctx := context.Background()

	wf := &models.Workflow{
		WorkspaceID: "ws-1",
		Name:        "Synced Flow",
		Source:      models.WorkflowSourceGitHub,
		SourcePath:  "flows/dev.yml",
	}
	if err := repo.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	got, err := repo.GetWorkflow(ctx, wf.ID)
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	if got.Source != models.WorkflowSourceGitHub || got.SourcePath != "flows/dev.yml" {
		t.Fatalf("get: source roundtrip failed: %q %q", got.Source, got.SourcePath)
	}

	listed, err := repo.ListWorkflows(ctx, "ws-1", true)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if len(listed) != 1 || listed[0].Source != models.WorkflowSourceGitHub {
		t.Fatalf("list: source roundtrip failed: %+v", listed)
	}

	got.SourcePath = "flows/renamed.yml"
	if err := repo.UpdateWorkflow(ctx, got); err != nil {
		t.Fatalf("update workflow: %v", err)
	}
	updated, err := repo.GetWorkflow(ctx, wf.ID)
	if err != nil {
		t.Fatalf("get updated workflow: %v", err)
	}
	if updated.SourcePath != "flows/renamed.yml" {
		t.Fatalf("update: source_path not persisted: %q", updated.SourcePath)
	}
}

// TestWorkflowSource_SchemaReplay verifies the source/source_path migrations
// are idempotent: opening the repository a second time on the same database
// (which replays initSchema + runMigrations) must succeed and keep the data.
func TestWorkflowSource_SchemaReplay(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workflow-source-replay.db")
	ctx := context.Background()

	openRepo := func() (*Repository, *sqlx.DB) {
		dbConn, err := db.OpenSQLite(dbPath)
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
		repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
		if err != nil {
			t.Fatalf("new repo: %v", err)
		}
		return repo, sqlxDB
	}

	repo, conn := openRepo()
	wf := &models.Workflow{
		WorkspaceID: "ws-1",
		Name:        "Synced Flow",
		Source:      models.WorkflowSourceGitHub,
		SourcePath:  "flows/dev.yml",
	}
	if err := repo.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	_ = conn.Close()

	repo, conn = openRepo()
	defer func() { _ = conn.Close() }()
	got, err := repo.GetWorkflow(ctx, wf.ID)
	if err != nil {
		t.Fatalf("get workflow after replay: %v", err)
	}
	if got.Source != models.WorkflowSourceGitHub || got.SourcePath != "flows/dev.yml" {
		t.Fatalf("replay lost provenance: %q %q", got.Source, got.SourcePath)
	}
}

// TestWorkflowSource_DefaultsToManual verifies workflows created without an
// explicit source read back as manual (both fresh inserts and the scan-side
// normalization of the migration default).
func TestWorkflowSource_DefaultsToManual(t *testing.T) {
	repo := newRepoForWorkflowSourceTests(t)
	ctx := context.Background()

	wf := &models.Workflow{WorkspaceID: "ws-1", Name: "Personal Flow"}
	if err := repo.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	got, err := repo.GetWorkflow(ctx, wf.ID)
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	if got.Source != models.WorkflowSourceManual {
		t.Fatalf("expected manual source default, got %q", got.Source)
	}
	if got.SourcePath != "" {
		t.Fatalf("expected empty source_path, got %q", got.SourcePath)
	}
}
