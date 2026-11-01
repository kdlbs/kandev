package plugins

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics"
	analyticsmodels "github.com/kandev/kandev/internal/analytics/models"
	analyticsrepository "github.com/kandev/kandev/internal/analytics/repository"
	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	analyticsservice "github.com/kandev/kandev/internal/analytics/service"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// @covers AC-COSTS-USAGE-HISTORY-004.1 AC-COSTS-USAGE-HISTORY-004.5 AC-COSTS-USAGE-HISTORY-003.7
func TestUsageHistoryHostRetainedWireAdmissionCanonicalAndDeletion(t *testing.T) {
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	tasks, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	analytics, err := analyticssqlite.NewWithDB(database, database)
	require.NoError(t, err)
	tasks.SetUsageHistoryRetainer(analytics)
	ctx := context.Background()
	started := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	require.NoError(t, tasks.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "history-ws", Name: "History"}))
	require.NoError(t, tasks.CreateTask(ctx, &taskmodels.Task{ID: "history-task", WorkspaceID: "history-ws", Title: "History"}))
	require.NoError(t, tasks.CreateTaskSession(ctx, &taskmodels.TaskSession{ID: "history-session", TaskID: "history-task", StartedAt: started, Metadata: map[string]any{"acp": map[string]any{"session_id": "trusted-external-context"}}}))
	svc := analyticsservice.New(analytics)
	fixture := newTestDataHost(manifest.Capabilities{APIRead: []string{"session_usage"}, APIWrite: []string{"session_usage"}})
	fixture.tasks.workspaces = []*taskmodels.Workspace{{ID: "history-ws"}}
	fixture.host.pluginID = "fixture-plugin"
	fixture.host.usageDep = func() sessionUsageSource { return svc }
	host := dialPluginHostOverWire(t, fixture.host)
	history, ok := pluginsdk.HostUsageHistory(host)
	require.True(t, ok)
	finishHostUsageMigration(t, analytics)
	page, err := history.ListTargets(ctx, pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "history-ws"})
	require.NoError(t, err)
	require.Len(t, page.Targets, 1)
	target := page.Targets[0]
	require.NotNil(t, target.SessionStartedAt)
	require.Equal(t, started.Format(time.RFC3339Nano), *target.SessionStartedAt)
	require.Equal(t, "trusted-external-context", *target.ExternalSessionID)
	max := int64(math.MaxInt64)
	currency := "EUR"
	item := pluginsdk.UsageObservationV1{SubjectBinding: target.SubjectBinding, ItemID: "wire-item", SourceRecordID: "provider-row", SourceVersion: "fixture-collector-1", Revision: 1,
		Tokens: &pluginsdk.UsageTokenAmountsV1{InputTokens: &max, TotalTokens: &max}, Cost: &pluginsdk.UsageCostV1{Amount: &max, Currency: &currency, Basis: pluginsdk.UsageCostBasisReported}, Quality: &pluginsdk.UsageObservationQualityV1{TokenCoverageComplete: true, CostCoverageComplete: true}}
	item.PayloadDigest, err = item.ComputePayloadDigestV1()
	require.NoError(t, err)
	report, err := history.Report(ctx, pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{item}})
	require.NoError(t, err)
	require.Equal(t, "plugin:fixture-plugin", report.CollectorNamespace)
	require.Equal(t, "accepted", report.Results[0].Status)
	require.Equal(t, item.PayloadDigest, report.Results[0].CurrentDigest)
	rows, info, err := host.Usage().ListCanonical(ctx, pluginsdk.SessionUsageFilter{WorkspaceID: "history-ws", SessionIDs: []string{"history-session"}}, pluginsdk.Page{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, rows)
	require.Equal(t, "preparing", info.ProjectionStatus)
	require.True(t, info.ProjectionPending)
	require.Empty(t, info.ProjectionGeneration)
	finishHostHistoryProjection(t, analytics)
	rows, info, err = host.Usage().ListCanonical(ctx, pluginsdk.SessionUsageFilter{WorkspaceID: "history-ws", SessionIDs: []string{"history-session"}}, pluginsdk.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, max, *rows[0].TotalTokens)
	require.Equal(t, "EUR", rows[0].Currency)
	require.Equal(t, "ready", info.ProjectionStatus)
	require.False(t, info.ProjectionPending)
	publication := info.ProjectionGeneration
	require.NotEmpty(t, publication)
	require.NoError(t, tasks.DeleteTask(ctx, "history-task"))
	retained, err := history.ListTargets(ctx, pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "history-ws"})
	require.NoError(t, err)
	require.Len(t, retained.Targets, 1)
	require.Equal(t, target.SessionStartedAt, retained.Targets[0].SessionStartedAt)
	require.Equal(t, target.ExternalSessionID, retained.Targets[0].ExternalSessionID)
	amount := int64(70)
	item.Revision = 2
	item.Tokens = &pluginsdk.UsageTokenAmountsV1{InputTokens: &amount, TotalTokens: &amount}
	item.PayloadDigest, err = item.ComputePayloadDigestV1()
	require.NoError(t, err)
	report, err = history.Report(ctx, pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{item}})
	require.NoError(t, err)
	require.Equal(t, "accepted", report.Results[0].Status)
	require.EqualValues(t, 2, report.Results[0].CurrentRevision)
	rows, info, err = host.Usage().ListCanonical(ctx, pluginsdk.SessionUsageFilter{WorkspaceID: "history-ws"}, pluginsdk.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, max, *rows[0].TotalTokens)
	require.Equal(t, publication, info.ProjectionGeneration)
	require.True(t, info.ProjectionPending)
	finishHostHistoryProjection(t, analytics)
	rows, info, err = host.Usage().ListCanonical(ctx, pluginsdk.SessionUsageFilter{WorkspaceID: "history-ws"}, pluginsdk.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 70, *rows[0].TotalTokens)
	require.False(t, info.ProjectionPending)
	require.NotEqual(t, publication, info.ProjectionGeneration)
	_, err = history.ListTargets(ctx, pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "foreign-ws"})
	require.Equal(t, codes.NotFound, status.Code(err))
	fixture.host.capabilities = manifest.Capabilities{}
	_, err = history.ListTargets(ctx, pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "history-ws"})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func finishHostUsageMigration(t *testing.T, repo *analyticssqlite.Repository) {
	t.Helper()
	for range 2000 {
		done, err := repo.BackfillUsageHistory(context.Background(), 200)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("usage history migration did not finish")
}

func finishHostHistoryProjection(t *testing.T, repo *analyticssqlite.Repository) {
	t.Helper()
	migrated := false
	for range 2000 {
		done, err := repo.BackfillUsageHistory(context.Background(), 200)
		require.NoError(t, err)
		if done {
			migrated = true
			break
		}
	}
	require.True(t, migrated, "the public saved view requires all retention watermarks")
	for range 2000 {
		done, err := repo.RunUsageHistoryProjectionBatch(context.Background(), 200)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("retained wire fixture projection did not finish")
}

type pendingUsageProjectionSource struct{}

func (pendingUsageProjectionSource) UpsertPluginSessionUsage(context.Context, string, string, []analyticsmodels.SessionUsageMeasurement) ([]analyticsmodels.SessionUsageUpsertResult, error) {
	return nil, nil
}
func (pendingUsageProjectionSource) ListCanonicalSessionUsageMeasurements(context.Context, analyticsmodels.SessionUsageFilter) ([]analyticsmodels.SessionUsageMeasurement, error) {
	return nil, nil
}
func (pendingUsageProjectionSource) ListCanonicalSessionUsagePage(context.Context, analyticsmodels.SessionUsageFilter) ([]analyticsmodels.SessionUsageMeasurement, analyticsmodels.UsageProjectionState, error) {
	return nil, analyticsmodels.UsageProjectionState{Status: "preparing", Generation: "fixture-generation-7", Pending: true}, nil
}

func TestUsageHistoryHostEmptyCanonicalPagePreservesProjectionState(t *testing.T) {
	fixture := newTestDataHost(manifest.Capabilities{APIRead: []string{"session_usage"}})
	fixture.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	fixture.host.usageDep = func() sessionUsageSource { return pendingUsageProjectionSource{} }
	host := dialPluginHostOverWire(t, fixture.host)
	rows, info, err := host.Usage().ListCanonical(context.Background(), pluginsdk.SessionUsageFilter{WorkspaceID: "ws-1"}, pluginsdk.Page{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, rows)
	require.True(t, info.ProjectionPending)
	require.Equal(t, "preparing", info.ProjectionStatus)
	require.Equal(t, "fixture-generation-7", info.ProjectionGeneration)
	history, ok := pluginsdk.HostUsageHistory(host)
	require.True(t, ok)
	_, err = history.ListTargets(context.Background(), pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.Equal(t, codes.Unimplemented, status.Code(err))
}

type usageErrorSource struct {
	err error
}

func (s usageErrorSource) ListCanonicalSessionUsageMeasurements(context.Context, analyticsmodels.SessionUsageFilter) ([]analyticsmodels.SessionUsageMeasurement, error) {
	return nil, s.err
}

func (s usageErrorSource) ListUsageCollectionTargets(context.Context, string, pluginsdk.UsageCollectionTargetQueryV1) (pluginsdk.UsageCollectionTargetPageV1, error) {
	return pluginsdk.UsageCollectionTargetPageV1{}, s.err
}

func (s usageErrorSource) ReportUsageObservations(context.Context, string, pluginsdk.UsageObservationBatchV1) (pluginsdk.UsageObservationBatchResultV1, error) {
	return pluginsdk.UsageObservationBatchResultV1{}, s.err
}

// @covers AC-COSTS-USAGE-HISTORY-002.5
func TestUsageHostErrorsArePublicOnly(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		code    codes.Code
		message string
	}{
		{name: "task not found", err: analyticsrepository.ErrUsageNotVisible, code: codes.NotFound, message: "usage target not found"},
		{name: "storage failure", err: errors.New("disk secret /tmp/internal"), code: codes.Unavailable, message: "retained usage is unavailable"},
		{name: "projection changed", err: analytics.ErrUsageProjectionChanged, code: codes.FailedPrecondition, message: "usage publication changed; restart the saved query"},
		{name: "deadline", err: context.DeadlineExceeded, code: codes.DeadlineExceeded, message: "usage request timed out"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newTestDataHost(manifest.Capabilities{APIRead: []string{"session_usage"}, APIWrite: []string{"session_usage"}})
			fixture.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-a"}}
			fixture.host.pluginID = "fixture-plugin"
			fixture.host.usageDep = func() sessionUsageSource { return usageErrorSource{err: tc.err} }
			host := dialPluginHostOverWire(t, fixture.host)
			history, ok := pluginsdk.HostUsageHistory(host)
			require.True(t, ok)
			ctx := context.Background()
			calls := map[string]error{
				"canonical": func() error {
					_, _, err := host.Usage().ListCanonical(ctx, pluginsdk.SessionUsageFilter{WorkspaceID: "ws-a"}, pluginsdk.Page{})
					return err
				}(),
				"targets": func() error {
					_, err := history.ListTargets(ctx, pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-a"})
					return err
				}(),
				"report": func() error {
					_, err := history.Report(ctx, pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{{ItemID: "item"}}})
					return err
				}(),
			}
			for path, err := range calls {
				require.Error(t, err, path)
				require.Equal(t, tc.code, status.Code(err), path)
				require.Equal(t, tc.message, status.Convert(err).Message(), path)
				require.NotContains(t, status.Convert(err).Message(), "disk secret", path)
			}
			// A foreign workspace is rejected before the source can answer.
			_, err := history.ListTargets(ctx, pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-foreign"})
			require.Equal(t, codes.NotFound, status.Code(err))
			require.Equal(t, "usage target not found", status.Convert(err).Message())
		})
	}
}
