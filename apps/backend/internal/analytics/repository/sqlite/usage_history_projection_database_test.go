package sqlite_test

import (
	"context"
	"path/filepath"
	"strings"
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

type projectionDatabaseFixture struct {
	database *sqlx.DB
	first    *analyticssqlite.Repository
	second   *analyticssqlite.Repository
	scope    string
	item     pluginsdk.UsageObservationV1
}

func projectionDatabases(t *testing.T, postgres bool) (*sqlx.DB, *sqlx.DB) {
	t.Helper()
	var first, second *sqlx.DB
	if postgres {
		dsn := testutil.PostgresDSNFromEnv(t)
		first = testutil.OpenIsolatedPostgres(t, dsn)
		var version int
		require.NoError(t, first.Get(&version, `SHOW server_version_num`))
		require.GreaterOrEqual(t, version, 160000)
		var schema string
		require.NoError(t, first.Get(&schema, `SELECT current_schema()`))
		raw, err := db.OpenPostgres(testutil.PostgresDSNWithSearchPath(t, dsn, schema), 4, 4)
		if err != nil {
			t.Fatal("open independent PostgreSQL projection pool")
		}
		second = sqlx.NewDb(raw, "pgx")
	} else {
		path := filepath.Join(t.TempDir(), "projection.db")
		for _, target := range []**sqlx.DB{&first, &second} {
			raw, err := db.OpenSQLite(path)
			require.NoError(t, err)
			*target = sqlx.NewDb(raw, "sqlite3")
			pool := *target
			t.Cleanup(func() { _ = pool.Close() })
		}
	}
	if postgres {
		t.Cleanup(func() { _ = second.Close() })
	}
	first.SetMaxOpenConns(4)
	return first, second
}

func newProjectionDatabaseFixture(t *testing.T, postgres bool) projectionDatabaseFixture {
	t.Helper()
	database, other := projectionDatabases(t, postgres)
	tasks, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	first, err := analyticssqlite.NewWithDB(database, database)
	require.NoError(t, err)
	second, err := analyticssqlite.NewWithDB(other, other)
	require.NoError(t, err)
	tasks.SetUsageHistoryRetainer(first)
	ctx := context.Background()
	require.NoError(t, tasks.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "projection-ws", Name: "Projection", OrgID: "captured-org"}))
	require.NoError(t, tasks.CreateTask(ctx, &taskmodels.Task{ID: "projection-task", WorkspaceID: "projection-ws", Title: "Projection"}))
	require.NoError(t, tasks.CreateTaskSession(ctx, &taskmodels.TaskSession{ID: "projection-session", TaskID: "projection-task", StartedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}))
	completeExternalUsageMigration(t, first)
	targets, err := first.ListUsageCollectionTargets(ctx, "collector", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "projection-ws"})
	require.NoError(t, err)
	model := "model"
	item := pluginsdk.UsageObservationV1{SubjectBinding: targets.Targets[0].SubjectBinding, ItemID: "item", SourceRecordID: "item", SourceVersion: "fixture", Revision: 1, ModelID: &model, Tokens: &pluginsdk.UsageTokenAmountsV1{InputTokens: models.Int64(100), TotalTokens: models.Int64(100)}, Quality: &pluginsdk.UsageObservationQualityV1{TokenCoverageComplete: true}}
	item = sealProjectionItem(t, item, 1, 100)
	projectionReport(t, first, item)
	for {
		done, err := first.BackfillUsageHistory(ctx, 200)
		require.NoError(t, err)
		if done {
			break
		}
	}
	var scope string
	require.NoError(t, database.Get(&scope, `SELECT scope_id FROM usage_scopes LIMIT 1`))
	return projectionDatabaseFixture{database: database, first: first, second: second, scope: scope, item: item}
}

