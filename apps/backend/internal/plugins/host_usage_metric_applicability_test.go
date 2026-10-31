package plugins

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	analyticsmodels "github.com/kandev/kandev/internal/analytics/models"
	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	analyticsservice "github.com/kandev/kandev/internal/analytics/service"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func metricApplicabilityHost(t *testing.T) (*analyticssqlite.Repository, pluginsdk.Host, *sqlx.DB) {
	t.Helper()
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "metrics.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	tasks, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	repo, err := analyticssqlite.NewWithDB(database, database)
	require.NoError(t, err)
	tasks.SetUsageHistoryRetainer(repo)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, tasks.CreateWorkspace(t.Context(), &taskmodels.Workspace{ID: "metric-ws", Name: "Metrics"}))
	require.NoError(t, tasks.CreateTask(t.Context(), &taskmodels.Task{ID: "metric-task", WorkspaceID: "metric-ws", Title: "Metrics", CreatedAt: at, UpdatedAt: at}))
	require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{ID: "metric-session", TaskID: "metric-task", StartedAt: at, Metadata: map[string]any{"acp": map[string]any{"session_id": "metric-thread"}}}))
	fixture := newTestDataHost(manifest.Capabilities{APIRead: []string{"session_usage"}, APIWrite: []string{"session_usage"}})
	fixture.tasks.workspaces = []*taskmodels.Workspace{{ID: "metric-ws"}}
	fixture.host.pluginID = "fixture-plugin"
	svc := analyticsservice.New(repo)
	fixture.host.usageDep = func() sessionUsageSource { return svc }
	return repo, dialPluginHostOverWire(t, fixture.host), database
}

func metricApplicabilityNative(t *testing.T, repo *analyticssqlite.Repository, database *sqlx.DB) {
	t.Helper()
	at := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	var taskIncarnation, sessionIncarnation string
	require.NoError(t, database.QueryRowx(`SELECT task_incarnation_id,session_incarnation_id FROM usage_subjects WHERE session_id='metric-session'`).Scan(&taskIncarnation, &sessionIncarnation))
	tx, err := database.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	for _, actor := range []string{"metric-actor-a", "metric-actor-b"} {
		require.NoError(t, repo.AdmitBindingTx(t.Context(), tx, usagehistory.Binding{ID: actor, Kind: usagehistory.BindingKindExecution, WorkspaceID: "metric-ws", TaskID: "metric-task", TaskIncarnationID: taskIncarnation, SessionID: "metric-session", SessionIncarnationID: sessionIncarnation, ActorID: actor, ActorStatus: "attributed", ActorKind: "user", ActorSource: "execution", CapturedAt: at}))
	}
	require.NoError(t, repo.RetainNativeUsageTx(t.Context(), tx, usagehistory.NativeUsage{BindingID: "metric-actor-a", UsageEventID: "metric-event", TaskID: "metric-task", SessionID: "metric-session", ProviderThreadID: "metric-thread", Model: "Sonnet", Provider: "anthropic", InputTokens: 100, OutputTokens: analyticsmodels.Int64(0), TotalTokens: 100, ReportedTotalTokens: analyticsmodels.Int64(100), UsageCompleteness: "complete", CostPresent: true, CostSubcents: 10, CostSource: "models_dev_list", CostBasis: "estimated", Estimated: true, OccurredAt: at, CollectedAt: at}))
	require.NoError(t, tx.Commit())
}

func TestUsageHistoryHostCanonicalMetricApplicabilityMatchesReport(t *testing.T) {
	for _, segmented := range []bool{false, true} {
		t.Run(fmt.Sprintf("segmented=%t", segmented), func(t *testing.T) { checkCanonicalMetricApplicability(t, segmented) })
	}
}

