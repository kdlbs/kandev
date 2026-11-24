package acp

import (
	"strconv"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

const copilotQuestionToolTitle = "kandev-ask_user_question_kandev"

const copilotInterruptionWarningSuffix = " tool call(s) had no recorded result at resume. Files or other state may be partially changed. Completed tool results were retained; unresolved operations were not automatically retried. Verify the current state before continuing."

type copilotInterruptionState struct {
	sessionID         string
	coreMCPDelivered  bool
	pendingQuestionID string
	interruptedCallID string
	interruptionSeen  bool
	identityAmbiguous bool
	promptGeneration  uint64
	promptActive      bool
}

func copilotCoreMCPDelivered(servers []types.McpServer, delivery *types.ProjectMCPDelivery) bool {
	for _, server := range servers {
		if server.IsInjectedBuiltin() && server.BuiltinPackage == mcpprofile.PackageCore {
			return true
		}
	}
	project := validProjectMCPDelivery(delivery)
	return project != nil && project.Delivers(mcpprofile.PackageCore)
}

func (a *Adapter) beginCopilotInterruptionLoad(sessionID string, coreMCPDelivered bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.copilotInterruption = copilotInterruptionState{
		sessionID:        sessionID,
		coreMCPDelivered: coreMCPDelivered,
	}
}

func (a *Adapter) activateCopilotInterruptionPromptLocked(sessionID string, generation uint64) {
	state := &a.copilotInterruption
	if state.sessionID != sessionID || generation == 0 {
		*state = copilotInterruptionState{}
		return
	}
	state.promptGeneration = generation
	state.promptActive = true
	state.interruptedCallID = ""
	state.interruptionSeen = false
}

func (a *Adapter) activateCopilotInterruptionPromptIfLoadedLocked(sessionID string, generation uint64) bool {
	state := &a.copilotInterruption
	if a.agentID != copilotAgentID || state.sessionID != sessionID || state.promptGeneration != 0 {
		return false
	}
	a.activateCopilotInterruptionPromptLocked(sessionID, generation)
	return true
}

func (a *Adapter) clearCopilotInterruptionLocked() {
	a.copilotInterruption = copilotInterruptionState{}
}

func (a *Adapter) finishCopilotInterruptionPrompt(sessionID string, generation uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := &a.copilotInterruption
	if state.sessionID == sessionID && state.promptGeneration == generation {
		a.clearCopilotInterruptionLocked()
	}
}

func (a *Adapter) observeCopilotInterruptionUpdate(
	sessionID string,
	update acp.SessionUpdate,
	promptGeneration uint64,
) string {
	if a.agentID != copilotAgentID {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	state := &a.copilotInterruption
	if a.isLoadingSession {
		if a.loadingSessionID == sessionID && state.sessionID == sessionID {
			observeCopilotInterruptionReplay(state, update)
		}
		return ""
	}
	if !state.promptActive || state.sessionID != sessionID || a.sessionID != sessionID ||
		state.promptGeneration != promptGeneration {
		return ""
	}
	return observeCopilotRecoveryPrompt(state, update)
}

func observeCopilotInterruptionReplay(state *copilotInterruptionState, update acp.SessionUpdate) {
	switch {
	case update.ToolCall != nil:
		call := update.ToolCall
		if call.Title != copilotQuestionToolTitle || call.Kind != acp.ToolKindOther ||
			call.Status != acp.ToolCallStatusPending {
			return
		}
		id := string(call.ToolCallId)
		if id == "" {
			state.identityAmbiguous = true
			return
		}
		if state.pendingQuestionID == "" {
			state.pendingQuestionID = id
		} else if state.pendingQuestionID != id {
			state.identityAmbiguous = true
		}
	case update.ToolCallUpdate != nil:
		call := update.ToolCallUpdate
		if terminalToolCallStatus(call.Status) && state.pendingQuestionID == string(call.ToolCallId) {
			state.pendingQuestionID = ""
		}
	}
}

func terminalToolCallStatus(status *acp.ToolCallStatus) bool {
	if status == nil {
		return false
	}
	return *status == acp.ToolCallStatusCompleted || *status == acp.ToolCallStatusFailed
}

func observeCopilotRecoveryPrompt(state *copilotInterruptionState, update acp.SessionUpdate) string {
	switch {
	case update.ToolCall != nil:
		state.promptActive = false
		return ""
	case update.ToolCallUpdate != nil:
		if interruptedCopilotCall(update.ToolCallUpdate) {
			id := string(update.ToolCallUpdate.ToolCallId)
			state.interruptionSeen = true
			switch {
			case id == "":
				state.identityAmbiguous = true
			case state.interruptedCallID == "":
				state.interruptedCallID = id
			case state.interruptedCallID != id:
				state.identityAmbiguous = true
			}
		}
	case update.AgentMessageChunk != nil:
		classification := copilotRecoveryClassification(state, update.AgentMessageChunk)
		state.promptActive = false
		return classification
	}
	return ""
}

func interruptedCopilotCall(update *acp.SessionToolCallUpdate) bool {
	if update.Status == nil || *update.Status != acp.ToolCallStatusFailed {
		return false
	}
	raw, ok := update.RawOutput.(map[string]any)
	if !ok {
		return false
	}
	code, _ := raw["code"].(string)
	return code == toolStatusInterrupted
}

func copilotRecoveryClassification(
	state *copilotInterruptionState,
	chunk *acp.SessionUpdateAgentMessageChunk,
) string {
	if !state.interruptionSeen || chunk.MessageId != nil || chunk.Content.Text == nil {
		return ""
	}
	warningCount, ok := copilotInterruptionWarningCount(chunk.Content.Text.Text)
	if !ok {
		return ""
	}
	if warningCount == 1 && state.interruptedCallID == state.pendingQuestionID &&
		state.interruptedCallID != "" && !state.identityAmbiguous && state.coreMCPDelivered {
		return streams.CopilotInterruptionClarification
	}
	return streams.CopilotInterruptionTool
}

func copilotInterruptionWarningCount(text string) (int, bool) {
	const prefix = "Warning: "
	if !strings.HasPrefix(text, prefix) || !strings.HasSuffix(text, copilotInterruptionWarningSuffix) {
		return 0, false
	}
	countText := strings.TrimSuffix(strings.TrimPrefix(text, prefix), copilotInterruptionWarningSuffix)
	count, err := strconv.Atoi(countText)
	return count, err == nil && count > 0
}
