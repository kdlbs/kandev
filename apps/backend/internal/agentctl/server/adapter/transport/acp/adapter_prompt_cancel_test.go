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

	go func() {
		time.Sleep(10 * time.Millisecond)
		close(turn.rpcDone)
	}()
	close(turn.abortCh)

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
