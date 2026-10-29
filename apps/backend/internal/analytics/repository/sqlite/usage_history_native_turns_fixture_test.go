package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	internaldb "github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

const nativeTurnsWorkspace = "native-turns-workspace"
const nativeTurnsTask = "native-turns-task"
const nativeTurnsSession = "native-turns-session"
const nativeTurnsHostSource = "plugin:turns-host"

type nativeTurnsFixture struct {
	repo   *Repository
	tasks  *tasksqlite.Repository
	db     *sqlx.DB
	scope  string
	reopen func() *sqlx.DB
}

func newNativeTurnsFixture(t *testing.T, postgres bool, count int) nativeTurnsFixture {
	t.Helper()
	database, reopen := nativeTurnsDatabases(t, postgres)
	tasks, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	repo, err := NewWithDB(database, database)
	require.NoError(t, err)
	require.NoError(t, repo.ensureNativeUsageAccountingRepairSchema(t.Context()))
	tasks.SetUsageHistoryRetainer(repo)
	require.NoError(t, tasks.CreateWorkspace(t.Context(), &taskmodels.Workspace{ID: nativeTurnsWorkspace, Name: "Native turns", OrgID: "native-turns-captured-org"}))
	require.NoError(t, tasks.CreateTask(t.Context(), &taskmodels.Task{ID: nativeTurnsTask, WorkspaceID: nativeTurnsWorkspace, Title: "Native turns"}))
	require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{ID: nativeTurnsSession, TaskID: nativeTurnsTask}))
	turns := []*taskmodels.Turn{
		{ID: "native-root-a", TaskID: nativeTurnsTask, TaskSessionID: nativeTurnsSession},
		{ID: "native-root-b", TaskID: nativeTurnsTask, TaskSessionID: nativeTurnsSession},
	}
	for _, turn := range turns {
		require.NoError(t, tasks.CreateTurn(t.Context(), turn))
	}
	for i := range count {
		event := nativeTurnsEvent(i, turns)
		require.NoError(t, tasks.CreateTaskUsageEvent(t.Context(), event))
	}
	var scope string
	require.NoError(t, database.Get(&scope, database.Rebind(`SELECT scope_id FROM usage_scopes WHERE workspace_id=?`), nativeTurnsWorkspace))
	return nativeTurnsFixture{repo: repo, tasks: tasks, db: database, scope: scope, reopen: reopen}
}

func nativeTurnsDatabases(t *testing.T, postgres bool) (*sqlx.DB, func() *sqlx.DB) {
	t.Helper()
	if postgres {
		dsn := testutil.PostgresDSNFromEnv(t)
		first := testutil.OpenIsolatedPostgres(t, dsn)
		var schema string
		require.NoError(t, first.Get(&schema, `SELECT current_schema()`))
		return first, func() *sqlx.DB {
			raw, err := internaldb.OpenPostgres(testutil.PostgresDSNWithSearchPath(t, dsn, schema), 1, 1)
			if err != nil {
				t.Fatal("open native turn repair PostgreSQL pool")
			}
			pool := sqlx.NewDb(raw, "pgx")
			t.Cleanup(func() { _ = pool.Close() })
			return pool
		}
	}
	path := filepath.Join(t.TempDir(), "native-turns.db")
	open := func() *sqlx.DB {
		raw, err := internaldb.OpenSQLite(path)
		require.NoError(t, err)
		pool := sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { _ = pool.Close() })
		return pool
	}
	return open(), open
}

func nativeTurnsEvent(i int, turns []*taskmodels.Turn) *taskmodels.TaskUsageEvent {
	point := time.Date(2026, 10, 28, 23, 58, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute)
	event := &taskmodels.TaskUsageEvent{
		UsageEventID: fmt.Sprintf("native-response-%03d", i), TaskID: nativeTurnsTask, SessionID: nativeTurnsSession,
		Model: fmt.Sprintf("native-model-%d", i%2), Provider: fmt.Sprintf("native-provider-%d", i%2),
		AgentType: "native-cli", ProviderThreadID: "provider-thread", ProviderTurnID: "provider-without-root", ProviderResponseID: fmt.Sprintf("response-%03d", i),
		NativeScope: "direct", MeasurementSource: "response", UsageCompleteness: "exact", UsageSchemaVersion: 1,
		TokensIn: 10, TokensOut: models.Int64(10), TokensCachedRead: models.Int64(0), TokensCachedWrite: models.Int64(0),
		TokensThought: models.Int64(0), ReasoningOutputTokens: models.Int64(5), ReportedCacheWriteTokens: models.Int64(0),
		TokensTotal: 20, ReportedTotalTokens: models.Int64(20), CostSubcents: 5,
		CostSource: usageCostSourceReported, ContractVersion: 1, OccurredAt: point, CreatedAt: point,
	}
	turn := turns[0]
	if i == 2 {
		turn = turns[1]
	}
	if i != 3 {
		event.TurnID, event.UsageBindingID, event.ProviderTurnID = turn.ID, turn.UsageBindingID, "provider-"+turn.ID
	}
	return event
}

