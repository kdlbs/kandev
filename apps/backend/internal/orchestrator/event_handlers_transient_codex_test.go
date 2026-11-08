package orchestrator

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/stretchr/testify/require"
)

const codexRetryExhausted429 = "exceeded retry limit, last status: 429 Too Many Requests"

type fixedCodexRetryClock struct{ now time.Time }

func (c fixedCodexRetryClock) Now() time.Time { return c.now }

func (fixedCodexRetryClock) WaitUntil(ctx context.Context, _ time.Time) bool {
	<-ctx.Done()
	return false
}

// @covers AC-PLATFORM-RATE-THROTTLE-RETRY-001.1 AC-PLATFORM-RATE-THROTTLE-RETRY-001.2 AC-PLATFORM-RATE-THROTTLE-RETRY-001.3 AC-PLATFORM-RATE-THROTTLE-RETRY-001.4
func TestCodexRateLimitRetryEpisode(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

	t.Run("classifies and admits the bounded episode", func(t *testing.T) {
		svc, mc := newTransientTestService(t)
		svc.config.RetryClock = fixedCodexRetryClock{now: now}
		t.Cleanup(svc.cancelAllTransientRetries)

		failure := watcher.AgentEventData{
			TaskID:       "t1",
			SessionID:    "s1",
			AgentID:      "codex-acp",
			ErrorMessage: codexRetryExhausted429,
		}
		classified := classifyKanbanFailure(failure)
		require.Equal(t, routingerr.CodeRateLimited, classified.Code)
		require.Equal(t, routingerr.ConfHigh, classified.Confidence)
		require.Nil(t, classified.ResetHint)

		wantDelays := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second}
		for index, wantDelay := range wantDelays {
			attempt := index + 1
			executionID := "execution-" + strconv.Itoa(attempt)
			generation := uint64(attempt + 6)
			svc.beginPromptAttempt("s1", executionID, generation, false)
			failure.AgentExecutionID = executionID
			failure.PromptGeneration = generation
			require.True(t, svc.handleTransientFailure(context.Background(), failure), "attempt %d should be admitted", attempt)

			value, ok := svc.transientRetries.Load("s1")
			require.True(t, ok)
			entry := value.(*transientRetryEntry)
			require.Equal(t, attempt, entry.attempt)
			require.Equal(t, now.Add(wantDelay), entry.retryAt)
			require.NotEmpty(t, mc.sessionMessages)
			latest := mc.sessionMessages[len(mc.sessionMessages)-1]
			require.Equal(t, attempt, latest.metadata["attempt"])
			require.Equal(t, int(wantDelay.Seconds()), latest.metadata["retry_in_seconds"])
		}

		svc.beginPromptAttempt("s1", "execution-6", 12, false)
		failure.AgentExecutionID = "execution-6"
		failure.PromptGeneration = 12
		require.False(t, svc.handleTransientFailure(context.Background(), failure), "the sixth failure must exhaust the episode")
		_, active := svc.transientRetries.Load("s1")
		require.False(t, active)
	})

	t.Run("cancellation stops the Codex retry owner", func(t *testing.T) {
		svc, mc := newTransientTestService(t)
		svc.config.RetryClock = fixedCodexRetryClock{now: now}
		t.Cleanup(svc.cancelAllTransientRetries)
		svc.beginPromptAttempt("s1", "execution-1", 7, false)
		require.True(t, svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
			TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", AgentID: "codex-acp",
			PromptGeneration: 7, ErrorMessage: codexRetryExhausted429,
		}))

		require.True(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))
		_, active := svc.transientRetries.Load("s1")
		require.False(t, active)
		require.Equal(t, "cancelled", mc.sessionMessages[len(mc.sessionMessages)-1].metadata["recovery_disposition"])
	})

	t.Run("completed output with unresolved tool effects is not replayed", func(t *testing.T) {
		svc, _ := newTransientTestService(t)
		t.Cleanup(svc.cancelAllTransientRetries)
		svc.beginPromptAttempt("s1", "execution-1", 7, false)
		svc.observePromptAttempt("s1", "execution-1", 7, true, true)
		require.False(t, svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
			TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", AgentID: "codex-acp",
			PromptGeneration: 7, EvidenceKnown: true, OutputObserved: true, EffectObserved: true,
			ErrorMessage: codexRetryExhausted429,
		}))
		_, active := svc.transientRetries.Load("s1")
		require.False(t, active)
	})

	t.Run("stale and uncertain terminal evidence is not replayed", func(t *testing.T) {
		t.Run("stale generation", func(t *testing.T) {
			svc, _ := newTransientTestService(t)
			t.Cleanup(svc.cancelAllTransientRetries)
			svc.beginPromptAttempt("s1", "current-execution", 8, false)
			require.False(t, svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
				TaskID: "t1", SessionID: "s1", AgentExecutionID: "stale-execution", AgentID: "codex-acp",
				PromptGeneration: 7, ErrorMessage: codexRetryExhausted429,
			}))
		})

		t.Run("uncertain dispatch", func(t *testing.T) {
			svc, _ := newTransientTestService(t)
			t.Cleanup(svc.cancelAllTransientRetries)
			require.False(t, svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
				TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", AgentID: "codex-acp",
				PromptGeneration: 7, DeliveryOutcome: "uncertain", ErrorMessage: codexRetryExhausted429,
			}))
		})
	})
}
