package sqlite

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/analytics/models"
)

func TestUsageHistoryNativeAccountingReasoningMapping(t *testing.T) {
	cases := []struct {
		name              string
		thought, subset   *int64
		want              *int64
		inside, supported bool
	}{
		{name: "unknown", supported: true},
		{name: "ordinary zero", thought: models.Int64(0), want: models.Int64(0), supported: true},
		{name: "ordinary thought", thought: models.Int64(3), want: models.Int64(3), supported: true},
		{name: "known subset zero", thought: models.Int64(0), subset: models.Int64(0), want: models.Int64(0), inside: true, supported: true},
		{name: "codex subset", thought: models.Int64(0), subset: models.Int64(5), want: models.Int64(5), inside: true, supported: true},
		{name: "subset without thought", subset: models.Int64(5), want: models.Int64(5), inside: true, supported: true},
		{name: "ordinary thought with empty subset", thought: models.Int64(3), subset: models.Int64(0), want: models.Int64(3), supported: true},
		{name: "unsupported mixed relationship", thought: models.Int64(3), subset: models.Int64(5)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nativeUsageReasoningForPublic(tc.thought, tc.subset)
			require.Equal(t, tc.want, got.Tokens)
			require.Equal(t, tc.inside, got.InsideOutput)
			require.Equal(t, tc.supported, got.RelationshipKnown)
		})
	}
}

func TestUsageHistoryNativeAccountingCanonicalGuardIsVersioned(t *testing.T) {
	fact := models.UsageCanonicalFact{
		Measurement: models.SessionUsageMeasurement{
			Source: usageMixedValue, Turns: models.Int64(1), ReasoningTokens: models.Int64(0), ReasoningOutputTokens: models.Int64(5),
			OutputTokens: models.Int64(10), TotalTokens: models.Int64(20), SourceTotalTokens: models.Int64(20),
			CostSubcents: models.Int64(5), TokenSemanticsVersion: 1,
		},
		TokenSource: &models.UsageMetricSource{Source: usageSourceNative},
		CostSource:  &models.UsageMetricSource{Source: "plugin:cost-only"},
	}
	version := normalizeNativeCanonicalAccounting(&fact, 0)
	require.Equal(t, nativeUsageAccountingRepairVersion, version)
	require.Nil(t, fact.Measurement.Turns)
	require.EqualValues(t, 5, *fact.Measurement.ReasoningTokens)
	require.True(t, fact.Measurement.ReasoningInsideOutput)
	require.EqualValues(t, 10, *fact.Measurement.OutputTokens)
	require.EqualValues(t, 20, *fact.Measurement.TotalTokens)
	require.EqualValues(t, 20, *fact.Measurement.SourceTotalTokens)
	require.EqualValues(t, 5, *fact.Measurement.CostSubcents)
	first := fact
	version = normalizeNativeCanonicalAccounting(&fact, version)
	require.Equal(t, nativeUsageAccountingRepairVersion, version)
	require.Equal(t, first, fact, "stage/read must not reinterpret the derived subset as a raw thought amount")

	host := first
	host.Measurement.Turns = models.Int64(7)
	host.TokenSource = &models.UsageMetricSource{Source: nativeTurnsHostSource}
	host.CostSource = &models.UsageMetricSource{Source: usageSourceNative}
	before := host
	require.Zero(t, normalizeNativeCanonicalAccounting(&host, 0))
	require.Equal(t, before, host, "cost provenance cannot erase a legitimate Host token/turn amount")
}

func TestUsageHistoryNativeAccountingRepairFindsAlreadyClearedTurns(t *testing.T) {
	f := newNativeTurnsFixture(t, false, 4)
	finishNativeTurnsBackfill(t, f.repo)
	_, err := f.db.Exec(`UPDATE session_usage_buckets SET turns=NULL, native_accounting_version=0 WHERE source='native'`)
	require.NoError(t, err)
	require.Equal(t, 4, nativeTurnsPending(t, f.db), "a prior turn repair must not hide the reasoning repair")
	head := nativeTurnsHead(t, f.db)
	done, err := f.repo.repairNativeUsageAccountingBatch(t.Context(), 200)
	require.NoError(t, err)
	require.True(t, done)
	assertNativeAccountingMutations(t, f.repo, head, 4)
	require.Zero(t, nativeTurnsPending(t, f.db))
}
