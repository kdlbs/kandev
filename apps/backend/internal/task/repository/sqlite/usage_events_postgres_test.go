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

// TestPostgresListTaskUsageEvents_EstimatedRoundTrips pins the INTEGER-to-bool
// scan used by the read path. The insert stores the flag as 0/1 for both
// dialects, so this keeps the Postgres driver conversion explicit.
func TestPostgresListTaskUsageEvents_EstimatedRoundTrips(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresTask(t, repo, "task-estimated-pg")

	event := newTestUsageEvent("evt-estimated-pg", "task-estimated-pg", "")
	event.Estimated = true
	if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateTaskUsageEvent: %v", err)
	}

	events, err := repo.ListTaskUsageEvents(context.Background(), "task-estimated-pg", 0)
	if err != nil {
		t.Fatalf("ListTaskUsageEvents: %v", err)
	}
	if len(events) != 1 || !events[0].Estimated {
		t.Fatalf("events = %+v, want one estimated event", events)
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

// TestPostgresCreateTaskUsageEvent_TokensOutCrossesInt32ThroughRealWriter
// covers AC-28 for tokens_out specifically: session_usage_postgres_test.go
// only pushes tokens_cached_in (already BIGINT before this feature) past
// int32, and TestPostgresTaskSessionsRollupColumns_LegacyIntegerSurvivesWidening
// pushes cost_subcents past int32 via a raw UPDATE that bypasses the ledger
// writer entirely - a version of either test passing before the migration
// widened tokens_out/cost_subcents to BIGINT would prove nothing about this
// column. This test seeds the rollup near the int32 ceiling via the real
// IncrementTaskSessionUsageTx path, then commits a further ledger event
// through CreateTaskUsageEvent (the insert+rollup transaction AC-11
// requires) whose tokens_out delta crosses the boundary, and asserts that
// transaction commits with the correct summed value.
func TestPostgresCreateTaskUsageEvent_TokensOutCrossesInt32ThroughRealWriter(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresTaskSession(t, repo, "task-out-overflow-pg", "session-out-overflow-pg")

	const nearInt32Ceiling = int64(2_147_483_600) // int32 max is 2,147,483,647
	if err := repo.IncrementTaskSessionUsageTx(
		context.Background(), nil, "session-out-overflow-pg", 0, 0, nearInt32Ceiling, 0,
	); err != nil {
		t.Fatalf("seed rollup near int32 ceiling: %v", err)
	}

	event := newTestUsageEvent("evt-out-overflow-pg", "task-out-overflow-pg", "session-out-overflow-pg")
	crossingDelta := int64(200)
	event.TokensOut = &crossingDelta
	event.TokensTotal = event.TokensIn + crossingDelta
	if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateTaskUsageEvent must not error when tokens_out crosses int32: %v", err)
	}

	_, _, tokensOut, _ := readTaskSessionRollup(t, repo, "session-out-overflow-pg")
	want := nearInt32Ceiling + crossingDelta
	if tokensOut != want {
		t.Errorf("tokens_out = %d, want %d (a delta crossing int32 must commit through the real writer)",
			tokensOut, want)
	}
}

// TestPostgresCreateTaskUsageEvent_CostSubcentsCrossesInt32ThroughRealWriter
// is TestPostgresCreateTaskUsageEvent_TokensOutCrossesInt32ThroughRealWriter's
// twin for cost_subcents, closing the other half of the gap: the only
// existing beyond-int32 coverage for this column
// (TestPostgresTaskSessionsRollupColumns_LegacyIntegerSurvivesWidening) uses
// a raw UPDATE, not CreateTaskUsageEvent.
func TestPostgresCreateTaskUsageEvent_CostSubcentsCrossesInt32ThroughRealWriter(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresTaskSession(t, repo, "task-cost-overflow-pg", "session-cost-overflow-pg")

	const nearInt32Ceiling = int64(2_147_483_600) // int32 max is 2,147,483,647
	if err := repo.IncrementTaskSessionUsageTx(
		context.Background(), nil, "session-cost-overflow-pg", 0, 0, 0, nearInt32Ceiling,
	); err != nil {
		t.Fatalf("seed rollup near int32 ceiling: %v", err)
	}

	event := newTestUsageEvent("evt-cost-overflow-pg", "task-cost-overflow-pg", "session-cost-overflow-pg")
	event.CostSubcents = 200
	if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateTaskUsageEvent must not error when cost_subcents crosses int32: %v", err)
	}

	_, _, _, costSubcents := readTaskSessionRollup(t, repo, "session-cost-overflow-pg")
	want := nearInt32Ceiling + 200
	if costSubcents != want {
		t.Errorf("cost_subcents = %d, want %d (a delta crossing int32 must commit through the real writer)",
			costSubcents, want)
	}
}
