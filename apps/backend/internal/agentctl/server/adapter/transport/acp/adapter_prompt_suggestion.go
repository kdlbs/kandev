package acp

import (
	"encoding/json"
	"strings"

	acp "github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// claudeSDKMessageMethod is the extension notification the Claude ACP adapter
// uses to forward raw Claude Agent SDK messages selected by emitRawSDKMessages.
const claudeSDKMessageMethod = "_claude/sdkMessage"

const claudePromptSuggestionType = "prompt_suggestion"

// agentAdvertisesClaudeCode reports whether the initialize response carries
// the `_meta.claudeCode` namespace that Claude Code bridges advertise.
func agentAdvertisesClaudeCode(caps acp.AgentCapabilities) bool {
	return claudeCodeMeta(caps.Meta) != nil
}

// nativePromptSuggestions reports whether this session asks a Claude Code agent
// for native suggestions: the instance requested them and the agent advertised
// Claude Code on initialize.
func (a *Adapter) nativePromptSuggestions() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.PromptSuggestions && a.claudeCodeAgent
}

// promptSuggestionSessionMeta returns the session _meta that asks Claude Code to
// emit prompt suggestions and forward them, or nil when they were not negotiated.
func (a *Adapter) promptSuggestionSessionMeta() map[string]any {
	if !a.nativePromptSuggestions() {
		return nil
	}
	return map[string]any{
		"claudeCode": map[string]any{
			"options":            map[string]any{"promptSuggestions": true},
			"emitRawSDKMessages": []any{map[string]any{"type": claudePromptSuggestionType}},
		},
	}
}

// handleExtensionNotification dispatches inbound ACP extension notifications.
func (a *Adapter) handleExtensionNotification(method string, params json.RawMessage) {
	if method == claudeSDKMessageMethod {
		a.handleClaudeSDKMessage(params)
	}
}

type claudeSDKMessageParams struct {
	SessionID string `json:"sessionId"`
	Message   struct {
		Type       string `json:"type"`
		Suggestion string `json:"suggestion"`
	} `json:"message"`
}

// handleClaudeSDKMessage turns a forwarded prompt_suggestion into a stream
// event bound to its originating prompt generation. It is dropped for another
// session, or when its originating turn cannot be established.
func (a *Adapter) handleClaudeSDKMessage(params json.RawMessage) {
	if !a.nativePromptSuggestions() {
		return
	}
	var msg claudeSDKMessageParams
	if err := json.Unmarshal(params, &msg); err != nil || msg.Message.Type != claudePromptSuggestionType {
		return
	}
	text := strings.TrimSpace(msg.Message.Suggestion)
	if text == "" {
		return
	}
	a.mu.Lock()
	sessionID := a.sessionID
	a.mu.Unlock()
	if sessionID == "" || msg.SessionID != sessionID {
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
		Text:             text,
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
