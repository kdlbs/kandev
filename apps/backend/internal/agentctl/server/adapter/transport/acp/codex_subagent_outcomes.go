package acp

import (
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func (a *Adapter) trackCodexSubagentControlLocked(
	sessionID string,
	toolCallID string,
	payload *streams.NormalizedPayload,
) {
	if a.agentID != codexAgentID || sessionID == "" || toolCallID == "" ||
		payload == nil || payload.Generic() == nil {
		return
	}
	control := payload.Generic().SubagentControl
	if control != streams.SubagentControlWait && control != streams.SubagentControlClose {
		return
	}
	if a.codexControlToolCalls == nil {
		a.codexControlToolCalls = make(map[string]map[string]streams.SubagentControl)
	}
	byToolCall := a.codexControlToolCalls[sessionID]
	if byToolCall == nil {
		byToolCall = make(map[string]streams.SubagentControl)
		a.codexControlToolCalls[sessionID] = byToolCall
	}
	byToolCall[toolCallID] = control
}

// codexControlCardUpdates applies terminal child reports before the provider's
// own control-tool update is emitted.
func (a *Adapter) codexControlCardUpdates(
	sessionID string,
	update *acpsdk.SessionToolCallUpdate,
) []AgentEvent {
	if a.agentID != codexAgentID || update == nil {
		return nil
	}
	toolCallID := string(update.ToolCallId)
	status := ""
	if update.Status != nil {
		status = string(*update.Status)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	control, tracked := a.codexControlToolCalls[sessionID][toolCallID]
	if !tracked {
		return nil
	}
	if codexControlToolTerminalStatus(status) {
		a.forgetCodexControlToolLocked(sessionID, toolCallID)
	}
	if status != string(acpsdk.ToolCallStatusCompleted) {
		return nil
	}
	states := codexControlAgentStates(update.RawInput)
	if len(states) == 0 {
		return nil
	}
	var events []AgentEvent
	for childID, rawState := range states {
		if event := a.codexControlChildUpdateLocked(sessionID, control, childID, rawState); event != nil {
			events = append(events, *event)
		}
	}
	return events
}

func (a *Adapter) forgetCodexControlToolLocked(sessionID, toolCallID string) {
	byToolCall := a.codexControlToolCalls[sessionID]
	delete(byToolCall, toolCallID)
	if len(byToolCall) == 0 {
		delete(a.codexControlToolCalls, sessionID)
	}
}

func codexControlAgentStates(rawInput any) map[string]any {
	input, ok := rawInput.(map[string]any)
	if !ok {
		return nil
	}
	states, _ := input["agentsStates"].(map[string]any)
	return states
}

func (a *Adapter) codexControlChildUpdateLocked(
	sessionID string,
	control streams.SubagentControl,
	childID string,
	rawState any,
) *AgentEvent {
	state, ok := rawState.(map[string]any)
	if !ok {
		return nil
	}
	childStatus, _ := state["status"].(string)
	childStatus = strings.TrimSpace(childStatus)
	correlation, found := a.findCodexSubagentByChildLocked(sessionID, childID)
	if !found || correlation.payload == nil || correlation.payload.SubagentTask() == nil {
		return nil
	}
	child := correlation.payload.SubagentTask()
	childStatus, applies := codexControlEffectiveStatus(control, childStatus, child.Status)
	if !applies {
		return nil
	}
	previousStatus := child.Status
	previousResult := child.ResultText
	fillCodexStatus(child, childStatus)
	if message, ok := state["message"].(string); ok && strings.TrimSpace(message) != "" {
		child.ResultText = message
	}
	if previousStatus == child.Status && previousResult == child.ResultText {
		return nil
	}
	event := &AgentEvent{
		Type:              streams.EventTypeToolUpdate,
		SessionID:         sessionID,
		ToolCallID:        correlation.emittedToolCallID,
		ParentToolCallID:  correlation.parentToolCallID,
		ToolStatus:        toolStatusComplete,
		NormalizedPayload: correlation.payload.Snapshot(),
	}
	if codexSubagentPayloadTerminal(correlation.payload) {
		a.markCodexSubagentTerminalLocked(sessionID, correlation.emittedToolCallID)
	}
	return event
}

func codexControlEffectiveStatus(
	control streams.SubagentControl,
	incomingStatus string,
	currentStatus string,
) (string, bool) {
	incomingRank := codexSubagentStatusRank(incomingStatus)
	if incomingRank == 0 {
		return "", false
	}
	currentRank := codexSubagentStatusRank(currentStatus)
	if control == streams.SubagentControlClose && incomingRank < 3 {
		if currentRank == 3 {
			return "", false
		}
		return toolStatusShutdown, true
	}
	if incomingRank < currentRank {
		return "", false
	}
	return incomingStatus, true
}

func codexControlToolTerminalStatus(status string) bool {
	switch status {
	case string(acpsdk.ToolCallStatusCompleted), string(acpsdk.ToolCallStatusFailed), toolStatusError, toolStatusCancelled:
		return true
	default:
		return false
	}
}
