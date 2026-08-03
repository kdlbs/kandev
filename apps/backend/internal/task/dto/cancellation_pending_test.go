package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeCancellationPendingProvider struct {
	pending bool
}

func (p fakeCancellationPendingProvider) CancellationPending(string) bool {
	return p.pending
}

type fakeCancellationSnapshotProvider struct {
	pending  bool
	revision uint64
}

func (p fakeCancellationSnapshotProvider) CancellationPending(string) bool {
	return p.pending
}

func (p fakeCancellationSnapshotProvider) CancellationPendingSnapshot(string) (bool, uint64) {
	return p.pending, p.revision
}

func TestTaskSessionCancellationPendingSerializesExplicitFalse(t *testing.T) {
	for name, value := range map[string]any{
		"full":    TaskSessionDTO{},
		"summary": TaskSessionSummaryDTO{},
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(value)
			require.NoError(t, err)

			var body map[string]any
			require.NoError(t, json.Unmarshal(encoded, &body))
			require.Contains(t, body, "cancellation_pending")
			require.Equal(t, false, body["cancellation_pending"])
			require.Contains(t, body, "cancellation_revision")
			require.Equal(t, float64(0), body["cancellation_revision"])
		})
	}
}

func TestEnrichCancellationPendingWritesProviderValue(t *testing.T) {
	for _, pending := range []bool{true, false} {
		t.Run(map[bool]string{true: "pending", false: "settled"}[pending], func(t *testing.T) {
			full := &TaskSessionDTO{ID: "s1", CancellationPending: !pending}
			EnrichCancellationPending(full, fakeCancellationPendingProvider{pending: pending})
			require.Equal(t, pending, full.CancellationPending)

			summary := &TaskSessionSummaryDTO{ID: "s1", CancellationPending: !pending}
			EnrichCancellationPendingSummary(summary, fakeCancellationPendingProvider{pending: pending})
			require.Equal(t, pending, summary.CancellationPending)
		})
	}
}

func TestEnrichCancellationPendingWritesRevisionWithSnapshot(t *testing.T) {
	full := &TaskSessionDTO{ID: "s1"}
	summary := &TaskSessionSummaryDTO{ID: "s1"}
	provider := fakeCancellationSnapshotProvider{pending: true, revision: 9}

	EnrichCancellationPending(full, provider)
	EnrichCancellationPendingSummary(summary, provider)

	require.True(t, full.CancellationPending)
	require.Equal(t, uint64(9), full.CancellationRevision)
	require.True(t, summary.CancellationPending)
	require.Equal(t, uint64(9), summary.CancellationRevision)
}
