package acp

import (
	"fmt"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const copilotInterruptedWarning = "Warning: 1 tool call(s) had no recorded result at resume. Files or other state may be partially changed. Completed tool results were retained; unresolved operations were not automatically retried. Verify the current state before continuing."

const copilotReadyNotice = "Info: Plugins required by your organization admin are ready."

func TestCopilotOrgPluginNotices(t *testing.T) {
	a := newTestAdapterForAgent(copilotAgentID)
	t.Cleanup(func() { _ = a.Close() })
	if events := replayCopilotFixture(t, a, "copilot-org-plugin-notices.jsonl"); len(events) != 0 {
		t.Errorf("routine notices emitted %d events, want zero", len(events))
	}
	for _, tc := range []struct{ text, id string }{
		{copilotReadyNotice, "reply-1"},
		{"Info: Updating plugins required by your organization admin: example@marketplace...", "reply-2"},
		{copilotReadyNotice + " More details.", ""},
		{"Info: Plugin configuration changed.", ""},
		{"Warning: Plugin update failed.", ""},
		{"Error: Required plugin unavailable.", ""},
	} {
		update := warningChunkText(tc.text)
		if tc.id != "" {
			update.AgentMessageChunk.MessageId = acpsdk.Ptr(acpsdk.MessageId(tc.id))
		}
		a.handleACPUpdate(makeNotification("session-1", update), 1)
		events := drainEvents(a)
		if len(events) != 1 || events[0].Type != streams.EventTypeMessageChunk || events[0].Text != tc.text || events[0].ProtocolMessageID != tc.id {
			t.Errorf("text %q with ID %q: unexpected events %+v", tc.text, tc.id, events)
		}
	}
	other := newTestAdapterForAgent("custom-acp")
	t.Cleanup(func() { _ = other.Close() })
	other.handleACPUpdate(makeNotification("session-1", warningChunkText(copilotReadyNotice)), 0)
	if events := drainEvents(other); len(events) != 1 || events[0].Text != copilotReadyNotice {
		t.Errorf("other agent emitted %+v, want original ready text", events)
	}
}

func TestCopilotInterruptedToolRecoveryNotice(t *testing.T) {
	a := newTestAdapterForAgent(copilotAgentID)
	t.Cleanup(func() { _ = a.Close() })
	a.beginCopilotInterruptionLoad("session-1", true)
	a.mu.Lock()
	a.sessionID = "session-1"
	a.loadingSessionID = "session-1"
	a.isLoadingSession = true
	a.mu.Unlock()

	a.handleACPUpdate(makeNotification("session-1", acpsdk.SessionUpdate{
		ToolCall: &acpsdk.SessionUpdateToolCall{
			ToolCallId: "question-1",
			Title:      "kandev-ask_user_question_kandev",
			Kind:       acpsdk.ToolKindOther,
			Status:     acpsdk.ToolCallStatusPending,
		},
	}), 0)
	a.mu.Lock()
	a.isLoadingSession = false
	a.loadingSessionID = ""
	activated := a.activateCopilotInterruptionPromptIfLoadedLocked("session-1", 7)
	a.mu.Unlock()
	if !activated {
		t.Fatal("resumed Copilot prompt was not activated after the native binding barrier")
	}

	failed := acpsdk.ToolCallStatusFailed
	a.handleACPUpdate(makeNotification("session-1", acpsdk.SessionUpdate{
		ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
			ToolCallId: "question-1",
			Status:     &failed,
			RawOutput:  map[string]any{"code": "interrupted"},
		},
	}), 7)
	a.handleACPUpdate(makeNotification("session-1", warningChunkText(copilotReadyNotice)), 7)
	a.handleACPUpdate(makeNotification("session-1", acpsdk.SessionUpdate{
		AgentMessageChunk: &acpsdk.SessionUpdateAgentMessageChunk{Content: acpsdk.TextBlock(copilotInterruptedWarning)},
	}), 7)
	a.handleACPUpdate(makeNotification("session-1", acpsdk.SessionUpdate{
		AgentMessageChunk: &acpsdk.SessionUpdateAgentMessageChunk{Content: acpsdk.TextBlock("Here is the next step.")},
	}), 7)

	events := drainEvents(a)
	if len(events) != 3 {
		t.Fatalf("got %d events, want failed tool update, notice, and reply: %+v", len(events), events)
	}
	notice := events[1]
	if notice.Type != streams.EventTypeMessageChunk || notice.Text != copilotInterruptedWarning {
		t.Fatalf("notice event = %+v, want the original warning as a message chunk", notice)
	}
	if got := notice.Data["copilot_interruption_notice"]; got != "clarification" {
		t.Fatalf("notice classification = %#v, want clarification", got)
	}
	if events[2].Text != "Here is the next step." || len(events[2].Data) != 0 {
		t.Fatalf("reply event = %+v, want regular assistant text", events[2])
	}
}

