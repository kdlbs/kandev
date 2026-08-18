package sqlite

// Cross-package integration coverage for the commit_capture_activated_at
// legacy/NULL comparison in ListSessionCodeStats. The rest of this package's
// tests build task_sessions.started_at and kandev_meta rows by hand (raw
// RFC3339 strings via execOrFatal), which never exercises the actual text
// format task/repository/sqlite writes in production: started_at is bound as
// a Go time.Time through the sqlite3 driver, and commit_capture_activated_at
// is written by migrateSessionCommitsDedupeAndActivation via
// time.Now().UTC().Format(time.RFC3339Nano) — two independent code paths
// that are not guaranteed to agree on a text format. dialect.DateTimeOf
// normalizes both sides before comparing for exactly this reason; these
// tests prove that normalization against the real writers, not a synthetic
// stand-in for them.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// newRealSchemaTestDB opens a fresh SQLite file and boots the REAL
// task/repository/sqlite schema on it (creates kandev_meta and runs
// migrateSessionCommitsDedupeAndActivation for real), then attaches an
// analytics Repository to the same connection — mirroring how
// backendapp/storage.go wires both repositories onto one shared pool.
func newRealSchemaTestDB(t *testing.T) (*sqlx.DB, *tasksqlite.Repository, *Repository) {
	t.Helper()
	tmpDir := t.TempDir()
	rawDB, err := db.OpenSQLite(filepath.Join(tmpDir, "activation.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	sqlxDB := sqlx.NewDb(rawDB, "sqlite3")

	taskRepo, err := tasksqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("task repo NewWithDB: %v", err)
	}
	analyticsRepo, err := NewWithDB(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("analytics repo NewWithDB: %v", err)
	}
	return sqlxDB, taskRepo, analyticsRepo
}

// Regression: a session started well after the real activation marker
// (written by the real migration, not a hand-formatted string) must get a
// real *0, not nil, when it has no commits — proving dialect.DateTimeOf
// correctly normalizes the actual driver-written started_at text against
// the actual RFC3339Nano-formatted kandev_meta value.
func TestListSessionCodeStats_RealSchemaPostActivationSessionReturnsZero(t *testing.T) {
	sqlxDB, taskRepo, analyticsRepo := newRealSchemaTestDB(t)
	ctx := context.Background()

	var activatedAt string
	if err := sqlxDB.Get(&activatedAt, `SELECT value FROM kandev_meta WHERE key = 'commit_capture_activated_at'`); err != nil {
		t.Fatalf("read real activation marker: %v", err)
	}
	if _, err := time.Parse(time.RFC3339Nano, activatedAt); err != nil {
		t.Fatalf("activation marker %q is not RFC3339Nano: %v", activatedAt, err)
	}

	if err := taskRepo.CreateTask(ctx, &taskmodels.Task{ID: "task-post", Title: "T1"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := taskRepo.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID: "sess-post", TaskID: "task-post", StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	results, err := analyticsRepo.ListSessionCodeStats(ctx, models.SessionCodeStatsFilter{})
	if err != nil {
		t.Fatalf("ListSessionCodeStats: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 session, got %d: %+v", len(results), results)
	}
	stat := results[0]
	if stat.LinesAddedCommitted == nil || *stat.LinesAddedCommitted != 0 {
		t.Fatalf("LinesAddedCommitted = %v, want *0 (real driver-written started_at compared against "+
			"real RFC3339Nano activation marker) — a raw-text format mismatch would misclassify this "+
			"post-activation session as legacy", stat.LinesAddedCommitted)
	}
	if stat.LinesDeletedCommitted == nil || *stat.LinesDeletedCommitted != 0 {
		t.Fatalf("LinesDeletedCommitted = %v, want *0", stat.LinesDeletedCommitted)
	}
}

// Regression companion: a session whose real, driver-written started_at
// predates the real activation marker must get nil, not 0.
func TestListSessionCodeStats_RealSchemaPreActivationSessionReturnsNil(t *testing.T) {
	sqlxDB, taskRepo, analyticsRepo := newRealSchemaTestDB(t)
	ctx := context.Background()

	if err := taskRepo.CreateTask(ctx, &taskmodels.Task{ID: "task-pre", Title: "T1"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	// A real driver-written started_at, deliberately long before "now" (and
	// therefore before the activation marker the migration just wrote at
	// boot time, which is always ~"now").
	legacyStartedAt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := taskRepo.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID: "sess-pre", TaskID: "task-pre", StartedAt: legacyStartedAt,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	var rawStartedAt string
	if err := sqlxDB.Get(&rawStartedAt, `SELECT started_at FROM task_sessions WHERE id = 'sess-pre'`); err != nil {
		t.Fatalf("read raw started_at: %v", err)
	}
	t.Logf("real driver-written started_at = %q", rawStartedAt)

	results, err := analyticsRepo.ListSessionCodeStats(ctx, models.SessionCodeStatsFilter{})
	if err != nil {
		t.Fatalf("ListSessionCodeStats: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 session, got %d: %+v", len(results), results)
	}
	stat := results[0]
	if stat.LinesAddedCommitted != nil {
		t.Fatalf("LinesAddedCommitted = %v, want nil (real driver-written started_at predates the real "+
			"activation marker)", *stat.LinesAddedCommitted)
	}
	if stat.LinesDeletedCommitted != nil {
		t.Fatalf("LinesDeletedCommitted = %v, want nil", *stat.LinesDeletedCommitted)
	}
}
