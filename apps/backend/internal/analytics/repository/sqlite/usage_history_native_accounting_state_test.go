package sqlite

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kandev/kandev/internal/analytics/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryNativeAccountingReadinessNoSourceReads(t *testing.T) {
	f := newNativeTurnsFixture(t, false, 4)
	finishNativeTurnsBackfill(t, f.repo)
	completeFixtureProjection(t, f.repo)
	poisonPublishedNativeTurns(t, f.repo)
	check := func(pending bool) {
		withNativeAccountingSourcesDenied(t, f.repo, func() {
			report, err := f.repo.ListSessionUsage(t.Context(), models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace, GroupBy: "model"})
			require.NoError(t, err)
			require.Equal(t, pending, report.ProjectionPending)
			require.Nil(t, report.Totals.Turns)
			if pending {
				require.Nil(t, report.Totals.ReasoningTokens)
			} else {
				require.EqualValues(t, 20, *report.Totals.ReasoningTokens)
			}
			summaries, metadata, err := f.repo.ListUsageDailySummaries(t.Context(), models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace, Timezone: "UTC"})
			require.NoError(t, err)
			require.Equal(t, pending, metadata.Pending)
			require.NotEmpty(t, summaries)
			for _, summary := range summaries {
				require.Nil(t, summary.Turns)
				if pending {
					require.Nil(t, summary.ReasoningTokens)
				}
			}
		})
	}
	check(true)
	repairAllNativeTurns(t, f.repo, 2)
	check(true)
	completeFixtureProjection(t, f.repo)
	check(false)
}

func withNativeAccountingSourcesDenied(t *testing.T, repo *Repository, read func()) {
	t.Helper()
	set := func(deny bool) {
		connection, err := repo.db.Conn(t.Context())
		require.NoError(t, err)
		defer func() { _ = connection.Close() }()
		require.NoError(t, connection.Raw(func(raw any) error {
			var authorizer func(int, string, string, string) int
			if deny {
				authorizer = func(op int, table, _, _ string) int {
					if op == sqlite3.SQLITE_READ && (table == usageMeasurementsTable || table == usageBucketsTable || table == "task_usage_events") {
						return sqlite3.SQLITE_DENY
					}
					return sqlite3.SQLITE_OK
				}
			}
			raw.(*sqlite3.SQLiteConn).RegisterAuthorizer(authorizer)
			return nil
		}))
	}
	set(true)
	defer set(false)
	read()
}

func TestUsageHistoryNativeAccountingReadinessCurrentWriteSQLite(t *testing.T) {
	exerciseNativeAccountingReadiness(t, false)
}

func TestUsageHistoryNativeAccountingReadinessCurrentWritePostgres(t *testing.T) {
	exerciseNativeAccountingReadiness(t, true)
}

func TestUsageHistoryNativeAccountingReadinessPublicationLockPostgres(t *testing.T) {
	f := newNativeTurnsFixture(t, true, 1)
	finishNativeTurnsBackfill(t, f.repo)
	completeFixtureProjection(t, f.repo)
	writer := f.reopen()
	readTx, err := f.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = readTx.Rollback() }()
	metadata, err := f.repo.usageProjectionMetadataTx(t.Context(), readTx, f.scope)
	require.NoError(t, err)
	require.False(t, metadata.Pending)
	writeTx, err := writer.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = writeTx.Rollback() }()
	_, err = writeTx.ExecContext(t.Context(), f.db.Rebind(`SELECT scope_id FROM usage_projection_scopes WHERE scope_id=? FOR UPDATE NOWAIT`), f.scope)
	var lockError *pgconn.PgError
	require.ErrorAs(t, err, &lockError)
	require.Equal(t, "55P03", lockError.Code, "readiness and publication must share one stable scope snapshot")
	require.NoError(t, writeTx.Rollback())
	require.NoError(t, readTx.Commit())
	nextWriteTx, err := writer.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = nextWriteTx.Rollback() }()
	_, _, err = f.repo.lockUsageProjectionScopeTx(t.Context(), nextWriteTx, f.scope)
	require.NoError(t, err, "the normal publisher lock must be available after the read snapshot ends")
}

