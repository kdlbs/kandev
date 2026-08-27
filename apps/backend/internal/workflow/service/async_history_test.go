package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/workflow/models"
)

// TestEnqueueStepTransition_PersistsThroughQueue exercises the actual async
// path (EnqueueStepTransition -> runHistoryWriter -> CreateStepTransition)
// rather than calling CreateStepTransition directly, so a regression in the
// queue/writer plumbing itself would be caught here.
func TestEnqueueStepTransition_PersistsThroughQueue(t *testing.T) {
	svc, _ := setupTestService(t)
	actorID := "user-1"

	svc.EnqueueStepTransition("sess-async-1", "step-a", "step-b", models.StepTransitionTriggerManual, &actorID, nil)

	require.Eventually(t, func() bool {
		history, err := svc.ListHistoryBySession(context.Background(), "sess-async-1")
		return err == nil && len(history) == 1
	}, 2*time.Second, 5*time.Millisecond, "enqueued transition was never persisted")

	history, err := svc.ListHistoryBySession(context.Background(), "sess-async-1")
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, "step-b", history[0].ToStepID)
	require.Equal(t, models.StepTransitionTriggerManual, history[0].Trigger)
}

// TestEnqueueStepTransition_PreservesFIFOOrder proves the single writer
// goroutine drains the queue in enqueue order — the history rows must read
// back as a single monotonic step chain.
func TestEnqueueStepTransition_PreservesFIFOOrder(t *testing.T) {
	svc, _ := setupTestService(t)
	const steps = 20

	for i := 0; i < steps; i++ {
		from := stepName(i)
		to := stepName(i + 1)
		svc.EnqueueStepTransition("sess-async-fifo", from, to, models.StepTransitionTriggerManual, nil, nil)
	}

	require.Eventually(t, func() bool {
		history, err := svc.ListHistoryBySession(context.Background(), "sess-async-fifo")
		return err == nil && len(history) == steps
	}, 2*time.Second, 5*time.Millisecond, "not all enqueued transitions were persisted")

	history, err := svc.ListHistoryBySession(context.Background(), "sess-async-fifo")
	require.NoError(t, err)
	require.Len(t, history, steps)
	for i, row := range history {
		require.Equal(t, stepName(i+1), row.ToStepID, "row %d out of FIFO order", i)
	}
}

func stepName(i int) string {
	return "step-" + string(rune('a'+i))
}

// TestEnqueueStepTransition_QueueFullDropsWithoutBlocking builds the Service
// directly with a one-slot queue and no writer goroutine draining it, so the
// second enqueue call must hit the full-queue branch and return immediately
// rather than block the caller.
func TestEnqueueStepTransition_QueueFullDropsWithoutBlocking(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	svc := &Service{
		logger:       log,
		historyQueue: make(chan historyWrite, 1),
	}

	svc.EnqueueStepTransition("sess-full-1", "", "step-a", models.StepTransitionTriggerManual, nil, nil)
	require.Len(t, svc.historyQueue, 1)

	done := make(chan struct{})
	go func() {
		svc.EnqueueStepTransition("sess-full-2", "", "step-b", models.StepTransitionTriggerManual, nil, nil)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("EnqueueStepTransition blocked on a full queue instead of dropping")
	}
	require.Len(t, svc.historyQueue, 1, "queue-full enqueue must be dropped, not buffered")
}

// TestEnqueueStepTransition_RejectsAfterClose covers the enqueue/Close race
// the maintainer flagged: once Close has returned, the writer goroutine is
// gone, so anything still landing in historyQueue would sit there forever.
// EnqueueStepTransition must reject rather than silently queue it.
func TestEnqueueStepTransition_RejectsAfterClose(t *testing.T) {
	svc, _ := setupTestService(t)
	require.NoError(t, svc.Close())

	svc.EnqueueStepTransition("sess-after-close", "", "step-a", models.StepTransitionTriggerManual, nil, nil)

	require.Empty(t, svc.historyQueue, "enqueue after Close must be dropped, not silently queued")
}

// TestEnqueueStepTransition_ConcurrentWithClose_NoRace races many concurrent
// enqueues against Close under go test -race to prove the historyMu guard
// actually serializes the check-and-send with the closed-flag flip.
func TestEnqueueStepTransition_ConcurrentWithClose_NoRace(t *testing.T) {
	svc, _ := setupTestService(t)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			svc.EnqueueStepTransition("sess-race", "", "step", models.StepTransitionTriggerManual, nil, nil)
		}
	}()

	require.NoError(t, svc.Close())
	wg.Wait()
}
