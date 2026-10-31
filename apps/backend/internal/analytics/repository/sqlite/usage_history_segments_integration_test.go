package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func segmentContractFixture(t *testing.T, postgres bool) *Repository {
	t.Helper()
	if !postgres {
		repo, _ := historyRetainedContractFixture(t)
		_, err := repo.db.Exec(`UPDATE task_sessions SET metadata='{"acp":{"session_id":"segment-thread"}}' WHERE id='session-1'`)
		require.NoError(t, err)
		return repo
	}
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	var version string
	require.NoError(t, conn.QueryRow(`SHOW server_version`).Scan(&version))
	t.Logf("PostgreSQL %s", version)
	tasks, err := tasksqlite.NewWithDB(conn, conn, nil)
	require.NoError(t, err)
	repo, err := NewWithDB(conn, conn)
	require.NoError(t, err)
	tasks.SetUsageHistoryRetainer(repo)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, tasks.CreateWorkspace(t.Context(), &taskmodels.Workspace{ID: "ws-1", Name: "Segments"}))
	require.NoError(t, tasks.CreateTask(t.Context(), &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1", Title: "Task", CreatedAt: at, UpdatedAt: at}))
	require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{ID: "session-1", TaskID: "task-1", StartedAt: at, Metadata: map[string]any{"acp": map[string]any{"session_id": "segment-thread"}}}))
	return repo
}

func segmentBinding(t *testing.T, repo *Repository, collector, session string) string {
	t.Helper()
	page, err := repo.ListUsageCollectionTargets(t.Context(), collector, pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1", Limit: 200})
	require.NoError(t, err)
	for _, target := range page.Targets {
		if target.SessionID != nil && *target.SessionID == session {
			require.Equal(t, "eligible", target.SourceScopeStatus)
			return target.SubjectBinding
		}
	}
	t.Fatal("eligible retained target was not issued")
	return ""
}

func reportSegmentBatch(t *testing.T, repo *Repository, collector string, items ...pluginsdk.UsageObservationV1) {
	t.Helper()
	for start := 0; start < len(items); start += maxSessionUsageBatch {
		batch := pluginsdk.UsageObservationBatchV1{SchemaVersion: pluginsdk.UsageObservationSchemaVersionV2, Observations: items[start:min(start+maxSessionUsageBatch, len(items))]}
		result, err := repo.ReportUsageObservations(t.Context(), collector, batch)
		require.NoError(t, err)
		require.Equal(t, "accepted", result.Status)
		for _, receipt := range result.Results {
			require.Contains(t, []string{"accepted", "unchanged"}, receipt.Status)
		}
	}
}

func requireSegmentTotals(t *testing.T, repo *Repository, tokens, cost int64) *models.TokenUsageReport {
	t.Helper()
	report, err := repo.ListSessionUsage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	require.NoError(t, err)
	require.Equal(t, "ready", report.ProjectionStatus)
	require.False(t, report.ProjectionPending)
	require.Equal(t, models.Int64(tokens), report.Totals.TotalTokens)
	require.Equal(t, models.Int64(cost), report.Totals.CostSubcents)
	rows, state, err := repo.ListRetainedCanonicalUsagePage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Equal(t, report.ProjectionGeneration, state.Generation)
	var savedTokens, savedCost int64
	for _, row := range rows {
		if row.TotalTokens != nil {
			savedTokens += *row.TotalTokens
		}
		if row.CostSubcents != nil {
			savedCost += *row.CostSubcents
		}
	}
	require.Equal(t, tokens, savedTokens)
	require.Equal(t, cost, savedCost)
	return report
}

func TestUsageHistorySegmentsSharedSubjectsAndCollectorAlternatives(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", postgres), func(t *testing.T) {
			repo := segmentContractFixture(t, postgres)
			first := segmentBinding(t, repo, "collector-a", "session-1")
			a := multiModelAdapterObservation(t, first, "source-a", "Sonnet", "anthropic", 100, false)
			b := multiModelAdapterObservation(t, first, "source-b", "Opus", "anthropic", 200, false)
			reportSegmentBatch(t, repo, "collector-a", a, b)
			tasks, err := tasksqlite.NewWithDB(repo.db, repo.ro, nil)
			require.NoError(t, err)
			tasks.SetUsageHistoryRetainer(repo)
			require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{ID: "shared-session", TaskID: "task-1", StartedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Metadata: map[string]any{"acp": map[string]any{"session_id": "segment-thread"}}}))
			shared := segmentBinding(t, repo, "collector-a", "shared-session")
			for _, item := range []pluginsdk.UsageObservationV1{a, b} {
				copy := item
				copy.SubjectBinding, copy.ItemID = shared, item.ItemID+"-shared"
				sealContractObservation(t, &copy)
				reportSegmentBatch(t, repo, "collector-a", copy)
			}
			other := segmentBinding(t, repo, "collector-z", "session-1")
			for i, original := range []pluginsdk.UsageObservationV1{a, b} {
				copy := original
				copy.SubjectBinding, copy.ItemID = other, fmt.Sprintf("other-%d", i)
				copy.Segment = &pluginsdk.UsageSegmentIdentityV1{DecompositionID: "different-local-layout", SegmentID: fmt.Sprint(i)}
				copy.Tokens = &pluginsdk.UsageTokenAmountsV1{InputTokens: models.Int64(900), TotalTokens: models.Int64(900)}
				sealContractObservation(t, &copy)
				reportSegmentBatch(t, repo, "collector-z", copy)
			}
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)
			report := requireSegmentTotals(t, repo, 300, 300)
			require.True(t, report.Totals.Partial, "incompatible source decompositions remain explicit alternatives")
			tasksReport, err := repo.ListSessionUsage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "task"})
			require.NoError(t, err)
			require.Equal(t, models.Int64(300), tasksReport.Totals.TotalTokens)
			for _, row := range tasksReport.Rows {
				require.Empty(t, row.SubjectID)
				require.Empty(t, row.TaskID)
				require.Empty(t, row.SessionID)
				require.False(t, row.TaskAvailable)
			}
		})
	}
}

