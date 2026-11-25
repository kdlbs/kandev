package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

const (
	copilotInterruptionFixtureEnv  = "E2E_MOCK_COPILOT_INTERRUPTION"
	copilotInterruptionSetupPrompt = "/e2e:copilot-interruption-setup"
	copilotInterruptionWarning     = "Warning: 1 tool call(s) had no recorded result at resume. Files or other state may be partially changed. Completed tool results were retained; unresolved operations were not automatically retried. Verify the current state before continuing."
)

func copilotInterruptionFixtureEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(copilotInterruptionFixtureEnv)), "true")
}

func copilotInterruptionQuestionID(sessionID acp.SessionId) acp.ToolCallId {
	return acp.ToolCallId("copilot-interruption-question-" + string(sessionID))
}

func (a *mockAgent) emitCopilotInterruptionReplay(ctx context.Context, sessionID acp.SessionId) error {
	if !copilotInterruptionFixtureEnabled() {
		return nil
	}
	a.mu.Lock()
	if a.copilotInterruptionRecoveryPending == nil {
		a.copilotInterruptionRecoveryPending = make(map[acp.SessionId]bool)
	}
	a.copilotInterruptionRecoveryPending[sessionID] = true
	conn := a.conn
	a.mu.Unlock()
	traceACP("copilot_interruption_replay", string(sessionID), nil)
	if conn == nil {
		return fmt.Errorf("copilot interruption fixture has no ACP connection")
	}
	return conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: sessionID,
		Update: acp.StartToolCall(
			copilotInterruptionQuestionID(sessionID),
			"kandev-ask_user_question_kandev",
			acp.WithStartKind(acp.ToolKindOther),
			acp.WithStartStatus(acp.ToolCallStatusPending),
			acp.WithStartRawInput(clarificationQuestionArgs()),
		),
	})
}

func (a *mockAgent) takeCopilotInterruptionRecovery(sessionID acp.SessionId) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.copilotInterruptionRecoveryPending[sessionID] {
		return false
	}
	delete(a.copilotInterruptionRecoveryPending, sessionID)
	return true
}

func (a *mockAgent) handleCopilotInterruptionFixture(
	ctx context.Context,
	sessionID acp.SessionId,
	prompt string,
) (acp.PromptResponse, error, bool) {
	if !copilotInterruptionFixtureEnabled() {
		return acp.PromptResponse{}, nil, false
	}
	if strings.TrimSpace(stripKandevSystem(prompt)) == copilotInterruptionSetupPrompt {
		e := a.copilotInterruptionEmitter(ctx, sessionID)
		toolID := copilotInterruptionQuestionID(sessionID)
		e.startTool(toolID, "kandev-ask_user_question_kandev", acp.ToolKindOther, clarificationQuestionArgs())
		if _, err := e.callMCPTool("kandev", "ask_user_question_kandev", clarificationQuestionArgs()); err != nil {
			e.completeToolWithMeta(toolID, acp.ToolCallStatusFailed, map[string]any{"error": err.Error()}, nil)
			return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil, true
		}
		e.completeTool(toolID, map[string]any{"status": "answered"})
		e.textWithID("The clarification was answered.")
		return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil, true
	}
	if !a.takeCopilotInterruptionRecovery(sessionID) {
		return acp.PromptResponse{}, nil, false
	}
	traceACP("copilot_interruption_recovery", string(sessionID), nil)
	e := a.copilotInterruptionEmitter(ctx, sessionID)
	status := acp.ToolCallStatusFailed
	if err := e.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: sessionID,
		Update: acp.SessionUpdate{ToolCallUpdate: &acp.SessionToolCallUpdate{
			ToolCallId: copilotInterruptionQuestionID(sessionID),
			Status:     &status,
			RawOutput:  map[string]any{"code": "interrupted"},
		}},
	}); err != nil {
		return acp.PromptResponse{}, err, true
	}
	traceACP("copilot_interruption_warning", string(sessionID), nil)
	e.text(copilotInterruptionWarning)
	e.textWithID("Here is the next step.")
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil, true
}

func (a *mockAgent) copilotInterruptionEmitter(ctx context.Context, sessionID acp.SessionId) *emitter {
	a.mu.Lock()
	servers := cloneMCPServerDefs(a.sessionMCPServers[sessionID])
	supportsCompaction := a.clientSupportsCompaction
	a.mu.Unlock()
	return &emitter{
		ctx: ctx, conn: a.conn, sid: sessionID, mcpServers: servers,
		supportsCompaction: supportsCompaction,
	}
}
