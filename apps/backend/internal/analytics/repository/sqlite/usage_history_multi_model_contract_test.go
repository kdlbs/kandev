package sqlite

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type multiModelContractCase struct {
	FirstModel, SecondModel       string
	FirstProvider, SecondProvider string
	FirstDaily, SecondDaily       bool
}

type multiModelAmounts struct{ Tokens, Cost int64 }

type multiModelRetainedEvidence struct {
	ItemID         string `db:"item_id"`
	SourceRecordID string `db:"source_record_id"`
	Work           string `db:"selection_partition"`
	Coverage       string `db:"selection_coverage"`
	Model          string `db:"model"`
	Provider       string `db:"provider"`
	Tokens         int64  `db:"total_tokens"`
	Cost           int64  `db:"cost_subcents"`
}

// @covers AC-COSTS-USAGE-HISTORY-004.2 AC-COSTS-USAGE-HISTORY-006.1
func TestUsageHistoryMultiModelContractDistinctAdapterRows(t *testing.T) {
	for _, daily := range []bool{false, true} {
		coverage := "lifetime"
		if daily {
			coverage = "daily"
		}
		for _, pair := range []struct{ name, firstModel, secondModel, firstProvider, secondProvider string }{
			{"models_same_provider", "Sonnet", "Opus", "anthropic", "anthropic"},
			{"models_different_providers", "Sonnet", "Opus", "anthropic", "bedrock"},
			{"same_model_different_providers", "Sonnet", "Sonnet", "anthropic", "bedrock"},
		} {
			t.Run(coverage+"/"+pair.name, func(t *testing.T) {
				runMultiModelContract(t, multiModelContractCase{
					FirstModel: pair.firstModel, SecondModel: pair.secondModel,
					FirstProvider: pair.firstProvider, SecondProvider: pair.secondProvider,
					FirstDaily: daily, SecondDaily: daily,
				})
			})
		}
	}
}

// @covers AC-COSTS-USAGE-HISTORY-003.5 AC-COSTS-USAGE-HISTORY-006.1
func TestUsageHistoryMultiModelContractLifetimeIsSegmentLocal(t *testing.T) {
	for _, pair := range []struct{ name, secondModel, secondProvider string }{
		{"different_model", "Opus", "anthropic"},
		{"different_provider", "Sonnet", "bedrock"},
	} {
		t.Run(pair.name, func(t *testing.T) {
			runMultiModelContract(t, multiModelContractCase{
				FirstModel: "Sonnet", SecondModel: pair.secondModel,
				FirstProvider: "anthropic", SecondProvider: pair.secondProvider,
				SecondDaily: true,
			})
		})
	}
}

func TestUsageHistoryMultiModelLegacyCumulativeMigration(t *testing.T) {
	for _, daily := range []bool{false, true} {
		t.Run(fmt.Sprintf("daily=%t", daily), func(t *testing.T) {
			repo, api := historyRetainedContractFixture(t)
			_, err := repo.db.Exec(`UPDATE task_sessions SET metadata = '{"acp":{"session_id":"legacy-model-transcript"}}' WHERE id='session-1'`)
			require.NoError(t, err)
			_, err = api.ListUsageCollectionTargets(t.Context(), "legacy-cost", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
			require.NoError(t, err)
			var input []models.SessionUsageMeasurement
			for i, model := range []string{"Sonnet", "Opus"} {
				amount := int64(i+1) * 100
				row := usageFixtureMeasurement(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC))
				row.Source, row.SourceRecordID, row.UsageIdentity = "plugin:legacy-cost", "legacy-model-transcript", "session-model-cost"
				row.TranscriptID, row.Model, row.Provider = "legacy-model-transcript", model, "anthropic"
				row.InputTokens, row.TotalTokens, row.CostSubcents = models.Int64(amount), models.Int64(amount), models.Int64(amount)
				row.OutputTokens, row.CacheReadTokens, row.CacheWriteTokens, row.ReasoningTokens = models.Int64(0), models.Int64(0), models.Int64(0), models.Int64(0)
				row.PayloadDigest = "legacy-" + model
				if daily {
					start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
					end := start.AddDate(0, 0, 1)
					row.CoverageStart, row.CoverageEnd = &start, &end
					row.SourceDate, row.SourceTimezone, row.CoverageKey = "2026-01-02", "UTC", "2026-01-02"
				}
				input = append(input, row)
			}
			result, err := repo.upsertSessionUsageFixture(t.Context(), input)
			require.NoError(t, err)
			require.Len(t, result, 2)
			for _, row := range result {
				require.Equal(t, models.UsageWriteApplied, row.Status)
			}
			// Persisted old-DTO rows have no observation metadata declaration.
			var metadata int
			require.NoError(t, repo.db.Get(&metadata, `SELECT COUNT(*) FROM session_usage_measurements WHERE observation_metadata<>'' UNION ALL SELECT COUNT(*) FROM session_usage_buckets WHERE observation_metadata<>'' LIMIT 1`))
			require.Zero(t, metadata)
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)
			assertMultiModelReads(t, repo, multiModelContractCase{FirstDaily: daily, SecondDaily: daily}, map[string]multiModelAmounts{
				"Sonnet@anthropic": {Tokens: 100, Cost: 100}, "Opus@anthropic": {Tokens: 200, Cost: 200},
			})
		})
	}
}

