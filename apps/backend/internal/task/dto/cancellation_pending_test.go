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
