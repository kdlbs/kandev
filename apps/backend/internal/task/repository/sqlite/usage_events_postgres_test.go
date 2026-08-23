package sqlite

// Postgres parity coverage for CreateTaskUsageEvent (docs/specs/task-cost-ledger/spec.md
// AC-11, AC-32): the insert-failure classification calls internaldb.IsForeignKeyViolation
// and internaldb.IsTransientError, which branch on a typed pgconn.PgError on
// Postgres versus a string-matched go-sqlite3 error on SQLite - this file
// proves the classification (and the resulting retry behavior) also holds
// against a real Postgres driver error, not just SQLite's.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresCreateTaskUsageEvent_HappyPath_InsertsRowAndIncrementsRollup(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresTaskSession(t, repo, "task-happy-pg", "session-happy-pg")

	event := newTestUsageEvent("evt-happy-pg", "task-happy-pg", "session-happy-pg")
	if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateTaskUsageEvent: %v", err)
	}

	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM task_usage_events`); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1", count)
	}

	tokensIn, tokensCachedIn, tokensOut, costSubcents := readTaskSessionRollup(t, repo, "session-happy-pg")
	if tokensIn != 100 || tokensCachedIn != 25 || tokensOut != 30 || costSubcents != 42 {
		t.Errorf("rollup = (%d,%d,%d,%d), want (100,25,30,42)", tokensIn, tokensCachedIn, tokensOut, costSubcents)
	}
}

func TestPostgresCreateTaskUsageEvent_DuplicateUsageEventID_ReturnsErrDuplicateNoRollup(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresTaskSession(t, repo, "task-dup-pg", "session-dup-create-pg")

	first := newTestUsageEvent("evt-dup-create-pg", "task-dup-pg", "session-dup-create-pg")
	if err := repo.CreateTaskUsageEvent(context.Background(), first); err != nil {
		t.Fatalf("first CreateTaskUsageEvent: %v", err)
	}

	redelivered := newTestUsageEvent("evt-dup-create-pg", "task-dup-pg", "session-dup-create-pg")
	err = repo.CreateTaskUsageEvent(context.Background(), redelivered)
	if err != ErrDuplicateUsageEvent {
		t.Fatalf("redelivered CreateTaskUsageEvent error = %v, want ErrDuplicateUsageEvent", err)
	}

	tokensIn, _, _, _ := readTaskSessionRollup(t, repo, "session-dup-create-pg")
	if tokensIn != 100 {
		t.Errorf("tokens_in = %d, want 100 (redelivery must not double-increment the rollup)", tokensIn)
	}
}

func TestPostgresCreateTaskUsageEvent_SessionForeignKeyViolation_RetriesOnceWithSessionCleared(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresTask(t, repo, "task-fk-retry-pg")

	event := newTestUsageEvent("evt-fk-retry-pg", "task-fk-retry-pg", "session-does-not-exist-pg")
	if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateTaskUsageEvent: %v", err)
	}

	var sessionID *string
	if err := repo.db.QueryRowx(repo.db.Rebind(
		`SELECT session_id FROM task_usage_events WHERE usage_event_id = ?`), "evt-fk-retry-pg",
	).Scan(&sessionID); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if sessionID != nil {
		t.Errorf("session_id = %v, want nil (FK retry must clear it)", *sessionID)
	}
}

func TestPostgresCreateTaskUsageEvent_SecondForeignKeyFailure_ReturnsError(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}

	event := newTestUsageEvent("evt-fk-double-pg", "task-does-not-exist-pg", "session-does-not-exist-pg")
	createErr := repo.CreateTaskUsageEvent(context.Background(), event)
	if createErr == nil {
		t.Fatal("expected an error when both task_id and session_id are invalid, got nil")
	}
	if createErr == ErrDuplicateUsageEvent {
		t.Fatalf("error = ErrDuplicateUsageEvent, want a foreign-key failure")
	}

	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM task_usage_events`); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 0 {
		t.Errorf("row count = %d, want 0 (both attempts must roll back)", count)
	}
}