func runMultiModelContract(t *testing.T, scenario multiModelContractCase) {
	t.Helper()
	repo, api := historyRetainedContractFixture(t)
	_, err := repo.db.Exec(`UPDATE task_sessions SET metadata = '{"acp":{"session_id":"multi-model-transcript"}}' WHERE id='session-1'`)
	require.NoError(t, err)
	targets, err := api.ListUsageCollectionTargets(t.Context(), "session-cost", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, targets.Targets, 1)
	require.Equal(t, "multi-model-transcript", *targets.Targets[0].ExternalSessionID)
	binding := targets.Targets[0].SubjectBinding
	first := multiModelAdapterObservation(t, binding, "model-record-1", scenario.FirstModel, scenario.FirstProvider, 100, scenario.FirstDaily)
	second := multiModelAdapterObservation(t, binding, "model-record-2", scenario.SecondModel, scenario.SecondProvider, 200, scenario.SecondDaily)
	require.NotEqual(t, first.ItemID, second.ItemID)
	require.NotEqual(t, first.SourceRecordID, second.SourceRecordID)
	require.Nil(t, first.WorkIdentity)
	require.Nil(t, second.WorkIdentity)
	for _, item := range []pluginsdk.UsageObservationV1{first, second} {
		result, reportErr := api.ReportUsageObservations(t.Context(), "session-cost", pluginsdk.UsageObservationBatchV1{SchemaVersion: pluginsdk.UsageObservationSchemaVersionV2, Observations: []pluginsdk.UsageObservationV1{item}})
		require.NoError(t, reportErr)
		require.Len(t, result.Results, 1)
		receipt := result.Results[0]
		require.Equal(t, "accepted", receipt.Status)
		require.Equal(t, "proven", receipt.WorkIdentityStatus)
	}
	retained := multiModelRetainedRows(t, repo)
	require.Len(t, retained, 2)
	require.Equal(t, retained[0].Work, retained[1].Work)
	if scenario.FirstDaily == scenario.SecondDaily {
		require.Equal(t, retained[0].Coverage, retained[1].Coverage)
	} else {
		require.NotEqual(t, retained[0].Coverage, retained[1].Coverage)
	}
	t.Logf("accepted adapter rows: %+v", retained)
	expected := map[string]multiModelAmounts{
		scenario.FirstModel + "@" + scenario.FirstProvider:   {Tokens: 100, Cost: 100},
		scenario.SecondModel + "@" + scenario.SecondProvider: {Tokens: 200, Cost: 200},
	}
	selected := selectContractPartition(t, repo, first.ItemID)
	require.False(t, selected.Preparing)
	var selectedRows []models.SessionUsageMeasurement
	for _, fact := range selected.Facts {
		selectedRows = append(selectedRows, fact.Measurement)
	}
	assertMultiModelAmounts(t, "bounded selector", expected, selectedRows)
	// Use actual bounded backfill and projection work, never seeded readiness markers.
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)
	assertMultiModelAmounts(t, "committed projection", expected, multiModelPublishedRows(t, repo, retained[0].Work))
	assertMultiModelReads(t, repo, scenario, expected)
}

func multiModelAdapterObservation(t *testing.T, binding, record, model, provider string, amount int64, daily bool) pluginsdk.UsageObservationV1 {
	t.Helper()
	date, zone := "", ""
	if daily {
		date, zone = "2026-01-02", "UTC"
	}
	// Match tokscaleUsageAdapter's item shape: a stable source record, scoped
	// item identity, no WorkIdentity, and optional source-calendar coverage.
	encoded, err := json.Marshal([]string{"tokscale-v1", binding, record, date, zone})
	require.NoError(t, err)
	item := contractObservation(t, binding, fmt.Sprintf("sha256:%x", sha256.Sum256(encoded)), 1, model, amount)
	item.SourceRecordID, item.SourceVersion = record, "session-cost-tokscale-adapter-v1"
	item.Segment = &pluginsdk.UsageSegmentIdentityV1{DecompositionID: "tokscale-session-models-v1", SegmentID: record}
	item.ModelProvider, item.AgentClient = stringPtr(provider), stringPtr("claude")
	item.RawSourceLabels = map[string]string{"source": "claude"}
	item.Tokens.OutputTokens, item.Tokens.CacheReadTokens = models.Int64(0), models.Int64(0)
	item.Tokens.CacheWriteTokens, item.Tokens.ReasoningTokens = models.Int64(0), models.Int64(0)
	item.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(amount), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisEstimated}
	item.Quality.CostCoverageComplete = true
	if daily {
		item.Coverage = &pluginsdk.UsageCoverageV1{
			CoverageStart: stringPtr("2026-01-02T00:00:00Z"), CoverageEnd: stringPtr("2026-01-03T00:00:00Z"),
			SourceDate: &date, SourceTimezone: &zone,
		}
	}
	sealContractObservation(t, &item)
	return item
}

