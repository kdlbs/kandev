package acp

import (
	"encoding/json"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

// claudeSDKMessageMethod is the extension notification the Claude ACP adapter
// uses to forward raw Claude Agent SDK messages selected by emitRawSDKMessages.
const claudeSDKMessageMethod = "_claude/sdkMessage"

const claudePromptSuggestionType = "prompt_suggestion"

// claudePromptSuggestionFeatureKey is the Claude bridge extension declaring
// native next-prompt suggestion support. Neither inspected upstream bridge
// release advertises it; it is a Kandev opt-in contract, not standard ACP.
const claudePromptSuggestionFeatureKey = "promptSuggestions"

// Verified compatibility evidence for the exact official bridge Kandev pins.
// The pair identifies the implementation whose options/raw-notification contract
// was inspected; it is a compatibility assertion, not attestation.
const (
	verifiedClaudeSuggestionBridgeName    = "@agentclientprotocol/claude-agent-acp"
	verifiedClaudeSuggestionBridgeVersion = "0.81.2"
)

// claudePromptSuggestionEvidence is the bounded debug reason for the support
// decision. It is never sent to consumers.
type claudePromptSuggestionEvidence string

const (
	claudeSuggestionEvidenceExplicit       claudePromptSuggestionEvidence = "explicit"
	claudeSuggestionEvidenceVerifiedLegacy claudePromptSuggestionEvidence = "verified_legacy"
	claudeSuggestionEvidenceUnsupported    claudePromptSuggestionEvidence = "unsupported"
)

// claudePromptSuggestionSupport decides whether an initialize response supplies
// enough evidence that the connected integration implements native next-prompt
// suggestions. The configured agent name and the selected model never
// participate.
//
// A literal `_meta.claudeCode.promptSuggestions` declaration wins. Literal true
// asserts the options/raw-notification contract; literal false or a malformed
// present value denies support without a compatibility fallback. Only when the
// declaration is absent does the verified pinned bridge tuple qualify.
func claudePromptSuggestionSupport(
	agentName, agentVersion string,
	capabilities acp.AgentCapabilities,
) (bool, claudePromptSuggestionEvidence) {
	claudeCode := claudeCodeMeta(capabilities.Meta)
	if claudeCode != nil {
		if raw, present := claudeCode[claudePromptSuggestionFeatureKey]; present {
			supported, ok := raw.(bool)
			if !ok || !supported {
				return false, claudeSuggestionEvidenceUnsupported
			}
			return true, claudeSuggestionEvidenceExplicit
		}
	}
	if verifiedClaudeSuggestionBridge(agentName, agentVersion, claudeCode) {
		return true, claudeSuggestionEvidenceVerifiedLegacy
	}
	return false, claudeSuggestionEvidenceUnsupported
}

// verifiedClaudeSuggestionBridge matches the exact inspected official bridge
// identity/version and its observed well-formed metadata shape. Queueing remains
// an independent contract, so its advertisement supplies no suggestion
// authority outside this tuple.
func verifiedClaudeSuggestionBridge(agentName, agentVersion string, claudeCode map[string]any) bool {
	if agentName != verifiedClaudeSuggestionBridgeName || agentVersion != verifiedClaudeSuggestionBridgeVersion {
		return false
	}
	if claudeCode == nil {
		return false
	}
	queueing, ok := claudeCode["promptQueueing"].(bool)
	return ok && queueing
}

// claudePromptSuggestionSessionMeta builds the session _meta for a supported
// integration. A supported but unrequested session receives an explicit disable
// instruction and no raw-message filter, so a warm provider session cannot keep
// generating suggestions. Unsupported integrations receive no configuration.
func claudePromptSuggestionSessionMeta(supported, requested bool) map[string]any {
	if !supported {
		return nil
	}
	claudeCode := map[string]any{
		"options": map[string]any{"promptSuggestions": requested},
	}
	if requested {
		claudeCode["emitRawSDKMessages"] = []any{map[string]any{"type": claudePromptSuggestionType}}
	}
	return map[string]any{"claudeCode": claudeCode}
}

type claudeSDKMessageParams struct {
	SessionID string `json:"sessionId"`
	Message   struct {
		Type       string `json:"type"`
		Suggestion string `json:"suggestion"`
	} `json:"message"`
}

// claudePromptSuggestionNotification is a normalized forwarded suggestion.
type claudePromptSuggestionNotification struct {
	SessionID string
	Text      string
}

// decodeClaudePromptSuggestion normalizes a `_claude/sdkMessage` payload. It
// rejects other message types, malformed payloads, and blank suggestions.
func decodeClaudePromptSuggestion(params json.RawMessage) (claudePromptSuggestionNotification, bool) {
	var msg claudeSDKMessageParams
	if err := json.Unmarshal(params, &msg); err != nil || msg.Message.Type != claudePromptSuggestionType {
		return claudePromptSuggestionNotification{}, false
	}
	text := strings.TrimSpace(msg.Message.Suggestion)
	if text == "" {
		return claudePromptSuggestionNotification{}, false
	}
	return claudePromptSuggestionNotification{SessionID: msg.SessionID, Text: text}, true
}