func TestCopilotInterruptedToolRecoveryClassification(t *testing.T) {
	tests := []struct {
		name            string
		agentID         string
		core            bool
		load            []acpsdk.SessionUpdate
		prompt          []acpsdk.SessionUpdate
		generation      uint64
		want            string
		wrongSession    bool
		staleGeneration bool
		failedLoad      bool
	}{
		{
			name:    "another tool call",
			agentID: copilotAgentID, core: true,
			load:       []acpsdk.SessionUpdate{pendingQuestion("question-1")},
			prompt:     []acpsdk.SessionUpdate{interruptedCall("shell-1"), warningChunk(1)},
			generation: 7, want: streams.CopilotInterruptionTool,
		},
		{
			name:    "available commands update between interruption and warning",
			agentID: copilotAgentID, core: true,
			load: []acpsdk.SessionUpdate{pendingQuestion("question-1")},
			prompt: []acpsdk.SessionUpdate{
				interruptedCall("question-1"),
				{AvailableCommandsUpdate: &acpsdk.SessionAvailableCommandsUpdate{}},
				warningChunk(1),
			},
			generation: 7, want: streams.CopilotInterruptionClarification,
		},
		{
			name:    "multiple interrupted calls",
			agentID: copilotAgentID, core: true,
			load:       []acpsdk.SessionUpdate{pendingQuestion("question-1")},
			prompt:     []acpsdk.SessionUpdate{interruptedCall("question-1"), interruptedCall("shell-1"), warningChunk(2)},
			generation: 7, want: streams.CopilotInterruptionTool,
		},
		{
			name:    "conflicting replay question identities",
			agentID: copilotAgentID, core: true,
			load:       []acpsdk.SessionUpdate{pendingQuestion("question-1"), pendingQuestion("question-2")},
			prompt:     []acpsdk.SessionUpdate{interruptedCall("question-1"), warningChunk(1)},
			generation: 7, want: streams.CopilotInterruptionTool,
		},
		{
			name:    "missing replay identity",
			agentID: copilotAgentID, core: true,
			load:       []acpsdk.SessionUpdate{pendingQuestion("")},
			prompt:     []acpsdk.SessionUpdate{interruptedCall("question-1"), warningChunk(1)},
			generation: 7, want: streams.CopilotInterruptionTool,
		},
		{
			name:    "core MCP was not delivered",
			agentID: copilotAgentID, core: false,
			load:       []acpsdk.SessionUpdate{pendingQuestion("question-1")},
			prompt:     []acpsdk.SessionUpdate{interruptedCall("question-1"), warningChunk(1)},
			generation: 7, want: streams.CopilotInterruptionTool,
		},
		{
			name:    "completed replay question",
			agentID: copilotAgentID, core: true,
			load:       []acpsdk.SessionUpdate{pendingQuestion("question-1"), terminalQuestion("question-1")},
			prompt:     []acpsdk.SessionUpdate{interruptedCall("question-1"), warningChunk(1)},
			generation: 7, want: streams.CopilotInterruptionTool,
		},
		{
			name:    "other provider",
			agentID: "claude-acp", core: true,
			load:       []acpsdk.SessionUpdate{pendingQuestion("question-1")},
			prompt:     []acpsdk.SessionUpdate{interruptedCall("question-1"), warningChunk(1)},
			generation: 7,
		},
		{
			name:    "wrong session",
			agentID: copilotAgentID, core: true,
			load:         []acpsdk.SessionUpdate{pendingQuestion("question-1")},
			prompt:       []acpsdk.SessionUpdate{interruptedCall("question-1"), warningChunk(1)},
			generation:   7,
			wrongSession: true,
		},
		{
			name:    "stale prompt generation",
			agentID: copilotAgentID, core: true,
			load:            []acpsdk.SessionUpdate{pendingQuestion("question-1")},
			prompt:          []acpsdk.SessionUpdate{interruptedCall("question-1"), warningChunk(1)},
			generation:      7,
			staleGeneration: true,
		},
		{
			name:    "failed load clears evidence",
			agentID: copilotAgentID, core: true,
			load:       []acpsdk.SessionUpdate{pendingQuestion("question-1")},
			prompt:     []acpsdk.SessionUpdate{interruptedCall("question-1"), warningChunk(1)},
			generation: 7,
			failedLoad: true,
		},
		{
			name:    "warning text without failed update",
			agentID: copilotAgentID, core: true,
			load:       []acpsdk.SessionUpdate{pendingQuestion("question-1")},
			prompt:     []acpsdk.SessionUpdate{warningChunk(1)},
			generation: 7,
		},
		{
			name:    "warning with an assistant message identity",
			agentID: copilotAgentID, core: true,
			load:       []acpsdk.SessionUpdate{pendingQuestion("question-1")},
			prompt:     []acpsdk.SessionUpdate{interruptedCall("question-1"), warningChunkWithID(1)},
			generation: 7,
		},
		{
			name:    "changed warning shape",
			agentID: copilotAgentID, core: true,
			load:       []acpsdk.SessionUpdate{pendingQuestion("question-1")},
			prompt:     []acpsdk.SessionUpdate{interruptedCall("question-1"), warningChunkText(copilotInterruptedWarning + " See details.")},
			generation: 7,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAdapterForAgent(tc.agentID)
			t.Cleanup(func() { _ = a.Close() })
			a.beginCopilotInterruptionLoad("session-1", tc.core)
			a.mu.Lock()
			a.sessionID = "session-1"
			a.loadingSessionID = "session-1"
			a.isLoadingSession = true
			a.mu.Unlock()
			for _, update := range tc.load {
				a.handleACPUpdate(makeNotification("session-1", update), 0)
			}
			a.mu.Lock()
			a.isLoadingSession = false
			a.loadingSessionID = ""
			if tc.failedLoad {
				a.clearCopilotInterruptionLocked()
			} else {
				a.activateCopilotInterruptionPromptLocked("session-1", tc.generation)
			}
			a.mu.Unlock()
			for _, update := range tc.prompt {
				sessionID := "session-1"
				generation := tc.generation
				if tc.wrongSession {
					sessionID = "session-2"
				}
				if tc.staleGeneration {
					generation++
				}
				a.handleACPUpdate(makeNotification(sessionID, update), generation)
			}
			if got := copilotNoticeClassification(drainEvents(a)); got != tc.want {
				t.Fatalf("classification = %q, want %q", got, tc.want)
			}
		})
	}
}

