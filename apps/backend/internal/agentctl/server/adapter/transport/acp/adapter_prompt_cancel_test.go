package acp

import (
	"context"
	"errors"
	"testing"
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

func TestWaitForPromptRPCAfterUserCancel_CompletesAfterAbort(t *testing.T) {
	a := newTestAdapter()
	turn := &promptTurnState{
		endTurn: func(error) {},
		rpcDone: make(chan struct{}),
		abortCh: make(chan struct{}),
	}
	a.promptTurn = turn

	// Abort fires first, RPC completes before the inner timeout. Both channels
	// are closed up front so the inner select picks rpcDone deterministically
	// without needing a sleep-based scheduling delay.
	close(turn.abortCh)
	close(turn.rpcDone)

	if err := a.waitForPromptRPCAfterUserCancel(turn); err != nil {
		t.Fatalf("expected nil after rpc completed, got %v", err)
	}
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