func TestUsageHistorySegmentsCorrectionAtomicPublicationAndTombstone(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", postgres), func(t *testing.T) {
			repo := segmentContractFixture(t, postgres)
			binding := segmentBinding(t, repo, "collector-a", "session-1")
			a := multiModelAdapterObservation(t, binding, "record-a", "Sonnet", "anthropic", 100, false)
			b := multiModelAdapterObservation(t, binding, "record-b", "Opus", "anthropic", 200, false)
			reportSegmentBatch(t, repo, "collector-a", a, b)
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)
			before := requireSegmentTotals(t, repo, 300, 300)
			key := segmentFactKey(t, repo, "Sonnet")
			a.Revision, a.ModelID, a.ModelProvider = 2, stringPtr("corrected-Sonnet"), stringPtr("bedrock")
			a.Tokens.InputTokens, a.Tokens.TotalTokens, a.Cost.Amount = models.Int64(40), models.Int64(40), models.Int64(20)
			sealContractObservation(t, &a)
			reportSegmentBatch(t, repo, "collector-a", a)
			pending, err := repo.ListSessionUsage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
			require.NoError(t, err)
			require.Equal(t, before.ProjectionGeneration, pending.ProjectionGeneration)
			require.True(t, pending.ProjectionPending)
			require.Equal(t, models.Int64(300), pending.Totals.TotalTokens)
			completeFixtureProjection(t, repo)
			after := requireSegmentTotals(t, repo, 240, 220)
			require.NotEqual(t, before.ProjectionGeneration, after.ProjectionGeneration)
			require.Equal(t, key, segmentFactKey(t, repo, "corrected-Sonnet"))
			tombstone := pluginsdk.UsageObservationTombstoneV1{SubjectBinding: binding, ItemID: b.ItemID, Revision: 2}
			tombstone.PayloadDigest, err = tombstone.ComputePayloadDigestV1()
			require.NoError(t, err)
			result, err := repo.ReportUsageObservations(t.Context(), "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 2, Tombstones: []pluginsdk.UsageObservationTombstoneV1{tombstone}})
			require.NoError(t, err)
			require.Equal(t, "accepted", result.Status)
			completeFixtureProjection(t, repo)
			requireSegmentTotals(t, repo, 40, 20)
		})
	}
}

