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

func (a *Adapter) retainableACPApplicationError(err error) (*streams.ProviderError, bool) {
	mockRetainedError := a.dialect.retainedApplicationErr != nil && a.dialect.retainedApplicationErr(err)
	if a.agentID != claudeAgentID && a.agentID != codexAgentID && !mockRetainedError {
		return nil, false
	}
	var requestErr *sdk.RequestError
	if !errors.As(err, &requestErr) || requestErr == nil || requestErr.Message == "" {
		return nil, false
	}
	classified := routingerr.Classify(routingerr.Input{
		Phase:      routingerr.PhasePromptSend,
		ProviderID: a.agentID,
		Stderr:     requestErr.Message,
	})
	if classified.Confidence != routingerr.ConfHigh || classified.UserAction {
		return nil, false
	}
	switch classified.Code {
	case routingerr.CodeProviderOverloaded, routingerr.CodeModelCapacity, routingerr.CodeRateLimited:
	default:
		return nil, false
	}
	providerError := ProviderErrorFromError(err, a.agentID, "")
	if providerError == nil || !providerError.Valid() {
		return nil, false
	}
	return providerError, true
}
