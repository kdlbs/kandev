package acp

import (
	"errors"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func (a *Adapter) promptErrorForTrace(err error) error {
	if a.agentID == cursorAgentID && cursorPromptStreamClosed(err) {
		return errors.New("cursor prompt stream closed")
	}
	return err
}

func (a *Adapter) sendCursorPromptStreamClosedFailure(
	sessionID string,
	promptGeneration uint64,
	turn *promptTurnState,
	disposition streams.PromptFailureDisposition,
	err error,
) bool {
	if a.agentID != cursorAgentID || disposition != streams.PromptFailureDispositionRetainRuntime ||
		!cursorPromptStreamClosed(err) {
		return false
	}
	a.sendCursorTransportFailure(
		sessionID,
		promptGeneration,
		a.continuationSafetySnapshot(turn),
		disposition,
		routingerr.CursorPromptStreamClosedMessage,
		time.Now().UTC(),
		false,
	)
	return true
}

func (a *Adapter) sendCursorTransportFailure(
	sessionID string,
	promptGeneration uint64,
	continuationSafety *streams.ContinuationSafetySnapshot,
	disposition streams.PromptFailureDisposition,
	safeMessage string,
	occurredAt time.Time,
	identityComplete bool,
) {
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	a.cancelAsyncTurnComplete(sessionID)
	a.sendUpdate(AgentEvent{
		Type:                     streams.EventTypeError,
		SessionID:                sessionID,
		PromptGeneration:         promptGeneration,
		Error:                    safeMessage,
		ContinuationSafety:       continuationSafety,
		PromptFailureDisposition: disposition,
		ProviderError: &streams.ProviderError{
			Source:                     streams.ProviderErrorSourceCursorACP,
			ProviderID:                 acpcompat.CursorAgentID,
			Message:                    safeMessage,
			OccurredAt:                 occurredAt,
			DiagnosticIdentityComplete: identityComplete,
		},
	})
}
