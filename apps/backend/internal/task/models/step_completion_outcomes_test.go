package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestStepCompletionReceiptJSONRoundTrip covers the source-specific completion contract.
func TestStepCompletionReceiptJSONRoundTrip(t *testing.T) {
	entry := int64(7)
	signal := PendingStepCompletionSignal{
		SignalID:                 "signal-1",
		SourceTurnID:             "turn-2",
		WorkflowStepTransitionID: &entry,
		StepID:                   "step-build",
		SignaledAt:               time.Now().UTC(),
	}
	receipt := NewStepCompletionReceipt("task", "session", signal, signal.SignaledAt)
	require.Equal(t, StepCompletionOutcomePending, receipt.Outcome)
	require.True(t, receipt.MatchesSource(signal))

	raw, err := json.Marshal(map[string]interface{}{
		SessionMetaKeyLastStepCompletionReceipt: receipt,
	})
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &decoded))

	loaded, ok := LoadStepCompletionReceipt(decoded)
	require.True(t, ok)
	require.Equal(t, receipt.SignalID, loaded.SignalID)
	require.Equal(t, receipt.SourceTurnID, loaded.SourceTurnID)
	require.Equal(t, receipt.Outcome, loaded.Outcome)
	require.True(t, loaded.SameEntry(&entry))
}

// TestStepCompletionReceiptProof covers the source-specific completion contract.
func TestStepCompletionReceiptProof(t *testing.T) {
	entry := int64(3)
	receipt := NewStepCompletionReceipt("task", "session", PendingStepCompletionSignal{
		SignalID: "signal-1", StepID: "step-build", WorkflowStepTransitionID: &entry,
	}, time.Now().UTC())
	require.False(t, receipt.HasProof())

	committed := receipt.WithTransition(StepCompletionTransition{LedgerID: 9, WorkflowID: "wf", StepID: "step-review"})
	require.True(t, committed.HasProof())
	require.Equal(t, StepCompletionOutcomeTransitionApplied, committed.Outcome)

	queued := receipt.WithTransition(StepCompletionTransition{LedgerID: 9, StepID: "step-review", Queued: true})
	require.Equal(t, StepCompletionOutcomeDestinationQueued, queued.Outcome)
	require.True(t, queued.HasProof())
}

// TestLoadStepCompletionReceiptRejectsIncomplete covers the source-specific completion contract.
func TestLoadStepCompletionReceiptRejectsIncomplete(t *testing.T) {
	for name, value := range map[string]interface{}{
		"missing signal": map[string]interface{}{"step_id": "step", "outcome": StepCompletionOutcomePending},
		"missing step":   map[string]interface{}{"signal_id": "signal", "outcome": StepCompletionOutcomePending},
		"missing outcome": map[string]interface{}{
			"signal_id": "signal", "step_id": "step",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, ok := LoadStepCompletionReceipt(map[string]interface{}{
				SessionMetaKeyLastStepCompletionReceipt: value,
			})
			require.False(t, ok)
		})
	}
}
