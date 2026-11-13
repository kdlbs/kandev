package acp

import (
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const quotaNoticeBufferBytes = 1024

// relativeQuotaNoticeRule states its reset as a relative duration that only
// parses once the sentence is complete.
const relativeQuotaNoticeRule = "provider.quota_notice.v1"

// observeUsageLimitNotice examines the start of this turn's assistant output;
// agent prose that merely quotes a provider error must remain ordinary output.
// It reports whether text is a fragment of a recognized notice that had not
// been delivered before this chunk, so the caller can mark that fragment as
// provider diagnostic rather than ordinary output. Text after the notice was
// delivered is never marked.
func (a *Adapter) observeUsageLimitNotice(sessionID string, promptGeneration uint64, text string) bool {
	a.mu.RLock()
	current := a.sessionID == sessionID && a.quotaNoticeAmbiguousSessionID != sessionID
	a.mu.RUnlock()
	if a.agentID == "" || text == "" || sessionID == "" || !current {
		return false
	}
	turn := a.currentPromptTurn()
	if turn == nil || promptGeneration == 0 || turn.promptGeneration != promptGeneration {
		return false
	}
	buffer, classified := turn.classifyQuotaNotice(text, a.agentID)
	if !actionableQuotaNotice(classified) {
		return false
	}
	if classified.ClassifierRule == relativeQuotaNoticeRule && classified.ResetHint == nil {
		// The notice opens before its reset sentence ends: a chunk can stop
		// after "1 hour" while ", 30 minutes." is still in flight. The waiter
		// settles the prompt on the first notice, so it is held until a later
		// chunk completes the duration and the reset hint is known.
		return true
	}
	message := streams.SanitizeProviderMessage(buffer)
	if message == "" {
		return false
	}
	turn.markQuotaNoticeDelivered()
	select {
	case turn.providerErrorCh <- openCodeStderrDiagnostic{SessionID: sessionID, ProviderError: streams.ProviderError{Source: streams.ProviderErrorSourceAgentMessage, ProviderID: a.agentID, Message: message, DiagnosticIdentityComplete: streams.IsCompleteProviderDiagnostic(buffer), OccurredAt: time.Now().UTC(), ResetAt: classified.ResetHint}}:
	default:
	}
	return true
}

func (turn *promptTurnState) markQuotaNoticeDelivered() {
	turn.evidenceMu.Lock()
	defer turn.evidenceMu.Unlock()
	turn.quotaNoticeDelivered = true
}

func actionableQuotaNotice(classified *routingerr.Error) bool {
	return classified != nil && classified.Confidence == routingerr.ConfHigh && classified.FallbackAllowed &&
		(classified.Code == routingerr.CodeQuotaLimited || classified.Code == routingerr.CodeRateLimited)
}

func (turn *promptTurnState) classifyQuotaNotice(text, agentID string) (string, *routingerr.Error) {
	turn.evidenceMu.Lock()
	defer turn.evidenceMu.Unlock()
	if turn.quotaNoticeOwnerAmbiguous || turn.quotaNoticeDelivered {
		return "", nil
	}
	if len(turn.quotaNoticeText) < quotaNoticeBufferBytes {
		turn.quotaNoticeText += text
		if len(turn.quotaNoticeText) > quotaNoticeBufferBytes {
			turn.quotaNoticeText = turn.quotaNoticeText[:quotaNoticeBufferBytes]
		}
	}
	buffer := turn.quotaNoticeText
	if turn.quotaNoticeClassified != buffer {
		turn.quotaNoticeClassified = buffer
		turn.quotaNoticeClass = routingerr.ClassifyAgentNotice(routingerr.Input{Phase: routingerr.PhasePromptSend, ProviderID: agentID, Stderr: buffer})
	}
	return buffer, turn.quotaNoticeClass
}

func (a *Adapter) markQuotaNoticeSessionAmbiguous(sessionID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if sessionID != "" && a.sessionID == sessionID {
		a.quotaNoticeAmbiguousSessionID = sessionID
	}
}
