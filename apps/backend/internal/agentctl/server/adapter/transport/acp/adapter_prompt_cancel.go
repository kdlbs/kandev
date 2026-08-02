package acp

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

func (a *Adapter) registerPromptTurn(
	parent context.Context,
	promptGeneration uint64,
) (context.Context, *promptTurnState) {
	promptCtx, turn := newPromptTurnState(parent, promptGeneration, false)
	a.promptTurnMu.Lock()
	a.promptTurn = turn
	a.promptTurnMu.Unlock()
	return promptCtx, turn
}

func newPromptTurnState(
	parent context.Context,
	promptGeneration uint64,
	allowHandoff bool,
) (context.Context, *promptTurnState) {
	promptCtx, endTurn := context.WithCancelCause(parent)
	turn := &promptTurnState{
		endTurn:          endTurn,
		rpcDone:          make(chan struct{}),
		abortCh:          make(chan struct{}),
		handoffCh:        make(chan struct{}),
		providerErrorCh:  make(chan openCodeStderrDiagnostic, 1),
		promptGeneration: promptGeneration,
		allowHandoff:     allowHandoff,
	}
	return promptCtx, turn
}

// acquirePromptTurn obtains prompt ownership. Human prompts may atomically
// inherit the existing gate token after an authoritative provider handoff.
// Synthetic prompts never listen to handoffCh, so ScheduleWakeup remains
// serialized until the owning RPC really returns.
func (a *Adapter) acquirePromptTurn(
	ctx context.Context,
	turn *promptTurnState,
	human bool,
) error {
	for {
		handoffCh, transferred := a.tryTransferPromptTurn(turn, human)
		if transferred {
			return nil
		}

		select {
		case a.promptGate <- struct{}{}:
			a.promptTurnMu.Lock()
			turn.gateOwned = true
			a.promptTurn = turn
			a.promptTurnMu.Unlock()
			return nil
		case <-handoffCh:
			// Re-check under promptTurnMu. Another human caller may have won the
			// transfer and installed a non-idle successor.
		case <-ctx.Done():
			return ctx.Err()
		case <-a.lifetimeCtx.Done():
			return context.Cause(a.lifetimeCtx)
		}
	}
}

func (a *Adapter) tryTransferPromptTurn(
	turn *promptTurnState,
	human bool,
) (<-chan struct{}, bool) {
	a.promptTurnMu.Lock()
	defer a.promptTurnMu.Unlock()
	current := a.promptTurn
	if !human || current == nil || !current.allowHandoff || current.finishing {
		return nil, false
	}
	if !current.handedOff || !current.gateOwned {
		return current.handoffCh, false
	}
	current.gateOwned = false
	turn.gateOwned = true
	a.promptTurn = turn
	return nil, true
}

// claimPromptTurnCompletion decides whether turn still owns prompt-end
// finalization. A predecessor whose gate token was transferred must not sweep
// tool state, consume usage, clear the successor trace, or emit completion.
func (a *Adapter) claimPromptTurnCompletion(turn *promptTurnState) bool {
	a.promptTurnMu.Lock()
	defer a.promptTurnMu.Unlock()
	if a.promptTurn != turn || !turn.gateOwned {
		return false
	}
	turn.finishing = true
	return true
}

// finishPromptTurn releases whichever turn still owns the physical gate token.
// Transferred predecessors are harmless no-ops.
func (a *Adapter) finishPromptTurn(turn *promptTurnState) {
	a.promptTurnMu.Lock()
	if a.promptTurn == turn {
		a.promptTurn = nil
	}
	releaseGate := turn.gateOwned
	turn.gateOwned = false
	a.promptTurnMu.Unlock()
	if releaseGate {
		<-a.promptGate
	}
}

// invalidatePromptTurnOwnership detaches the prompt captured before a session
// replacement began. A successor may inherit ownership while session/new or
// session/load is in flight; the identity guard must not drain that successor's
// gate. The old RPC may still return, but it no longer owns finalization.
func (a *Adapter) invalidatePromptTurnOwnership(turn *promptTurnState) {
	if turn == nil {
		return
	}
	a.promptTurnMu.Lock()
	if a.promptTurn != turn {
		a.promptTurnMu.Unlock()
		return
	}
	a.promptTurn = nil
	releaseGate := turn.gateOwned
	turn.gateOwned = false
	a.promptTurnMu.Unlock()
	if releaseGate {
		<-a.promptGate
	}
}

// markPromptHandoff records the generation-matched provider boundary and wakes
// waiting human callers. It deliberately leaves the physical gate token in
// place so synthetic wakeups cannot consume the handoff.
func (a *Adapter) markPromptHandoff(sessionID string, promptGeneration uint64) bool {
	if promptGeneration == 0 {
		return false
	}
	a.promptTurnMu.Lock()
	defer a.promptTurnMu.Unlock()
	turn := a.promptTurn
	if turn == nil ||
		turn.promptGeneration != promptGeneration ||
		!turn.allowHandoff ||
		!turn.gateOwned ||
		turn.finishing {
		return false
	}
	if !turn.handedOff {
		a.protectActiveBackgroundWorkForHandoff(sessionID)
		turn.handedOff = true
		close(turn.handoffCh)
	}
	return true
}

func (a *Adapter) currentPromptGeneration() uint64 {
	turn := a.currentPromptTurn()
	if turn == nil {
		return 0
	}
	return turn.promptGeneration
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

// signalPromptTurnAbort wakes the in-flight prompt waiter via abortCh. It does
// not cancel promptCtx here: for a compliant agent, conn.Cancel triggers the
// agent to close its session/prompt RPC normally with stop_reason=cancelled, and
// cancelling promptCtx pre-emptively would race that response and surface as
// context.Canceled. promptCtx is cancelled only on the join-timeout branches
// below, after the agent has had its chance to acknowledge.
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
		if turn.endTurn != nil {
			turn.endTurn(ErrTurnCancelNotAcknowledged)
		}
		return fmt.Errorf("%w: in-flight session/prompt did not end within %s",
			ErrTurnCancelNotAcknowledged, promptCancelJoinTimeout)
	}
}

// waitForPromptRPCAfterUserCancel blocks until the in-flight session/prompt RPC
// finishes or a correlated OpenCode provider diagnostic settles it. If the user
// cancels while this RPC is running, it waits briefly for the agent to stop;
// otherwise it abandons the RPC so the prompt gate is released.
func (a *Adapter) waitForPromptRPCAfterUserCancel(turn *promptTurnState, sessionID string) error {
	if turn == nil {
		return nil
	}
	for {
		select {
		case <-turn.rpcDone:
			return nil
		case diagnostic := <-turn.providerErrorCh:
			if sessionID == "" || diagnostic.SessionID != sessionID {
				continue
			}
			providerErr := &providerPromptError{ProviderError: diagnostic.ProviderError}
			if turn.endTurn != nil {
				turn.endTurn(providerErr)
			}
			select {
			case <-turn.rpcDone:
				return providerErr
			case <-time.After(promptCancelJoinTimeout):
				return providerErr
			}
		case <-turn.abortCh:
			select {
			case <-turn.rpcDone:
				return nil
			case <-time.After(promptCancelJoinTimeout):
				if turn.endTurn != nil {
					turn.endTurn(ErrTurnCancelNotAcknowledged)
				}
				a.logger.Warn("in-flight session/prompt did not end after cancel; releasing prompt gate",
					zap.Duration("timeout", promptCancelJoinTimeout))
				return errPromptAbandonedAfterCancel
			}
		}
	}
}
