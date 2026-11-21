package sqlite

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/analytics/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestUsageHistoryNativeAccountingRepairSQLite(t *testing.T) {
	exerciseNativeAccountingRepair(t, false)
}

func TestUsageHistoryNativeAccountingRepairPostgres(t *testing.T) {
	exerciseNativeAccountingRepair(t, true)
}

func exerciseNativeAccountingRepair(t *testing.T, postgres bool) {
	t.Helper()
	f := newNativeTurnsFixture(t, postgres, 205)
	f.addHostTurns(t)
	f.addUndatedNative(t)
	finishNativeTurnsBackfill(t, f.repo)
	completeFixtureProjection(t, f.repo)
	poisonPublishedNativeTurns(t, f.repo)
	require.Equal(t, 206, nativeTurnsPending(t, f.db))
	before := nativeTurnsSourceSnapshot(t, f.repo)
	ledger, rollup := nativeTurnsLedgerState(t, f.repo)
	head := nativeTurnsHead(t, f.db)
	assertNativeTurnsAggregateGuard(t, f.repo)

	done, err := f.repo.repairNativeUsageAccountingBatch(t.Context(), 1000)
	require.NoError(t, err)
	require.False(t, done)
	require.Equal(t, 6, nativeTurnsPending(t, f.db), "one transaction must repair at most 200 rows across both representations")
	assertNativeAccountingMutations(t, f.repo, head, 200)
	pausedHead := nativeTurnsHead(t, f.db)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = f.repo.repairNativeUsageAccountingBatch(canceled, 200)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, pausedHead, nativeTurnsHead(t, f.db))
	require.Equal(t, 6, nativeTurnsPending(t, f.db))

	resumed := resumeNativeTurnsFixture(t, f)
	done, err = resumed.repairNativeUsageAccountingBatch(t.Context(), 200)
	require.NoError(t, err)
	require.True(t, done)
	require.Zero(t, nativeTurnsPending(t, resumed.db))
	assertNativeAccountingMutations(t, resumed, head, 206)
	for key, row := range before {
		if row.Measurement.Source == usageSourceNative {
			row.Measurement.Turns = nil
			before[key] = row
		}
	}
	require.Equal(t, before, nativeTurnsSourceSnapshot(t, resumed), "repair changes no raw token/cost/category, identity, revision, digest, calendar, or collector evidence")
	afterLedger, afterRollup := nativeTurnsLedgerState(t, resumed)
	require.Equal(t, ledger, afterLedger)
	require.Equal(t, rollup, afterRollup)
	idleHead := nativeTurnsHead(t, resumed.db)
	done, err = resumed.repairNativeUsageAccountingBatch(t.Context(), 200)
	require.NoError(t, err)
	require.True(t, done)
	require.Equal(t, idleHead, nativeTurnsHead(t, resumed.db), "restart/no-op repair must not allocate another mutation sequence")

	completeFixtureProjection(t, resumed)
	assertPublishedNativeTurnsUnavailable(t, resumed, f.scope)
	generation, err := resumed.StartUsageHistoryRebuild(t.Context(), f.scope)
	require.NoError(t, err)
	drainProjection(t, resumed, f.scope, generation)
	assertPublishedNativeTurnsUnavailable(t, resumed, f.scope)
}

func assertNativeAccountingMutations(t *testing.T, repo *Repository, after int64, count int) {
	t.Helper()
	var payloads []string
	require.NoError(t, repo.db.Select(&payloads, repo.db.Rebind(`SELECT payload FROM usage_projection_log WHERE commit_sequence>? ORDER BY commit_sequence`), after))
	require.Len(t, payloads, count)
	for _, payload := range payloads {
		var replay usageProjectionReplay
		require.NoError(t, decodeUsageProjection(payload, &replay))
		change := replay.Mutation
		require.Equal(t, nativeUsageAccountingRepairOperation, change.Operation)
		require.NotNil(t, change.Before)
		require.NotNil(t, change.After)
		require.Equal(t, usageSourceNative, change.Before.Source)
		require.Nil(t, change.After.Turns)
		change.Before.Turns = nil
		require.Equal(t, change.Before, change.After)
		require.Equal(t, change.OldPartition, change.NewPartition)
	}
}

