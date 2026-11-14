package plugins

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	analyticsmodels "github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func producerFixtureID(parts ...string) string {
	encoded, _ := json.Marshal(parts)
	return fmt.Sprintf("sha256:%x", sha256.Sum256(encoded))
}

// This fixture uses the explicit DTO emitted by the pinned v4.15.1 adapter:
// producer session/model series, merged provider/client labels, and checked
// category totals. Core receives the declaration; it does not infer members.
func TestUsageHistoryHostOrdinaryTokscaleSegmentsRead300(t *testing.T) {
	for _, daily := range []bool{false, true} {
		t.Run(fmt.Sprintf("daily=%t", daily), func(t *testing.T) {
			repo, host, _ := metricApplicabilityHost(t)
			history, ok := pluginsdk.HostUsageHistory(host)
			require.True(t, ok)
			page, err := history.ListTargets(t.Context(), pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "metric-ws", CurrentTarget: &pluginsdk.UsageCollectionCurrentTargetQueryV1{SessionID: "metric-session"}})
			require.NoError(t, err)
			require.Len(t, page.Targets, 1)
			batch := pluginsdk.UsageObservationBatchV1{SchemaVersion: 2}
			for i, model := range []string{"gpt-5", "claude-sonnet-4-5"} {
				amount := int64(i+1) * 100
				provider, client, currency := []string{"openai", "anthropic"}[i], []string{"codex", "claude"}[i], "USD"
				date, zone := "", ""
				if daily {
					date, zone = "2026-01-02", "UTC"
				}
				record := producerFixtureID("tokscale-session-model", "metric-thread", model)
				item := pluginsdk.UsageObservationV1{SubjectBinding: page.Targets[0].SubjectBinding,
					ItemID: producerFixtureID("tokscale-v1", page.Targets[0].SubjectBinding, record, date, zone), SourceRecordID: record, SourceVersion: "session-cost-tokscale-adapter-v1", Revision: 1,
					Segment: &pluginsdk.UsageSegmentIdentityV1{DecompositionID: "tokscale-session-model-v1", SegmentID: producerFixtureID("tokscale-session-model-v1", "metric-thread", model)},
					ModelID: &model, ModelProvider: &provider, RawSourceLabels: map[string]string{"provider": provider, "client": client},
					Tokens: &pluginsdk.UsageTokenAmountsV1{InputTokens: &amount, OutputTokens: analyticsmodels.Int64(0), CacheReadTokens: analyticsmodels.Int64(0), CacheWriteTokens: analyticsmodels.Int64(0), ReasoningTokens: analyticsmodels.Int64(0), TotalTokens: &amount},
					Cost:   &pluginsdk.UsageCostV1{Amount: analyticsmodels.Int64(amount * 100), Currency: &currency, Basis: pluginsdk.UsageCostBasisEstimated}, Quality: &pluginsdk.UsageObservationQualityV1{TokenCoverageComplete: true, CostCoverageComplete: true}}
				if daily {
					start, end := "2026-01-02T00:00:00Z", "2026-01-03T00:00:00Z"
					item.Coverage = &pluginsdk.UsageCoverageV1{CoverageStart: &start, CoverageEnd: &end, SourceDate: &date, SourceTimezone: &zone}
				}
				item.PayloadDigest, err = item.ComputePayloadDigestV1()
				require.NoError(t, err)
				batch.Observations = append(batch.Observations, item)
			}
			result, err := history.Report(t.Context(), batch)
			require.NoError(t, err)
			require.Equal(t, "accepted", result.Status)
			finishHostHistoryProjection(t, repo)
			report, err := repo.ListSessionUsage(t.Context(), analyticsmodels.SessionUsageFilter{WorkspaceID: "metric-ws", GroupBy: "model"})
			require.NoError(t, err)
			require.Equal(t, analyticsmodels.Int64(300), report.Totals.TotalTokens)
			require.Equal(t, analyticsmodels.Int64(30000), report.Totals.CostSubcents)
			rows, state, err := host.Usage().ListCanonical(t.Context(), pluginsdk.SessionUsageFilter{WorkspaceID: "metric-ws"}, pluginsdk.Page{Limit: 200})
			require.NoError(t, err)
			require.Equal(t, report.ProjectionGeneration, state.ProjectionGeneration)
			require.Len(t, rows, 2)
			var tokens, cost int64
			for _, row := range rows {
				require.False(t, row.TokenMetricAbsent)
				require.False(t, row.CostMetricAbsent)
				tokens, cost = tokens+*row.TotalTokens, cost+*row.CostSubcents
			}
			require.Equal(t, int64(300), tokens)
			require.Equal(t, int64(30000), cost)
		})
	}
}
