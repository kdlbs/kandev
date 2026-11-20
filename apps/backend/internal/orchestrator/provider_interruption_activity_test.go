package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

const continuationActivityMetaKey = "recovery_activity_confirmed"

const (
	acceptedContinuationExecutionID = "execution-2"
	acceptedContinuationGeneration  = uint64(9)
)

func acceptedContinuationFixture(t *testing.T) (*Service, *transientRetryEntry) {
	t.Helper()
	svc, _, data := continuationFailureFixture(t)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	entry := currentTransientRetryEntry(t, svc)
	entry.mu.Lock()
	entry.started = 1
	entry.mu.Unlock()
	svc.beginPromptAttempt("s1", acceptedContinuationExecutionID, acceptedContinuationGeneration, false)
	svc.recordContinuationAcceptance("s1", entry)
	return svc, entry
}

type continuationActivityPayloadFunc func(executionID string, generation uint64) *lifecycle.AgentStreamEventPayload

func continuationActivityPayload(
	eventType, executionID string,
	generation uint64,
	data lifecycle.AgentStreamEventData,
) *lifecycle.AgentStreamEventPayload {
	data.Type = eventType
	data.PromptGeneration = generation
	return &lifecycle.AgentStreamEventPayload{
		TaskID: "t1", SessionID: "s1", ExecutionID: executionID, Data: &data,
	}
}

func acceptedContinuationIdentity(t *testing.T, entry *transientRetryEntry) (string, uint64) {
	t.Helper()
	entry.mu.Lock()
	defer entry.mu.Unlock()
	require.NotEmpty(t, entry.acceptedExecution)
	require.NotZero(t, entry.acceptedGeneration)
	return entry.acceptedExecution, entry.acceptedGeneration
}

func latestRetryStatus(t *testing.T, svc *Service) mockSessionMessage {
	t.Helper()
	mc := svc.messageCreator.(*mockMessageCreator)
	mc.mu.Lock()
	defer mc.mu.Unlock()
	for index := len(mc.sessionMessages) - 1; index >= 0; index-- {
		message := mc.sessionMessages[index]
		if message.messageType == "status" && message.metadata["retrying"] == true {
			return message
		}
	}
	t.Fatal("no retry status message was persisted")
	return mockSessionMessage{}
}

func requireConfirmedContinuationStatus(t *testing.T, message mockSessionMessage) {
	t.Helper()
	require.Equal(t, true, message.metadata[continuationActivityMetaKey])
	require.Equal(t, "default", message.metadata["variant"])
	require.Equal(t, "Agent resumed", message.content)
	require.Equal(t, recoveryModeContinue, message.metadata["recovery_mode"])
	require.Equal(t, "continuing", message.metadata["recovery_phase"])
	require.NotContains(t, message.metadata, "retry_in_seconds")
	require.NotContains(t, message.metadata, "retry_at")
	actions, ok := message.metadata["actions"].([]map[string]interface{})
	require.True(t, ok)
	require.NotNil(t, actionByTestID(actions, recoveryCancelRetryButtonTestID))
}

