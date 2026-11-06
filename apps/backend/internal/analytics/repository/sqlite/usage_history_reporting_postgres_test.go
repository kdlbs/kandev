package sqlite_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// @covers AC-COSTS-USAGE-HISTORY-005.1 AC-COSTS-USAGE-HISTORY-005.3 AC-COSTS-USAGE-HISTORY-005.4
func TestUsageHistoryPostgresReportingCalendarAndPagination(t *testing.T) {
	f := newProjectionDatabaseFixture(t, true)
	ctx := context.Background()
	point := "2026-03-29T01:00:00Z"
	item := f.item
	item.ItemID = "point"
	item.SourceRecordID = "point"
	item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: &point}
	item = sealProjectionItem(t, item, 1, 30)
	projectionReport(t, f.first, item)
	for range 100 {
		done, err := f.first.BackfillUsageHistory(ctx, 200)
		require.NoError(t, err)
		if done {
			break
		}
	}
	generation, err := f.first.StartUsageHistoryRebuild(ctx, f.scope)
	require.NoError(t, err)
	finishExternalProjection(t, f.first, f.scope, generation)
	start := time.Date(2026, 3, 28, 18, 15, 0, 0, time.UTC)
	end := start.Add(48 * time.Hour)
	filter := models.SessionUsageFilter{WorkspaceID: "projection-ws", GroupBy: "day", Timezone: "Asia/Kathmandu", Start: &start, End: &end, Limit: 1}
	report, err := f.first.ListSessionUsage(ctx, filter)
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	require.Equal(t, "2026-03-29", report.Rows[0].Period)
	require.EqualValues(t, 30, *report.Totals.TotalTokens)
	require.NotEmpty(t, report.ProjectionGeneration)
	filter.Offset = 1
	empty, err := f.second.ListSessionUsage(ctx, filter)
	require.NoError(t, err)
	require.Empty(t, empty.Rows)
	require.EqualValues(t, 30, *empty.Totals.TotalTokens)
	require.Equal(t, report.ProjectionGeneration, empty.ProjectionGeneration)
}

// The reader pauses at the live-link query, after reading page amounts but
// before reading totals. An independent projector cannot publish through it.
func TestUsageHistoryPostgresReportingPublicationSnapshot(t *testing.T) {
	f := newProjectionDatabaseFixture(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	var writers sync.WaitGroup
	for range 100 {
		done, err := f.first.BackfillUsageHistory(ctx, 200)
		require.NoError(t, err)
		if done {
			break
		}
	}
	generation, err := f.first.StartUsageHistoryRebuild(ctx, f.scope)
	require.NoError(t, err)
	finishExternalProjection(t, f.first, f.scope, generation)
	before, err := f.first.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "projection-ws", GroupBy: "task"})
	require.NoError(t, err)
	projectionReport(t, f.first, sealProjectionItem(t, f.item, 2, 40))
	locker, err := f.database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { cancel(); _ = locker.Rollback(); writers.Wait() }()
	_, err = locker.Exec(`LOCK TABLE tasks IN ACCESS EXCLUSIVE MODE`)
	require.NoError(t, err)
	type result struct {
		report *models.TokenUsageReport
		err    error
	}
	read := make(chan result, 1)
	writers.Add(1)
	go func() {
		defer writers.Done()
		report, err := f.first.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "projection-ws", GroupBy: "task"})
		read <- result{report, err}
	}()
	require.Eventually(t, func() bool {
		var blocked bool
		err := f.database.Get(&blocked, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE relation='tasks'::regclass AND mode='AccessShareLock' AND NOT granted)`)
		return err == nil && blocked
	}, 5*time.Second, time.Millisecond, "report must pause at the actual live-link read")
	published := make(chan error, 1)
	writers.Add(1)
	go func() {
		defer writers.Done()
		for range 100 {
			done, err := f.second.AdvanceUsageHistoryProjection(ctx, f.scope, generation, 17)
			if err != nil {
				published <- err
				return
			}
			if done {
				published <- nil
				return
			}
		}
		published <- fmt.Errorf("publication did not finish")
	}()
	require.Eventually(t, func() bool {
		var blocked bool
		err := f.database.Get(&blocked, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE 'SELECT state FROM usage_scope_fences%')`)
		return err == nil && blocked
	}, 5*time.Second, time.Millisecond, "independent publication must wait on the reader's captured-scope fence")
	require.NoError(t, locker.Commit())
	got := <-read
	require.NoError(t, got.err)
	require.EqualValues(t, 100, *got.report.Rows[0].TotalTokens)
	require.EqualValues(t, 100, *got.report.Totals.TotalTokens)
	require.Equal(t, before.ProjectionGeneration, got.report.ProjectionGeneration)
	require.NoError(t, <-published)
	after, err := f.first.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "projection-ws", GroupBy: "task"})
	require.NoError(t, err)
	require.EqualValues(t, 40, *after.Totals.TotalTokens)
	require.NotEqual(t, got.report.ProjectionGeneration, after.ProjectionGeneration)
}
