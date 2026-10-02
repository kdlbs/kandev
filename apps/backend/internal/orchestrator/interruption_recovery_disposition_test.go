package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-003.2
func TestInterruptionRecoveryDisposition_NoRetryDoesNotClaimExhaustion(t *testing.T) {
	for _, evidence := range []struct {
		name   string
		known  bool
		output bool
		effect bool
	}{
		{name: "missing_evidence"},
		{name: "assistant_output", known: true, output: true},
		{name: "tool_work", known: true, effect: true},
	} {
		t.Run(evidence.name, func(t *testing.T) {
			svc, mc := newTransientTestService(t)
			t.Cleanup(svc.cancelAllTransientRetries)
			if evidence.known {
				armTransientPromptEvidence(svc)
			}
			data := watcher.AgentEventData{
				TaskID: "t1", SessionID: "s1", AgentID: "cursor-acp",
				AgentExecutionID: "execution-1", PromptGeneration: 7,
				ErrorMessage:   cursorRetriableConnectionStalled,
				FailureDetails: "HTTP/2 stream closed with error code CANCEL (0x8)",
				EvidenceKnown:  evidence.known, OutputObserved: evidence.output, EffectObserved: evidence.effect,
			}
			require.False(t, svc.handleTransientFailure(context.Background(), data))
			_, scheduled := svc.transientRetries.Load("s1")
			require.False(t, scheduled)
			require.NoError(t, svc.createRecoveryStatusMessage(context.Background(), data, ""))
			require.Len(t, mc.sessionMessages, 1)
			message := mc.sessionMessages[0]
			require.NotContains(t, message.content, "after several retries")
			require.Equal(t, "provider_interrupted", message.metadata["failure_kind"])
			require.Equal(t, true, message.metadata["recovery_actions"])
			require.NotEmpty(t, message.metadata["error_output"])
		})
	}
}
