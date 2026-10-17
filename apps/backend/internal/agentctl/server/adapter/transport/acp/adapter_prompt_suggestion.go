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
// event. A suggestion is only valid for the turn that just completed, so it is
// dropped when another prompt is already in flight or the session changed.
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
	if a.currentPromptTurn() != nil {
		a.logger.Debug("dropping prompt suggestion for a superseded turn", zap.String("session_id", sessionID))
		return
	}
	a.sendUpdate(AgentEvent{Type: streams.EventTypePromptSuggestion, SessionID: sessionID, Text: text})
}
