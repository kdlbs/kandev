package acp

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"
)

func TestWaitForPromptRPCAfterCancel_Acknowledged(t *testing.T) {
	turn := &promptTurnState{rpcDone: make(chan struct{})}
	close(turn.rpcDone)

	if err := waitForPromptRPCAfterCancel(turn); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestWaitForPromptRPCAfterCancel_TimesOut(t *testing.T) {
	prev := promptCancelJoinTimeout
	promptCancelJoinTimeout = 20 * time.Millisecond
	t.Cleanup(func() { promptCancelJoinTimeout = prev })

	turn := &promptTurnState{rpcDone: make(chan struct{})}
	err := waitForPromptRPCAfterCancel(turn)
	if !errors.Is(err, ErrTurnCancelNotAcknowledged) {
		t.Fatalf("expected ErrTurnCancelNotAcknowledged, got %v", err)
	}
}

func TestWaitForPromptRPCAfterUserCancel_AbortReleasesWhenRPCStuck(t *testing.T) {
	prev := promptCancelJoinTimeout
	promptCancelJoinTimeout = 20 * time.Millisecond
	t.Cleanup(func() { promptCancelJoinTimeout = prev })

	a := newTestAdapter()
	turn := &promptTurnState{
		endTurn: func(error) {},
		rpcDone: make(chan struct{}),
		abortCh: make(chan struct{}),
	}
	a.promptTurn = turn
	close(turn.abortCh)

	err := a.waitForPromptRPCAfterUserCancel(turn)
	if !errors.Is(err, errPromptAbandonedAfterCancel) {
		t.Fatalf("expected errPromptAbandonedAfterCancel, got %v", err)
	}
}

func TestIsPromptAbandonedAfterCancel(t *testing.T) {
	if !IsPromptAbandonedAfterCancel(errPromptAbandonedAfterCancel) {
		t.Fatal("expected direct sentinel to match")
	}
	if !IsPromptAbandonedAfterCancel(fmt.Errorf("wrapped: %w", errPromptAbandonedAfterCancel)) {
		t.Fatal("expected wrapped sentinel to match")
	}
	if !IsPromptAbandonedAfterCancel(errors.New("prompt failed: prompt abandoned after cancel")) {
		t.Fatal("expected string-wrapped sentinel to match")
	}
	if IsPromptAbandonedAfterCancel(errors.New("different prompt failure")) {
		t.Fatal("unexpected match for unrelated error")
	}
}

func TestWaitForPromptRPCAfterUserCancel_CompletesAfterAbort(t *testing.T) {
	// synctest lets us deterministically exercise the abort path: close abortCh
	// first, run waitForPromptRPCAfterUserCancel in a goroutine, synctest.Wait
	// until it blocks inside the inner select on rpcDone, then close rpcDone.
	// Without synctest there's no way to deterministically distinguish the
	// "outer select picked abort, then inner picked rpcDone" path from the
	// "outer select picked rpcDone directly" path when both channels are ready.
	synctest.Test(t, func(t *testing.T) {
		a := newTestAdapter()
		defer func() { _ = a.Close() }() // drain the update worker before synctest exits
		turn := &promptTurnState{
			endTurn: func(error) {},
			rpcDone: make(chan struct{}),
			abortCh: make(chan struct{}),
		}
		a.promptTurn = turn
		close(turn.abortCh)

		done := make(chan error, 1)
		go func() {
			done <- a.waitForPromptRPCAfterUserCancel(turn)
		}()

		// Wait until the goroutine is blocked inside the inner select.
		synctest.Wait()
		close(turn.rpcDone)

		if err := <-done; err != nil {
			t.Fatalf("expected nil after rpc completed, got %v", err)
		}
	})
}

func TestRegisterPromptTurn_CancelCause(t *testing.T) {
	a := newTestAdapter()
	ctx, turn := a.registerPromptTurn(context.Background())
	defer a.clearPromptTurn(turn)

	turn.endTurn(ErrTurnCancelNotAcknowledged)
	if !errors.Is(context.Cause(ctx), ErrTurnCancelNotAcknowledged) {
		t.Fatalf("expected cancel cause on prompt ctx, got %v", context.Cause(ctx))
	}
}

// signalPromptTurnAbort must only wake the waiter via abortCh — it must NOT
// cancel promptCtx, because a compliant agent will close session/prompt
// naturally after receiving session/cancel and we don't want to race that
// response with a context.Canceled. promptCtx is cancelled only on the
// timeout branches of the waiters.
func TestSignalPromptTurnAbort_DoesNotCancelPromptCtx(t *testing.T) {
	a := newTestAdapter()
	ctx, turn := a.registerPromptTurn(context.Background())
	defer a.clearPromptTurn(turn)

	turn.rpcDone = make(chan struct{})
	turn.abortCh = make(chan struct{})

	got := a.signalPromptTurnAbort()
	if got != turn {
		t.Fatalf("expected signalPromptTurnAbort to return current turn")
	}
	select {
	case <-turn.abortCh:
	default:
		t.Fatalf("expected abortCh to be closed")
	}
	if context.Cause(ctx) != nil {
		t.Fatalf("expected promptCtx to be alive, got cause %v", context.Cause(ctx))
	}
}

func TestWaitForPromptRPCAfterUserCancel_CancelsPromptCtxOnTimeout(t *testing.T) {
	prev := promptCancelJoinTimeout
	promptCancelJoinTimeout = 20 * time.Millisecond
	t.Cleanup(func() { promptCancelJoinTimeout = prev })

	a := newTestAdapter()
	ctx, turn := a.registerPromptTurn(context.Background())
	defer a.clearPromptTurn(turn)
	turn.rpcDone = make(chan struct{})
	turn.abortCh = make(chan struct{})

	close(turn.abortCh)
	if err := a.waitForPromptRPCAfterUserCancel(turn); !errors.Is(err, errPromptAbandonedAfterCancel) {
		t.Fatalf("expected errPromptAbandonedAfterCancel, got %v", err)
	}
	if !errors.Is(context.Cause(ctx), ErrTurnCancelNotAcknowledged) {
		t.Fatalf("expected promptCtx cancelled with ErrTurnCancelNotAcknowledged on timeout, got %v",
			context.Cause(ctx))
	}
}

// Symmetric to TestWaitForPromptRPCAfterUserCancel_CancelsPromptCtxOnTimeout —
// guards against future edits dropping the endTurn call from
// waitForPromptRPCAfterCancel's timeout branch (the Cancel() caller-side path).
func TestWaitForPromptRPCAfterCancel_CancelsPromptCtxOnTimeout(t *testing.T) {
	prev := promptCancelJoinTimeout
	promptCancelJoinTimeout = 20 * time.Millisecond
	t.Cleanup(func() { promptCancelJoinTimeout = prev })

	a := newTestAdapter()
	ctx, turn := a.registerPromptTurn(context.Background())
	defer a.clearPromptTurn(turn)
	turn.rpcDone = make(chan struct{})

	if err := waitForPromptRPCAfterCancel(turn); !errors.Is(err, ErrTurnCancelNotAcknowledged) {
		t.Fatalf("expected ErrTurnCancelNotAcknowledged, got %v", err)
	}
	if !errors.Is(context.Cause(ctx), ErrTurnCancelNotAcknowledged) {
		t.Fatalf("expected promptCtx cancelled with ErrTurnCancelNotAcknowledged on timeout, got %v",
			context.Cause(ctx))
	}
}

func TestNormalizePromptErrorAfterCancel_MapsTimeoutCanceledRPCToAbandonedPrompt(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(ErrTurnCancelNotAcknowledged)

	err := normalizePromptErrorAfterCancel(ctx, context.Canceled)
	if !errors.Is(err, errPromptAbandonedAfterCancel) {
		t.Fatalf("expected errPromptAbandonedAfterCancel, got %v", err)
	}
}