func nativeTurnsLedgerState(t *testing.T, repo *Repository) ([]string, []int64) {
	t.Helper()
	var rows []struct {
		Event   string `db:"usage_event_id"`
		Turn    string `db:"turn_id"`
		Binding string `db:"usage_binding_id"`
		Total   int64  `db:"tokens_total"`
		Cost    int64  `db:"cost_subcents"`
	}
	require.NoError(t, repo.db.Select(&rows, `SELECT usage_event_id, COALESCE(turn_id,'') AS turn_id, usage_binding_id, tokens_total, cost_subcents FROM task_usage_events ORDER BY usage_event_id`))
	var events []string
	for _, row := range rows {
		events = append(events, row.Event+"/"+row.Turn+"/"+row.Binding)
	}
	var in, out, cost, total, ledgerCost int64
	require.NoError(t, repo.db.QueryRowx(repo.db.Rebind(`SELECT tokens_in, tokens_out, cost_subcents FROM task_sessions WHERE id=?`), nativeTurnsSession).Scan(&in, &out, &cost))
	require.NoError(t, repo.db.QueryRowx(`SELECT COALESCE(SUM(tokens_total),0), COALESCE(SUM(cost_subcents),0) FROM task_usage_events`).Scan(&total, &ledgerCost))
	return events, []int64{in, out, cost, total, ledgerCost}
}

func assertNativeTurnsAggregateGuard(t *testing.T, repo *Repository) {
	t.Helper()
	var turns sql.NullInt64
	var missing int64
	query := `SELECT ` + nativeUsageTurnsAggregateSQL() + `, ` + nativeUsageTurnsMissingAggregateSQL() + ` FROM (
		SELECT * FROM usage_daily_summaries UNION ALL SELECT * FROM usage_lifetime_summaries) summaries`
	require.NoError(t, repo.db.QueryRowx(query).Scan(&turns, &missing))
	require.False(t, turns.Valid, "an old published native response count cannot become a public turn amount")
	require.Positive(t, missing)
	require.NoError(t, repo.db.QueryRowx(repo.db.Rebind(query+` WHERE source=?`), nativeTurnsHostSource).Scan(&turns, &missing))
	require.True(t, turns.Valid)
	require.EqualValues(t, 7, turns.Int64)
	require.Zero(t, missing)
	reasoningQuery := `SELECT ` + nativeUsageReasoningAggregateSQL(true) + `, ` + nativeUsageReasoningMissingAggregateSQL(true) + ` FROM (
		SELECT * FROM usage_daily_summaries UNION ALL SELECT * FROM usage_lifetime_summaries) summaries`
	require.NoError(t, repo.db.QueryRowx(reasoningQuery).Scan(&turns, &missing))
	require.False(t, turns.Valid, "an old summary lacks the private subset; its zero must remain unavailable until publication")
	require.Positive(t, missing)
	require.NoError(t, repo.db.QueryRowx(repo.db.Rebind(reasoningQuery+` WHERE source=?`), nativeTurnsHostSource).Scan(&turns, &missing))
	require.True(t, turns.Valid)
	require.Zero(t, turns.Int64, "known Host zero reasoning must remain supported")
}

func assertPublishedNativeTurnsUnavailable(t *testing.T, repo *Repository, scope string) {
	t.Helper()
	var knownNative, missingNative int
	for _, table := range []string{"usage_projection_contributions", "usage_daily_summaries", "usage_lifetime_summaries"} {
		query := `SELECT COUNT(*) FROM ` + table + ` WHERE scope_id=? AND source='native' AND metric_kind='tokens' AND turns IS NOT NULL
			AND generation=(SELECT active_generation FROM usage_projection_scopes WHERE scope_id=?)`
		require.NoError(t, repo.db.Get(&knownNative, repo.db.Rebind(query), scope, scope))
		require.Zero(t, knownNative, table)
	}
	require.NoError(t, repo.db.Get(&missingNative, repo.db.Rebind(`SELECT COUNT(*) FROM usage_daily_summaries WHERE scope_id=? AND source='native' AND metric_kind='tokens' AND turns_missing=token_count AND token_count>0
		AND generation=(SELECT active_generation FROM usage_projection_scopes WHERE scope_id=?)`), scope, scope))
	require.Positive(t, missingNative)
	facts, _, err := repo.ListUsageCanonicalFacts(t.Context(), models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace, Limit: 200})
	require.NoError(t, err)
	for _, fact := range facts {
		if nativeCanonicalTokenSource(fact) {
			require.Nil(t, fact.Measurement.Turns)
		}
	}
	host, _, err := repo.ListUsageCanonicalFacts(t.Context(), models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace, Source: nativeTurnsHostSource})
	require.NoError(t, err)
	require.Len(t, host, 1)
	require.EqualValues(t, 7, *host[0].Measurement.Turns)
}

