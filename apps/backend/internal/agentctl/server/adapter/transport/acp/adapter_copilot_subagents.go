package acp

import (
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type copilotSubagentSessionState struct {
	openForeground       map[string]struct{}
	backgroundByAgent    map[string]map[string]struct{}
	unresolvedBackground map[string]struct{}
	parentByAgent        map[string]string
}

func (a *Adapter) copilotSubagentSessionLocked(sessionID string) *copilotSubagentSessionState {
	if a.copilotSubagents == nil {
		a.copilotSubagents = make(map[string]*copilotSubagentSessionState)
	}
	state := a.copilotSubagents[sessionID]
	if state == nil {
		state = &copilotSubagentSessionState{
			openForeground:       make(map[string]struct{}),
			backgroundByAgent:    make(map[string]map[string]struct{}),
			unresolvedBackground: make(map[string]struct{}),
			parentByAgent:        make(map[string]string),
		}
		a.copilotSubagents[sessionID] = state
	}
	return state
}

func (a *Adapter) trackCopilotSubagentCallLocked(
	sessionID string,
	toolCallID string,
	payload *streams.NormalizedPayload,
) {
	if a.agentID != copilotAgentID || sessionID == "" || toolCallID == "" ||
		payload == nil || payload.SubagentTask() == nil {
		return
	}
	state := a.copilotSubagentSessionLocked(sessionID)
	subagent := payload.SubagentTask()
	if subagent.IsAsync {
		if subagent.AgentID == "" {
			state.unresolvedBackground[toolCallID] = struct{}{}
		} else {
			addCopilotBackgroundParent(state, toolCallID, subagent.AgentID)
		}
		return
	}
	state.openForeground[toolCallID] = struct{}{}
}

func (a *Adapter) updateCopilotSubagentCallLocked(
	sessionID string,
	toolCallID string,
	payload *streams.NormalizedPayload,
	isTerminal bool,
) {
	if a.agentID != copilotAgentID || sessionID == "" || toolCallID == "" ||
		payload == nil || payload.SubagentTask() == nil {
		return
	}
	state := a.copilotSubagentSessionLocked(sessionID)
	if isTerminal {
		delete(state.openForeground, toolCallID)
	}
	subagent := payload.SubagentTask()
	if !subagent.IsAsync {
		return
	}
	if subagent.AgentID == "" {
		state.unresolvedBackground[toolCallID] = struct{}{}
		return
	}
	delete(state.unresolvedBackground, toolCallID)
	addCopilotBackgroundParent(state, toolCallID, subagent.AgentID)
}

func addCopilotBackgroundParent(state *copilotSubagentSessionState, toolCallID, agentID string) {
	parents := state.backgroundByAgent[agentID]
	if parents == nil {
		parents = make(map[string]struct{})
		state.backgroundByAgent[agentID] = parents
	}
	parents[toolCallID] = struct{}{}
}

func (a *Adapter) copilotSubagentParentToolCallID(
	sessionID, toolCallID string,
	meta map[string]any,
) string {
	agentID := copilotChildAgentID(meta)
	if agentID == "" {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.copilotSubagentParentToolCallIDLocked(sessionID, agentID, toolCallID)
}

func (a *Adapter) copilotSubagentParentToolCallIDLocked(sessionID, agentID, toolCallID string) string {
	if agentID == "" {
		return ""
	}
	state := a.copilotSubagents[sessionID]
	if state == nil {
		return ""
	}
	if parent, found := state.parentByAgent[agentID]; found {
		if parent == toolCallID {
			return ""
		}
		return parent
	}
	if candidates, matched := state.backgroundByAgent[agentID]; matched {
		if len(candidates) != 1 {
			return ""
		}
		for parent := range candidates {
			if parent == toolCallID {
				return ""
			}
			state.parentByAgent[agentID] = parent
			return parent
		}
	}
	if len(state.unresolvedBackground) > 0 {
		return ""
	}
	parent := state.uniqueOpenForegroundParent(toolCallID)
	if parent == "" {
		return ""
	}
	state.parentByAgent[agentID] = parent
	return parent
}

func copilotChildAgentID(meta map[string]any) string {
	copilot, ok := nestedMap(meta, "github.com/copilot")
	if !ok {
		return ""
	}
	agentID, _ := copilot["agentId"].(string)
	return strings.TrimSpace(agentID)
}

func (s *copilotSubagentSessionState) uniqueOpenForegroundParent(toolCallID string) string {
	boundParents := make(map[string]struct{}, len(s.parentByAgent))
	for _, parent := range s.parentByAgent {
		boundParents[parent] = struct{}{}
	}
	var candidate string
	for parent := range s.openForeground {
		if parent == toolCallID {
			continue
		}
		if _, bound := boundParents[parent]; bound {
			continue
		}
		if candidate != "" {
			return ""
		}
		candidate = parent
	}
	return candidate
}

func (a *Adapter) clearCopilotSubagentsLocked(sessionID string) {
	if sessionID == "" {
		a.copilotSubagents = make(map[string]*copilotSubagentSessionState)
		return
	}
	delete(a.copilotSubagents, sessionID)
}