func multiModelRetainedRows(t *testing.T, repo *Repository) []multiModelRetainedEvidence {
	t.Helper()
	var rows []multiModelRetainedEvidence
	columns := `item_id,source_record_id,selection_partition,selection_coverage,model,provider,total_tokens,cost_subcents`
	query := `SELECT ` + columns + ` FROM session_usage_measurements WHERE source='plugin:session-cost'
		UNION ALL SELECT ` + columns + ` FROM session_usage_buckets WHERE source='plugin:session-cost' ORDER BY source_record_id`
	require.NoError(t, repo.db.Select(&rows, query))
	return rows
}

func multiModelPublishedRows(t *testing.T, repo *Repository, work string) []models.SessionUsageMeasurement {
	t.Helper()
	var payloads []string
	require.NoError(t, repo.db.Select(&payloads, repo.db.Rebind(`SELECT f.payload FROM usage_canonical_facts f
		JOIN usage_projection_scopes s ON s.scope_id=f.scope_id AND s.active_generation=f.generation
		JOIN usage_projection_applied a ON a.scope_id=f.scope_id AND a.generation=f.generation AND a.work_key=f.work_key AND a.build_id=f.build_id
		WHERE f.work_key=? ORDER BY f.fact_key`), work))
	var rows []models.SessionUsageMeasurement
	for _, payload := range payloads {
		var fact models.UsageCanonicalFact
		require.NoError(t, decodeUsageProjection(payload, &fact))
		rows = append(rows, fact.Measurement)
	}
	return rows
}

func assertMultiModelReads(t *testing.T, repo *Repository, scenario multiModelContractCase, expected map[string]multiModelAmounts) {
	t.Helper()
	filter := models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model", Timezone: "UTC"}
	if scenario.FirstDaily && scenario.SecondDaily {
		start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 0, 1)
		filter.Start, filter.End = &start, &end
	}
	report, err := repo.ListSessionUsage(t.Context(), filter)
	require.NoError(t, err)
	require.Equal(t, "ready", report.ProjectionStatus)
	require.False(t, report.ProjectionPending)
	require.NotEmpty(t, report.ProjectionGeneration)
	var publicRows []models.SessionUsageMeasurement
	for _, row := range report.Rows {
		publicRows = append(publicRows, models.SessionUsageMeasurement{Model: row.Model, Provider: row.Provider, TotalTokens: row.TotalTokens, CostSubcents: row.CostSubcents})
	}
	assertMultiModelAmounts(t, "public report", expected, publicRows)
	assert.Equal(t, models.Int64(300), report.Totals.TotalTokens, "public total must include both independent model/provider records")
	assert.Equal(t, models.Int64(300), report.Totals.CostSubcents, "public cost must include both independent model/provider records")
	saved, state, err := repo.ListRetainedCanonicalUsagePage(t.Context(), filter)
	require.NoError(t, err)
	require.Equal(t, "ready", state.Status)
	require.False(t, state.Pending)
	require.Equal(t, report.ProjectionGeneration, state.Generation)
	assertMultiModelAmounts(t, "saved canonical read", expected, saved)
	t.Logf("publication=%s public_total=%v saved_rows=%d", state.Generation, report.Totals.TotalTokens, len(saved))
	if scenario.FirstDaily != scenario.SecondDaily {
		start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 0, 1)
		filter.Start, filter.End = &start, &end
		dated, _, datedErr := repo.ListRetainedCanonicalUsagePage(t.Context(), filter)
		require.NoError(t, datedErr)
		assertMultiModelAmounts(t, "saved dated positive control", map[string]multiModelAmounts{
			scenario.SecondModel + "@" + scenario.SecondProvider: {Tokens: 200, Cost: 200},
		}, dated)
	}
}

func assertMultiModelAmounts(t *testing.T, layer string, expected map[string]multiModelAmounts, rows []models.SessionUsageMeasurement) {
	t.Helper()
	actual := map[string]multiModelAmounts{}
	var tokens, cost int64
	for _, row := range rows {
		if !assert.NotNil(t, row.TotalTokens, "%s: known token total", layer) || !assert.NotNil(t, row.CostSubcents, "%s: known cost", layer) {
			continue
		}
		key := row.Model + "@" + row.Provider
		value := actual[key]
		value.Tokens, value.Cost = value.Tokens+*row.TotalTokens, value.Cost+*row.CostSubcents
		actual[key] = value
		tokens, cost = tokens+*row.TotalTokens, cost+*row.CostSubcents
	}
	t.Logf("%s: rows=%d tokens=%d cost_subcents=%d dimensions=%v", layer, len(rows), tokens, cost, actual)
	assert.Equal(t, expected, actual, "%s must retain each independent model/provider contribution", layer)
}
