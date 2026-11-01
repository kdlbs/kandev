package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func retainSegmentNative(t *testing.T, repo *Repository, amount int64, configure ...func(*usagehistory.NativeUsage)) {
	t.Helper()
	var taskIncarnation, sessionIncarnation string
	require.NoError(t, repo.db.QueryRowx(`SELECT task_incarnation_id,session_incarnation_id FROM usage_subjects WHERE task_id='task-1' AND session_id='session-1'`).Scan(&taskIncarnation, &sessionIncarnation))
	at := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, repo.AdmitBindingTx(t.Context(), tx, usagehistory.Binding{
		ID: "segment-native-execution", Kind: usagehistory.BindingKindExecution, WorkspaceID: "ws-1", TaskID: "task-1",
		TaskIncarnationID: taskIncarnation, SessionID: "session-1", SessionIncarnationID: sessionIncarnation,
		ActorID: "native-actor", ActorStatus: "attributed", ActorKind: "user", ActorSource: "execution", CapturedAt: at,
	}))
	native := usagehistory.NativeUsage{
		BindingID: "segment-native-execution", UsageEventID: "segment-native-event", TaskID: "task-1", SessionID: "session-1",
		ProviderThreadID: "segment-thread", Model: "Sonnet", Provider: "anthropic", InputTokens: amount,
		OutputTokens: models.Int64(0), TotalTokens: amount, UsageCompleteness: "complete", OccurredAt: at, CollectedAt: at,
	}
	for _, apply := range configure {
		apply(&native)
	}
	require.NoError(t, repo.RetainNativeUsageTx(t.Context(), tx, native))
	require.NoError(t, tx.Commit())
}

func TestUsageHistorySegmentsPreferFullerCoverageAndAvailableCost(t *testing.T) {
	for _, daily := range []bool{false, true} {
		t.Run(fmt.Sprintf("daily=%t", daily), func(t *testing.T) {
			repo := segmentContractFixture(t, false)
			binding := segmentBinding(t, repo, "collector-a", "session-1")
			a := multiModelAdapterObservation(t, binding, "sonnet", "Sonnet", "anthropic", 100, daily)
			b := multiModelAdapterObservation(t, binding, "opus", "Opus", "anthropic", 200, daily)
			reportSegmentBatch(t, repo, "collector-a", a, b)
			retainSegmentNative(t, repo, 20)
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)
			report := requireSegmentTotals(t, repo, 300, 300)
			require.True(t, report.Totals.Partial, "a partial native overlap does not establish complete equivalent coverage")
		})
	}
}

func TestUsageHistorySegmentsIndependentItemRevisionAndFreshness(t *testing.T) {
	for _, equalRevision := range []bool{false, true} {
		t.Run(fmt.Sprintf("equal_revision=%t", equalRevision), func(t *testing.T) {
			repo := segmentContractFixture(t, false)
			binding := segmentBinding(t, repo, "collector-a", "session-1")
			old := multiModelAdapterObservation(t, binding, "old-item", "Sonnet", "anthropic", 100, false)
			if !equalRevision {
				old.Revision = 100
			}
			sealContractObservation(t, &old)
			reportSegmentBatch(t, repo, "collector-a", old)
			tasks, err := tasksqlite.NewWithDB(repo.db, repo.ro, nil)
			require.NoError(t, err)
			tasks.SetUsageHistoryRetainer(repo)
			require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{ID: "new-binding", TaskID: "task-1", StartedAt: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), Metadata: map[string]any{"acp": map[string]any{"session_id": "segment-thread"}}}))
			newer := multiModelAdapterObservation(t, segmentBinding(t, repo, "collector-a", "new-binding"), "new-item", "Sonnet", "anthropic", 300, false)
			newer.Segment = old.Segment
			sealContractObservation(t, &newer)
			reportSegmentBatch(t, repo, "collector-a", newer)
			var times []time.Time
			require.NoError(t, repo.db.Select(&times, `SELECT collected_at FROM session_usage_measurements ORDER BY collected_at`))
			require.Len(t, times, 2)
			require.True(t, times[1].After(times[0]), "the host accepted the independent item later")
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)
			report := requireSegmentTotals(t, repo, 300, 300)
			require.True(t, report.Totals.Partial, "the shared source has no single subject owner")
			rows, _, err := repo.ListRetainedCanonicalUsagePage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Empty(t, rows[0].SubjectID)
		})
	}
}

func TestUsageHistorySegmentsUnknownCostCannotHideKnownEstimate(t *testing.T) {
	repo := segmentContractFixture(t, false)
	unknown := multiModelAdapterObservation(t, segmentBinding(t, repo, "collector-a", "session-1"), "unknown", "Sonnet", "anthropic", 100, false)
	unknown.Cost.Amount, unknown.Cost.Basis = nil, pluginsdk.UsageCostBasisUnknown
	sealContractObservation(t, &unknown)
	reportSegmentBatch(t, repo, "collector-a", unknown)
	known := multiModelAdapterObservation(t, segmentBinding(t, repo, "collector-z", "session-1"), "known", "Sonnet", "anthropic", 100, false)
	known.Quality.Stale = true
	sealContractObservation(t, &known)
	reportSegmentBatch(t, repo, "collector-z", known)
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)
	report := requireSegmentTotals(t, repo, 100, 100)
	require.True(t, report.Totals.Estimated)
	rows, _, err := repo.ListRetainedCanonicalUsagePage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	var cost *models.SessionUsageMeasurement
	for i := range rows {
		if rows[i].CostSubcents != nil {
			cost = &rows[i]
		}
	}
	require.NotNil(t, cost)
	require.True(t, cost.Stale)
	require.True(t, cost.Estimated)
}
