package sqlite

// Postgres parity coverage for ListSessionCodeStats' commit-capture-activation
// comparison. dialect.NaiveUTCTimestampOf / dialect.DateTimeOf are dialect-
// sensitive by design: task_sessions.started_at is a naive TIMESTAMP column
// (no embedded zone) always written as UTC wall-clock time, and
// kandev_meta.value is text carrying an explicit RFC3339Nano "Z" offset. A
// bare `(started_at)::timestamptz` cast on Postgres reinterprets the naive
// value in the session's `timezone` GUC instead of UTC, silently shifting it
// by the server's offset and misclassifying a post-activation session as
// legacy — this reproduces only against a real, non-UTC-configured Postgres
// server; a SQLite-only test suite cannot catch it. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set; CI runs these in postgres-boot.

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresListSessionCodeStatsActivationComparison drives both the
// task-repository and analytics-repository writers/readers over the SAME
// Postgres connection (mirroring backendapp/storage.go's shared writer/reader
// pool), so both the real driver-written started_at text and the real
// RFC3339Nano-formatted activation marker are exercised — not a synthetic
// stand-in for either.
func TestPostgresListSessionCodeStatsActivationComparison(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	taskRepo, err := tasksqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("task repo NewWithDB: %v", err)
	}
	analyticsRepo, err := NewWithDB(db, db)
	if err != nil {
		t.Fatalf("analytics repo NewWithDB: %v", err)
	}

	var activatedAt string
	if err := db.Get(&activatedAt, db.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`), commitCaptureActivatedAtMetaKey); err != nil {
		t.Fatalf("read activation marker: %v", err)
	}
	if _, err := time.Parse(time.RFC3339Nano, activatedAt); err != nil {
		t.Fatalf("activation marker %q is not RFC3339Nano: %v", activatedAt, err)
	}

	ctx := context.Background()
	if err := taskRepo.CreateTask(ctx, &taskmodels.Task{ID: "task-pg-post", Title: "T1"}); err != nil {
		t.Fatalf("CreateTask post: %v", err)
	}
	// Started moments after the activation marker was written (real
	// driver-bound time.Time, not a hand-formatted string).
	if err := taskRepo.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID: "sess-pg-post", TaskID: "task-pg-post", StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTaskSession post: %v", err)
	}

	if err := taskRepo.CreateTask(ctx, &taskmodels.Task{ID: "task-pg-pre", Title: "T2"}); err != nil {
		t.Fatalf("CreateTask pre: %v", err)
	}
	if err := taskRepo.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID: "sess-pg-pre", TaskID: "task-pg-pre", StartedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateTaskSession pre: %v", err)
	}

	results, err := analyticsRepo.ListSessionCodeStats(ctx, models.SessionCodeStatsFilter{})
	if err != nil {
		t.Fatalf("ListSessionCodeStats: %v", err)
	}
	byID := make(map[string]*models.SessionCodeStats, len(results))
	for _, r := range results {
		byID[r.SessionID] = r
	}

	post, ok := byID["sess-pg-post"]
	if !ok {
		t.Fatalf("sess-pg-post missing from results: %+v", results)
	}
	if post.LinesAddedCommitted == nil || *post.LinesAddedCommitted != 0 {
		t.Errorf("post-activation session: LinesAddedCommitted = %v, want *0 — "+
			"a naive-timestamp-vs-session-timezone bug would misclassify this as legacy", post.LinesAddedCommitted)
	}
	if post.LinesDeletedCommitted == nil || *post.LinesDeletedCommitted != 0 {
		t.Errorf("post-activation session: LinesDeletedCommitted = %v, want *0", post.LinesDeletedCommitted)
	}

	pre, ok := byID["sess-pg-pre"]
	if !ok {
		t.Fatalf("sess-pg-pre missing from results: %+v", results)
	}
	if pre.LinesAddedCommitted != nil {
		t.Errorf("pre-activation session: LinesAddedCommitted = %v, want nil", *pre.LinesAddedCommitted)
	}
	if pre.LinesDeletedCommitted != nil {
		t.Errorf("pre-activation session: LinesDeletedCommitted = %v, want nil", *pre.LinesDeletedCommitted)
	}
}
