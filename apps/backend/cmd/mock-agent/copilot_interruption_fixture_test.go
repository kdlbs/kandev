package main

import (
	"context"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestCopilotInterruptionReplayEmitsOnePendingCoreQuestion(t *testing.T) {
	t.Setenv(copilotInterruptionFixtureEnv, "true")
	const sessionID = acp.SessionId("copilot-recovery-session")
	updater := &recordingSessionUpdater{}
	agent := &mockAgent{conn: updater}

	if err := agent.emitCopilotInterruptionReplay(context.Background(), sessionID); err != nil {
		t.Fatalf("emit replay question: %v", err)
	}
	if len(updater.updates) != 1 {
		t.Fatalf("replay updates = %d, want one pending question", len(updater.updates))
	}
	question := updater.updates[0].Update.ToolCall
	if question == nil || question.ToolCallId != copilotInterruptionQuestionID(sessionID) ||
		question.Title != "kandev-ask_user_question_kandev" || question.Kind != acp.ToolKindOther ||
		question.Status != acp.ToolCallStatusPending {
		t.Fatalf("replayed question = %+v, want the pending Core question identity", question)
	}
	if !agent.takeCopilotInterruptionRecovery(sessionID) || agent.takeCopilotInterruptionRecovery(sessionID) {
		t.Fatal("recovery prompt marker was not consumed exactly once")
	}
}

func TestCopilotInterruptionRecoveryPromptEmitsCapturedFrames(t *testing.T) {
	t.Setenv(copilotInterruptionFixtureEnv, "true")
	const sessionID = acp.SessionId("copilot-recovery-session")
	updater := &recordingSessionUpdater{}
	agent := &mockAgent{
		conn:                               updater,
		copilotInterruptionRecoveryPending: map[acp.SessionId]bool{sessionID: true},
	}

	response, err := agent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: sessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock("Continue after skipping the saved question")},
	})
	if err != nil || response.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("recovery prompt = %+v, err %v", response, err)
	}
	if len(updater.updates) != 3 {
		t.Fatalf("recovery updates = %d, want interrupted update, warning, and reply", len(updater.updates))
	}
	failed := updater.updates[0].Update.ToolCallUpdate
	if failed == nil || failed.ToolCallId != copilotInterruptionQuestionID(sessionID) ||
		failed.Status == nil || *failed.Status != acp.ToolCallStatusFailed ||
		failed.RawOutput.(map[string]any)["code"] != "interrupted" {
		t.Fatalf("interrupted update = %+v", failed)
	}
	warning := updater.updates[1].Update.AgentMessageChunk
	if warning == nil || warning.MessageId != nil || warning.Content.Text == nil ||
		warning.Content.Text.Text != copilotInterruptionWarning {
		t.Fatalf("warning frame = %+v, want the exact uncorrelated recovery warning", warning)
	}
	reply := updater.updates[2].Update.AgentMessageChunk
	if reply == nil || reply.MessageId == nil || reply.Content.Text == nil || reply.Content.Text.Text != "Here is the next step." {
		t.Fatalf("reply frame = %+v, want a separate identified assistant reply", reply)
	}
}
