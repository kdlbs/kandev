package lifecycle

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func pendingPromptExecution() *AgentExecution {
	execution := &AgentExecution{
		ID:           "execution-pending",
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	execution.dispatchedPromptPending.Store(true)
	return execution
}

func TestWaitForPendingDispatchedPrompt_TimesOutWithoutClearingGate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		execution := pendingPromptExecution()
		result := make(chan error, 1)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go func() {
			result <- waitForPendingDispatchedPrompt(ctx, execution)
		}()

		time.Sleep(10 * time.Second)
		synctest.Wait()
		select {
		case err := <-result:
			var timeoutErr *PendingDispatchedPromptTimeoutError
			if !errors.As(err, &timeoutErr) {
				t.Fatalf("error = %v, want PendingDispatchedPromptTimeoutError", err)
			}
			if timeoutErr.ExecutionID != execution.ID {
				t.Fatalf("timeout execution ID = %q, want %q", timeoutErr.ExecutionID, execution.ID)
			}
			if !execution.dispatchedPromptPending.Load() {
				t.Fatal("timeout cleared the pending prompt gate")
			}
		default:
			t.Fatal("pending prompt wait did not return at its timeout")
		}
	})
}

func TestWaitForPendingDispatchedPrompt_ReturnsCallerCancellationWithoutClearingGate(t *testing.T) {
	execution := pendingPromptExecution()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForPendingDispatchedPrompt(ctx, execution)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v, want context canceled", err)
	}
	if !execution.dispatchedPromptPending.Load() {
		t.Fatal("caller cancellation cleared the pending prompt gate")
	}
}

func TestWaitForPendingDispatchedPrompt_ConsumesCompletionSignal(t *testing.T) {
	execution := pendingPromptExecution()
	execution.promptDoneCh <- PromptCompletionSignal{StopReason: "end_turn"}

	if err := waitForPendingDispatchedPrompt(context.Background(), execution); err != nil {
		t.Fatalf("waitForPendingDispatchedPrompt: %v", err)
	}
	if execution.dispatchedPromptPending.Load() {
		t.Fatal("completion signal did not clear the pending prompt gate")
	}
}
