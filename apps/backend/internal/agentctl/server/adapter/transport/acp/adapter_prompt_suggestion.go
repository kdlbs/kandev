package acp

import (
	"encoding/json"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// promptSuggestionsEffective reports whether this session may emit native
// suggestions: the provider supports them and this launch asked for them.
func (a *Adapter) promptSuggestionsEffective() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.PromptSuggestions && a.promptSuggestionSupport
}

// promptSuggestionSessionMeta returns the session _meta the Claude integration
// requires for the negotiated support/request pair, or nil when unsupported.
func (a *Adapter) promptSuggestionSessionMeta() map[string]any {
	a.mu.Lock()
	supported := a.promptSuggestionSupport
	a.mu.Unlock()
	return claudePromptSuggestionSessionMeta(supported, a.cfg.PromptSuggestions)
}

// handleExtensionNotification dispatches inbound ACP extension notifications.
func (a *Adapter) handleExtensionNotification(method string, params json.RawMessage) {
	if method == claudeSDKMessageMethod {
		a.handleClaudeSDKMessage(params)
	}
}

// handleClaudeSDKMessage turns a forwarded prompt_suggestion into a stream
// event bound to its originating prompt generation. It is dropped when this
// session did not enable suggestions, for another session, or when its
// originating turn cannot be established.
func (a *Adapter) handleClaudeSDKMessage(params json.RawMessage) {
	if !a.promptSuggestionsEffective() {
		return
	}
	notification, ok := decodeClaudePromptSuggestion(params)
	if !ok {
		return
	}
	a.mu.Lock()
	sessionID := a.sessionID
	a.mu.Unlock()
	if sessionID == "" || notification.SessionID != sessionID {
		return
	}
	generation := a.claimPromptSuggestionOrigin()
	if generation == 0 {
		a.logger.Debug("dropping prompt suggestion without a trustworthy originating turn",
			zap.String("session_id", sessionID))
		return
	}
	a.sendUpdate(AgentEvent{
		Type:             streams.EventTypePromptSuggestion,
		SessionID:        sessionID,
		Text:             notification.Text,
		PromptGeneration: generation,
	})
}

// promptSuggestionTurn records whether a prompt was sent while an earlier
// prompt's suggestion could still arrive.
type promptSuggestionTurn struct {
	sent     bool
	overlaps bool
}

// promptSuggestionOrigin is the suggestion window of the most recently
// finished prompt. Claude Code's prompt_suggestion carries no turn reference,
// so a suggestion is attributed to the finished prompt whose window is open,
// and only when no other prompt could have produced it.
type promptSuggestionOrigin struct {
	open       bool
	generation uint64 // 0 when the finished prompt's origin is not trustworthy
}

// markPromptSentForSuggestion closes the previous prompt's window. A prompt
// sent while that window is still open overlaps it: a delayed suggestion from
// the earlier prompt could arrive after this one finishes.
func (a *Adapter) markPromptSentForSuggestion(turn *promptTurnState) {
	if turn == nil {
		return
	}
	a.promptTurnMu.Lock()
	defer a.promptTurnMu.Unlock()
	turn.suggestion = promptSuggestionTurn{sent: true, overlaps: a.suggestionOrigin.open}
	a.suggestionOrigin = promptSuggestionOrigin{}
}

// resetPromptSuggestionOrigin discards an open window only after a proven
// conversation replacement, and the captured prompt's suggestion bookkeeping
// while it still owns the prompt turn. A failed transition or a same-ID restore
// keeps the old conversation, so its window stays open to make the successor
// prompt ambiguous rather than clean.
func (a *Adapter) resetPromptSuggestionOrigin(priorTurn *promptTurnState) {
	a.promptTurnMu.Lock()
	defer a.promptTurnMu.Unlock()
	a.suggestionOrigin = promptSuggestionOrigin{}
	if priorTurn != nil && a.promptTurn == priorTurn {
		priorTurn.suggestion = promptSuggestionTurn{}
	}
}

// recordPromptFinishedForSuggestionLocked opens the finished prompt's window.
// A predecessor that finishes while a successor is in flight makes the
// successor's origin ambiguous instead. Callers hold promptTurnMu.
func (a *Adapter) recordPromptFinishedForSuggestionLocked(turn *promptTurnState) {
	if turn == nil || !turn.suggestion.sent {
		return
	}
	if a.promptTurn != turn {
		if a.promptTurn != nil {
			a.promptTurn.suggestion.overlaps = true
		}
		return
	}
	generation := turn.promptGeneration
	if turn.suggestion.overlaps {
		generation = 0
	}
	a.suggestionOrigin = promptSuggestionOrigin{open: true, generation: generation}
}

// claimPromptSuggestionOrigin returns the originating prompt generation for a
// suggestion arriving now, or 0 when it has none: a prompt is in flight, no
// window is open, or the window's origin is not trustworthy. A window yields at
// most one suggestion.
func (a *Adapter) claimPromptSuggestionOrigin() uint64 {
	a.promptTurnMu.Lock()
	defer a.promptTurnMu.Unlock()
	if a.promptTurn != nil || !a.suggestionOrigin.open {
		return 0
	}
	generation := a.suggestionOrigin.generation
	a.suggestionOrigin = promptSuggestionOrigin{}
	return generation
}