func exerciseNativeAccountingReadiness(t *testing.T, postgres bool) {
	t.Helper()
	f := newNativeTurnsFixture(t, postgres, 4)
	finishNativeTurnsBackfill(t, f.repo)
	completeFixtureProjection(t, f.repo)
	poisonPublishedNativeTurns(t, f.repo)
	turn, err := f.tasks.GetTurn(t.Context(), "native-root-a")
	require.NoError(t, err)
	event := nativeTurnsEvent(99, []*taskmodels.Turn{turn, turn})
	event.CostSubcents, event.ReasoningOutputTokens = 0, models.Int64(0)
	require.NoError(t, f.tasks.CreateTaskUsageEvent(t.Context(), event))
	var version int
	require.NoError(t, f.db.Get(&version, f.db.Rebind(`SELECT native_accounting_version FROM session_usage_buckets WHERE source_record_id=?`), event.UsageEventID))
	require.Equal(t, nativeUsageAccountingRepairVersion, version)
	require.Equal(t, 4, nativeTurnsPending(t, f.db), "a current write cannot complete the old source cohort")
	assertNativeAccountingSourceState(t, f.repo, f.scope, false)
	done, err := f.repo.repairNativeUsageAccountingBatch(t.Context(), 2)
	require.NoError(t, err)
	require.False(t, done)
	resumed := resumeNativeTurnsFixture(t, f)
	assertNativeAccountingSourceState(t, resumed, f.scope, false)
	filter := models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace, GroupBy: "model"}
	report, err := resumed.ListSessionUsage(t.Context(), filter)
	require.NoError(t, err)
	require.True(t, report.ProjectionPending)
	require.Nil(t, report.Totals.ReasoningTokens)
	repairAllNativeTurns(t, resumed, 2)
	assertNativeAccountingSourceState(t, resumed, f.scope, true)
	report, err = resumed.ListSessionUsage(t.Context(), filter)
	require.NoError(t, err)
	require.True(t, report.ProjectionPending, "last source page is not a publication watermark")
	require.Nil(t, report.Totals.ReasoningTokens)
	completeFixtureProjection(t, resumed)
	report, err = resumed.ListSessionUsage(t.Context(), filter)
	require.NoError(t, err)
	require.False(t, report.ProjectionPending)
	require.EqualValues(t, 20, *report.Totals.ReasoningTokens)
	require.Nil(t, report.Totals.Turns)
	facts, _, err := resumed.ListUsageCanonicalFacts(t.Context(), filter)
	require.NoError(t, err)
	found := false
	for _, fact := range facts {
		if fact.Measurement.SourceRecordID == event.UsageEventID {
			found = true
			require.NotNil(t, fact.Measurement.CostSubcents)
			require.Zero(t, *fact.Measurement.CostSubcents)
			require.NotNil(t, fact.Measurement.ReasoningTokens)
			require.Zero(t, *fact.Measurement.ReasoningTokens)
			require.False(t, fact.TokenMetricAbsent)
			require.False(t, fact.CostMetricAbsent)
		}
	}
	require.True(t, found, "the new source's measured zeros must remain selected canonical evidence")
	var amount, reasoning int64
	require.NoError(t, resumed.db.QueryRowx(resumed.db.Rebind(`SELECT cost_subcents,reasoning_output_tokens FROM session_usage_buckets WHERE source_record_id=?`), event.UsageEventID).Scan(&amount, &reasoning))
	require.Zero(t, amount)
	require.Zero(t, reasoning)
}

func TestUsageHistoryNativeAccountingReadinessRollbackAndCancellation(t *testing.T) {
	f := newNativeTurnsFixture(t, false, 4)
	finishNativeTurnsBackfill(t, f.repo)
	completeFixtureProjection(t, f.repo)
	poisonPublishedNativeTurns(t, f.repo)
	type accountingState struct {
		Version  int   `db:"native_accounting_source_version"`
		Required int64 `db:"native_accounting_required_sequence"`
		Head     int64 `db:"commit_sequence"`
	}
	query := `SELECT native_accounting_source_version,native_accounting_required_sequence,commit_sequence FROM usage_projection_scopes WHERE scope_id=?`
	var before accountingState
	require.NoError(t, f.db.Get(&before, f.db.Rebind(query), f.scope))
	tx, err := f.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, _, err = f.repo.lockUsageProjectionScopeTx(t.Context(), tx, f.scope)
	require.NoError(t, err)
	refs, err := f.repo.nativeUsageAccountingRepairReferencesTx(t.Context(), tx, f.scope, 1)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	change, err := f.repo.repairNativeUsageAccountingRowTx(t.Context(), tx, f.scope, refs[0])
	require.NoError(t, err)
	require.NotNil(t, change)
	var during accountingState
	require.NoError(t, tx.Get(&during, f.db.Rebind(query), f.scope))
	require.Equal(t, before.Version, during.Version)
	require.Greater(t, during.Required, before.Required)
	require.Equal(t, during.Head, during.Required)
	require.NoError(t, tx.Rollback())
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = f.repo.repairNativeUsageAccountingBatch(canceled, 1)
	require.ErrorIs(t, err, context.Canceled)
	var after accountingState
	require.NoError(t, f.db.Get(&after, f.db.Rebind(query), f.scope))
	require.Equal(t, before, after, "source repair, dirty state, and publication sequence must roll back together")
	require.Equal(t, 4, nativeTurnsPending(t, f.db))
}

func assertNativeAccountingSourceState(t *testing.T, repo *Repository, scope string, complete bool) {
	t.Helper()
	var version int
	var required, head int64
	require.NoError(t, repo.db.QueryRowx(repo.db.Rebind(`SELECT native_accounting_source_version,native_accounting_required_sequence,commit_sequence FROM usage_projection_scopes WHERE scope_id=?`), scope).Scan(&version, &required, &head))
	if complete {
		require.Equal(t, nativeUsageAccountingRepairVersion, version)
		require.Equal(t, head, required)
	} else {
		require.Less(t, version, nativeUsageAccountingRepairVersion)
	}
}

func TestUsageHistoryNativeAccountingReadinessEmptyAndExternalScope(t *testing.T) {
	f := newNativeTurnsFixture(t, false, 0)
	var registered int
	require.NoError(t, f.db.Get(&registered, `SELECT COUNT(*) FROM usage_projection_scopes`))
	require.Zero(t, registered, "an old empty scope has no projection row to bootstrap from")
	finishNativeTurnsBackfill(t, f.repo)
	completeFixtureProjection(t, f.repo)
	assertNativeAccountingSourceState(t, f.repo, f.scope, true)
	f.addHostTurns(t)
	completeFixtureProjection(t, f.repo)
	withNativeAccountingSourcesDenied(t, f.repo, func() {
		report, err := f.repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace, GroupBy: "model"})
		require.NoError(t, err)
		require.False(t, report.ProjectionPending)
		require.EqualValues(t, 70, *report.Totals.TotalTokens)
		require.EqualValues(t, 7, *report.Totals.Turns)
		require.Zero(t, *report.Totals.ReasoningTokens)
	})
}
