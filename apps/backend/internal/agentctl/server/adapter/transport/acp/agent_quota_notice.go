package acp

import (
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const quotaNoticeBufferBytes = 1024

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
	buffer, classified, occurredAt := turn.classifyQuotaNotice(text, a.agentID)
	if !actionableQuotaNotice(classified) {
		return false
	}
	if relativeQuotaNoticeContainsOutput(classified, buffer) {
		turn.closeQuotaNotice()
		return false
	}
	message := streams.SanitizeProviderMessage(buffer)
	diagnostic := openCodeStderrDiagnostic{SessionID: sessionID, ProviderError: streams.ProviderError{
		Source: streams.ProviderErrorSourceAgentMessage, ProviderID: a.agentID, Message: message,
		DiagnosticIdentityComplete: streams.IsCompleteProviderDiagnostic(buffer), OccurredAt: occurredAt, ResetAt: classified.ResetHint,
	}}
	if quotaNoticeAwaitingReset(classified) {
		// Hold a partial reset sentence until a later chunk completes it. A
		// prompt that returns before then settles from the held notice.
		turn.holdQuotaNotice(diagnostic)
		return true
	}
	if message == "" {
		return false
	}
	turn.deliverQuotaNotice(diagnostic, classified.ClassifierRule == routingerr.RuleRelativeQuotaNotice)
	return true
}

// relativeQuotaNoticeContainsOutput reports whether a relative notice carries
// assistant output after its reset phrase, punctuated or not.
func relativeQuotaNoticeContainsOutput(classified *routingerr.Error, text string) bool {
	return classified.ClassifierRule == routingerr.RuleRelativeQuotaNotice &&
		(routingerr.RelativeQuotaNoticeHasTrailingOutput(text) || !routingerr.IsRelativeQuotaNoticePrefix(text))
}

func quotaNoticeAwaitingReset(classified *routingerr.Error) bool {
	return classified.ClassifierRule == routingerr.RuleRelativeQuotaNotice && classified.ResetHint == nil
}

func (turn *promptTurnState) holdQuotaNotice(diagnostic openCodeStderrDiagnostic) {
	turn.evidenceMu.Lock()
	defer turn.evidenceMu.Unlock()
	if !turn.quotaNoticeDelivered {
		turn.pendingQuotaNotice = &diagnostic
	}
}

func (turn *promptTurnState) deliverQuotaNotice(diagnostic openCodeStderrDiagnostic, relative bool) {
	turn.evidenceMu.Lock()
	defer turn.evidenceMu.Unlock()
	if turn.quotaNoticeDelivered {
		return
	}
	turn.quotaNoticeDelivered = true
	turn.pendingQuotaNotice = nil
	if relative {
		turn.completedQuotaNotice = &diagnostic
	}
	select {
	case turn.providerErrorCh <- diagnostic:
	default:
	}
}

// closeQuotaNotice ends notice correlation for this turn and drops any held or
// completed notice, so later text cannot settle the prompt as a quota failure.
func (turn *promptTurnState) closeQuotaNotice() {
	turn.evidenceMu.Lock()
	defer turn.evidenceMu.Unlock()
	turn.quotaNoticeDelivered = true
	turn.pendingQuotaNotice = nil
	turn.completedQuotaNotice = nil
}

// quotaNoticeOnNormalPromptReturn settles a prompt that returned normally
// while its last assistant text was a relative quota notice. The notice may
// have been completed by the final drained chunk, or the stream may have ended
// on a whole duration without the sentence terminator.
func (turn *promptTurnState) quotaNoticeOnNormalPromptReturn(sessionID string) error {
	turn.evidenceMu.Lock()
	defer turn.evidenceMu.Unlock()
	if turn.quotaNoticeOwnerAmbiguous {
		return nil
	}
	if pending := turn.pendingQuotaNotice; pending != nil && pending.SessionID == sessionID {
		if resetAt := routingerr.RelativeQuotaNoticeResetAt(turn.quotaNoticeText, pending.ProviderError.OccurredAt); resetAt != nil {
			providerError := pending.ProviderError
			providerError.ResetAt = resetAt
			return &providerPromptError{ProviderError: providerError}
		}
	}
	if completed := turn.completedQuotaNotice; completed != nil && completed.SessionID == sessionID {
		return &providerPromptError{ProviderError: completed.ProviderError}
	}
	return nil
}

func actionableQuotaNotice(classified *routingerr.Error) bool {
	return classified != nil && classified.Confidence == routingerr.ConfHigh && classified.FallbackAllowed &&
		(classified.Code == routingerr.CodeQuotaLimited || classified.Code == routingerr.CodeRateLimited)
}

func (turn *promptTurnState) classifyQuotaNotice(text, agentID string) (string, *routingerr.Error, time.Time) {
	turn.evidenceMu.Lock()
	defer turn.evidenceMu.Unlock()
	if turn.quotaNoticeOwnerAmbiguous {
		return "", nil, time.Time{}
	}
	if turn.quotaNoticeDelivered {
		// Assistant text after a completed notice means the prompt produced
		// output, so the notice must no longer settle a normal return.
		if strings.TrimSpace(text) != "" {
			turn.completedQuotaNotice = nil
		}
		return "", nil, time.Time{}
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
		turn.quotaNoticeOccurredAt = time.Now().UTC()
		turn.quotaNoticeClass = routingerr.ClassifyAgentNotice(routingerr.Input{
			Phase: routingerr.PhasePromptSend, ProviderID: agentID, Stderr: buffer, OccurredAt: turn.quotaNoticeOccurredAt,
		})
	}
	return buffer, turn.quotaNoticeClass, turn.quotaNoticeOccurredAt
}

func (a *Adapter) markQuotaNoticeSessionAmbiguous(sessionID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if sessionID != "" && a.sessionID == sessionID {
		a.quotaNoticeAmbiguousSessionID = sessionID
	}
}
