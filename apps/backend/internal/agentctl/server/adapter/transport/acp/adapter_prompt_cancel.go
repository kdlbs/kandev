package acp

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

func (a *Adapter) registerPromptTurn(parent context.Context) (context.Context, *promptTurnState) {
	promptCtx, endTurn := context.WithCancelCause(parent)
	turn := &promptTurnState{
		endTurn: endTurn,
		rpcDone: make(chan struct{}),
		abortCh: make(chan struct{}),
	}
	a.promptTurnMu.Lock()
	a.promptTurn = turn
	a.promptTurnMu.Unlock()
	return promptCtx, turn
}

func (a *Adapter) clearPromptTurn(turn *promptTurnState) {
	a.promptTurnMu.Lock()
	if a.promptTurn == turn {
		a.promptTurn = nil
	}
	a.promptTurnMu.Unlock()
}

func (a *Adapter) currentPromptTurn() *promptTurnState {
	a.promptTurnMu.Lock()
	defer a.promptTurnMu.Unlock()
	return a.promptTurn
}

// signalPromptTurnAbort is called from Cancel to interrupt the in-flight RPC.
func (a *Adapter) signalPromptTurnAbort() *promptTurnState {
	turn := a.currentPromptTurn()
	if turn == nil {
		return nil
	}
	select {
	case <-turn.abortCh:
	default:
		close(turn.abortCh)
	}
	if turn.endTurn != nil {
		turn.endTurn(ErrTurnCancelNotAcknowledged)
	}
	return turn
}

func waitForPromptRPCAfterCancel(turn *promptTurnState) error {
	if turn == nil {
		return nil
	}
	select {
	case <-turn.rpcDone:
		return nil
	case <-time.After(promptCancelJoinTimeout):
		return fmt.Errorf("%w: in-flight session/prompt did not end within %s",
			ErrTurnCancelNotAcknowledged, promptCancelJoinTimeout)
	}
}

// waitForPromptRPCAfterUserCancel blocks until the in-flight session/prompt RPC
// finishes. If the user cancels while this RPC is running, it waits briefly for
// the agent to stop; otherwise it abandons the RPC so the prompt gate is released.
func (a *Adapter) waitForPromptRPCAfterUserCancel(turn *promptTurnState) error {
	if turn == nil {
		return nil
	}
	select {
	case <-turn.rpcDone:
		return nil
	case <-turn.abortCh:
		select {
		case <-turn.rpcDone:
			return nil
		case <-time.After(promptCancelJoinTimeout):
			a.logger.Warn("in-flight session/prompt did not end after cancel; releasing prompt gate",
				zap.Duration("timeout", promptCancelJoinTimeout))
			return errPromptAbandonedAfterCancel
		}
	}
}