func sealProjectionItem(t *testing.T, item pluginsdk.UsageObservationV1, revision, tokens int64) pluginsdk.UsageObservationV1 {
	t.Helper()
	item.Revision = revision
	item.Tokens = &pluginsdk.UsageTokenAmountsV1{InputTokens: models.Int64(tokens), TotalTokens: models.Int64(tokens)}
	digest, err := item.ComputePayloadDigestV1()
	require.NoError(t, err)
	item.PayloadDigest = digest
	return item
}

func projectionReport(t *testing.T, repo *analyticssqlite.Repository, item pluginsdk.UsageObservationV1) {
	t.Helper()
	result, err := repo.ReportUsageObservations(context.Background(), "collector", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{item}})
	require.NoError(t, err)
	require.Equal(t, "accepted", result.Results[0].Status)
}

func finishExternalProjection(t *testing.T, repo *analyticssqlite.Repository, scope, generation string) {
	t.Helper()
	for range 2000 {
		done, err := repo.AdvanceUsageHistoryProjection(context.Background(), scope, generation, 17)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("projection did not finish")
}

func finishProjectionBatches(t *testing.T, repo *analyticssqlite.Repository) {
	t.Helper()
	for range 2000 {
		done, err := repo.RunUsageHistoryProjectionBatch(context.Background(), 200)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("projection backlog did not finish")
}

func TestUsageHistoryProjectionIndependentDatabaseWriters(t *testing.T) {
	runProjectionDatabaseWriters(t, false)
}

func TestUsageHistoryPostgresProjectionIndependentDatabaseWriters(t *testing.T) {
	runProjectionDatabaseWriters(t, true)
}

func runProjectionDatabaseWriters(t *testing.T, postgres bool) {
	f := newProjectionDatabaseFixture(t, postgres)
	ctx := context.Background()
	generation, err := f.first.StartUsageHistoryRebuild(ctx, f.scope)
	require.NoError(t, err)
	finishExternalProjection(t, f.first, f.scope, generation)
	tx, err := f.database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, _, err = f.first.MutateUsageObservationsTx(ctx, tx, "collector", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{sealProjectionItem(t, f.item, 2, 80)}})
	require.NoError(t, err)
	started, finished := make(chan struct{}), make(chan error, 1)
	newest := sealProjectionItem(t, f.item, 3, 40)
	go func() {
		close(started)
		_, err := f.second.ReportUsageObservations(ctx, "collector", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{newest}})
		finished <- err
	}()
	<-started
	require.NoError(t, tx.Commit())
	require.NoError(t, <-finished)
	var sequence int64
	require.NoError(t, f.database.Get(&sequence, `SELECT MAX(commit_sequence) FROM usage_projection_log`))
	require.EqualValues(t, 3, sequence)
	shadow, err := f.second.StartUsageHistoryRebuild(ctx, f.scope)
	require.NoError(t, err)
	correction := sealProjectionItem(t, f.item, 4, 25)
	raceProjectionPublication(t, f, generation, shadow, correction)
	finishExternalProjection(t, f.second, f.scope, shadow)
	rows, state, err := f.first.ListCanonicalUsagePage(ctx, models.SessionUsageFilter{WorkspaceID: "projection-ws"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 25, *rows[0].TotalTokens)
	require.True(t, strings.HasPrefix(state.Generation, shadow+"."))
	require.False(t, state.Pending)
}

func raceProjectionPublication(t *testing.T, f projectionDatabaseFixture, active, shadow string, correction pluginsdk.UsageObservationV1) {
	t.Helper()
	start, finished := make(chan struct{}), make(chan error, 3)
	for _, operation := range []func() error{
		func() error {
			_, err := f.first.AdvanceUsageHistoryProjection(context.Background(), f.scope, active, 17)
			return err
		},
		func() error {
			_, err := f.second.AdvanceUsageHistoryProjection(context.Background(), f.scope, shadow, 17)
			return err
		},
		func() error {
			_, err := f.first.ReportUsageObservations(context.Background(), "collector", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{correction}})
			return err
		},
	} {
		go func(operation func() error) { <-start; finished <- operation() }(operation)
	}
	close(start)
	for range 3 {
		require.NoError(t, <-finished)
	}
}
