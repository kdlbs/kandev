package sqlite_test

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryConcurrentIndependentWriters(t *testing.T) {
	runHistoryConcurrentContract(t, false)
}
func TestUsageHistoryPostgresConcurrentIndependentWriters(t *testing.T) {
	runHistoryConcurrentContract(t, true)
}

func runHistoryConcurrentContract(t *testing.T, postgres bool) {
	var database *sqlx.DB
	if postgres {
		database = testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	} else {
		raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "contract.db"))
		require.NoError(t, err)
		database = sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { _ = database.Close() })
	}
	database.SetMaxOpenConns(4)
	database.SetMaxIdleConns(4)
	tasks, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	first, err := analyticssqlite.NewWithDB(database, database)
	require.NoError(t, err)
	second, err := analyticssqlite.NewWithDB(database, database)
	require.NoError(t, err)
	tasks.SetUsageHistoryRetainer(first)
	ctx := context.Background()
	started := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	require.NoError(t, tasks.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "concurrent-ws", Name: "Concurrent", OrgID: "retained-org"}))
	require.NoError(t, tasks.CreateTask(ctx, &taskmodels.Task{ID: "concurrent-task", WorkspaceID: "concurrent-ws", Title: "Concurrent"}))
	require.NoError(t, tasks.CreateTaskSession(ctx, &taskmodels.TaskSession{ID: "concurrent-session", TaskID: "concurrent-task", StartedAt: started, Metadata: map[string]any{"acp": map[string]any{"session_id": "external-thread"}}}))
	completeExternalUsageMigration(t, first)
	page, err := first.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "concurrent-ws"})
	require.NoError(t, err)
	require.Len(t, page.Targets, 1)
	value := int64(math.MaxInt64)
	model := "model"
	item := pluginsdk.UsageObservationV1{SubjectBinding: page.Targets[0].SubjectBinding, ItemID: "concurrent-item", SourceRecordID: "provider-record", SourceVersion: "fixture-v1", Revision: 1, ModelID: &model, Tokens: &pluginsdk.UsageTokenAmountsV1{InputTokens: &value, TotalTokens: &value}, Quality: &pluginsdk.UsageObservationQualityV1{TokenCoverageComplete: true}}
	seal := func(input pluginsdk.UsageObservationV1) pluginsdk.UsageObservationV1 {
		digest, err := input.ComputePayloadDigestV1()
		require.NoError(t, err)
		input.PayloadDigest = digest
		return input
	}
	item = seal(item)
	type outcome struct {
		result pluginsdk.UsageObservationBatchResultV1
		err    error
	}
	race := func(a, b pluginsdk.UsageObservationV1) []pluginsdk.UsageObservationWriteResultV1 {
		start := make(chan struct{})
		done := make(chan outcome, 2)
		for i, input := range []pluginsdk.UsageObservationV1{a, b} {
			repo := first
			if i == 1 {
				repo = second
			}
			go func(repo *analyticssqlite.Repository, input pluginsdk.UsageObservationV1) {
				<-start
				result, err := repo.ReportUsageObservations(ctx, "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{input}})
				done <- outcome{result, err}
			}(repo, input)
		}
		close(start)
		receipts := make([]pluginsdk.UsageObservationWriteResultV1, 0, 2)
		for range 2 {
			out := <-done
			require.NoError(t, out.err)
			require.Len(t, out.result.Results, 1)
			receipts = append(receipts, out.result.Results[0])
		}
		return receipts
	}
	receipts := race(item, item)
	statuses := map[string]int{}
	for _, receipt := range receipts {
		statuses[receipt.Status]++
		require.EqualValues(t, 1, receipt.CurrentRevision)
		require.Equal(t, item.PayloadDigest, receipt.CurrentDigest)
		require.Equal(t, value, *receipt.CurrentObservation.Tokens.TotalTokens)
	}
	require.Equal(t, 1, statuses["accepted"])
	require.Equal(t, 1, statuses["unchanged"])
	a, b := item, item
	a.Revision, b.Revision = 2, 3
	a.Tokens = &pluginsdk.UsageTokenAmountsV1{InputTokens: models.Int64(80), TotalTokens: models.Int64(80)}
	b.Tokens = &pluginsdk.UsageTokenAmountsV1{InputTokens: models.Int64(60), TotalTokens: models.Int64(60)}
	a, b = seal(a), seal(b)
	for _, receipt := range race(a, b) {
		require.Contains(t, []string{"accepted", "stale"}, receipt.Status)
		require.GreaterOrEqual(t, receipt.CurrentRevision, int64(2))
		require.NotNil(t, receipt.CurrentObservation)
	}
	current, err := first.ReportUsageObservations(ctx, "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{b}})
	require.NoError(t, err)
	require.Equal(t, "unchanged", current.Results[0].Status)
	require.EqualValues(t, 3, current.Results[0].CurrentRevision)
	a = b
	a.Revision, b.Revision = 4, 4
	a.Tokens = &pluginsdk.UsageTokenAmountsV1{InputTokens: models.Int64(40), TotalTokens: models.Int64(40)}
	b.Tokens = &pluginsdk.UsageTokenAmountsV1{InputTokens: models.Int64(30), TotalTokens: models.Int64(30)}
	a, b = seal(a), seal(b)
	receipts = race(a, b)
	statuses = map[string]int{}
	var winner string
	for _, receipt := range receipts {
		statuses[receipt.Status]++
		require.EqualValues(t, 4, receipt.CurrentRevision)
		if receipt.Status == "accepted" {
			winner = receipt.CurrentDigest
		}
	}
	require.Equal(t, 1, statuses["accepted"])
	require.Equal(t, 1, statuses["conflict"])
	for _, receipt := range receipts {
		require.Equal(t, winner, receipt.CurrentDigest)
	}
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	correction := a
	correction.Revision = 5
	correction = seal(correction)
	result, changes, err := first.MutateUsageObservationsTx(ctx, tx, "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{correction}})
	require.NoError(t, err)
	require.Equal(t, "accepted", result.Results[0].Status)
	require.Len(t, changes, 1)
	require.NotEmpty(t, changes[0].BeforePayload)
	require.NotEmpty(t, changes[0].AfterPayload)
	require.NoError(t, tx.Rollback())
	var revision int64
	require.NoError(t, database.Get(&revision, database.Rebind(`SELECT revision FROM usage_observation_items WHERE source = ? AND item_id = ?`), "plugin:collector-a", item.ItemID))
	require.EqualValues(t, 4, revision)
	require.NoError(t, tasks.DeleteTask(ctx, "concurrent-task"))
	replacement, err := second.ListUsageCollectionTargets(ctx, "replacement-collector", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "concurrent-ws"})
	require.NoError(t, err)
	require.Len(t, replacement.Targets, 1)
	require.Equal(t, page.Targets[0].SessionStartedAt, replacement.Targets[0].SessionStartedAt)
	require.Equal(t, page.Targets[0].ExternalSessionID, replacement.Targets[0].ExternalSessionID)
	correction.Revision = 5
	result, err = second.ReportUsageObservations(ctx, "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{correction}})
	require.NoError(t, err)
	require.Equal(t, "accepted", result.Results[0].Status)
	migrated := false
	for range 100 {
		done, backfillErr := second.BackfillUsageHistory(ctx, 200)
		require.NoError(t, backfillErr)
		if done {
			migrated = true
			break
		}
	}
	require.True(t, migrated, "all retention watermarks must finish before saved canonical reads")
	finishProjectionBatches(t, second)
	rows, err := second.ListCanonicalSessionUsageMeasurements(ctx, models.SessionUsageFilter{WorkspaceID: "concurrent-ws"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 40, *rows[0].TotalTokens)
}
