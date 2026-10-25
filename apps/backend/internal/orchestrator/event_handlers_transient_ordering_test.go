package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/stretchr/testify/require"
)

type blockingTransientRetryTurnService struct {
	fakeOrderingTurnService
	completeStarted chan struct{}
	releaseComplete chan struct{}
}

func (s *blockingTransientRetryTurnService) CompleteTurn(ctx context.Context, turnID string) error {
	close(s.completeStarted)
	<-s.releaseComplete
	return s.fakeOrderingTurnService.CompleteTurn(ctx, turnID)
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.25
func TestHandleTransientFailure_ArmsRetryAfterTurnParking(t *testing.T) {
	svc, _ := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	turns := &blockingTransientRetryTurnService{
		fakeOrderingTurnService: fakeOrderingTurnService{
			bus:    &recordingEventBus{},
			turnID: "turn-1",
		},
		completeStarted: make(chan struct{}),
		releaseComplete: make(chan struct{}),
	}
	svc.SetTurnService(turns)
	armTransientPromptEvidence(svc)

	done := make(chan bool, 1)
	go func() {
		done <- svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "execution-1",
			PromptGeneration: 7,
			ErrorMessage:     overloaded529,
		})
	}()

	select {
	case <-turns.completeStarted:
	case <-time.After(time.Second):
		t.Fatal("turn completion did not start")
	}

	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	entry.mu.Lock()
	armedWhileTurnOpen := entry.armed
	entry.mu.Unlock()
	require.False(t, armedWhileTurnOpen, "retry timer armed before the failed turn was parked")

	close(turns.releaseComplete)
	require.True(t, <-done)
	entry.mu.Lock()
	armedAfterParking := entry.armed
	entry.mu.Unlock()
	require.True(t, armedAfterParking)
}
