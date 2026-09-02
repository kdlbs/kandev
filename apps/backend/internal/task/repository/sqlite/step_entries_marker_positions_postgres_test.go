package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
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
