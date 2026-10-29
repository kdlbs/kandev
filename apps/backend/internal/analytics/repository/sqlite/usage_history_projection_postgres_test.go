package sqlite_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/analytics/repository"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryPostgresProjectionPointCalendars(t *testing.T) {
	f := newProjectionDatabaseFixture(t, true)
	ctx := context.Background()
	point := time.Date(2026, 1, 2, 12, 0, 0, 123456000, time.UTC)
	item := sealProjectionItem(t, f.item, 2, 100)
	value := point.Format(time.RFC3339Nano)
	item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: &value}
	digest, err := item.ComputePayloadDigestV1()
	require.NoError(t, err)
	item.PayloadDigest = digest
	projectionReport(t, f.first, item)
	tx, err := f.database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, f.first.RetainNativeUsageTx(ctx, tx, usagehistory.NativeUsage{UsageEventID: "native-point", TaskID: "projection-task", SessionID: "projection-session", Model: "model", InputTokens: 20, TotalTokens: 20, UsageCompleteness: "complete", OccurredAt: point.Add(time.Hour), CollectedAt: point.Add(time.Hour)}))
	require.NoError(t, tx.Commit())
	generation, err := f.first.StartUsageHistoryRebuild(ctx, f.scope)
	require.NoError(t, err)
	finishExternalProjection(t, f.first, f.scope, generation)
	var points, total int64
	require.NoError(t, f.database.Get(&points, `SELECT COUNT(*) FROM usage_canonical_facts WHERE coverage_kind = 'point'`))
	require.EqualValues(t, 2, points, "PostgreSQL timestamp precision must not turn point facts into coarse intervals or days")
	require.NoError(t, f.database.Get(&total, `SELECT SUM(total_tokens) FROM usage_daily_summaries WHERE metric_kind = 'tokens' AND calendar = 'UTC'`))
	require.EqualValues(t, 120, total)
	end := point.Add(2 * time.Hour)
	rows, _, err := f.first.ListCanonicalUsagePage(ctx, models.SessionUsageFilter{WorkspaceID: "projection-ws", Start: &point, End: &end})
	require.NoError(t, err)
	require.Len(t, rows, 2, "occurrence bounds must include a point exactly at the inclusive start")
}

func TestUsageHistoryPostgresGenerationCapturedScopeErasure(t *testing.T) {
	f := newProjectionDatabaseFixture(t, true)
	ctx := context.Background()
	_, err := f.database.Exec(`UPDATE workspaces SET org_id = 'new-org' WHERE id = 'projection-ws'`)
	require.NoError(t, err)
	generation, err := f.first.StartUsageHistoryRebuild(ctx, f.scope)
	require.NoError(t, err)
	finishExternalProjection(t, f.first, f.scope, generation)
	rows, _, err := f.first.ListCanonicalUsagePage(ctx, models.SessionUsageFilter{WorkspaceID: "projection-ws"})
	require.NoError(t, err)
	require.Empty(t, rows, "the new live organization cannot read captured historical ownership")
	tx, err := f.database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	_, err = f.first.UsageSourceUpperBoundsTx(ctx, tx, f.scope)
	require.ErrorIs(t, err, repository.ErrUsageNotVisible)
	require.NoError(t, tx.Rollback())
	shadow, err := f.second.StartUsageHistoryRebuild(ctx, f.scope)
	require.NoError(t, err)
	_, err = f.second.AdvanceUsageHistoryProjection(ctx, f.scope, shadow, 1)
	require.NoError(t, err)
	require.NoError(t, f.first.BeginOrganizationUsageErasure(ctx, "captured-org"))
	_, err = f.second.AdvanceUsageHistoryProjection(ctx, f.scope, shadow, 17)
	require.Error(t, err, "erasure must fence the shadow before source or checkpoint writes")
	require.Contains(t, err.Error(), "erasing")
	require.NoError(t, f.first.EraseOrganizationUsage(ctx, "captured-org"))
	for _, table := range []string{"usage_projection_scopes", "usage_projection_generations", "usage_projection_checkpoints", "usage_projection_log", "usage_projection_items", "usage_projection_work", "usage_projection_applied", "usage_projection_resolutions", "usage_projection_replay_groups", "usage_projection_builds", "usage_projection_history", "usage_projection_source_versions", "usage_projection_partitions", "usage_projection_summary_queue", "usage_canonical_facts", "usage_projection_contributions", "usage_daily_summaries", "usage_lifetime_summaries"} {
		var count int
		require.NoError(t, f.database.Get(&count, f.database.Rebind(`SELECT COUNT(*) FROM `+table+` WHERE scope_id = ?`), f.scope))
		require.Zero(t, count, table)
	}
	_, err = f.second.AdvanceUsageHistoryProjection(ctx, f.scope, shadow, 17)
	require.ErrorIs(t, err, repository.ErrUsageNotVisible)
}

func TestUsageHistoryPostgresGenerationCutoverCrashRecovery(t *testing.T) {
	f := newProjectionDatabaseFixture(t, true)
	ctx := context.Background()
	active, err := f.first.StartUsageHistoryRebuild(ctx, f.scope)
	require.NoError(t, err)
	finishExternalProjection(t, f.first, f.scope, active)
	shadow, err := f.second.StartUsageHistoryRebuild(ctx, f.scope)
	require.NoError(t, err)
	projectionReport(t, f.first, sealProjectionItem(t, f.item, 2, 40))
	_, err = f.database.Exec(`CREATE FUNCTION projection_cutover_crash() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'projection crash'; END $$`)
	require.NoError(t, err)
	_, err = f.database.Exec(`CREATE TRIGGER projection_cutover_crash BEFORE UPDATE OF active_generation ON usage_projection_scopes
		FOR EACH ROW WHEN (OLD.active_generation IS DISTINCT FROM NEW.active_generation) EXECUTE FUNCTION projection_cutover_crash()`)
	require.NoError(t, err)
	failed := false
	for range 40 {
		_, err = f.second.AdvanceUsageHistoryProjection(ctx, f.scope, shadow, 17)
		if err != nil {
			require.Contains(t, err.Error(), "projection crash")
			failed = true
			break
		}
	}
	require.True(t, failed)
	rows, state, err := f.first.ListCanonicalUsagePage(ctx, models.SessionUsageFilter{WorkspaceID: "projection-ws"})
	require.NoError(t, err)
	require.EqualValues(t, 100, *rows[0].TotalTokens)
	require.True(t, strings.HasPrefix(state.Generation, active+"."))
	require.True(t, state.Pending)
	_, err = f.database.Exec(`DROP TRIGGER projection_cutover_crash ON usage_projection_scopes`)
	require.NoError(t, err)
	_, err = f.database.Exec(`DROP FUNCTION projection_cutover_crash()`)
	require.NoError(t, err)
	finishExternalProjection(t, f.first, f.scope, shadow)
	rows, state, err = f.second.ListCanonicalUsagePage(ctx, models.SessionUsageFilter{WorkspaceID: "projection-ws"})
	require.NoError(t, err)
	require.EqualValues(t, 40, *rows[0].TotalTokens)
	require.True(t, strings.HasPrefix(state.Generation, shadow+"."))
}
