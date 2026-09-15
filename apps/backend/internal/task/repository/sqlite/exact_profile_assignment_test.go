package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func TestExactProfileAssignmentSchemaExists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exact-profile-assignment.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	var tableName string
	if err := db.Get(&tableName, `
		SELECT name
		FROM sqlite_master
		WHERE type = 'table' AND name = 'task_exact_profile_assignments'
	`); err != nil {
		t.Fatalf("exact profile assignment table is missing: %v", err)
	}
}

func TestUpsertExactProfileAssignmentGuardsGeneration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exact-profile-assignment-generation.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	now := time.Now().UTC().Round(0)
	assignment := &models.ExactProfileAssignment{
		TaskID: "task-exact-assignment", WorkspaceID: "workspace-1", AgentProfileID: "profile-1",
		ProfileRevision: now, Generation: 1, SourceWorkflowID: "workflow-1", SourceWorkflowStepID: "step-1",
		SourceTaskState: "TODO",
	}
	if _, err := db.Exec(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		assignment.TaskID, assignment.WorkspaceID, "Exact profile assignment", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	changed, err := repo.UpsertExactProfileAssignment(context.Background(), assignment)
	if err != nil {
		t.Fatalf("create assignment: %v", err)
	}
	if !changed {
		t.Fatal("initial assignment was not recorded")
	}

	stale := *assignment
	stale.AgentProfileID = "profile-stale"
	changed, err = repo.UpsertExactProfileAssignment(context.Background(), &stale)
	if !errors.Is(err, models.ErrExactProfileAssignmentGeneration) {
		t.Fatalf("replay assignment error = %v, want generation rejection", err)
	}
	if changed {
		t.Fatal("same generation with a different payload must not replace assignment")
	}
}

func TestAssignExactProfileAssignmentActivatesAtomically(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exact-profile-assignment-atomic.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	now := time.Now().UTC().Round(0)
	assignment := &models.ExactProfileAssignment{
		TaskID: "task-atomic", WorkspaceID: "workspace-1", AgentProfileID: "profile-1",
		ProfileRevision: now, Generation: 1,
	}
	if _, err := db.Exec(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		assignment.TaskID, assignment.WorkspaceID, "Exact profile assignment", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	changed, err := repo.AssignExactProfileAssignment(context.Background(), assignment)
	if err != nil || !changed {
		t.Fatalf("assign exact profile = (%v, %v), want (true, nil)", changed, err)
	}
	stored, err := repo.GetExactProfileAssignment(context.Background(), assignment.TaskID)
	if err != nil || stored == nil || !stored.Active {
		t.Fatalf("stored assignment = %#v, %v; want active assignment", stored, err)
	}

	changed, err = repo.AssignExactProfileAssignment(context.Background(), assignment)
	if err != nil || changed {
		t.Fatalf("replay exact profile = (%v, %v), want (false, nil)", changed, err)
	}
}

func TestActivateExactProfileAssignmentRejectsStaleGeneration(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	if _, err := repo.UpsertExactProfileAssignment(context.Background(), assignment); err != nil {
		t.Fatal(err)
	}
	changed, err := repo.ActivateExactProfileAssignment(context.Background(), assignment.TaskID, 2)
	if err != nil || changed {
		t.Fatalf("stale activation = (%v, %v)", changed, err)
	}
	changed, err = repo.ActivateExactProfileAssignment(context.Background(), assignment.TaskID, 1)
	if err != nil || !changed {
		t.Fatalf("activation = (%v, %v)", changed, err)
	}
}

func newExactProfileAssignmentRepo(t *testing.T) (*Repository, *models.ExactProfileAssignment) {
	t.Helper()
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "exact.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Round(0)
	a := &models.ExactProfileAssignment{TaskID: "task-test", WorkspaceID: "workspace-1", AgentProfileID: "profile-1", ProfileRevision: now, Generation: 1}
	if _, err := db.Exec(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, a.TaskID, a.WorkspaceID, "Exact", now, now); err != nil {
		t.Fatal(err)
	}
	return repo, a
}
