package acp

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// These sanitized fixtures retain the parent/child identities and result shapes
// observed in foreground, parallel, and background Copilot ACP captures.
// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-002.1, AC-AGENTS-SUBAGENT-LIFECYCLE-002.2, AC-AGENTS-SUBAGENT-LIFECYCLE-002.3, AC-AGENTS-SUBAGENT-LIFECYCLE-002.4, AC-AGENTS-SUBAGENT-LIFECYCLE-002.5, AC-AGENTS-SUBAGENT-LIFECYCLE-002.6, AC-AGENTS-SUBAGENT-LIFECYCLE-004.1
func TestCopilotCapturedSubagentFrames(t *testing.T) {
	t.Run("foreground result and unique child link", func(t *testing.T) {
		a := newCopilotTestAdapter(t)
		events := replayCopilotFixture(t, a, "copilot-foreground.jsonl")
		parent := latestCopilotParent(events, "parent-foreground")
		if parent == nil || parent.NormalizedPayload.SubagentTask() == nil {
			t.Fatalf("missing foreground subagent card: %+v", parent)
		}
		payload := parent.NormalizedPayload.SubagentTask()
		if payload.Provider != "copilot" || payload.Name != "reader" || payload.Description != "Read a.txt contents" ||
			payload.Prompt != "Read a.txt and report its contents." || payload.SubagentType != "explore" {
			t.Fatalf("foreground card lost Copilot fields: %+v", payload)
		}
		if payload.ResultText != "apple" || payload.Status != toolStatusComplete {
			t.Fatalf("foreground result = %+v", payload)
		}
		child := findCopilotEvent(events, "child-foreground")
		if child == nil || child.ParentToolCallID != "parent-foreground" {
			t.Fatalf("the sole open foreground card should own the child call: %+v", child)
		}
	})

	t.Run("parallel children stay top-level without an identity match", func(t *testing.T) {
		a := newCopilotTestAdapter(t)
		events := replayCopilotFixture(t, a, "copilot-parallel.jsonl")
		for _, toolCallID := range []string{"child-a", "child-b"} {
			child := findCopilotEvent(events, toolCallID)
			if child == nil || child.ParentToolCallID != "" {
				t.Fatalf("ambiguous parallel call %q was nested: %+v", toolCallID, child)
			}
		}
		for _, toolCallID := range []string{"parent-a", "parent-b"} {
			parent := latestCopilotParent(events, toolCallID)
			if parent == nil || parent.NormalizedPayload.SubagentTask().Name == "" {
				t.Fatalf("missing named parallel card %q: %+v", toolCallID, parent)
			}
		}
	})

	t.Run("background identity binds each child call", func(t *testing.T) {
		a := newCopilotTestAdapter(t)
		events := replayCopilotFixture(t, a, "copilot-background.jsonl")
		parent := latestCopilotParent(events, "parent-background")
		if parent == nil || parent.NormalizedPayload.SubagentTask() == nil {
			t.Fatalf("missing background subagent card: %+v", parent)
		}
		payload := parent.NormalizedPayload.SubagentTask()
		if payload.Name != "delayed-file-read" || !payload.IsAsync ||
			payload.AgentID != "927657f7-346e-49b8-a127-27bc7ac72975" ||
			payload.Status != subagentAsyncLaunchedStatus {
			t.Fatalf("background card lost reported state or identity: %+v", payload)
		}
		if parent.NormalizedPayload.BackgroundWork() != nil {
			t.Fatal("Copilot background launch must not create background-work provenance")
		}
		for _, toolCallID := range []string{"child-background-a", "child-background-b"} {
			child := findCopilotEvent(events, toolCallID)
			if child == nil || child.ParentToolCallID != "parent-background" {
				t.Fatalf("background child %q was not bound by identity: %+v", toolCallID, child)
			}
		}
	})
}

func TestCopilotDialectControlLabelsAndProviderScope(t *testing.T) {
	dialect := newACPDialect("copilot-acp")
	for toolName, want := range map[string]streams.SubagentControl{
		"read_agent":  streams.SubagentControlRead,
		"write_agent": streams.SubagentControlMessage,
		"list_agents": streams.SubagentControlList,
	} {
		if got := dialect.parseSubagentControl(toolName, nil, nil); got != want {
			t.Errorf("control %q = %q, want %q", toolName, got, want)
		}
	}
	if _, ok := newACPDialect("other-agent").parseSubagentFrame(nil, "", map[string]any{
		"agent_type": "task",
		"prompt":     "Read a file",
	}); ok {
		t.Fatal("non-Copilot agent_type and prompt must not gain Copilot subagent recognition")
	}
}

func TestCopilotBackgroundResultRequiresMatchingAgentIDShape(t *testing.T) {
	result, ok := copilotSubagentResult(map[string]any{
		"content": "Agent started in background with agent_id: not-a-uuid",
	})
	if !ok || result.AgentID != "" || result.Status != subagentAsyncLaunchedStatus || !result.IsAsync {
		t.Fatalf("malformed background result acquired an identity: %+v, ok=%v", result, ok)
	}
}

// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-002.5
func TestCopilotChildDoesNotGuessForegroundWhenBackgroundIdentityIsUnknown(t *testing.T) {
	a := newCopilotTestAdapter(t)
	background := streams.NewSubagentTask("Background", "Do work", "explore")
	background.SubagentTask().IsAsync = true
	foreground := streams.NewSubagentTask("Foreground", "Do other work", "explore")

	a.mu.Lock()
	a.trackCopilotSubagentCallLocked("session-1", "background-unknown", background)
	a.trackCopilotSubagentCallLocked("session-1", "foreground-open", foreground)
	parent := a.copilotSubagentParentToolCallIDLocked("session-1", "child-agent", "child-call")
	a.mu.Unlock()

	if parent != "" {
		t.Fatalf("child was guessed under %q while a background launch had no ID", parent)
	}
}

// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-002.5
func TestCopilotChildResolverExcludesItsOwnOpenCard(t *testing.T) {
	a := newCopilotTestAdapter(t)
	a.mu.Lock()
	a.copilotSubagentSessionLocked("session-1").openForeground["nested-card"] = struct{}{}
	parent := a.copilotSubagentParentToolCallIDLocked("session-1", "nested-agent", "nested-card")
	a.mu.Unlock()

	if parent == "nested-card" {
		t.Fatalf("nested card resolved itself as its parent: %q", parent)
	}
}

func newCopilotTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	a := newTestAdapter()
	a.agentID = "copilot-acp"
	a.dialect = newACPDialect(a.agentID)
	a.normalizer = NewNormalizer(a.agentID)
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func replayCopilotFixture(t *testing.T, a *Adapter, name string) []AgentEvent {
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
	var events []AgentEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var frame acpsdk.SessionNotification
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatalf("decode sanitized Copilot fixture: %v", err)
		}
		a.handleACPUpdate(frame, 0)
		events = append(events, drainEvents(a)...)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func latestCopilotParent(events []AgentEvent, toolCallID string) *AgentEvent {
	var latest *AgentEvent
	for i := range events {
		if events[i].ToolCallID == toolCallID && events[i].NormalizedPayload != nil &&
			events[i].NormalizedPayload.SubagentTask() != nil {
			latest = &events[i]
		}
	}
	return latest
}

func findCopilotEvent(events []AgentEvent, toolCallID string) *AgentEvent {
	for i := range events {
		if events[i].ToolCallID == toolCallID {
			return &events[i]
		}
	}
	return nil
}