func nativeTurnsReplay(event *taskmodels.TaskUsageEvent) usagehistory.NativeUsage {
	return usagehistory.NativeUsage{
		BindingID: event.UsageBindingID, UsageEventID: event.UsageEventID, TaskID: event.TaskID, SessionID: event.SessionID, TurnID: event.TurnID,
		AgentClient: event.AgentType, Model: event.Model, Provider: event.Provider,
		ProviderThreadID: event.ProviderThreadID, ProviderTurnID: event.ProviderTurnID, ProviderResponseID: event.ProviderResponseID,
		NativeScope: event.NativeScope, MeasurementSource: event.MeasurementSource, UsageCompleteness: event.UsageCompleteness,
		UsageSchemaVersion: event.UsageSchemaVersion, ContractVersion: event.ContractVersion,
		InputTokens: event.TokensIn, OutputTokens: event.TokensOut, CacheReadTokens: event.TokensCachedRead, CacheWriteTokens: event.TokensCachedWrite,
		ThoughtTokens: event.TokensThought, ReasoningOutputTokens: event.ReasoningOutputTokens,
		ReportedCacheWriteTokens: event.ReportedCacheWriteTokens, ReportedTotalTokens: event.ReportedTotalTokens, TotalTokens: event.TokensTotal,
		CostSubcents: event.CostSubcents, CostPresent: true, CostBasis: models.UsageCostBasisReported, CostSource: event.CostSource,
		TokenSemanticsVersion: 1, ReasoningInsideOutput: true, OccurredAt: event.OccurredAt, CollectedAt: event.CreatedAt,
	}
}

func (f nativeTurnsFixture) addHostTurns(t *testing.T) {
	t.Helper()
	require.NoError(t, f.tasks.CreateTask(t.Context(), &taskmodels.Task{ID: "host-turns-task", WorkspaceID: nativeTurnsWorkspace, Title: "Host turns"}))
	require.NoError(t, f.tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{ID: "host-turns-session", TaskID: "host-turns-task"}))
	_, err := f.repo.UpsertSessionUsage(t.Context(), []models.SessionUsageMeasurement{{
		WorkspaceID: nativeTurnsWorkspace, TaskID: "host-turns-task", SessionID: "host-turns-session",
		Source: nativeTurnsHostSource, SourceRecordID: "host-turns", UsageIdentity: "host-turns", Model: "host-model", Provider: "host-provider",
		InputTokens: models.Int64(70), TotalTokens: models.Int64(70), ReasoningTokens: models.Int64(0), Turns: models.Int64(7), Revision: 1, PayloadDigest: "host-turns-digest",
		ObservedAt: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC),
	}})
	require.NoError(t, err)
}

func (f nativeTurnsFixture) addUndatedNative(t *testing.T) {
	t.Helper()
	tx, err := f.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	var id string
	require.NoError(t, tx.Get(&id, `SELECT id FROM session_usage_buckets WHERE source='native' ORDER BY id LIMIT 1`))
	row, err := f.repo.loadIndexedObservationTx(t.Context(), tx, usageBucketsTable, id)
	require.NoError(t, err)
	m := row.Measurement
	binding := usagehistory.Binding{
		ID: "undated-native-binding", Kind: usagehistory.BindingKindExecution,
		WorkspaceID: nativeTurnsWorkspace, OrganizationID: "native-turns-captured-org",
		TaskID: "legacy-native-task", TaskIncarnationID: "legacy-task-incarnation",
		SessionID: "legacy-native-session", SessionIncarnationID: "legacy-session-incarnation",
		ActorStatus: usagehistory.ActorStatusUnknown, ActorKind: usagehistory.ActorKindUnknown, ActorSource: "legacy",
	}
	require.NoError(t, f.repo.AdmitBindingTx(t.Context(), tx, binding))
	m.ID, m.SourceRecordID, m.UsageIdentity = "undated-native", "undated-native", "undated-native"
	m.TaskID, m.SessionID, m.UsageBindingID = binding.TaskID, binding.SessionID, binding.ID
	m.SubjectID = usageSubjectID(f.scope, binding.TaskID, binding.TaskIncarnationID, binding.SessionID, binding.SessionIncarnationID)
	m.CoverageStart, m.CoverageEnd, m.CoverageKey, m.SourceDate, m.SourceTimezone = nil, nil, "", "", ""
	m.Turns = models.Int64(1)
	_, err = f.repo.insertUsageTx(t.Context(), tx, m)
	require.NoError(t, err)
	require.NoError(t, f.repo.indexRetainedSourceRowTx(t.Context(), tx, usageMeasurementsTable, m.ID))
	require.NoError(t, tx.Commit())
}

