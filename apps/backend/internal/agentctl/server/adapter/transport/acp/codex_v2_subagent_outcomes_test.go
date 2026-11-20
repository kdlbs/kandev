package acp

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-001.1, AC-AGENTS-SUBAGENT-LIFECYCLE-001.2, AC-AGENTS-SUBAGENT-LIFECYCLE-001.3, AC-AGENTS-SUBAGENT-LIFECYCLE-001.4, AC-AGENTS-SUBAGENT-LIFECYCLE-001.5, AC-AGENTS-SUBAGENT-LIFECYCLE-001.6
func TestCodexCapturedParallelWaitUpdatesEachChildCard(t *testing.T) {
	a := newCodexOutcomeTestAdapter(t)
	frames := readCodexOutcomeFixture(t, "codex-v2-parallel-subagent-outcomes.jsonl")
	var events []AgentEvent
	for _, frame := range frames {
		a.handleACPUpdate(frame, 0)
		newEvents := drainEvents(a)
		events = append(events, newEvents...)
		if frame.Update.ToolCallUpdate != nil && codexFrameToolCallID(frame) == "wait-b" {
			if len(newEvents) < 2 || !isCodexChildUpdate(newEvents[0], "child-b") || newEvents[1].ToolCallID != "wait-b" {
				t.Fatalf("child card must update before the control tool row: %+v", newEvents)
			}
			childB := latestCodexChildEvent(events, "child-b")
			if childB == nil || childB.NormalizedPayload.SubagentTask().Status != "completed" ||
				childB.NormalizedPayload.SubagentTask().ResultText != "The command output was: beta" {
				t.Fatalf("first wait did not update its named child: %+v", childB)
			}
			childA := latestCodexChildEvent(events, "child-a")
			if childA == nil || childA.NormalizedPayload.SubagentTask().Status == "completed" ||
				childA.NormalizedPayload.SubagentTask().ResultText != "" {
				t.Fatalf("first wait changed the unnamed sibling: %+v", childA)
			}
		}
	}
	for childID, want := range map[string]string{
		"child-a": "The command output was: alpha",
		"child-b": "The command output was: beta",
	} {
		got := latestCodexChildEvent(events, childID)
		if got == nil || got.Type != streams.EventTypeToolUpdate {
			t.Fatalf("missing final card update for %s: %+v", childID, got)
		}
		payload := got.NormalizedPayload.SubagentTask()
		if payload.Status != "completed" || payload.ResultText != want {
			t.Fatalf("child %s final payload = %+v, want completed result %q", childID, payload, want)
		}
		if got.ToolCallID == "wait-a" || got.ToolCallID == "wait-b" {
			t.Fatalf("child %s update reused a control tool id %q", childID, got.ToolCallID)
		}
	}
	if !hasToolEvent(events, "wait-a") || !hasToolEvent(events, "wait-b") {
		t.Fatal("control tool rows must retain their own events")
	}
	assertCodexWaitDoesNotRegressTerminalCard(t, a, events)
}

// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-001.4, AC-AGENTS-SUBAGENT-LIFECYCLE-001.5
func TestCodexCapturedTimedOutWaitDoesNothingAndCloseShutsDownChild(t *testing.T) {
	a := newCodexOutcomeTestAdapter(t)
	frames := readCodexOutcomeFixture(t, "codex-v2-interrupted-subagent-outcomes.jsonl")
	var events []AgentEvent
	for _, frame := range frames {
		before := len(events)
		a.handleACPUpdate(frame, 0)
		newEvents := drainEvents(a)
		events = append(events, newEvents...)
		if frame.Update.ToolCallUpdate == nil {
			continue
		}
		switch codexFrameToolCallID(frame) {
		case "wait-timeout":
			for _, event := range newEvents {
				if isCodexChildUpdate(event, "child-interrupted") {
					t.Fatalf("timed-out wait changed the card: %+v", event)
				}
			}
			if latest := latestCodexChildEvent(events[:before], "child-interrupted"); latest == nil ||
				latest.NormalizedPayload.SubagentTask().Status != "pendingInit" {
				t.Fatalf("timed-out wait changed the prior child state: %+v", latest)
			}
		case "close-interrupted":
			updated := latestCodexChildEvent(events, "child-interrupted")
			if updated == nil || updated.NormalizedPayload.SubagentTask().Status != "shutdown" {
				t.Fatalf("completed close did not mark the child shutdown: %+v", updated)
			}
			if updated.NormalizedPayload.BackgroundWork() != nil {
				t.Fatalf("control outcome created background-work provenance: %+v", updated.NormalizedPayload.BackgroundWork())
			}
		}
	}
	if len(events) == 0 {
		t.Fatal("captured control frames did not produce their ordinary tool events")
	}
}

