package acp

import (
	"errors"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func (a *Adapter) promptFailureDisposition(
	conn *sdk.ClientSideConnection,
	sessionID string,
	turn *promptTurnState,
	promptGeneration uint64,
	notificationsDrained bool,
) streams.PromptFailureDisposition {
	if !notificationsDrained || !a.promptFailureTurnMatches(sessionID, turn, promptGeneration) {
		return ""
	}
	if !a.promptFailureConnectionUsable(conn, sessionID) {
		return ""
	}
	return streams.PromptFailureDispositionRetainRuntime
}

func (a *Adapter) promptFailureTurnMatches(
	sessionID string,
	turn *promptTurnState,
	promptGeneration uint64,
) bool {
	return sessionID != "" && turn != nil && promptGeneration != 0 &&
		turn.promptGeneration == promptGeneration && a.currentPromptTurn() == turn
}

func (a *Adapter) promptFailureConnectionUsable(conn *sdk.ClientSideConnection, sessionID string) bool {
	if conn == nil {
		return false
	}
	select {
	case <-conn.Done():
		return false
	default:
	}

	a.mu.RLock()
	usable := !a.closed && a.acpConn == conn && a.sessionID == sessionID &&
		a.agentInfo != nil && a.lifetimeCtx.Err() == nil
	a.mu.RUnlock()
	if !usable {
		return false
	}

	select {
	case <-conn.Done():
		return false
	default:
		return true
	}
}

// remotePromptRequestError reports the SDK request error when the peer itself
// returned it. The origin marker is the evidence boundary: local send, decode,
// deadline, and disconnect failures never satisfy it, whatever their message
// or Data says.
func remotePromptRequestError(err error) (*sdk.RequestError, bool) {
	var requestErr *sdk.RequestError
	if !errors.As(err, &requestErr) || !requestErr.IsRemoteResponse() {
		return nil, false
	}
	return requestErr, true
}

// retainedACPApplicationError projects a prompt error the peer itself returned.
func (a *Adapter) retainedACPApplicationError(err error) (*streams.ProviderError, bool) {
	if _, ok := remotePromptRequestError(err); !ok {
		return nil, false
	}
	providerID, modelID := a.ProviderErrorContext()
	providerError := ProviderErrorFromError(err, providerID, modelID)
	if providerError == nil || !providerError.Valid() {
		return nil, false
	}
	return providerError, true
}

// sendRetainedPromptError emits the retained-turn failure for a settled prompt
// error while leaving the process, connection, and native session intact.
func (a *Adapter) sendRetainedPromptError(
	sessionID string,
	promptGeneration uint64,
	turn *promptTurnState,
	notificationsDrained bool,
	disposition streams.PromptFailureDisposition,
	providerError *streams.ProviderError,
) {
	var capacityContinuation *streams.CapacityContinuationSnapshot
	if classified := routingerr.Classify(routingerr.Input{
		Phase: routingerr.PhasePromptSend, ProviderID: a.agentID, Stderr: providerError.Message,
	}); classified.Code == routingerr.CodeModelCapacity {
		capacityContinuation = a.capacityContinuationSnapshot(turn, notificationsDrained)
	}
	a.cancelAsyncTurnComplete(sessionID)
	a.sendUpdate(AgentEvent{
		Type:                     streams.EventTypeError,
		SessionID:                sessionID,
		PromptGeneration:         promptGeneration,
		Error:                    providerError.Message,
		ProviderError:            providerError,
		CapacityContinuation:     capacityContinuation,
		PromptFailureDisposition: disposition,
	})
}
