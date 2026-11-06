package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryLegacyFrozenTranscriptSurvivesLaterContext(t *testing.T) {
	repo, api := historyRetainedContractFixture(t)
	_, err := repo.db.Exec(`UPDATE task_sessions SET metadata='{"acp":{"session_id":"frozen-A"}}' WHERE id='session-1'`)
	require.NoError(t, err)
	_, err = api.ListUsageCollectionTargets(t.Context(), "legacy-frozen", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	first := usageFixtureMeasurement(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	first.Source, first.TranscriptID, first.SourceRecordID = "plugin:legacy-frozen", "frozen-A", "record-A"
	first.InputTokens, first.TotalTokens, first.CostSubcents = models.Int64(100), models.Int64(100), models.Int64(100)
	writes, err := repo.upsertSessionUsageFixture(t.Context(), []models.SessionUsageMeasurement{first})
	require.NoError(t, err)
	require.Len(t, writes, 1)
	oldID := writes[0].Measurement.ID
	oldWork := frozenWorkKey(t, repo, usageMeasurementsTable, oldID)
	_, err = repo.db.Exec(`UPDATE task_sessions SET metadata='{"acp":{"session_id":"current-B"}}' WHERE id='session-1'`)
	require.NoError(t, err)
	_, err = api.ListUsageCollectionTargets(t.Context(), "legacy-frozen", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	second := first
	second.TranscriptID, second.SourceRecordID, second.Model = "current-B", "record-B", "Opus"
	second.InputTokens, second.TotalTokens, second.CostSubcents = models.Int64(200), models.Int64(200), models.Int64(200)
	writes, err = repo.upsertSessionUsageFixture(t.Context(), []models.SessionUsageMeasurement{second})
	require.NoError(t, err)
	newWork := frozenWorkKey(t, repo, usageMeasurementsTable, writes[0].Measurement.ID)
	require.NotEqual(t, oldWork, newWork)
	repairFrozenWorkRow(t, repo, usageMeasurementsTable, oldID)
	assert.Equal(t, oldWork, frozenWorkKey(t, repo, usageMeasurementsTable, oldID), "trusted frozen A cannot be rebound under latest B")
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)
	assert.Equal(t, oldWork, frozenWorkKey(t, repo, usageMeasurementsTable, oldID))
	rows, err := repo.ListCanonicalSessionUsageMeasurements(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.EqualValues(t, 300, *rows[0].TotalTokens+*rows[1].TotalTokens)
}

func TestUsageHistoryNativeUnknownTranscriptNeverRebinds(t *testing.T) {
	repo, api := historyRetainedContractFixture(t)
	at := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	var taskIncarnation, sessionIncarnation string
	require.NoError(t, repo.db.QueryRowx(`SELECT task_incarnation_id,session_incarnation_id FROM usage_subjects WHERE task_id='task-1' AND session_id='session-1'`).Scan(&taskIncarnation, &sessionIncarnation))
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	require.NoError(t, repo.AdmitBindingTx(t.Context(), tx, usagehistory.Binding{
		ID: "unknown-native-execution", Kind: usagehistory.BindingKindExecution, WorkspaceID: "ws-1", TaskID: "task-1",
		TaskIncarnationID: taskIncarnation, SessionID: "session-1", SessionIncarnationID: sessionIncarnation,
		ActorID: "actor", ActorStatus: "attributed", ActorKind: "user", ActorSource: "execution", CapturedAt: at,
	}))
	require.NoError(t, repo.RetainNativeUsageTx(t.Context(), tx, usagehistory.NativeUsage{
		BindingID: "unknown-native-execution", UsageEventID: "unknown-native-event", TaskID: "task-1", SessionID: "session-1",
		Model: "native-model", Provider: "provider", InputTokens: 100, TotalTokens: 100,
		UsageCompleteness: "complete", OccurredAt: at, CollectedAt: at,
	}))
	require.NoError(t, tx.Commit())
	var id, transcript string
	require.NoError(t, repo.db.QueryRowx(`SELECT id,transcript_id FROM session_usage_buckets WHERE source_record_id='unknown-native-event'`).Scan(&id, &transcript))
	require.Empty(t, transcript)
	original := frozenWorkKey(t, repo, usageBucketsTable, id)
	for _, context := range []string{"later-B", "later-C"} {
		_, err = repo.db.Exec(`UPDATE task_sessions SET metadata=? WHERE id='session-1'`, `{"acp":{"session_id":"`+context+`"}}`)
		require.NoError(t, err)
		_, err = api.ListUsageCollectionTargets(t.Context(), "future", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
		require.NoError(t, err)
		var latest string
		require.NoError(t, repo.db.Get(&latest, `SELECT external_session_id FROM usage_collection_bindings WHERE subject_id=(SELECT subject_id FROM session_usage_buckets WHERE id=?) ORDER BY sequence DESC LIMIT 1`, id))
		require.Equal(t, context, latest, "positive control: the same subject really acquired the later context")
		repairFrozenWorkRow(t, repo, usageBucketsTable, id)
		assert.Equal(t, original, frozenWorkKey(t, repo, usageBucketsTable, id), "unknown original context cannot adopt later metadata")
	}
	require.NoError(t, repo.db.Get(&transcript, `SELECT transcript_id FROM session_usage_buckets WHERE id=?`, id))
	require.Empty(t, transcript)
}

func frozenWorkKey(t *testing.T, repo *Repository, table, id string) string {
	t.Helper()
	var work string
	require.NoError(t, repo.db.Get(&work, repo.db.Rebind(`SELECT selection_partition FROM `+table+` WHERE id=?`), id))
	return work
}

func repairFrozenWorkRow(t *testing.T, repo *Repository, table, id string) {
	t.Helper()
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	change, err := repo.repairUsageWorkRowTx(t.Context(), tx, selectionReference{ID: id, Table: table})
	require.NoError(t, err)
	require.NoError(t, repo.groupUsageProjectionMutationsTx(t.Context(), tx, []models.UsageMutation{change}))
	require.NoError(t, tx.Commit())
}
