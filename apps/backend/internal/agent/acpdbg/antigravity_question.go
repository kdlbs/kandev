package acpdbg

import (
	"errors"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

// methodSessionRequestPermission is the ACP request an agent uses to ask for
// an approval or, for Antigravity, a native question.
const methodSessionRequestPermission = "session/request_permission"

// ErrHumanQuestionPending reports that a capture stopped because the agent
// asked a question only a person can answer. The capture deliberately writes
// no response: selecting any offered option would invent the user's answer.
var ErrHumanQuestionPending = errors.New("agent is waiting for a human answer")

// isHumanQuestionFrame reports whether frame is a registered Antigravity
// native human question. Recognition is limited to the exact agent identity
// and the provider's tool-call prefix, so a synthetic --exec id and every
// other agent keep the normal diagnostic auto-reply.
func isHumanQuestionFrame(agentID string, frame Frame) bool {
	if frame.Method() != methodSessionRequestPermission {
		return false
	}
	params, ok := frame["params"].(map[string]any)
	if !ok {
		return false
	}
	toolCall, ok := params["toolCall"].(map[string]any)
	if !ok {
		return false
	}
	toolCallID, _ := toolCall["toolCallId"].(string)
	return acpcompat.IsHumanInputPermission(agentID, toolCallID)
}