func segmentFactKey(t *testing.T, repo *Repository, model string) string {
	t.Helper()
	var key string
	require.NoError(t, repo.db.Get(&key, repo.db.Rebind(`SELECT f.fact_key FROM usage_canonical_facts f JOIN usage_projection_applied a ON a.scope_id=f.scope_id AND a.generation=f.generation AND a.work_key=f.work_key AND a.build_id=f.build_id JOIN usage_projection_scopes s ON s.scope_id=f.scope_id AND s.active_generation=f.generation WHERE f.model=?`), model))
	return key
}

func TestUsageHistorySegmentsOversizedRestartParity(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", postgres), func(t *testing.T) {
			repo := segmentContractFixture(t, postgres)
			binding := segmentBinding(t, repo, "collector-a", "session-1")
			a := multiModelAdapterObservation(t, binding, "oversized-a", "Sonnet", "anthropic", 100, true)
			b := multiModelAdapterObservation(t, binding, "oversized-b", "Opus", "anthropic", 200, true)
			reportSegmentBatch(t, repo, "collector-a", a, b)
			ordinary := selectContractPartition(t, repo, a.ItemID)
			require.False(t, ordinary.Preparing)
			require.Len(t, ordinary.Facts, 2)
			var copies []pluginsdk.UsageObservationV1
			for i := range 221 {
				for _, original := range []pluginsdk.UsageObservationV1{a, b} {
					copy := original
					copy.ItemID = fmt.Sprintf("%s-copy-%03d", original.ItemID, i)
					sealContractObservation(t, &copy)
					copies = append(copies, copy)
					if i == 220 {
						// Equal cumulative snapshots keep the same amount and fact
						// key, while the newest independent item owns provenance.
						for j := range ordinary.Facts {
							if ordinary.Facts[j].Measurement.Model == *copy.ModelID {
								ordinary.Facts[j].TokenSource.ItemID = copy.ItemID
								ordinary.Facts[j].CostSource.ItemID = copy.ItemID
							}
						}
					}
				}
			}
			reportSegmentBatch(t, repo, "collector-a", copies...)
			bounded := selectContractPartition(t, repo, a.ItemID)
			require.True(t, bounded.Preparing)
			require.LessOrEqual(t, bounded.VisitedRows, 201)
			finishFixtureRetention(t, repo)
			var scope string
			require.NoError(t, repo.db.Get(&scope, `SELECT scope_id FROM usage_scopes LIMIT 1`))
			generation, err := repo.StartUsageHistoryRebuild(t.Context(), scope)
			require.NoError(t, err)
			checkpoint := false
			for range 100 {
				_, err := repo.AdvanceUsageHistoryProjection(t.Context(), scope, generation, 17)
				require.NoError(t, err)
				var count int
				require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM usage_projection_resolutions`))
				if count > 0 {
					checkpoint = true
					break
				}
			}
			require.True(t, checkpoint)
			restarted, err := NewWithDB(repo.db, repo.ro)
			require.NoError(t, err)
			drainProjection(t, restarted, scope, generation)
			facts, state, err := restarted.ListUsageCanonicalFacts(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day"})
			require.NoError(t, err)
			require.Equal(t, "ready", state.Status)
			require.ElementsMatch(t, usageParityEvidence(ordinary.Facts), usageParityEvidence(facts))
			requireSegmentTotals(t, restarted, 300, 300)
		})
	}
}