func finishNativeTurnsBackfill(t *testing.T, repo *Repository) {
	t.Helper()
	for range 100 {
		done, err := repo.BackfillUsageHistory(t.Context(), 200)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("native turn fixture backfill did not finish")
}

func nativeTurnsSourceSnapshot(t *testing.T, repo *Repository) map[string]indexedObservation {
	t.Helper()
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	result := make(map[string]indexedObservation)
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		var ids []string
		require.NoError(t, tx.Select(&ids, `SELECT id FROM `+table+` ORDER BY id`))
		for _, id := range ids {
			row, err := repo.loadIndexedObservationTx(t.Context(), tx, table, id)
			require.NoError(t, err)
			result[table+"/"+id] = row
		}
	}
	return result
}

func poisonPublishedNativeTurns(t *testing.T, repo *Repository) {
	t.Helper()
	// The old schema had no scope completion certificate. Downgrading its
	// published data must also represent that unknown durable source state.
	_, err := repo.db.Exec(`UPDATE usage_projection_scopes SET native_accounting_source_version=0,
		native_accounting_required_sequence=0`)
	require.NoError(t, err)
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		_, err := repo.db.Exec(`UPDATE ` + table + ` SET turns=1, native_accounting_version=0 WHERE source='native'`)
		require.NoError(t, err)
	}
	var facts []struct {
		Scope      string `db:"scope_id"`
		Generation string `db:"generation"`
		Build      string `db:"build_id"`
		Key        string `db:"fact_key"`
		Payload    string `db:"payload"`
	}
	require.NoError(t, repo.db.Select(&facts, `SELECT scope_id, generation, build_id, fact_key, payload FROM usage_canonical_facts`))
	for _, row := range facts {
		var fact models.UsageCanonicalFact
		require.NoError(t, decodeUsageProjection(row.Payload, &fact))
		if fact.TokenSource == nil || fact.TokenSource.Source != usageSourceNative {
			continue
		}
		fact.Measurement.Turns = models.Int64(1)
		if fact.Measurement.ReasoningOutputTokens != nil {
			fact.Measurement.ReasoningTokens = models.Int64(0)
		}
		// A payload accepted before this normalization had no canonical version.
		// Reflection keeps this fixture compilable before the owner adds the field.
		version := reflect.ValueOf(&fact).Elem().FieldByName("NativeAccountingVersion")
		if version.IsValid() {
			version.SetInt(0)
		}
		payload, err := encodeUsageProjection(fact)
		require.NoError(t, err)
		_, err = repo.db.Exec(repo.db.Rebind(`UPDATE usage_canonical_facts SET payload=? WHERE scope_id=? AND generation=? AND build_id=? AND fact_key=?`), payload, row.Scope, row.Generation, row.Build, row.Key)
		require.NoError(t, err)
	}
	_, err = repo.db.Exec(`UPDATE usage_projection_contributions SET turns=1, reasoning_tokens=0 WHERE metric_kind='tokens' AND source='native'`)
	require.NoError(t, err)
	for _, table := range []string{"usage_daily_summaries", "usage_lifetime_summaries"} {
		_, err := repo.db.Exec(`UPDATE ` + table + ` SET turns=token_count, turns_missing=0, reasoning_tokens=0, reasoning_missing=0 WHERE metric_kind='tokens' AND source='native'`)
		require.NoError(t, err)
	}
}

func nativeTurnsPending(t *testing.T, database *sqlx.DB) int {
	t.Helper()
	var count int
	require.NoError(t, database.Get(&count, `SELECT (SELECT COUNT(*) FROM session_usage_measurements WHERE `+nativeUsageAccountingPendingSQL+`)
		+ (SELECT COUNT(*) FROM session_usage_buckets WHERE `+nativeUsageAccountingPendingSQL+`)`))
	return count
}

func nativeTurnsHead(t *testing.T, database *sqlx.DB) int64 {
	t.Helper()
	var head int64
	require.NoError(t, database.Get(&head, `SELECT COALESCE(MAX(commit_sequence),0) FROM usage_projection_scopes`))
	return head
}

func resumeNativeTurnsFixture(t *testing.T, f nativeTurnsFixture) *Repository {
	t.Helper()
	other := f.reopen()
	repo, err := NewWithDB(other, other)
	require.NoError(t, err)
	require.NoError(t, repo.ensureNativeUsageAccountingRepairSchema(t.Context()))
	return repo
}

func repairAllNativeTurns(t *testing.T, repo *Repository, limit int) {
	t.Helper()
	for range 100 {
		done, err := repo.repairNativeUsageAccountingBatch(context.Background(), limit)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("native turn repair did not finish")
}
