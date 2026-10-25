package delivery_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/delivery"
	"github.com/kandev/kandev/internal/persistence"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

// newTestDB opens a file-backed SQLite database with foreign key
// enforcement on (required for the R5-F2 cascade behaviour under test),
// initializes the task schema (workspaces/repositories/tasks, the FK
// targets), and returns the raw handle.
func newTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "delivery-ledger.db")
	rawConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(rawConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := taskrepo.Provide(db, db, nil); err != nil {
		t.Fatalf("init task schema: %v", err)
	}
	return db
}

// newTestRepo returns a delivery repository and the underlying db handle.
func newTestRepo(t *testing.T) (*delivery.Repository, *sqlx.DB) {
	t.Helper()
	db := newTestDB(t)
	repo, err := delivery.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init delivery schema: %v", err)
	}
	return repo, db
}

func seedWorkspace(t *testing.T, db *sqlx.DB, id string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`), id, id, now, now); err != nil {
		t.Fatalf("seed workspace %s: %v", id, err)
	}
}

func seedRepository(t *testing.T, db *sqlx.DB, id, workspaceID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO repositories (id, workspace_id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), id, workspaceID, id, now, now); err != nil {
		t.Fatalf("seed repository %s: %v", id, err)
	}
}

func seedTask(t *testing.T, db *sqlx.DB, id, workspaceID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), id, workspaceID, id, now, now); err != nil {
		t.Fatalf("seed task %s: %v", id, err)
	}
}

func TestNewWithDB_CreatesTableAndActivates(t *testing.T) {
	_, db := newTestRepo(t)

	var name string
	if err := db.Get(&name, `SELECT name FROM sqlite_master WHERE type='table' AND name='task_delivery_ledger'`); err != nil {
		t.Fatalf("table missing: %v", err)
	}

	val, err := persistence.ReadMetaKey(db, "telemetry.delivery_ledger.activated_at")
	if err != nil {
		t.Fatalf("read activation key: %v", err)
	}
	if val == "" {
		t.Fatal("expected telemetry.delivery_ledger.activated_at to be written after first boot")
	}
	if _, err := time.Parse(time.RFC3339, val); err != nil {
		t.Fatalf("activation value %q not RFC3339: %v", val, err)
	}
}

func TestNewWithDB_ReplaySafeAndActivationNeverOverwritten(t *testing.T) {
	db := newTestDB(t)
	if _, err := delivery.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("first init: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`UPDATE kandev_meta SET value = ? WHERE key = ?`),
		"sentinel-value", "telemetry.delivery_ledger.activated_at"); err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}

	if _, err := delivery.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay 1: %v", err)
	}
	if _, err := delivery.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay 2: %v", err)
	}

	got, err := persistence.ReadMetaKey(db, "telemetry.delivery_ledger.activated_at")
	if err != nil {
		t.Fatalf("read activation key: %v", err)
	}
	if got != "sentinel-value" {
		t.Fatalf("activation key = %q after replay, want unchanged sentinel-value", got)
	}
}

// TestWorkspaceDeletion_CascadesLedgerRows is the R5-F2 regression test:
// deleting a workspace must not start failing once a ledger row exists,
// because repositories cascade from workspaces and the ledger's
// repository_id FK must cascade transitively.
func TestWorkspaceDeletion_CascadesLedgerRows(t *testing.T) {
	repo, db := newTestRepo(t)
	taskRepo, _, err := taskrepo.Provide(db, db, nil)
	if err != nil {
		t.Fatalf("task repo: %v", err)
	}

	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")

	ctx := context.Background()
	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: "task-1", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2},
		EvaluatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := taskRepo.DeleteWorkspace(ctx, "ws-1"); err != nil {
		t.Fatalf("delete workspace: %v (workspace deletion must not fail once a ledger row exists)", err)
	}

	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM task_delivery_ledger`); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("ledger rows after workspace delete = %d, want 0 (cascade)", count)
	}
}
