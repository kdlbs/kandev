//revive:disable:file-length-limit // Regression coverage keeps source-selection scenarios together.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
)

func seedUsageFixtures(t *testing.T, dbConn interface {
	Exec(query string, args ...any) (sql.Result, error)
}) {
	t.Helper()
	// This helper is intentionally kept local to the usage tests. The analytics
	// repository only needs task ownership and session identity to validate a
	// write; it must not depend on a task-service fixture package.
	queries := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, []any{"ws-1", "Usage", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, []any{"board-1", "ws-1", "Board", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO tasks (id, workspace_id, board_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, []any{"task-1", "ws-1", "board-1", "Task", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, []any{"session-1", "task-1", "agent-1", "RUNNING", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
	}
	for _, item := range queries {
		if _, err := dbConn.Exec(item.query, item.args...); err != nil {
			t.Fatalf("seed usage fixture: %v", err)
		}
	}
}

func seedSecondUsageWorkspace(t *testing.T, dbConn interface {
	Exec(query string, args ...any) (sql.Result, error)
}) {
	t.Helper()
	queries := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, []any{"ws-2", "Usage 2", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, []any{"board-2", "ws-2", "Board 2", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO tasks (id, workspace_id, board_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, []any{"task-2", "ws-2", "board-2", "Task 2", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{`INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, []any{"session-2", "task-2", "agent-1", "RUNNING", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
	}
	for _, item := range queries {
		if _, err := dbConn.Exec(item.query, item.args...); err != nil {
			t.Fatalf("seed second usage fixture: %v", err)
		}
	}
}

func usageFixtureMeasurement(observed time.Time) models.SessionUsageMeasurement {
	return models.SessionUsageMeasurement{
		WorkspaceID: "ws-1", TaskID: "task-1", SessionID: "session-1", TranscriptID: "transcript-1",
		Source: "plugin:test", SourceRecordID: "record-1", UsageIdentity: "lifetime:transcript-1",
		Model: "model-a", Provider: "provider-a", SourceVersion: "fixture-1", Revision: 10,
		PayloadDigest: "digest-10", ObservedAt: observed, CollectedAt: observed,
		InputTokens: models.Int64(20), OutputTokens: models.Int64(5), CacheReadTokens: models.Int64(2),
		CacheWriteTokens: models.Int64(1), ReasoningTokens: models.Int64(3), TotalTokens: models.Int64(31),
		Turns: models.Int64(2), CostSubcents: models.Int64(1250), Currency: "USD",
		CostBasis: models.UsageCostBasisEstimated, Coverage: models.UsageCoverageComplete,
		AttributionStatus: models.UsageAttributionAttributed,
	}
}

func createNativeUsageTable(dbConn interface {
	Exec(query string, args ...any) (sql.Result, error)
}) error {
	_, err := dbConn.Exec(`
		CREATE TABLE task_usage_events (
			id INTEGER PRIMARY KEY,
			task_id TEXT NOT NULL,
			session_id TEXT,
			model TEXT,
			provider TEXT,
			tokens_in BIGINT NOT NULL,
			tokens_cached_read BIGINT,
			tokens_cached_write BIGINT,
			tokens_out BIGINT,
			tokens_thought BIGINT,
			tokens_total BIGINT NOT NULL,
			cost_subcents BIGINT,
			cost_source TEXT NOT NULL DEFAULT 'provider_reported',
			estimated INTEGER NOT NULL DEFAULT 0,
			occurred_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`)
	return err
}

func insertNativeUsageEvent(t *testing.T, dbConn interface {
	Exec(query string, args ...any) (sql.Result, error)
}, id int, model, provider string, input, cost, total, output int64, costSource string, occurred time.Time) {
	t.Helper()
	_, err := dbConn.Exec(`
		INSERT INTO task_usage_events (
			id, task_id, session_id, model, provider, tokens_in, tokens_cached_read,
			tokens_cached_write, tokens_out, tokens_thought, tokens_total,
			cost_subcents, cost_source, occurred_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "task-1", "session-1", model, provider, input, 0, 0, output, 0, total, cost,
		costSource, occurred, occurred)
	if err != nil {
		t.Fatalf("insert native usage event: %v", err)
	}
}

func TestSessionUsageUpsertIsIdempotentAndAllowsNewerCorrections(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	ctx := context.Background()
	observed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	measurement := usageFixtureMeasurement(observed)

	results, err := repo.UpsertSessionUsage(ctx, []models.SessionUsageMeasurement{measurement})
	if err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}
	if got := results[0].Status; got != models.UsageWriteApplied {
		t.Fatalf("first status = %q, want applied", got)
	}

	results, err = repo.UpsertSessionUsage(ctx, []models.SessionUsageMeasurement{measurement})
	if err != nil {
		t.Fatalf("repeated upsert failed: %v", err)
	}
	if got := results[0].Status; got != models.UsageWriteUnchanged {
		t.Fatalf("repeated status = %q, want unchanged", got)
	}

	older := measurement
	older.Revision = 9
	older.PayloadDigest = "digest-9"
	older.InputTokens = models.Int64(999)
	older.TotalTokens = models.Int64(1000)
	results, err = repo.UpsertSessionUsage(ctx, []models.SessionUsageMeasurement{older})
	if err != nil {
		t.Fatalf("older upsert failed: %v", err)
	}
	if got := results[0].Status; got != models.UsageWriteStale {
		t.Fatalf("older status = %q, want stale", got)
	}

	conflict := measurement
	conflict.PayloadDigest = "different-payload"
	conflict.InputTokens = models.Int64(999)
	conflict.TotalTokens = models.Int64(1000)
	results, err = repo.UpsertSessionUsage(ctx, []models.SessionUsageMeasurement{conflict})
	if err != nil {
		t.Fatalf("conflicting upsert failed: %v", err)
	}
	if got := results[0].Status; got != models.UsageWriteConflict {
		t.Fatalf("conflict status = %q, want conflict", got)
	}

	correction := measurement
	correction.Revision = 11
	correction.PayloadDigest = "digest-11"
	correction.InputTokens = models.Int64(12)
	correction.TotalTokens = models.Int64(23)
	results, err = repo.UpsertSessionUsage(ctx, []models.SessionUsageMeasurement{correction})
	if err != nil {
		t.Fatalf("correction upsert failed: %v", err)
	}
	if got := results[0].Status; got != models.UsageWriteApplied {
		t.Fatalf("correction status = %q, want applied", got)
	}
	if results[0].Measurement == nil || results[0].Measurement.ID == "" {
		t.Fatalf("correction result lost stored database identity: %#v", results[0].Measurement)
	}
	firstID := results[0].Measurement.ID

	measurements, err := repo.ListSessionUsageMeasurements(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("listing usage failed: %v", err)
	}
	if len(measurements) != 1 {
		t.Fatalf("measurement count = %d, want 1", len(measurements))
	}
	if got := *measurements[0].InputTokens; got != 12 {
		t.Fatalf("corrected input = %d, want 12", got)
	}
	if got := measurements[0].Revision; got != 11 {
		t.Fatalf("stored revision = %d, want 11", got)
	}
	if measurements[0].ID != firstID {
		t.Fatalf("stored identity changed from %q to %q", firstID, measurements[0].ID)
	}
}

func TestSessionUsagePreservesUnknownAndZeroValues(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	known := usageFixtureMeasurement(now)
	zero := known
	zero.SourceRecordID = "record-2"
	zero.UsageIdentity = "lifetime:transcript-2"
	zero.Model = "model-b"
	zero.PayloadDigest = "zero"
	zero.InputTokens = models.Int64(0)
	zero.OutputTokens = nil
	zero.CacheReadTokens = models.Int64(0)
	zero.CacheWriteTokens = nil
	zero.ReasoningTokens = nil
	zero.TotalTokens = models.Int64(0)
	zero.Turns = models.Int64(0)
	zero.CostBasis = models.UsageCostBasisUnknown
	zero.CostSubcents = models.Int64(0)
	if _, err := repo.UpsertSessionUsage(ctx, []models.SessionUsageMeasurement{known, zero}); err != nil {
		t.Fatalf("upsert with nullable values failed: %v", err)
	}

	report, err := repo.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	if err != nil {
		t.Fatalf("listing report failed: %v", err)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("row count = %d, want 2", len(report.Rows))
	}
	var modelB models.TokenUsageRow
	for _, row := range report.Rows {
		if row.Model == "model-b" {
			modelB = row
		}
	}
	if modelB.InputTokens == nil || *modelB.InputTokens != 0 {
		t.Fatalf("zero input was not preserved: %#v", modelB.InputTokens)
	}
	if modelB.OutputTokens != nil || modelB.CostSubcents != nil {
		t.Fatalf("unknown values became zero: output=%v cost=%v", modelB.OutputTokens, modelB.CostSubcents)
	}
	if report.Totals.OutputTokens != nil || report.Totals.CostSubcents != nil {
		t.Fatalf("totals hid unknown values: output=%v cost=%v", report.Totals.OutputTokens, report.Totals.CostSubcents)
	}
}

func TestSessionUsagePrefersDatedBucketsOverLifetimeSnapshot(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	ctx := context.Background()
	base := usageFixtureMeasurement(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	base.InputTokens = models.Int64(100)
	base.TotalTokens = models.Int64(100)
	base.OutputTokens = models.Int64(0)
	base.CacheReadTokens = models.Int64(0)
	base.CacheWriteTokens = models.Int64(0)
	base.ReasoningTokens = models.Int64(0)
	base.CostSubcents = models.Int64(1000)
	if _, err := repo.UpsertSessionUsage(ctx, []models.SessionUsageMeasurement{base}); err != nil {
		t.Fatalf("lifetime upsert failed: %v", err)
	}

	dayOne := base
	dayOne.CoverageKey = "2026-01-02"
	dayOne.SourceDate = "2026-01-02"
	dayOne.PayloadDigest = "day-one"
	dayOne.Revision = 1
	dayOne.InputTokens = models.Int64(40)
	dayOne.TotalTokens = models.Int64(40)
	dayOne.Turns = models.Int64(1)
	dayOne.CostSubcents = models.Int64(400)
	dayTwo := dayOne
	dayTwo.CoverageKey = "2026-01-03"
	dayTwo.SourceDate = "2026-01-03"
	dayTwo.PayloadDigest = "day-two"
	dayTwo.Revision = 2
	dayTwo.InputTokens = models.Int64(60)
	dayTwo.TotalTokens = models.Int64(60)
	dayTwo.Turns = models.Int64(1)
	dayTwo.CostSubcents = models.Int64(600)
	if _, err := repo.UpsertSessionUsage(ctx, []models.SessionUsageMeasurement{dayOne, dayTwo}); err != nil {
		t.Fatalf("dated upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day"})
	if err != nil {
		t.Fatalf("all-time report failed: %v", err)
	}
	if report.Totals.InputTokens == nil || *report.Totals.InputTokens != 100 {
		t.Fatalf("all-time input = %v, want 100", report.Totals.InputTokens)
	}
	if report.Totals.CostSubcents == nil || *report.Totals.CostSubcents != 1000 {
		t.Fatalf("all-time cost = %v, want 1000", report.Totals.CostSubcents)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("all-time row count = %d, want 2 dated rows", len(report.Rows))
	}

	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	report, err = repo.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", Start: &start, End: &end, GroupBy: "day"})
	if err != nil {
		t.Fatalf("ranged report failed: %v", err)
	}
	if report.Totals.InputTokens == nil || *report.Totals.InputTokens != 40 {
		t.Fatalf("ranged input = %v, want 40", report.Totals.InputTokens)
	}
	if report.Totals.CostSubcents == nil || *report.Totals.CostSubcents != 400 {
		t.Fatalf("ranged cost = %v, want 400", report.Totals.CostSubcents)
	}
}

func TestSessionUsageKeepsLifetimeWhenDatedImportIsPartial(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	ctx := context.Background()
	base := usageFixtureMeasurement(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	base.InputTokens = models.Int64(100)
	base.TotalTokens = models.Int64(100)
	base.CostSubcents = models.Int64(1000)
	if _, err := repo.UpsertSessionUsage(ctx, []models.SessionUsageMeasurement{base}); err != nil {
		t.Fatalf("lifetime upsert failed: %v", err)
	}
	day := base
	day.UsageIdentity = "day:transcript-1:2026-01-03"
	day.CoverageKey = "2026-01-03"
	day.SourceDate = "2026-01-03"
	day.PayloadDigest = "partial-day-digest"
	day.InputTokens = models.Int64(20)
	day.TotalTokens = models.Int64(20)
	day.CostSubcents = models.Int64(200)
	if _, err := repo.UpsertSessionUsage(ctx, []models.SessionUsageMeasurement{day}); err != nil {
		t.Fatalf("partial dated upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	if err != nil {
		t.Fatalf("all-time report failed: %v", err)
	}
	if report.Totals.InputTokens == nil || *report.Totals.InputTokens != 100 {
		if report.Totals.InputTokens == nil {
			t.Fatalf("partial import removed lifetime input")
		}
		t.Fatalf("partial import shrank lifetime input to %d", *report.Totals.InputTokens)
	}
	if report.Totals.CostSubcents == nil || *report.Totals.CostSubcents != 1000 {
		if report.Totals.CostSubcents == nil {
			t.Fatalf("partial import removed lifetime cost")
		}
		t.Fatalf("partial import shrank lifetime cost to %d", *report.Totals.CostSubcents)
	}
}

func TestSessionUsageDatedRowsKeepLifetimeSummaryWhenImportIsPartial(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	ctx := context.Background()
	base := usageFixtureMeasurement(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	base.InputTokens = models.Int64(100)
	base.OutputTokens = models.Int64(0)
	base.CacheReadTokens = models.Int64(0)
	base.CacheWriteTokens = models.Int64(0)
	base.ReasoningTokens = models.Int64(0)
	base.TotalTokens = models.Int64(100)
	base.CostSubcents = models.Int64(1000)
	if _, err := repo.UpsertSessionUsage(ctx, []models.SessionUsageMeasurement{base}); err != nil {
		t.Fatalf("lifetime upsert failed: %v", err)
	}
	day := base
	day.UsageIdentity = "date:transcript-1:2026-01-03"
	day.CoverageKey = "2026-01-03"
	day.SourceDate = "2026-01-03"
	day.PayloadDigest = "partial-day-digest"
	day.InputTokens = models.Int64(20)
	day.TotalTokens = models.Int64(20)
	day.Turns = models.Int64(1)
	day.CostSubcents = models.Int64(200)
	if _, err := repo.UpsertSessionUsage(ctx, []models.SessionUsageMeasurement{day}); err != nil {
		t.Fatalf("dated upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day"})
	if err != nil {
		t.Fatalf("dated report failed: %v", err)
	}
	if report.Totals.TotalTokens == nil || *report.Totals.TotalTokens != 100 {
		t.Fatalf("dated all-time total = %v, want lifetime total 100", report.Totals.TotalTokens)
	}
	if len(report.Rows) != 1 || report.Rows[0].TotalTokens == nil || *report.Rows[0].TotalTokens != 20 {
		t.Fatalf("dated rows = %#v, want the imported 20-token bucket", report.Rows)
	}
	if !report.DatedCoverage || !report.UndatedCoverage {
		t.Fatalf("partial coverage = dated:%v undated:%v, want both", report.DatedCoverage, report.UndatedCoverage)
	}
}

func TestSessionUsageDoesNotPlaceLifetimeOnlyDataOnDatedAxis(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	measurement := usageFixtureMeasurement(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{measurement}); err != nil {
		t.Fatalf("lifetime upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{
		WorkspaceID: "ws-1",
		GroupBy:     "day",
	})
	if err != nil {
		t.Fatalf("dated report failed: %v", err)
	}
	if len(report.Rows) != 0 {
		t.Fatalf("dated rows = %#v, want no invented date", report.Rows)
	}
	if report.Totals.TotalTokens == nil || *report.Totals.TotalTokens != 31 {
		t.Fatalf("lifetime total = %v, want 31", report.Totals.TotalTokens)
	}
	if report.DatedCoverage || !report.UndatedCoverage {
		t.Fatalf("coverage = dated:%v undated:%v, want undated only", report.DatedCoverage, report.UndatedCoverage)
	}
}

func TestSessionUsageMarksMonthlyRowsPartialWhenRangeClipsCalendarMonth(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	jan := usageFixtureMeasurement(time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC))
	jan.SourceRecordID = "dated-jan"
	jan.UsageIdentity = "date:transcript-1:2026-01-20"
	jan.SourceDate = "2026-01-20"
	jan.SourceTimezone = "UTC"
	jan.CoverageKey = "2026-01-20"
	jan.PayloadDigest = "dated-jan-digest"
	feb := jan
	feb.SourceRecordID = "dated-feb"
	feb.UsageIdentity = "date:transcript-1:2026-02-05"
	feb.SourceDate = "2026-02-05"
	feb.CoverageKey = "2026-02-05"
	feb.PayloadDigest = "dated-feb-digest"
	feb.ObservedAt = time.Date(2026, 2, 5, 12, 0, 0, 0, time.UTC)
	feb.CollectedAt = feb.ObservedAt
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{jan, feb}); err != nil {
		t.Fatalf("dated usage upsert failed: %v", err)
	}

	start := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{
		WorkspaceID: "ws-1", Start: &start, End: &end, GroupBy: "month",
	})
	if err != nil {
		t.Fatalf("monthly report failed: %v", err)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("monthly rows = %d, want 2", len(report.Rows))
	}
	for _, row := range report.Rows {
		if row.Coverage != models.UsageCoveragePartial {
			t.Fatalf("monthly row %q coverage = %q, want partial", row.Period, row.Coverage)
		}
		if row.EffectiveCostPerMillion != nil {
			t.Fatalf("monthly row %q rate = %v, want unavailable", row.Period, row.EffectiveCostPerMillion)
		}
	}
	if !report.Totals.Partial || report.Totals.EffectiveCostPerMillion != nil {
		t.Fatalf("monthly totals = %#v, want partial with unavailable rate", report.Totals)
	}

	fullStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fullEnd := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	fullReport, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{
		WorkspaceID: "ws-1", Start: &fullStart, End: &fullEnd, GroupBy: "month",
	})
	if err != nil {
		t.Fatalf("full monthly report failed: %v", err)
	}
	if len(fullReport.Rows) != 2 {
		t.Fatalf("full monthly rows = %d, want 2", len(fullReport.Rows))
	}
	for _, row := range fullReport.Rows {
		if len(row.Period) != 7 || row.Period[4] != '-' {
			t.Fatalf("monthly period = %q, want YYYY-MM", row.Period)
		}
		if row.Coverage != models.UsageCoverageComplete || row.EffectiveCostPerMillion == nil {
			t.Fatalf("full monthly row %q = %#v, want complete coverage and a rate", row.Period, row)
		}
	}
}

func TestSessionUsageDerivesBucketIdentityForExplicitIntervals(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	start := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	measurement := usageFixtureMeasurement(start)
	measurement.SourceRecordID = "interval-record"
	measurement.UsageIdentity = "interval:transcript-1:2026-01-04"
	measurement.PayloadDigest = "interval-digest"
	measurement.CoverageStart = &start
	measurement.CoverageEnd = &end
	measurement.CoverageKey = ""
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{measurement}); err != nil {
		t.Fatalf("interval upsert failed: %v", err)
	}

	var lifetimeCount, bucketCount int
	if err := dbConn.Get(&lifetimeCount, `SELECT COUNT(*) FROM session_usage_measurements`); err != nil {
		t.Fatalf("count lifetime rows: %v", err)
	}
	if err := dbConn.Get(&bucketCount, `SELECT COUNT(*) FROM session_usage_buckets`); err != nil {
		t.Fatalf("count bucket rows: %v", err)
	}
	if lifetimeCount != 0 || bucketCount != 1 {
		t.Fatalf("interval storage = lifetime %d, bucket %d; want 0, 1", lifetimeCount, bucketCount)
	}
}

func TestSessionUsageRejectsCrossWorkspaceAndSessionOwnership(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	measurement := usageFixtureMeasurement(time.Now().UTC())
	measurement.WorkspaceID = "ws-other"
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{measurement}); !errors.Is(err, ErrInvalidSessionUsage) {
		t.Fatalf("workspace mismatch error = %v, want ErrInvalidSessionUsage", err)
	}
	measurement = usageFixtureMeasurement(time.Now().UTC())
	measurement.SessionID = "missing-session"
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{measurement}); !errors.Is(err, ErrInvalidSessionUsage) {
		t.Fatalf("session mismatch error = %v, want ErrInvalidSessionUsage", err)
	}
	var count int
	if err := dbConn.Get(&count, `SELECT COUNT(*) FROM session_usage_measurements`); err != nil {
		t.Fatalf("count usage rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("invalid writes changed row count to %d", count)
	}
}

func TestSessionUsageIdentityIsScopedByWorkspace(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	seedSecondUsageWorkspace(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	first := usageFixtureMeasurement(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	second := first
	second.WorkspaceID = "ws-2"
	second.TaskID = "task-2"
	second.SessionID = "session-2"
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{first}); err != nil {
		t.Fatalf("first workspace upsert failed: %v", err)
	}
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{second}); err != nil {
		t.Fatalf("second workspace upsert failed: %v", err)
	}

	firstRows, err := repo.ListSessionUsageMeasurements(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("list first workspace: %v", err)
	}
	secondRows, err := repo.ListSessionUsageMeasurements(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-2"})
	if err != nil {
		t.Fatalf("list second workspace: %v", err)
	}
	if len(firstRows) != 1 || firstRows[0].WorkspaceID != "ws-1" {
		t.Fatalf("first workspace rows = %#v", firstRows)
	}
	if len(secondRows) != 1 || secondRows[0].WorkspaceID != "ws-2" {
		t.Fatalf("second workspace rows = %#v", secondRows)
	}
}

func TestSessionUsageMarksOverlappingSourcesIncomplete(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	base := usageFixtureMeasurement(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	other := base
	other.Source = "plugin:other"
	other.SourceRecordID = "record-other"
	other.PayloadDigest = "digest-other"
	other.CostBasis = models.UsageCostBasisUnknown
	other.CostSubcents = nil
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{base, other}); err != nil {
		t.Fatalf("overlapping source upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	if err != nil {
		t.Fatalf("overlapping source report failed: %v", err)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("overlapping source rows = %d, want 1", len(report.Rows))
	}
	row := report.Rows[0]
	if row.Coverage != models.UsageCoveragePartial {
		t.Fatalf("overlapping source coverage = %q, want partial", row.Coverage)
	}
	if row.EffectiveCostPerMillion != nil {
		t.Fatalf("ambiguous effective rate = %v, want nil", *row.EffectiveCostPerMillion)
	}
	if row.CostSubcents == nil || *row.CostSubcents != 1250 {
		t.Fatalf("selected reported cost = %v, want 1250", row.CostSubcents)
	}
}

func TestSessionUsageKeepsProviderRowsSeparateAndComputesRate(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	first := usageFixtureMeasurement(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	first.TotalTokens = models.Int64(100)
	first.InputTokens = models.Int64(100)
	first.OutputTokens = models.Int64(0)
	first.CacheReadTokens = models.Int64(0)
	first.CacheWriteTokens = models.Int64(0)
	first.ReasoningTokens = models.Int64(0)
	first.CostSubcents = models.Int64(10000)
	second := first
	second.SourceRecordID = "record-2"
	second.UsageIdentity = "lifetime:transcript-2"
	second.Provider = "provider-b"
	second.PayloadDigest = "digest-2"
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{first, second}); err != nil {
		t.Fatalf("provider rows upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	if err != nil {
		t.Fatalf("provider rows report failed: %v", err)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("provider rows = %d, want 2", len(report.Rows))
	}
	for _, row := range report.Rows {
		if row.EffectiveCostPerMillion == nil || *row.EffectiveCostPerMillion != 10000 {
			t.Fatalf("provider %q effective rate = %v, want 10000", row.Provider, row.EffectiveCostPerMillion)
		}
	}
}

func TestSessionUsageKeepsDisjointExternalCoverageAlongsideNativeLedger(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	if _, err := dbConn.Exec(`
		CREATE TABLE task_usage_events (
			id INTEGER PRIMARY KEY,
			task_id TEXT NOT NULL,
			session_id TEXT,
			model TEXT,
			provider TEXT,
			tokens_in BIGINT NOT NULL,
			tokens_cached_read BIGINT,
			tokens_cached_write BIGINT,
			tokens_out BIGINT,
			tokens_thought BIGINT,
			tokens_total BIGINT NOT NULL,
			cost_subcents BIGINT,
			cost_source TEXT NOT NULL DEFAULT 'provider_reported',
			estimated INTEGER NOT NULL DEFAULT 0,
			occurred_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`); err != nil {
		t.Fatalf("create native usage table: %v", err)
	}
	nativeTime := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	if _, err := dbConn.Exec(`
		INSERT INTO task_usage_events (
			id, task_id, session_id, model, provider, tokens_in, tokens_cached_read,
			tokens_cached_write, tokens_out, tokens_thought, tokens_total,
			cost_subcents, cost_source, occurred_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		1, "task-1", "session-1", "model-a", "provider-a", 10, 0, 0, 2, 0, 12, 500, "provider_reported",
		nativeTime, nativeTime); err != nil {
		t.Fatalf("insert native usage event: %v", err)
	}

	dayTwo := usageFixtureMeasurement(nativeTime)
	dayTwo.SourceRecordID = "external-day-2"
	dayTwo.UsageIdentity = "day:transcript-1:2026-01-02"
	dayTwo.PayloadDigest = "external-day-2-digest"
	dayTwo.CoverageKey = "2026-01-02"
	dayTwo.SourceDate = "2026-01-02"
	dayTwo.SourceTimezone = "UTC"
	dayTwo.CostSubcents = models.Int64(200)
	dayThree := dayTwo
	dayThree.SourceRecordID = "external-day-3"
	dayThree.UsageIdentity = "day:transcript-1:2026-01-03"
	dayThree.PayloadDigest = "external-day-3-digest"
	dayThree.CoverageKey = "2026-01-03"
	dayThree.SourceDate = "2026-01-03"
	dayThree.CostSubcents = models.Int64(300)
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{dayTwo, dayThree}); err != nil {
		t.Fatalf("external dated upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day"})
	if err != nil {
		t.Fatalf("native plus external report failed: %v", err)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("native plus disjoint external rows = %d, want 2", len(report.Rows))
	}
	for _, row := range report.Rows {
		switch row.Period {
		case "2026-01-02":
			if row.CostSubcents == nil || *row.CostSubcents != 200 {
				t.Fatalf("full external day cost = %v, want 200", row.CostSubcents)
			}
		case "2026-01-03":
			if row.CostSubcents == nil || *row.CostSubcents != 300 {
				t.Fatalf("disjoint external day cost = %v, want 300", row.CostSubcents)
			}
		default:
			t.Fatalf("unexpected period %q", row.Period)
		}
	}
}

func TestSessionUsageMapsNativeCostSourcesAndKeepsUnknownCostUnavailable(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	if err := createNativeUsageTable(dbConn); err != nil {
		t.Fatalf("create native usage table: %v", err)
	}
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	insertNativeUsageEvent(t, dbConn, 1, "model-unpriced", "provider-a", 10, 0, 10, 0, "unpriced", now)
	insertNativeUsageEvent(t, dbConn, 2, "model-estimated", "provider-b", 20, 250, 20, 0, "models_dev_list", now)

	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	if err != nil {
		t.Fatalf("native report failed: %v", err)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("native rows = %d, want 2", len(report.Rows))
	}
	for _, row := range report.Rows {
		switch row.Model {
		case "model-unpriced":
			if row.CostSubcents != nil || row.CostBasis != models.UsageCostBasisUnknown || row.CostCoverage != models.UsageCoverageMissing {
				t.Fatalf("unpriced row exposed cost: %#v", row)
			}
			if row.EffectiveCostPerMillion != nil {
				t.Fatalf("unpriced effective rate = %v, want nil", row.EffectiveCostPerMillion)
			}
		case "model-estimated":
			if row.CostSubcents == nil || *row.CostSubcents != 250 || row.CostBasis != models.UsageCostBasisEstimated || row.CostCoverage != models.UsageCoverageComplete {
				t.Fatalf("estimated row cost fields = %#v", row)
			}
		default:
			t.Fatalf("unexpected native model %q", row.Model)
		}
	}
}

func TestSessionUsageUsesEquivalentNativeCoverageWithoutDoubleCounting(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	if err := createNativeUsageTable(dbConn); err != nil {
		t.Fatalf("create native usage table: %v", err)
	}
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	insertNativeUsageEvent(t, dbConn, 1, "model-a", "provider-a", 31, 200, 31, 0, "provider_reported", now)

	external := usageFixtureMeasurement(now)
	external.SourceRecordID = "external-day"
	external.UsageIdentity = "day:transcript-1:2026-01-02"
	external.CoverageKey = "2026-01-02"
	external.SourceDate = "2026-01-02"
	external.Turns = models.Int64(1)
	// Keep the coverage interval equivalent while making the values disagree.
	// Source selection must use the documented native reported precedence for
	// equivalent coverage; it must not require the two sources to happen to
	// report identical totals.
	external.InputTokens = models.Int64(999)
	external.OutputTokens = models.Int64(0)
	external.CacheReadTokens = models.Int64(0)
	external.CacheWriteTokens = models.Int64(0)
	external.ReasoningTokens = models.Int64(0)
	external.TotalTokens = models.Int64(999)
	external.CostSubcents = models.Int64(100)
	external.CoverageStart = &now
	coverageEnd := now.Add(time.Nanosecond)
	external.CoverageEnd = &coverageEnd
	external.PayloadDigest = "external-day-digest"
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{external}); err != nil {
		t.Fatalf("external day upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day"})
	if err != nil {
		t.Fatalf("equivalent report failed: %v", err)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("equivalent rows = %d, want 1", len(report.Rows))
	}
	row := report.Rows[0]
	if row.CostSubcents == nil || *row.CostSubcents != 200 || row.Source != "native" {
		t.Fatalf("equivalent native row = %#v", row)
	}
}

func TestSessionUsageMergesCostCorrectionWithoutReplacingNativeTokens(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	if err := createNativeUsageTable(dbConn); err != nil {
		t.Fatalf("create native usage table: %v", err)
	}
	start := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	insertNativeUsageEvent(t, dbConn, 1, "model-a", "provider-a", 10, 100, 10, 0, "unpriced", start)

	correction := usageFixtureMeasurement(start)
	correction.SourceRecordID = "external-cost-correction"
	correction.UsageIdentity = "cost:transcript-1"
	correction.PayloadDigest = "external-cost-correction-digest"
	correction.InputTokens = nil
	correction.OutputTokens = nil
	correction.CacheReadTokens = nil
	correction.CacheWriteTokens = nil
	correction.ReasoningTokens = nil
	correction.TotalTokens = nil
	correction.Turns = nil
	correction.CostSubcents = models.Int64(777)
	correction.CostBasis = models.UsageCostBasisReported
	correction.CostCoverage = models.UsageCoverageComplete
	correction.CoverageStart = &start
	end := start.Add(time.Nanosecond)
	correction.CoverageEnd = &end
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{correction}); err != nil {
		t.Fatalf("cost correction upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	if err != nil {
		t.Fatalf("cost correction report failed: %v", err)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("cost correction rows = %d, want 1", len(report.Rows))
	}
	row := report.Rows[0]
	if row.TotalTokens == nil || *row.TotalTokens != 10 {
		t.Fatalf("cost correction replaced native tokens: %#v", row)
	}
	if row.CostSubcents == nil || *row.CostSubcents != 777 || row.CostBasis != models.UsageCostBasisReported {
		t.Fatalf("cost correction was not selected: %#v", row)
	}
	if row.Source != usageMixedValue {
		t.Fatalf("cost correction source = %q, want mixed", row.Source)
	}
}

func TestSessionUsageKeepsPrunedTranscriptIdentitiesSeparate(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	first := usageFixtureMeasurement(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	first.SessionID = ""
	first.TranscriptID = "transcript-a"
	first.SourceRecordID = "record-a"
	first.UsageIdentity = "lifetime:transcript-a"
	first.PayloadDigest = "digest-a"
	second := first
	second.TranscriptID = "transcript-b"
	second.SourceRecordID = "record-b"
	second.UsageIdentity = "lifetime:transcript-b"
	second.PayloadDigest = "digest-b"
	second.InputTokens = models.Int64(7)
	second.TotalTokens = models.Int64(7)
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{first, second}); err != nil {
		t.Fatalf("pruned transcript upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	if err != nil {
		t.Fatalf("pruned transcript report failed: %v", err)
	}
	if len(report.Rows) != 1 || report.Totals.TotalTokens == nil || *report.Totals.TotalTokens != 38 {
		t.Fatalf("pruned transcript identities collapsed: rows=%#v totals=%#v", report.Rows, report.Totals)
	}
}

func TestSessionUsageResolvesOverlappingRowsWithDifferentCoverageKeys(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	first := usageFixtureMeasurement(start)
	first.SourceRecordID = "overlap-a"
	first.UsageIdentity = "interval-a"
	first.CoverageKey = "bucket-a"
	first.CoverageStart = &start
	first.CoverageEnd = &end
	first.PayloadDigest = "overlap-a-digest"
	second := first
	second.SourceRecordID = "overlap-b"
	second.UsageIdentity = "interval-b"
	second.CoverageKey = "bucket-b"
	second.PayloadDigest = "overlap-b-digest"
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{first, second}); err != nil {
		t.Fatalf("overlap upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	if err != nil {
		t.Fatalf("overlap report failed: %v", err)
	}
	if len(report.Rows) != 1 || report.Rows[0].TotalTokens == nil || *report.Rows[0].TotalTokens != 31 {
		t.Fatalf("overlapping coverage keys were double counted: %#v", report.Rows)
	}
	if report.Rows[0].Coverage != models.UsageCoveragePartial {
		t.Fatalf("overlapping coverage = %q, want partial", report.Rows[0].Coverage)
	}
}

func TestSessionUsageRejectsCategoriesLargerThanTotal(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	measurement := usageFixtureMeasurement(time.Now().UTC())
	measurement.TotalTokens = models.Int64(10)
	measurement.InputTokens = models.Int64(20)
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{measurement}); err == nil {
		t.Fatal("upsert accepted token categories larger than total")
	}
}

func TestSessionUsageUsesCompleteEquivalentNativeIntervalCoverage(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	if err := createNativeUsageTable(dbConn); err != nil {
		t.Fatalf("create native usage table: %v", err)
	}
	start := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	second := start.Add(time.Nanosecond)
	insertNativeUsageEvent(t, dbConn, 1, "model-a", "provider-a", 10, 100, 10, 0, "provider_reported", start)
	insertNativeUsageEvent(t, dbConn, 2, "model-a", "provider-a", 20, 200, 20, 0, "provider_reported", second)

	end := second.Add(time.Nanosecond)
	external := usageFixtureMeasurement(start)
	external.SourceRecordID = "external-interval"
	external.UsageIdentity = "interval:transcript-1:2026-01-02"
	external.PayloadDigest = "external-interval-digest"
	external.InputTokens = models.Int64(30)
	external.OutputTokens = models.Int64(0)
	external.CacheReadTokens = models.Int64(0)
	external.CacheWriteTokens = models.Int64(0)
	external.ReasoningTokens = models.Int64(0)
	external.TotalTokens = models.Int64(30)
	external.Turns = models.Int64(2)
	external.CostSubcents = models.Int64(300)
	external.CoverageStart = &start
	external.CoverageEnd = &end
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{external}); err != nil {
		t.Fatalf("external interval upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	if err != nil {
		t.Fatalf("complete equivalent report failed: %v", err)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("complete equivalent rows = %d, want 1", len(report.Rows))
	}
	row := report.Rows[0]
	if row.Source != "native" || row.TotalTokens == nil || *row.TotalTokens != 30 || row.CostSubcents == nil || *row.CostSubcents != 300 {
		total, cost := int64(-1), int64(-1)
		if row.TotalTokens != nil {
			total = *row.TotalTokens
		}
		if row.CostSubcents != nil {
			cost = *row.CostSubcents
		}
		t.Fatalf("complete equivalent native row = source:%q total:%d cost:%d row:%#v", row.Source, total, cost, row)
	}
}

func TestSessionUsageUsesNativeCoverageWhenExternalCandidateIsPartial(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	if err := createNativeUsageTable(dbConn); err != nil {
		t.Fatalf("create native usage table: %v", err)
	}
	start := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Nanosecond)
	insertNativeUsageEvent(t, dbConn, 1, "model-a", "provider-a", 10, 100, 10, 0, "provider_reported", start)

	external := usageFixtureMeasurement(start)
	external.SourceRecordID = "external-partial-interval"
	external.UsageIdentity = "partial-interval:transcript-1"
	external.PayloadDigest = "external-partial-interval-digest"
	external.InputTokens = models.Int64(10)
	external.OutputTokens = models.Int64(0)
	external.CacheReadTokens = models.Int64(0)
	external.CacheWriteTokens = models.Int64(0)
	external.ReasoningTokens = models.Int64(0)
	external.TotalTokens = models.Int64(10)
	external.Turns = models.Int64(1)
	external.CostSubcents = models.Int64(100)
	external.Coverage = models.UsageCoveragePartial
	external.CoverageStart = &start
	external.CoverageEnd = &end
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{external}); err != nil {
		t.Fatalf("partial external interval upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	if err != nil {
		t.Fatalf("partial external interval report failed: %v", err)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("partial external interval rows = %d, want 1", len(report.Rows))
	}
	row := report.Rows[0]
	if row.Source != "native" || row.Coverage != models.UsageCoverageComplete || row.CostSubcents == nil || *row.CostSubcents != 100 {
		t.Fatalf("partial external interval selection = %#v, want complete native coverage", row)
	}
}

func TestSessionUsageMatchesUnknownProviderToKnownNativeCoverage(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	if err := createNativeUsageTable(dbConn); err != nil {
		t.Fatalf("create native usage table: %v", err)
	}
	start := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Nanosecond)
	insertNativeUsageEvent(t, dbConn, 1, "model-a", "provider-a", 10, 100, 10, 0, "provider_reported", start)

	external := usageFixtureMeasurement(start)
	external.SourceRecordID = "external-unknown-provider"
	external.UsageIdentity = "unknown-provider:transcript-1"
	external.PayloadDigest = "external-unknown-provider-digest"
	external.Provider = ""
	external.InputTokens = models.Int64(10)
	external.OutputTokens = models.Int64(0)
	external.CacheReadTokens = models.Int64(0)
	external.CacheWriteTokens = models.Int64(0)
	external.ReasoningTokens = models.Int64(0)
	external.TotalTokens = models.Int64(10)
	external.Turns = models.Int64(1)
	external.CostSubcents = models.Int64(100)
	external.CoverageStart = &start
	external.CoverageEnd = &end
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{external}); err != nil {
		t.Fatalf("unknown-provider external upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	if err != nil {
		t.Fatalf("unknown-provider report failed: %v", err)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("unknown-provider rows = %d, want 1", len(report.Rows))
	}
	row := report.Rows[0]
	if row.Provider != "provider-a" || row.TotalTokens == nil || *row.TotalTokens != 10 {
		t.Fatalf("unknown-provider canonical row = %#v, want one known native row", row)
	}
}

func TestSessionUsageKeepsLifetimeWhenNativeCoverageIsOnlyPartial(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	if err := createNativeUsageTable(dbConn); err != nil {
		t.Fatalf("create native usage table: %v", err)
	}
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	insertNativeUsageEvent(t, dbConn, 1, "model-a", "provider-a", 10, 50, 10, 0, "provider_reported", now)

	external := usageFixtureMeasurement(now)
	external.InputTokens = models.Int64(100)
	external.OutputTokens = models.Int64(0)
	external.CacheReadTokens = models.Int64(0)
	external.CacheWriteTokens = models.Int64(0)
	external.ReasoningTokens = models.Int64(0)
	external.TotalTokens = models.Int64(100)
	external.CostSubcents = models.Int64(500)
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{external}); err != nil {
		t.Fatalf("external lifetime upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	if err != nil {
		t.Fatalf("partial native report failed: %v", err)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("partial native rows = %d, want 1", len(report.Rows))
	}
	row := report.Rows[0]
	if row.Source != "plugin:test" || row.InputTokens == nil || *row.InputTokens != 100 {
		t.Fatalf("partial native selection = %#v, want the fuller lifetime observation", row)
	}
	if row.CostSubcents == nil || *row.CostSubcents != 500 {
		t.Fatalf("partial native cost = %v, want 500", row.CostSubcents)
	}
}

func TestSessionUsageKeepsExternalDayWhenOneNativeEventOnlyPartiallyCoversIt(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	if err := createNativeUsageTable(dbConn); err != nil {
		t.Fatalf("create native usage table: %v", err)
	}
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	insertNativeUsageEvent(t, dbConn, 1, "model-a", "provider-a", 31, 200, 31, 0, "provider_reported", now)

	external := usageFixtureMeasurement(now)
	external.SourceRecordID = "external-day"
	external.UsageIdentity = "date:transcript-1:2026-01-02"
	external.CoverageKey = "2026-01-02"
	external.SourceDate = "2026-01-02"
	external.SourceTimezone = "UTC"
	external.InputTokens = models.Int64(100)
	external.OutputTokens = models.Int64(0)
	external.CacheReadTokens = models.Int64(0)
	external.CacheWriteTokens = models.Int64(0)
	external.ReasoningTokens = models.Int64(0)
	external.TotalTokens = models.Int64(100)
	external.CostSubcents = models.Int64(500)
	external.PayloadDigest = "external-day-digest"
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{external}); err != nil {
		t.Fatalf("external day upsert failed: %v", err)
	}

	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day"})
	if err != nil {
		t.Fatalf("partial day report failed: %v", err)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("partial day rows = %d, want 1", len(report.Rows))
	}
	row := report.Rows[0]
	if row.Source != "plugin:test" || row.InputTokens == nil || *row.InputTokens != 100 {
		t.Fatalf("partial day selection = %#v, want the fuller external observation", row)
	}
	if row.CostSubcents == nil || *row.CostSubcents != 500 {
		t.Fatalf("partial day cost = %v, want 500", row.CostSubcents)
	}
}

func TestSessionUsageReportsHistoryMetadataOutsideSelectedRange(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	collected := time.Date(2026, 1, 3, 1, 0, 0, 0, time.UTC)
	measurement := usageFixtureMeasurement(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC))
	measurement.SourceRecordID = "dated-old"
	measurement.UsageIdentity = "date:transcript-1:2026-01-02"
	measurement.SourceDate = "2026-01-02"
	measurement.SourceTimezone = "UTC"
	measurement.CoverageKey = "2026-01-02"
	measurement.CollectedAt = collected
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{measurement}); err != nil {
		t.Fatalf("dated usage upsert failed: %v", err)
	}

	start := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{
		WorkspaceID: "ws-1", Start: &start, End: &end, GroupBy: "day",
	})
	if err != nil {
		t.Fatalf("out-of-range report failed: %v", err)
	}
	if report.HasHistory != true || report.HasRangeData {
		t.Fatalf("out-of-range metadata = has_history:%v has_range_data:%v, want history without range data", report.HasHistory, report.HasRangeData)
	}
	if !report.DatedCoverage || report.UndatedCoverage {
		t.Fatalf("dated metadata = dated:%v undated:%v, want dated only", report.DatedCoverage, report.UndatedCoverage)
	}
	if report.LastUpdated == nil || !report.LastUpdated.Equal(collected) {
		t.Fatalf("last updated = %v, want %v", report.LastUpdated, collected)
	}
}

func TestSessionUsageReportsNativeHistoryMetadataOutsideSelectedRange(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	if err := createNativeUsageTable(dbConn); err != nil {
		t.Fatalf("create native usage table: %v", err)
	}
	occurred := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	insertNativeUsageEvent(t, dbConn, 1, "model-a", "provider-a", 10, 50, 10, 0, "provider_reported", occurred)

	start := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{
		WorkspaceID: "ws-1", Start: &start, End: &end, GroupBy: "day",
	})
	if err != nil {
		t.Fatalf("native out-of-range report failed: %v", err)
	}
	if !report.HasHistory || !report.HasNativeHistory || report.HasRangeData {
		t.Fatalf("native out-of-range metadata = %#v, want history without range data", report)
	}
	if !report.DatedCoverage || report.UndatedCoverage {
		t.Fatalf("native coverage metadata = dated:%v undated:%v, want dated only", report.DatedCoverage, report.UndatedCoverage)
	}
	if report.LastUpdated == nil || !report.LastUpdated.Equal(occurred) {
		t.Fatalf("native last updated = %v, want %v", report.LastUpdated, occurred)
	}
}

func TestSessionUsageReportsLifetimeFreshnessForDatedView(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	collected := time.Date(2026, 1, 3, 1, 0, 0, 0, time.UTC)
	measurement := usageFixtureMeasurement(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC))
	measurement.CoverageKey = ""
	measurement.SourceDate = ""
	measurement.UsageIdentity = "lifetime:transcript-lifetime"
	measurement.SourceRecordID = "lifetime-only"
	measurement.CollectedAt = collected
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{measurement}); err != nil {
		t.Fatalf("lifetime usage upsert failed: %v", err)
	}

	start := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{
		WorkspaceID: "ws-1", Start: &start, End: &end, GroupBy: "day",
	})
	if err != nil {
		t.Fatalf("lifetime metadata report failed: %v", err)
	}
	if !report.HasHistory || report.HasRangeData {
		t.Fatalf("lifetime metadata = has_history:%v has_range_data:%v, want history without range data", report.HasHistory, report.HasRangeData)
	}
	if !report.UndatedCoverage || report.DatedCoverage {
		t.Fatalf("lifetime coverage = dated:%v undated:%v, want undated only", report.DatedCoverage, report.UndatedCoverage)
	}
	if report.LastUpdated == nil || !report.LastUpdated.Equal(collected) {
		t.Fatalf("lifetime last updated = %v, want %v", report.LastUpdated, collected)
	}
}

func TestSessionUsageFollowsTaskRetentionRules(t *testing.T) {
	dbConn := createTestDB(t)
	seedUsageFixtures(t, dbConn)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}
	measurement := usageFixtureMeasurement(time.Now().UTC())
	if _, err := repo.UpsertSessionUsage(context.Background(), []models.SessionUsageMeasurement{measurement}); err != nil {
		t.Fatalf("usage upsert failed: %v", err)
	}

	if _, err := dbConn.Exec(`DELETE FROM task_sessions WHERE id = ?`, "session-1"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	var sessionID string
	if err := dbConn.Get(&sessionID, `SELECT COALESCE(session_id, '') FROM session_usage_measurements WHERE source_record_id = ?`, measurement.SourceRecordID); err != nil {
		t.Fatalf("read retained usage: %v", err)
	}
	if sessionID != "" {
		t.Fatalf("pruned session id = %q, want empty reference", sessionID)
	}
	retained, err := repo.ListSessionUsageMeasurements(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("list after session prune: %v", err)
	}
	if len(retained) != 1 {
		t.Fatalf("retained measurements = %d, want 1", len(retained))
	}

	if _, err := dbConn.Exec(`DELETE FROM tasks WHERE id = ?`, "task-1"); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	var count int
	if err := dbConn.Get(&count, `SELECT COUNT(*) FROM session_usage_measurements`); err != nil {
		t.Fatalf("count usage after task delete: %v", err)
	}
	if count != 0 {
		t.Fatalf("usage rows after task delete = %d, want 0", count)
	}
}

func TestUsageRowsKeepUnknownNumericValuesLastWhenSortingDescending(t *testing.T) {
	knownCost := int64(100)
	knownTokens := int64(10)
	rows := []models.TokenUsageRow{
		{Key: "unknown", CostSubcents: nil, TotalTokens: nil},
		{Key: "known", CostSubcents: &knownCost, TotalTokens: &knownTokens},
	}

	sortUsageRows(rows, "cost", false)
	if rows[0].Key != "known" || rows[1].Key != "unknown" {
		t.Fatalf("descending cost order = [%s, %s], want known before unknown", rows[0].Key, rows[1].Key)
	}
	sortUsageRows(rows, "tokens", false)
	if rows[0].Key != "known" || rows[1].Key != "unknown" {
		t.Fatalf("descending token order = [%s, %s], want known before unknown", rows[0].Key, rows[1].Key)
	}
}
