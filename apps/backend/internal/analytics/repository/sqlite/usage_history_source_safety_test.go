package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryTransferredTranscriptRejectsLegacyDTO(t *testing.T) {
	repo, _ := historyContractFixtureInOrg(t, "org-a")
	_, err := repo.db.Exec(`UPDATE workspaces SET org_id='org-b' WHERE id='ws-1'`)
	require.NoError(t, err)
	_, err = repo.UpsertSessionUsage(t.Context(), []models.SessionUsageMeasurement{usageFixtureMeasurement(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))})
	require.Error(t, err, "the compatibility mapper must enforce the same source-scope boundary")
	var count int
	require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM session_usage_measurements`))
	require.Zero(t, count)
}

func TestUsageHistorySourceSafetySurvivesTransferRestartAndErasure(t *testing.T) {
	for _, erase := range []bool{false, true} {
		t.Run(fmt.Sprintf("erase=%t", erase), func(t *testing.T) {
			repo, api := historyContractFixtureInOrg(t, "org-a")
			_, err := repo.db.Exec(`CREATE TABLE task_usage_events (id INTEGER PRIMARY KEY, usage_event_id TEXT, usage_binding_id TEXT, task_id TEXT)`)
			require.NoError(t, err)
			page, err := api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
			require.NoError(t, err)
			old := contractObservation(t, page.Targets[0].SubjectBinding, "historical-a", 1, "model", 100)
			require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", old).Status)
			_, err = repo.db.Exec(`UPDATE workspaces SET org_id='org-b' WHERE id='ws-1'`)
			require.NoError(t, err)
			assertUnsafeSourceTarget(t, repo)
			if erase {
				require.NoError(t, repo.EraseOrganizationUsage(t.Context(), "org-a"))
			}
			restarted, err := NewWithDB(repo.db, repo.ro)
			require.NoError(t, err)
			assertUnsafeSourceTarget(t, restarted)
			if !erase {
				_, err = repo.db.Exec(`UPDATE workspaces SET org_id='org-a' WHERE id='ws-1'`)
				require.NoError(t, err)
				assertUnsafeSourceTarget(t, restarted)
				old.Revision = 2
				sealContractObservation(t, &old)
				require.Equal(t, "rejected", reportContractObservation(t, restarted, "collector-a", old).Status)
				var amount int64
				require.NoError(t, repo.db.Get(&amount, `SELECT total_tokens FROM session_usage_measurements WHERE item_id='historical-a'`))
				require.EqualValues(t, 100, amount, "a scope guard must preserve already accepted historical evidence")
			}
		})
	}
}

func assertUnsafeSourceTarget(t *testing.T, repo *Repository) {
	t.Helper()
	page, err := repo.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, page.Targets, 1)
	target := page.Targets[0]
	require.Equal(t, "mixed", target.SourceScopeStatus)
	require.Nil(t, target.ExternalSessionID)
	require.Nil(t, target.SessionStartedAt)
	require.Nil(t, target.CoverageStart)
	for _, dated := range []bool{false, true} {
		item := contractObservation(t, target.SubjectBinding, fmt.Sprintf("foreign-history-%t", dated), 1, "model", 100)
		if dated {
			item.Coverage = &pluginsdk.UsageCoverageV1{CoverageStart: stringPtr("2026-01-01T00:00:00Z"), CoverageEnd: stringPtr("2026-01-03T00:00:00Z")}
		}
		sealContractObservation(t, &item)
		require.Equal(t, "rejected", reportContractObservation(t, repo, "collector-a", item).Status)
	}
}

func TestUsageHistoryOldBindingUsesCoverageAndExactNativeActor(t *testing.T) {
	repo, api := historyContractFixture(t)
	first := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	admitSharedWorkActor(t, repo, "actor-a", first)
	page, err := api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, repo.RetainNativeUsageTx(t.Context(), tx, usagehistory.NativeUsage{BindingID: "actor-a", UsageEventID: "proven-event", TaskID: "task-1", SessionID: "session-1", ProviderThreadID: "transcript-1", Model: "model", InputTokens: 30, TotalTokens: 30, UsageCompleteness: "complete", OccurredAt: first, CollectedAt: first}))
	require.NoError(t, tx.Commit())
	admitSharedWorkActor(t, repo, "actor-b", first.AddDate(0, 0, 1))
	priorDay := contractObservation(t, page.Targets[0].SubjectBinding, "prior-day", 1, "model", 30)
	priorDay.Coverage = &pluginsdk.UsageCoverageV1{CoverageStart: stringPtr("2026-01-01T00:00:00Z"), CoverageEnd: stringPtr("2026-01-02T00:00:00Z")}
	sealContractObservation(t, &priorDay)
	receipt := reportContractObservation(t, api, "collector-a", priorDay)
	require.Equal(t, "accepted", receipt.Status)
	require.Equal(t, "attributed", receipt.ActorAttributionStatus, "a proven historical interval must not acquire a later actor")
	cost := contractObservation(t, page.Targets[0].SubjectBinding, "exact-cost", 1, "model", 30)
	cost.WorkIdentity = &pluginsdk.UsageWorkIdentityV1{Scheme: "provider_event", ID: "proven-event"}
	cost.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: stringPtr(first.Format(time.RFC3339Nano))}
	cost.Tokens = nil
	cost.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(10), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
	cost.Quality.CostCoverageComplete = true
	sealContractObservation(t, &cost)
	require.Equal(t, "attributed", reportContractObservation(t, api, "collector-a", cost).ActorAttributionStatus)
	var actor string
	require.NoError(t, repo.db.Get(&actor, `SELECT actor_id FROM session_usage_buckets WHERE item_id='exact-cost'`))
	require.Equal(t, "actor-a", actor)
}

func TestUsageHistoryNativeEventKeepsOriginalTranscriptContext(t *testing.T) {
	repo, _ := historyContractFixture(t)
	first := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	admitSharedWorkActor(t, repo, "actor-a", first)
	native := usagehistory.NativeUsage{BindingID: "actor-a", UsageEventID: "context-event", TaskID: "task-1", SessionID: "session-1", Model: "model", InputTokens: 30, TotalTokens: 30, UsageCompleteness: "complete", OccurredAt: first, CollectedAt: first}
	retain := func() {
		tx, err := repo.db.BeginTxx(t.Context(), nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		require.NoError(t, repo.RetainNativeUsageTx(t.Context(), tx, native))
		require.NoError(t, tx.Commit())
	}
	retain()
	var transcript string
	require.NoError(t, repo.db.Get(&transcript, `SELECT transcript_id FROM session_usage_buckets WHERE source_record_id='context-event'`))
	require.Equal(t, "transcript-1", transcript)
	_, err := repo.db.Exec(`UPDATE task_sessions SET metadata='{"acp":{"session_id":"next-context"}}' WHERE id='session-1'`)
	require.NoError(t, err)
	_, err = repo.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	retain()
	completeFixtureProjection(t, repo)
	require.NoError(t, repo.db.Get(&transcript, `SELECT transcript_id FROM session_usage_buckets WHERE source_record_id='context-event'`))
	require.Equal(t, "transcript-1", transcript, "later source context cannot move an immutable native event")
}

func TestUsageHistoryProviderEventDoesNotProveWholeTranscriptCoverage(t *testing.T) {
	repo, api := historyContractFixture(t)
	point := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	admitSharedWorkActor(t, repo, "actor-a", point)
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, repo.RetainNativeUsageTx(t.Context(), tx, usagehistory.NativeUsage{BindingID: "actor-a", UsageEventID: "one-event", TaskID: "task-1", SessionID: "session-1", Model: "model", InputTokens: 30, TotalTokens: 30, UsageCompleteness: "complete", OccurredAt: point, CollectedAt: point}))
	require.NoError(t, tx.Commit())
	page, err := api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	item := contractObservation(t, page.Targets[0].SubjectBinding, "forged-event-coverage", 1, "model", 300)
	item.WorkIdentity = &pluginsdk.UsageWorkIdentityV1{Scheme: "provider_event", ID: "one-event"}
	sealContractObservation(t, &item)
	require.Equal(t, "rejected", reportContractObservation(t, api, "collector-a", item).Status)
}