func codexFrameToolCallID(frame acpsdk.SessionNotification) string {
	if frame.Update.ToolCall != nil {
		return string(frame.Update.ToolCall.ToolCallId)
	}
	if frame.Update.ToolCallUpdate != nil {
		return string(frame.Update.ToolCallUpdate.ToolCallId)
	}
	return ""
}

func newCodexOutcomeTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	a := newCodexCorrelationTestAdapter()
	a.dialect = newACPDialect(codexAgentID)
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func readCodexOutcomeFixture(t *testing.T, name string) []acpsdk.SessionNotification {
	t.Helper()
	file, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close fixture: %v", err)
		}
	}()
	var frames []acpsdk.SessionNotification
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var frame acpsdk.SessionNotification
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("decode sanitized Codex fixture: %v", err)
		}
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return frames
}

func assertCodexWaitDoesNotRegressTerminalCard(t *testing.T, a *Adapter, prior []AgentEvent) {
	t.Helper()
	frames := []string{
		`{"sessionUpdate":"tool_call","toolCallId":"wait-stale","title":"wait","kind":"other","status":"in_progress","rawInput":{"senderThreadId":"parent","receiverThreadIds":[],"agentsStates":{},"status":"inProgress"}}`,
		`{"sessionUpdate":"tool_call_update","toolCallId":"wait-stale","status":"completed","rawInput":{"senderThreadId":"parent","receiverThreadIds":["child-a","child-unknown"],"agentsStates":{"child-a":{"status":"running","message":"stale progress"},"child-unknown":{"status":"completed","message":"unmatched result"}},"status":"completed"}}`,
	}
	for _, update := range frames {
		var notification acpsdk.SessionNotification
		if err := json.Unmarshal([]byte(`{"sessionId":"codex-session","update":`+update+`}`), &notification); err != nil {
			t.Fatal(err)
		}
		a.handleACPUpdate(notification, 0)
		for _, event := range drainEvents(a) {
			if isCodexChildUpdate(event, "child-a") || isCodexChildUpdate(event, "child-unknown") {
				t.Fatalf("non-terminal or unknown state updated a card: %+v", event)
			}
		}
	}
	childA := latestCodexChildEvent(prior, "child-a")
	if childA == nil || childA.NormalizedPayload.SubagentTask().Status != "completed" ||
		childA.NormalizedPayload.SubagentTask().ResultText != "The command output was: alpha" {
		t.Fatalf("non-terminal or unknown state changed the terminal child: %+v", childA)
	}
}

func latestCodexChildEvent(events []AgentEvent, childID string) *AgentEvent {
	var latest *AgentEvent
	for i := range events {
		if isCodexChildUpdate(events[i], childID) {
			latest = &events[i]
		}
	}
	return latest
}

func isCodexChildUpdate(event AgentEvent, childID string) bool {
	return event.NormalizedPayload != nil && event.NormalizedPayload.SubagentTask() != nil &&
		event.NormalizedPayload.SubagentTask().ChildSessionID == childID
}

func hasToolEvent(events []AgentEvent, toolCallID string) bool {
	for _, event := range events {
		if event.ToolCallID == toolCallID {
			return true
		}
	}
	return false
}