func pendingQuestion(id string) acpsdk.SessionUpdate {
	return acpsdk.SessionUpdate{ToolCall: &acpsdk.SessionUpdateToolCall{
		ToolCallId: acpsdk.ToolCallId(id), Title: copilotQuestionToolTitle,
		Kind: acpsdk.ToolKindOther, Status: acpsdk.ToolCallStatusPending,
	}}
}

func terminalQuestion(id string) acpsdk.SessionUpdate {
	status := acpsdk.ToolCallStatusCompleted
	return acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
		ToolCallId: acpsdk.ToolCallId(id), Status: &status,
	}}
}

func interruptedCall(id string) acpsdk.SessionUpdate {
	status := acpsdk.ToolCallStatusFailed
	return acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
		ToolCallId: acpsdk.ToolCallId(id), Status: &status,
		RawOutput: map[string]any{"code": "interrupted"},
	}}
}

func warningChunk(count int) acpsdk.SessionUpdate {
	text := copilotInterruptedWarning
	if count != 1 {
		text = fmt.Sprintf("Warning: %d%s", count, copilotInterruptionWarningSuffix)
	}
	return warningChunkText(text)
}

func warningChunkWithID(count int) acpsdk.SessionUpdate {
	update := warningChunk(count)
	messageID := acpsdk.MessageId("warning-message-id")
	update.AgentMessageChunk.MessageId = &messageID
	return update
}

func warningChunkText(text string) acpsdk.SessionUpdate {
	return acpsdk.SessionUpdate{AgentMessageChunk: &acpsdk.SessionUpdateAgentMessageChunk{
		Content: acpsdk.TextBlock(text),
	}}
}

func copilotNoticeClassification(events []AgentEvent) string {
	for _, event := range events {
		if event.Type != streams.EventTypeMessageChunk || event.Text == "" || event.Data == nil {
			continue
		}
		if value, ok := event.Data[streams.AgentEventDataCopilotInterruptionNotice].(string); ok {
			return value
		}
	}
	return ""
}
