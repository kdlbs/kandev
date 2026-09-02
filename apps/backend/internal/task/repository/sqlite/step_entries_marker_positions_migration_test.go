package sqlite

// Same-database replay coverage for the workflow_step_entries.marker_positions
// ADD COLUMN migration (AC-OFFICE-STEP-ENTRY-DISPATCH-002.4/.9), mandated by
// ADR 0027 for every schema change: workflow_step_entries predates this
// column (introduced in #2907), so an operator's existing rows must survive
// a boot of the new binary. See postgres_schema_test.go's
// TestPostgresMarkerPositionsMigrationOnLegacyDB for the Postgres
// counterpart.

import (
	"context"
	"testing"
)

func TestMarkerPositionsMigrationOnLegacyDB(t *testing.T) {
	repo := newStepEntriesTestRepo(t)

	entryID := allocateOneStepEntry(t, repo, "task-legacy-marker-positions", "step-review")
	rows := stepEntryRowsForTask(t, repo, "task-legacy-marker-positions")
	if len(rows) != 1 || rows[0].id != entryID {
		t.Fatalf("setup: expected one row with id %d, got %+v", entryID, rows)
	}

	// Rewind to a pre-marker_positions schema: drop the column so the row now
	// looks like one written by a binary that predates it.
	if _, err := repo.db.Exec(`ALTER TABLE workflow_step_entries DROP COLUMN marker_positions`); err != nil {
		t.Fatalf("simulate legacy schema (drop marker_positions): %v", err)
	}

	// Re-run migrations, exactly as a boot of the new binary would. The
	// idempotent ADD COLUMN must re-add marker_positions without erroring.
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations on legacy DB: %v", err)
	}

	var markerPositions string
	var stepID, digest string
	err := repo.db.QueryRowContext(context.Background(), repo.db.Rebind(`
		SELECT step_id, digest, marker_positions FROM workflow_step_entries WHERE id = ?
	`), entryID).Scan(&stepID, &digest, &markerPositions)
	if err != nil {
		t.Fatalf("legacy row must survive the migration: %v", err)
	}
	if stepID != "step-review" {
		t.Errorf("step_id lost across migration: got %q", stepID)
	}
	if digest != "digest" {
		t.Errorf("digest lost across migration: got %q", digest)
	}
	// The column default backfills every pre-existing row to '' — the
	// migration deliberately does not (and cannot) reconstruct the original
	// allocated position set from the step's current declaration, which may
	// have changed since the row was written.
	if markerPositions != "" {
		t.Errorf("marker_positions = %q, want \"\" (column default for a row that predates the column)", markerPositions)
	}
}