func checkCanonicalMetricApplicability(t *testing.T, segmented bool) {
	t.Helper()
	repo, host, database := metricApplicabilityHost(t)
	metricApplicabilityNative(t, repo, database)
	history, ok := pluginsdk.HostUsageHistory(host)
	require.True(t, ok)
	page, err := history.ListTargets(t.Context(), pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "metric-ws", CurrentTarget: &pluginsdk.UsageCollectionCurrentTargetQueryV1{SessionID: "metric-session"}})
	require.NoError(t, err)
	require.Len(t, page.Targets, 1)
	model, provider, currency := "Sonnet", "anthropic", "USD"
	start, end, date, zone := "2026-01-02T00:00:00Z", "2026-01-03T00:00:00Z", "2026-01-02", "UTC"
	item := pluginsdk.UsageObservationV1{SubjectBinding: page.Targets[0].SubjectBinding, ItemID: "metric-day-cost", SourceRecordID: "metric-day-cost", SourceVersion: "cost-only-v1", Revision: 1,
		Segment: &pluginsdk.UsageSegmentIdentityV1{DecompositionID: "session-model-costs-v1", SegmentID: "stable-member"}, ModelID: &model, ModelProvider: &provider,
		Cost: &pluginsdk.UsageCostV1{Amount: analyticsmodels.Int64(20), Currency: &currency, Basis: pluginsdk.UsageCostBasisReported}, Quality: &pluginsdk.UsageObservationQualityV1{CostCoverageComplete: true},
		Coverage: &pluginsdk.UsageCoverageV1{CoverageStart: &start, CoverageEnd: &end, SourceDate: &date, SourceTimezone: &zone}}
	item.PayloadDigest, err = item.ComputePayloadDigestV1()
	require.NoError(t, err)
	version := pluginsdk.UsageObservationSchemaVersionV1
	if segmented {
		version = pluginsdk.UsageObservationSchemaVersionV2
	} else {
		item.Segment = nil
		item.PayloadDigest, err = item.ComputePayloadDigestV1()
		require.NoError(t, err)
	}
	result, err := history.Report(t.Context(), pluginsdk.UsageObservationBatchV1{SchemaVersion: version, Observations: []pluginsdk.UsageObservationV1{item}})
	require.NoError(t, err)
	require.Equal(t, "accepted", result.Status)
	finishHostHistoryProjection(t, repo)
	report, err := repo.ListSessionUsage(t.Context(), analyticsmodels.SessionUsageFilter{WorkspaceID: "metric-ws", GroupBy: "model"})
	require.NoError(t, err)
	require.Equal(t, analyticsmodels.Int64(100), report.Totals.TotalTokens)
	require.Equal(t, analyticsmodels.Int64(20), report.Totals.CostSubcents)
	require.False(t, report.Totals.Estimated)
	require.Nil(t, report.Totals.EffectiveCostPerMillion, "mixed aggregate cost cannot be allocated to native actors")
	var tokens, cost int64
	var publication, cursor string
	var tokenOnly, costOnly int
	for range 2 {
		rows, state, err := host.Usage().ListCanonical(t.Context(), pluginsdk.SessionUsageFilter{WorkspaceID: "metric-ws"}, pluginsdk.Page{Limit: 1, Cursor: cursor})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "ready", state.ProjectionStatus)
		require.False(t, state.ProjectionPending)
		if publication != "" {
			require.Equal(t, publication, state.ProjectionGeneration)
		}
		publication, cursor = state.ProjectionGeneration, state.NextCursor
		row := rows[0]
		if !row.TokenMetricAbsent {
			require.NotNil(t, row.TotalTokens)
			tokens += *row.TotalTokens
		}
		if !row.CostMetricAbsent {
			require.NotNil(t, row.CostSubcents)
			cost += *row.CostSubcents
		}
		if row.CostMetricAbsent {
			tokenOnly++
		}
		if row.TokenMetricAbsent {
			costOnly++
		}
	}
	require.Empty(t, cursor)
	require.NotEmpty(t, publication)
	require.Equal(t, int64(100), tokens)
	require.Equal(t, int64(20), cost)
	require.Equal(t, 1, tokenOnly)
	require.Equal(t, 1, costOnly)
}

