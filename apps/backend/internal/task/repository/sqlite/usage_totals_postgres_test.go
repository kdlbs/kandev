package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

// GetTaskUsageTotals'/GetSessionUsageTotals' MIN/MAX(occurred_at) columns are
// dialect-sensitive: SQLite returns an aggregate over a TIMESTAMP column as
// TEXT, PostgreSQL returns a native time.Time, and parseUsageTotalsTimestamp
// branches on the scanned Go type to handle both. The SQLite-only tests in
// usage_totals_test.go cannot exercise the PostgreSQL branch at all.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

// TestPostgresGetTaskUsageTotals_SumsAndTimestampsRoundTrip is the Postgres
// counterpart to TestGetTaskUsageTotals_SumsAcrossSessionsAndClearedSessionRows
// and TestGetTaskUsageTotals_FirstAndLastEventAt_MinAndMaxOccurredAt: proves
// the aggregate SUM/COUNT columns and the MIN/MAX(occurred_at) timestamp scan
// both work against a real PostgreSQL instance, not just SQLite.
func TestPostgresGetTaskUsageTotals_SumsAndTimestampsRoundTrip(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresTaskSession(t, repo, "task-totals-pg", "session-totals-pg")
	ctx := context.Background()

	base := time.Date(2026, 8, 23, 4, 0, 0, 0, time.UTC)
	earliest := newTestUsageEvent("evt-totals-pg-1", "task-totals-pg", "session-totals-pg")
	earliest.OccurredAt = base
	earliest.CreatedAt = base
	if err := repo.CreateTaskUsageEvent(ctx, earliest); err != nil {
		t.Fatalf("CreateTaskUsageEvent(evt-totals-pg-1): %v", err)
	}
	latest := newTestUsageEvent("evt-totals-pg-2", "task-totals-pg", "session-totals-pg")
	latest.OccurredAt = base.Add(10 * time.Minute)
	latest.CreatedAt = latest.OccurredAt
	if err := repo.CreateTaskUsageEvent(ctx, latest); err != nil {
		t.Fatalf("CreateTaskUsageEvent(evt-totals-pg-2): %v", err)
	}

	totals, err := repo.GetTaskUsageTotals(ctx, "task-totals-pg")
	if err != nil {
		t.Fatalf("GetTaskUsageTotals: %v", err)
	}
	if totals.EventCount != 2 {
		t.Fatalf("EventCount = %d, want 2", totals.EventCount)
	}
	if totals.TokensIn != 200 {
		t.Errorf("TokensIn = %d, want 200", totals.TokensIn)
	}
	if totals.FirstEventAt == nil || !totals.FirstEventAt.Equal(base) {
		t.Errorf("FirstEventAt = %v, want %v", totals.FirstEventAt, base)
	}
	if totals.LastEventAt == nil || !totals.LastEventAt.Equal(latest.OccurredAt) {
		t.Errorf("LastEventAt = %v, want %v", totals.LastEventAt, latest.OccurredAt)
	}

	sessionTotals, err := repo.GetSessionUsageTotals(ctx, "session-totals-pg")
	if err != nil {
		t.Fatalf("GetSessionUsageTotals: %v", err)
	}
	if sessionTotals.EventCount != 2 {
		t.Errorf("session EventCount = %d, want 2", sessionTotals.EventCount)
	}
}

// TestPostgresGetTaskUsageTotals_NoRows_ReturnsZeroedTotalsWithNilTimestamps
// is the Postgres counterpart to TestGetTaskUsageTotals_NoRows_ReturnsZeroedTotals:
// a scope with no rows must scan a SQL NULL MIN/MAX into nil timestamps
// against PostgreSQL's native time.Time driver values too, not just SQLite's
// TEXT NULL.
func TestPostgresGetTaskUsageTotals_NoRows_ReturnsZeroedTotalsWithNilTimestamps(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresTaskSession(t, repo, "task-totals-empty-pg", "session-totals-empty-pg")

	totals, err := repo.GetTaskUsageTotals(context.Background(), "task-totals-empty-pg")
	if err != nil {
		t.Fatalf("GetTaskUsageTotals: %v", err)
	}
	if totals.EventCount != 0 {
		t.Errorf("EventCount = %d, want 0", totals.EventCount)
	}
	if totals.FirstEventAt != nil || totals.LastEventAt != nil {
		t.Errorf("timestamps = (%v, %v), want (nil, nil)", totals.FirstEventAt, totals.LastEventAt)
	}
	if !totals.OutputTokensComplete {
		t.Error("OutputTokensComplete = false, want true for a scope with no rows")
	}
}
