package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestWithPromptLifecycleLockRejectsRemovedOrReplacedExecution(t *testing.T) {
	for _, replace := range []bool{false, true} {
		name := "removed"
		if replace {
			name = "replaced"
		}
		t.Run(name, func(t *testing.T) {
			store := NewExecutionStore()
			original := newLockOrderTestExecution()
			require.NoError(t, store.Add(original))
			original.promptLifecycleMu.Lock()
			held := true
			release := func() {
				if held {
					held = false
					original.promptLifecycleMu.Unlock()
				}
			}
			t.Cleanup(release)
			done := make(chan error, 1)
			called := false
			go func() {
				done <- store.WithPromptLifecycleLock(original.ID, func(*AgentExecution) { called = true })
			}()
			require.Eventually(t, func() bool {
				return goroutineParkedIn("(*ExecutionStore).WithPromptLifecycleLock", "sync.(*Mutex).Lock")
			}, lockOrderDeadlockGuard, time.Millisecond, "helper must finish its lookup before removal")
			store.Remove(original.ID)
			if replace {
				require.NoError(t, store.Add(newLockOrderTestExecution()))
			}
			release()
			select {
			case err := <-done:
				require.ErrorIs(t, err, ErrExecutionNotFound)
			case <-time.After(lockOrderDeadlockGuard):
				t.Fatal("helper did not release after execution removal")
			}
			require.False(t, called, "stale registration must not reach callback")
			require.True(t, original.promptLifecycleMu.TryLock(), "old prompt lock must be released")
			original.promptLifecycleMu.Unlock()
			require.ErrorIs(t, store.WithPromptLifecycleLock("missing", func(*AgentExecution) {
				t.Fatal("missing execution reached callback")
			}), ErrExecutionNotFound)
			if replace {
				require.NoError(t, store.WithPromptLifecycleLock(original.ID, func(current *AgentExecution) {
					require.NotSame(t, original, current)
					current.promptTurnID = "current-turn"
				}))
			}
		})
	}
}

func TestPrepareRecoveredAvailabilityPreservesReplacement(t *testing.T) {
	mgr := newTestManager(t)
	original := newLockOrderTestExecution()
	require.NoError(t, mgr.executionStore.Add(original))
	mgr.executionStore.Remove(original.ID)
	replacement := newLockOrderTestExecution()
	replacement.setPromptTurnID("replacement-turn")
	replacement.recoveredPromptGenerationPending.Store(true)
	require.NoError(t, mgr.executionStore.Add(replacement))

	payload, err := mgr.prepareRecoveredAvailability(context.Background(), original)
	require.NoError(t, err)
	require.Nil(t, payload, "a stale caller must not publish replacement availability")
	require.Equal(t, v1.AgentStatusRunning, replacement.Status)
	require.True(t, replacement.recoveredPromptGenerationPending.Load())
	require.Equal(t, "replacement-turn", replacement.promptTurnIDSnapshot())
	require.Empty(t, mgr.eventBus.(*MockEventBus).PublishedEvents)

	payload, err = mgr.prepareRecoveredAvailability(context.Background(), replacement)
	require.NoError(t, err)
	require.NotNil(t, payload, "the current caller must still prepare availability")
	require.Equal(t, string(v1.AgentStatusReady), payload.Status)
	require.Equal(t, "replacement-turn", payload.TurnID)
	require.True(t, payload.RecoveredAvailabilityOnly)
	require.False(t, replacement.recoveredPromptGenerationPending.Load())
}