func TestUsageHistoryHostUnknownTokenObjectRemainsApplicable(t *testing.T) {
	repo, host, _ := metricApplicabilityHost(t)
	history, ok := pluginsdk.HostUsageHistory(host)
	require.True(t, ok)
	page, err := history.ListTargets(t.Context(), pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "metric-ws", CurrentTarget: &pluginsdk.UsageCollectionCurrentTargetQueryV1{SessionID: "metric-session"}})
	require.NoError(t, err)
	require.Len(t, page.Targets, 1)
	currency := "USD"
	item := pluginsdk.UsageObservationV1{SubjectBinding: page.Targets[0].SubjectBinding, ItemID: "unknown-token-object", SourceRecordID: "unknown-token-object", SourceVersion: "generic-v1", Revision: 1,
		Tokens: &pluginsdk.UsageTokenAmountsV1{}, Cost: &pluginsdk.UsageCostV1{Amount: analyticsmodels.Int64(20), Currency: &currency, Basis: pluginsdk.UsageCostBasisReported}, Quality: &pluginsdk.UsageObservationQualityV1{CostCoverageComplete: true}}
	item.PayloadDigest, err = item.ComputePayloadDigestV1()
	require.NoError(t, err)
	result, err := history.Report(t.Context(), pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{item}})
	require.NoError(t, err)
	require.Equal(t, "accepted", result.Status)
	finishHostHistoryProjection(t, repo)
	rows, _, err := host.Usage().ListCanonical(t.Context(), pluginsdk.SessionUsageFilter{WorkspaceID: "metric-ws"}, pluginsdk.Page{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.False(t, rows[0].TokenMetricAbsent)
	require.False(t, rows[0].CostMetricAbsent)
	require.Nil(t, rows[0].TotalTokens)
	require.Equal(t, analyticsmodels.Int64(20), rows[0].CostSubcents)
}

func TestUsageHistoryHostUnknownLifetimeTokensPreserveKnownPoint(t *testing.T) {
	for _, zero := range []bool{false, true} {
		t.Run(fmt.Sprintf("zero=%t", zero), func(t *testing.T) {
			repo, host, database := metricApplicabilityHost(t)
			metricApplicabilityNative(t, repo, database)
			history, ok := pluginsdk.HostUsageHistory(host)
			require.True(t, ok)
			page, err := history.ListTargets(t.Context(), pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "metric-ws", CurrentTarget: &pluginsdk.UsageCollectionCurrentTargetQueryV1{SessionID: "metric-session"}})
			require.NoError(t, err)
			require.Len(t, page.Targets, 1)
			model, provider, currency := "Sonnet", "anthropic", "USD"
			item := pluginsdk.UsageObservationV1{SubjectBinding: page.Targets[0].SubjectBinding, ItemID: "lifetime-tokens", SourceRecordID: "lifetime-tokens", SourceVersion: "generic-v1", Revision: 1, ModelID: &model, ModelProvider: &provider,
				Tokens: &pluginsdk.UsageTokenAmountsV1{}, Cost: &pluginsdk.UsageCostV1{Amount: analyticsmodels.Int64(20), Currency: &currency, Basis: pluginsdk.UsageCostBasisReported}, Quality: &pluginsdk.UsageObservationQualityV1{CostCoverageComplete: true}}
			want := int64(100)
			if zero {
				item.Tokens.InputTokens, item.Tokens.TotalTokens, item.Quality.TokenCoverageComplete, want = analyticsmodels.Int64(0), analyticsmodels.Int64(0), true, 0
			}
			item.PayloadDigest, err = item.ComputePayloadDigestV1()
			require.NoError(t, err)
			result, err := history.Report(t.Context(), pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{item}})
			require.NoError(t, err)
			require.Equal(t, "accepted", result.Status)
			finishHostHistoryProjection(t, repo)
			report, err := repo.ListSessionUsage(t.Context(), analyticsmodels.SessionUsageFilter{WorkspaceID: "metric-ws", GroupBy: "model"})
			require.NoError(t, err)
			require.Equal(t, analyticsmodels.Int64(want), report.Totals.TotalTokens)
			require.Equal(t, analyticsmodels.Int64(20), report.Totals.CostSubcents)
			require.Nil(t, report.Totals.EffectiveCostPerMillion)
			if !zero {
				require.True(t, report.Totals.Partial)
			}
			rows, _, err := host.Usage().ListCanonical(t.Context(), pluginsdk.SessionUsageFilter{WorkspaceID: "metric-ws"}, pluginsdk.Page{})
			require.NoError(t, err)
			var total int64
			for _, row := range rows {
				if !row.TokenMetricAbsent {
					require.NotNil(t, row.TotalTokens)
					total += *row.TotalTokens
				}
			}
			require.Equal(t, want, total)
		})
	}
}

func TestUsageHistoryHostIgnoresInputMetricApplicability(t *testing.T) {
	_, host, _ := metricApplicabilityHost(t)
	for _, zero := range []bool{false, true} {
		item := pluginsdk.SessionUsageMeasurement{TaskID: "metric-task", SessionID: "metric-session", TranscriptID: "metric-thread", SourceRecordID: "unknown", UsageIdentity: "unknown", Model: "unknown", SourceVersion: "old-dto", Revision: 1, PayloadDigest: "unknown", TokenMetricAbsent: true, CostMetricAbsent: true}
		if zero {
			item.SourceRecordID, item.UsageIdentity, item.Model, item.PayloadDigest = "zero", "zero", "zero", "zero"
			item.InputTokens, item.TotalTokens, item.CostSubcents = analyticsmodels.Int64(0), analyticsmodels.Int64(0), analyticsmodels.Int64(0)
			item.Currency, item.CostBasis, item.CostCoverage, item.Coverage = "USD", "reported", "complete", "complete"
		}
		results, err := host.Usage().UpsertBatch(t.Context(), "metric-ws", []pluginsdk.SessionUsageMeasurement{item})
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Equal(t, "applied", results[0].Status)
		rows, _, err := host.Usage().List(t.Context(), pluginsdk.SessionUsageFilter{WorkspaceID: "metric-ws", Model: item.Model}, pluginsdk.Page{})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.False(t, rows[0].TokenMetricAbsent)
		require.False(t, rows[0].CostMetricAbsent)
		if zero {
			require.NotNil(t, rows[0].TotalTokens)
			require.NotNil(t, rows[0].CostSubcents)
			require.Zero(t, *rows[0].TotalTokens)
			require.Zero(t, *rows[0].CostSubcents)
		} else {
			require.Nil(t, rows[0].TotalTokens)
			require.Nil(t, rows[0].CostSubcents)
		}
	}
}
