package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresCreateWorkspacePauseWithActivity_RejectsSecondPause is the
// PostgreSQL twin of TestCreateWorkspacePauseWithActivity_RejectsSecondPause.
// isUniqueConstraintErr originally matched only SQLite's error text, so a
// concurrent second pause on Postgres surfaced as a raw "insert workspace
// pause" error instead of the documented ErrWorkspaceAlreadyPaused —
// breaking AC-001.3 (the caller re-reads and returns the existing record)
// on every Postgres deployment. Running the real insert-conflict path
// against Postgres makes that dialect gap fail loudly.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresCreateWorkspacePauseWithActivity_RejectsSecondPause(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	first := &models.WorkspacePause{
		WorkspaceID:   "pg-ws-1",
		Reason:        "incident",
		CreatedBy:     "user-1",
		CreatedByKind: "user",
	}
	firstActivity := &models.ActivityEntry{
		WorkspaceID: "pg-ws-1",
		ActorType:   models.ActivityActorUser,
		ActorID:     "user-1",
		Action:      models.ActivityActionWorkspacePaused,
		TargetType:  models.ActivityTargetWorkspace,
		TargetID:    "pg-ws-1",
		Details:     "incident",
	}
	if err := repo.CreateWorkspacePauseWithActivity(ctx, first, firstActivity); err != nil {
		t.Fatalf("create first pause: %v", err)
	}

	second := &models.WorkspacePause{
		WorkspaceID:   "pg-ws-1",
		Reason:        "second attempt",
		CreatedBy:     "user-2",
		CreatedByKind: "user",
	}
	secondActivity := &models.ActivityEntry{
		WorkspaceID: "pg-ws-1",
		ActorType:   models.ActivityActorUser,
		ActorID:     "user-2",
		Action:      models.ActivityActionWorkspacePaused,
		TargetType:  models.ActivityTargetWorkspace,
		TargetID:    "pg-ws-1",
	}
	err = repo.CreateWorkspacePauseWithActivity(ctx, second, secondActivity)
	if !errors.Is(err, sqlite.ErrWorkspaceAlreadyPaused) {
		t.Fatalf("expected ErrWorkspaceAlreadyPaused, got %v", err)
	}

	active, err := repo.GetActiveWorkspacePause(ctx, "pg-ws-1")
	if err != nil {
		t.Fatalf("get active pause: %v", err)
	}
	if active == nil || active.ID != first.ID {
		t.Fatalf("expected the first pause to remain active, got %+v", active)
	}
}

// TestPostgresCreatePauseSkippedRoutineRun_DedupsPerPause is the PostgreSQL
// twin of TestCreatePauseSkippedRoutineRun_DedupsPerPause, proving
// AC-002.13's dedup (partial unique index idx_office_routine_run_pause_once)
// degrades from a clean no-op to a raw error on Postgres without a
// dialect-aware unique-constraint check.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresCreatePauseSkippedRoutineRun_DedupsPerPause(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	now := time.Now().UTC()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO office_routines (id, workspace_id, name, task_template, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "pg-routine-1", "pg-ws-1", "Nightly", "{}", now, now); err != nil {
		t.Fatalf("seed routine: %v", err)
	}

	created, err := repo.CreatePauseSkippedRoutineRun(ctx, "pg-routine-1", "", "cron", "pg-pause-1")
	if err != nil {
		t.Fatalf("first skip: %v", err)
	}
	if !created {
		t.Fatal("expected first skip to create a row")
	}

	created, err = repo.CreatePauseSkippedRoutineRun(ctx, "pg-routine-1", "", "cron", "pg-pause-1")
	if err != nil {
		t.Fatalf("second skip: %v", err)
	}
	if created {
		t.Fatal("expected second skip under the same pause to be a no-op")
	}

	var count int
	if err := repo.ReaderDB().Get(&count, `SELECT COUNT(*) FROM office_routine_runs WHERE routine_id = 'pg-routine-1'`); err != nil {
		t.Fatalf("count routine runs: %v", err)
	}
	if count != 1 {
		t.Fatalf("routine run row count = %d, want 1", count)
	}
}