func requireWarningContinuationStatus(t *testing.T, message mockSessionMessage) {
	t.Helper()
	require.NotEqual(t, true, message.metadata[continuationActivityMetaKey])
	require.Equal(t, metaVariantWarning, message.metadata["variant"])
	require.NotEmpty(t, message.metadata["failure_code"])
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-003.5
// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-003.6
func TestInterruptionContinuationActivityConfirmsAcceptedPrompt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event continuationActivityPayloadFunc
	}{
		{"assistant text", func(executionID string, generation uint64) *lifecycle.AgentStreamEventPayload {
			return continuationActivityPayload(streams.EventTypeMessageChunk, executionID, generation,
				lifecycle.AgentStreamEventData{Text: "resumed work"})
		}},
		{"reasoning", func(executionID string, generation uint64) *lifecycle.AgentStreamEventPayload {
			return continuationActivityPayload(streams.EventTypeReasoning, executionID, generation,
				lifecycle.AgentStreamEventData{Text: "considering the next step"})
		}},
		{"foreground tool call", func(executionID string, generation uint64) *lifecycle.AgentStreamEventPayload {
			return continuationActivityPayload(agentEventToolCall, executionID, generation,
				lifecycle.AgentStreamEventData{ToolCallID: "resumed-tool", ToolTitle: "Inspect fixture"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, entry := acceptedContinuationFixture(t)
			executionID, generation := acceptedContinuationIdentity(t, entry)
			requireWarningContinuationStatus(t, latestRetryStatus(t, svc))

			svc.handleAgentStreamEvent(context.Background(), tc.event(executionID, generation))

			requireConfirmedContinuationStatus(t, latestRetryStatus(t, svc))
		})
	}
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-003.6
// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-003.7
func TestInterruptionContinuationActivityRejectsStaleEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Service, *lifecycle.AgentStreamEventPayload, string, uint64)
		event  continuationActivityPayloadFunc
	}{
		{
			name: "previous execution",
			event: func(_ string, generation uint64) *lifecycle.AgentStreamEventPayload {
				return continuationActivityPayload(streams.EventTypeMessageChunk, "execution-1", generation,
					lifecycle.AgentStreamEventData{Text: "stale output"})
			},
		},
		{
			name: "previous prompt generation",
			event: func(executionID string, generation uint64) *lifecycle.AgentStreamEventPayload {
				return continuationActivityPayload(streams.EventTypeMessageChunk, executionID, generation-1,
					lifecycle.AgentStreamEventData{Text: "stale output"})
			},
		},
		{
			name: "missing prompt evidence",
			mutate: func(svc *Service, _ *lifecycle.AgentStreamEventPayload, _ string, _ uint64) {
				svc.dynamicAttemptEvidence.Delete("s1")
			},
			event: func(executionID string, generation uint64) *lifecycle.AgentStreamEventPayload {
				return continuationActivityPayload(streams.EventTypeMessageChunk, executionID, generation,
					lifecycle.AgentStreamEventData{Text: "unattributed output"})
			},
		},
		{
			name: "user text",
			event: func(executionID string, generation uint64) *lifecycle.AgentStreamEventPayload {
				return continuationActivityPayload(streams.EventTypeMessageChunk, executionID, generation,
					lifecycle.AgentStreamEventData{Text: "continue", Role: "user"})
			},
		},
		{
			name: "provider diagnostic",
			event: func(executionID string, generation uint64) *lifecycle.AgentStreamEventPayload {
				return continuationActivityPayload(streams.EventTypeMessageChunk, executionID, generation,
					lifecycle.AgentStreamEventData{Text: "rate limited", ProviderDiagnosticCandidate: true})
			},
		},
		{
			name: "empty chunk",
			event: func(executionID string, generation uint64) *lifecycle.AgentStreamEventPayload {
				return continuationActivityPayload(streams.EventTypeReasoning, executionID, generation,
					lifecycle.AgentStreamEventData{Text: "   "})
			},
		},
		{
			name: "nested tool frame",
			event: func(executionID string, generation uint64) *lifecycle.AgentStreamEventPayload {
				return continuationActivityPayload(agentEventToolCall, executionID, generation,
					lifecycle.AgentStreamEventData{ToolCallID: "child-tool", ParentToolCallID: "parent-tool"})
			},
		},
		{
			name: "background tool frame",
			event: func(executionID string, generation uint64) *lifecycle.AgentStreamEventPayload {
				return continuationActivityPayload(agentEventToolCall, executionID, generation,
					lifecycle.AgentStreamEventData{
						ToolCallID: "background-tool",
						Normalized: attestedBackgroundShellPayload("sleep 5"),
					})
			},
		},
		{
			name: "terminal tool call without live ownership",
			event: func(executionID string, generation uint64) *lifecycle.AgentStreamEventPayload {
				return continuationActivityPayload(agentEventToolCall, executionID, generation,
					lifecycle.AgentStreamEventData{
						ToolCallID: "terminal-tool",
						ToolStatus: agentEventComplete,
					})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, entry := acceptedContinuationFixture(t)
			executionID, generation := acceptedContinuationIdentity(t, entry)
			payload := tc.event(executionID, generation)
			if tc.mutate != nil {
				tc.mutate(svc, payload, executionID, generation)
			}

			svc.handleAgentStreamEvent(context.Background(), payload)

			requireWarningContinuationStatus(t, latestRetryStatus(t, svc))
		})
	}
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-003.7
func TestInterruptionContinuationActivityRejectsInheritedPendingToolResult(t *testing.T) {
	svc, entry := acceptedContinuationFixture(t)
	executionID, generation := acceptedContinuationIdentity(t, entry)
	// The predecessor prompt started this tool before the interruption. Its
	// late result carries the new prompt generation but was never created by
	// the accepted continuation.
	svc.recordToolOwnership("s1", "inherited-tool", executionID, toolOwnershipForeground)

	svc.handleAgentStreamEvent(context.Background(), continuationActivityPayload(
		agentEventToolUpdate, executionID, generation,
		lifecycle.AgentStreamEventData{ToolCallID: "inherited-tool", ToolStatus: agentEventComplete},
	))
	requireWarningContinuationStatus(t, latestRetryStatus(t, svc))

	svc.handleAgentStreamEvent(context.Background(), continuationActivityPayload(
		agentEventToolCall, executionID, generation,
		lifecycle.AgentStreamEventData{ToolCallID: "fresh-tool", ToolTitle: "Fresh work"},
	))
	requireConfirmedContinuationStatus(t, latestRetryStatus(t, svc))
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-003.5
// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-003.7
func TestInterruptionContinuationActivityPublicationOrdering(t *testing.T) {
	t.Run("activity before acceptance reconciles at acceptance", func(t *testing.T) {
		svc, _, data := continuationFailureFixture(t)
		require.True(t, svc.handleTransientFailure(context.Background(), data))
		entry := currentTransientRetryEntry(t, svc)
		svc.beginPromptAttempt("s1", "execution-2", 9, false)

		svc.handleAgentStreamEvent(context.Background(), continuationActivityPayload(
			streams.EventTypeMessageChunk, "execution-2", 9,
			lifecycle.AgentStreamEventData{Text: "resumed before acceptance"},
		))
		requireWarningContinuationStatus(t, latestRetryStatus(t, svc))

		svc.recordContinuationAcceptance("s1", entry)
		requireConfirmedContinuationStatus(t, latestRetryStatus(t, svc))
	})

	t.Run("late continuing phase write preserves confirmation", func(t *testing.T) {
		svc, entry := acceptedContinuationFixture(t)
		executionID, generation := acceptedContinuationIdentity(t, entry)
		svc.handleAgentStreamEvent(context.Background(), continuationActivityPayload(
			streams.EventTypeMessageChunk, executionID, generation,
			lifecycle.AgentStreamEventData{Text: "resumed work"},
		))
		requireConfirmedContinuationStatus(t, latestRetryStatus(t, svc))

		svc.updateContinuationPhase(context.Background(), "t1", "s1", entry, "continuing")
		message := latestRetryStatus(t, svc)
		requireConfirmedContinuationStatus(t, message)
		require.Equal(t, 1, message.metadata["attempts_started"])
	})

	t.Run("a failed publication retries on later activity", func(t *testing.T) {
		svc, entry := acceptedContinuationFixture(t)
		executionID, generation := acceptedContinuationIdentity(t, entry)
		mc := svc.messageCreator.(*mockMessageCreator)
		mc.mu.Lock()
		mc.sessionMessageErr = errors.New("storage unavailable")
		mc.mu.Unlock()

		svc.handleAgentStreamEvent(context.Background(), continuationActivityPayload(
			streams.EventTypeMessageChunk, executionID, generation,
			lifecycle.AgentStreamEventData{Text: "resumed work"},
		))
		requireWarningContinuationStatus(t, latestRetryStatus(t, svc))

		mc.mu.Lock()
		mc.sessionMessageErr = nil
		mc.mu.Unlock()
		svc.handleAgentStreamEvent(context.Background(), continuationActivityPayload(
			streams.EventTypeMessageChunk, executionID, generation,
			lifecycle.AgentStreamEventData{Text: "resumed work continued"},
		))
		requireConfirmedContinuationStatus(t, latestRetryStatus(t, svc))
	})

	t.Run("cancellation retires the confirmed notice", func(t *testing.T) {
		svc, entry := acceptedContinuationFixture(t)
		executionID, generation := acceptedContinuationIdentity(t, entry)
		svc.handleAgentStreamEvent(context.Background(), continuationActivityPayload(
			streams.EventTypeMessageChunk, executionID, generation,
			lifecycle.AgentStreamEventData{Text: "resumed work"},
		))
		requireConfirmedContinuationStatus(t, latestRetryStatus(t, svc))

		require.True(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))
		_, owned := svc.transientRetries.Load("s1")
		require.False(t, owned)
		require.True(t, hasRecoveryDisposition(t, svc, stopReasonCancelled))
	})
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1
// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-002.2
// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-003.7
func TestInterruptionContinuationActivityPreservesEpisodeBudget(t *testing.T) {
	svc, entry := acceptedContinuationFixture(t)
	executionID, generation := acceptedContinuationIdentity(t, entry)
	svc.handleAgentStreamEvent(context.Background(), continuationActivityPayload(
		streams.EventTypeMessageChunk, executionID, generation,
		lifecycle.AgentStreamEventData{Text: "resumed work"},
	))
	requireConfirmedContinuationStatus(t, latestRetryStatus(t, svc))

	data := watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", OwnerKind: "task", AgentID: "cursor-acp",
		AgentExecutionID: executionID, PromptGeneration: generation,
		ErrorMessage: cursorRetriableConnectionStalled, EvidenceKnown: true,
		OutputObserved: true, EffectObserved: true,
		ContinuationSafety: &streams.ContinuationSafetySnapshot{
			Support: streams.ContinuationNativeSavedHistoryV1, Known: true,
			PromptGeneration: generation, CompletedReads: 1,
		},
	}
	require.True(t, svc.handleTransientFailure(context.Background(), data))

	naive := latestRetryStatus(t, svc)
	requireWarningContinuationStatus(t, naive)
	require.Equal(t, 2, naive.metadata["attempt"])
	require.Equal(t, 10, naive.metadata["retry_in_seconds"], "the second attempt keeps the paced backoff ladder")
	require.Equal(t, 1, naive.metadata["attempts_started"])
	replacement := currentTransientRetryEntry(t, svc)
	require.NotSame(t, entry, replacement, "a later failure owns exactly one fresh retry entry")

	// A late frame from the previous accepted prompt cannot downgrade the new
	// warning, because the replacement entry carries no accepted identity.
	svc.handleAgentStreamEvent(context.Background(), continuationActivityPayload(
		streams.EventTypeMessageChunk, executionID, generation,
		lifecycle.AgentStreamEventData{Text: "late old output"},
	))
	requireWarningContinuationStatus(t, latestRetryStatus(t, svc))
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1
// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-003.5
func TestInterruptionContinuationActivityPreservesCapacityMetadata(t *testing.T) {
	svc, _, _, data, entry := startCapacityContinuationEpisode(t)
	entry.mu.Lock()
	entry.attempt = 6
	entry.started = 5
	windowEndsAt := entry.policy.windowEndsAt
	entry.mu.Unlock()
	require.False(t, windowEndsAt.IsZero(), "capacity episodes carry a bounded retry window")

	svc.beginPromptAttempt("s1", "execution-1", 7, false)
	svc.recordContinuationAcceptance("s1", entry)
	svc.handleAgentStreamEvent(context.Background(), continuationActivityPayload(
		streams.EventTypeMessageChunk, "execution-1", 7,
		lifecycle.AgentStreamEventData{Text: "resumed capacity work"},
	))

	message := latestRetryStatus(t, svc)
	requireConfirmedContinuationStatus(t, message)
	require.Equal(t, 6, message.metadata["attempt"])
	require.Equal(t, 5, message.metadata["attempts_started"])
	require.Equal(t, capacityMaxAttempts, message.metadata["max_attempts"])
	require.Equal(t, windowEndsAt.UTC().Format(time.RFC3339Nano), message.metadata["retry_window_ends_at"])
	require.NotContains(t, message.metadata, "retry_at")
	require.NotContains(t, message.metadata, "retry_in_seconds")

	svc.updateContinuationPhase(context.Background(), "t1", "s1", entry, "continuing")
	message = latestRetryStatus(t, svc)
	requireConfirmedContinuationStatus(t, message)
	require.Equal(t, 6, message.metadata["attempt"])
	require.Equal(t, 5, message.metadata["attempts_started"])
	require.Equal(t, capacityMaxAttempts, message.metadata["max_attempts"])
	require.Equal(t, windowEndsAt.UTC().Format(time.RFC3339Nano), message.metadata["retry_window_ends_at"])

	data.PromptGeneration = 7
	data.CapacityContinuation.PromptGeneration = 7
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	next := latestRetryStatus(t, svc)
	require.NotEqual(t, true, next.metadata[continuationActivityMetaKey])
	require.Equal(t, 7, next.metadata["attempt"])
	require.Equal(t, 5, next.metadata["attempts_started"])
	require.Equal(t, capacityMaxAttempts, next.metadata["max_attempts"])
	require.Equal(t, windowEndsAt.UTC().Format(time.RFC3339Nano), next.metadata["retry_window_ends_at"])
}

func currentTransientRetryEntry(t *testing.T, svc *Service) *transientRetryEntry {
	t.Helper()
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry, ok := value.(*transientRetryEntry)
	require.True(t, ok)
	return entry
}

func hasRecoveryDisposition(t *testing.T, svc *Service, disposition string) bool {
	t.Helper()
	mc := svc.messageCreator.(*mockMessageCreator)
	mc.mu.Lock()
	defer mc.mu.Unlock()
	for _, message := range mc.sessionMessages {
		if message.metadata["recovery_disposition"] == disposition {
			return true
		}
	}
	return false
}
