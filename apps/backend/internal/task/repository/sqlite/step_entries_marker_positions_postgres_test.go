package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/workflow/stepentry"
)

// TestPostgresMarkerPositionsMigrationOnLegacyDB is the Postgres counterpart
// to TestMarkerPositionsMigrationOnLegacyDB (SQLite): workflow_step_entries
// predates the marker_positions column (introduced in #2907), so ADR 0027
// asks for env-gated Postgres replay coverage of the ADD COLUMN migration
// too. It rewinds to a pre-marker_positions schema, re-runs migrations, and
// asserts the column comes back with its default while an existing row
// survives. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresMarkerPositionsMigrationOnLegacyDB(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-pg-marker-positions", "ws-pg-marker-positions", "Task pg", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	var entryID int64
	if err := db.QueryRowContext(ctx, db.Rebind(`
		INSERT INTO workflow_step_entries (task_id, step_id, entry_seq, digest, marker_positions, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id
	`), "task-pg-marker-positions", "step-review", 1, "digest", "0,1", now).Scan(&entryID); err != nil {
		t.Fatalf("seed workflow_step_entries row: %v", err)
	}

	// Rewind to a pre-marker_positions schema, then re-migrate as a new-binary
	// boot would.
	if _, err := db.Exec(`ALTER TABLE workflow_step_entries DROP COLUMN marker_positions`); err != nil {
		t.Fatalf("simulate legacy schema (drop marker_positions): %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations on legacy postgres DB: %v", err)
	}

	var stepID, digest, markerPositions string
	if err := db.QueryRowContext(ctx, db.Rebind(`
		SELECT step_id, digest, marker_positions FROM workflow_step_entries WHERE id = ?
	`), entryID).Scan(&stepID, &digest, &markerPositions); err != nil {
		t.Fatalf("legacy row must survive the migration: %v", err)
	}
	if stepID != "step-review" {
		t.Errorf("step_id lost across migration: got %q", stepID)
	}
	if digest != "digest" {
		t.Errorf("digest lost across migration: got %q", digest)
	}
	if markerPositions != "" {
		t.Errorf("marker_positions = %q, want \"\" (column default after ADD COLUMN)", markerPositions)
	}
}

// TestPostgresUpdateTaskAllocatesStepEntryAndPersistsMarkerPositions is the
// Postgres counterpart to TestUpdateTaskAllocatesStepEntryWhenPendingAllocationPresent
// and TestUpdateTaskPersistsAllocatedMarkerPositions (step_entries_test.go).
// Unlike TestPostgresMarkerPositionsMigrationOnLegacyDB above, which only
// replays the ADD COLUMN migration against a manually inserted row, this
// drives allocateStepEntryIfPending's actual INSERT ... RETURNING through
// the production UpdateTask write path, so it exercises this dialect-
// sensitive method's transaction, Rebind translation, position
// serialization, and AllocationResult propagation against real Postgres —
// per apps/backend/AGENTS.md:265 ("schema replay is insufficient"). Skips
// unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresUpdateTaskAllocatesStepEntryAndPersistsMarkerPositions(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-marker-alloc", Name: "PG Marker Alloc Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	task := &models.Task{
		ID:             "task-pg-marker-alloc",
		WorkspaceID:    "ws-pg-marker-alloc",
		WorkflowID:     "wf-pg-marker-alloc",
		WorkflowStepID: "step-a",
		Title:          "PG Marker Alloc Task",
		Priority:       "medium",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task.WorkflowStepID = "step-review"
	holder := &stepentry.AllocationResult{}
	allocCtx := stepentry.WithResultHolder(ctx, holder)
	allocCtx = stepentry.WithPendingAllocation(allocCtx, stepentry.PendingAllocation{
		StepID: "step-review",
		Digest: "digest-pg-1",
		Positions: []stepentry.EnginePosition{
			{Position: 0, Kind: "clear_decisions"},
			{Position: 2, Kind: "queue_run_for_each_participant"},
		},
	})
	if err := repo.UpdateTask(allocCtx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	if holder.EntryID == 0 {
		t.Fatalf("expected holder.EntryID to be populated, got 0")
	}
	if holder.EntrySeq != 1 {
		t.Fatalf("EntrySeq = %d, want 1", holder.EntrySeq)
	}

	var stepID, digest, markerPositions string
	if err := db.QueryRowContext(ctx, db.Rebind(`
		SELECT step_id, digest, marker_positions FROM workflow_step_entries WHERE id = ?
	`), holder.EntryID).Scan(&stepID, &digest, &markerPositions); err != nil {
		t.Fatalf("query allocated row: %v", err)
	}
	if stepID != "step-review" {
		t.Errorf("step_id = %q, want step-review", stepID)
	}
	if digest != "digest-pg-1" {
		t.Errorf("digest = %q, want digest-pg-1", digest)
	}
	if markerPositions != "0,2" {
		t.Errorf("marker_positions = %q, want %q (the allocated position set, not the column default)", markerPositions, "0,2")
	}

	// A second entry into the same (task, step) increments entry_seq, the
	// same RETURNING-id/COUNT(*) transaction Postgres exercises identically
	// to SQLite's single-writer pool — this is the invariant genuine
	// concurrent Postgres connections could break if the two statements
	// were not on one transaction, which schema replay alone cannot prove.
	// Move away first (plain UpdateTask, no pending allocation) so the
	// re-entry below is a genuine second arrival at step-review.
	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask (move away from step-review): %v", err)
	}

	holder2 := &stepentry.AllocationResult{}
	allocCtx2 := stepentry.WithResultHolder(ctx, holder2)
	allocCtx2 = stepentry.WithPendingAllocation(allocCtx2, stepentry.PendingAllocation{
		StepID:    "step-review",
		Digest:    "digest-pg-2",
		Positions: []stepentry.EnginePosition{{Position: 0, Kind: "clear_decisions"}},
	})
	task.WorkflowStepID = "step-review"
	if err := repo.UpdateTask(allocCtx2, task); err != nil {
		t.Fatalf("UpdateTask (re-enter step-review): %v", err)
	}
	if holder2.EntrySeq != 2 {
		t.Fatalf("second entry EntrySeq = %d, want 2", holder2.EntrySeq)
	}
}