func TestUsageHistoryNativeAccountingRepairRollback(t *testing.T) {
	f := newNativeTurnsFixture(t, false, 4)
	finishNativeTurnsBackfill(t, f.repo)
	poisonPublishedNativeTurns(t, f.repo)
	head := nativeTurnsHead(t, f.db)
	_, err := f.db.Exec(`CREATE TRIGGER fail_native_accounting_mutation BEFORE INSERT ON usage_projection_log
		WHEN NEW.commit_sequence > ` + strconv.FormatInt(head+1, 10) + ` BEGIN SELECT RAISE(ABORT, 'native accounting mutation failed'); END`)
	require.NoError(t, err)
	_, err = f.repo.repairNativeUsageAccountingBatch(t.Context(), 200)
	require.ErrorContains(t, err, "native accounting mutation failed")
	require.Equal(t, 4, nativeTurnsPending(t, f.db))
	require.Equal(t, head, nativeTurnsHead(t, f.db))
	var marked int
	require.NoError(t, f.db.Get(&marked, `SELECT COUNT(*) FROM session_usage_buckets WHERE native_accounting_version=1`))
	require.Zero(t, marked, "version, source correction, and mutations must roll back together")
	_, err = f.db.Exec(`DROP TRIGGER fail_native_accounting_mutation`)
	require.NoError(t, err)
	repairAllNativeTurns(t, f.repo, 2)
	require.Zero(t, nativeTurnsPending(t, f.db))
}

func TestUsageHistoryNativeAccountingRepairUsesCapturedErasureScope(t *testing.T) {
	f := newNativeTurnsFixture(t, false, 4)
	poisonPublishedNativeTurns(t, f.repo)
	_, err := f.db.Exec(f.db.Rebind(`UPDATE workspaces SET org_id='later-live-org' WHERE id=?`), nativeTurnsWorkspace)
	require.NoError(t, err)
	require.NoError(t, f.repo.BeginOrganizationUsageErasure(t.Context(), "native-turns-captured-org"))
	head := nativeTurnsHead(t, f.db)
	_, err = f.repo.repairNativeUsageAccountingBatch(t.Context(), 200)
	require.ErrorIs(t, err, errUsageScopeErasing)
	require.Equal(t, head, nativeTurnsHead(t, f.db))
	require.Equal(t, 4, nativeTurnsPending(t, f.db), "a changed live workspace must not bypass captured-org erasure")
}

func TestUsageHistoryNativeAccountingRepairPreservesNativeReplayAndDeletion(t *testing.T) {
	f := newNativeTurnsFixture(t, false, 4)
	finishNativeTurnsBackfill(t, f.repo)
	poisonPublishedNativeTurns(t, f.repo)
	repairAllNativeTurns(t, f.repo, 2)
	turn, err := f.tasks.GetTurn(t.Context(), "native-root-a")
	require.NoError(t, err)
	event := nativeTurnsEvent(0, []*taskmodels.Turn{turn, turn})
	tx, err := f.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, f.repo.RetainNativeUsageTx(t.Context(), tx, nativeTurnsReplay(event)), "canonical reasoning changes must not conflict with the raw native replay digest")
	require.NoError(t, tx.Commit())
	before := nativeTurnsSourceSnapshot(t, f.repo)
	for _, row := range before {
		if row.Measurement.Source == usageSourceNative {
			require.EqualValues(t, 0, *row.Measurement.ReasoningTokens)
			require.EqualValues(t, 5, *row.Measurement.ReasoningOutputTokens)
			require.Equal(t, row.Measurement.PayloadDigest, nativeUsageDigest(row.Measurement))
		}
	}
	require.NoError(t, f.tasks.DeleteTask(t.Context(), nativeTurnsTask), "preservation re-admission must not be stuck on a changed normalization digest")
	var count int
	require.NoError(t, f.db.Get(&count, `SELECT COUNT(*) FROM session_usage_buckets WHERE source='native'`))
	require.Equal(t, 4, count)
	require.NoError(t, f.db.Get(&count, `SELECT COUNT(*) FROM task_usage_events`))
	require.Zero(t, count)
}
