package acp

import (
	"encoding/json"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// These JSON shapes come from codex-acp 2.2.2's standard-client
// AcpToolCallRenderer, SubagentActivityReporter, and CollabAgentReporter.
const codexV2ActivityStart = `{"sessionUpdate":"tool_call","toolCallId":"activity-start","title":"Start subagent reviewer","kind":"other","status":"in_progress","rawInput":{"agentThreadId":"child-1","agentPath":"/root/reviewer","activityKind":"started"}}`
const codexV2SpawnStart = `{"sessionUpdate":"tool_call","toolCallId":"spawn","title":"spawnAgent","kind":"other","status":"in_progress","rawInput":{"prompt":"Review the adapter","senderThreadId":"parent","receiverThreadIds":[],"agentsStates":{},"model":null,"reasoningEffort":null,"status":"inProgress"}}`
const codexV2SpawnResult = `{"sessionUpdate":"tool_call_update","toolCallId":"spawn","title":"spawnAgent","status":"completed","rawInput":{"prompt":"Review the adapter","senderThreadId":"parent","receiverThreadIds":["child-1"],"agentsStates":{"child-1":{"status":"running","message":null}},"model":"gpt-6.1-sol","reasoningEffort":"high","status":"completed"}}`
const codexV2SpawnResultWithoutTitle = `{"sessionUpdate":"tool_call_update","toolCallId":"spawn","status":"completed","rawInput":{"prompt":"Review the adapter","senderThreadId":"parent","receiverThreadIds":["child-1"],"agentsStates":{"child-1":{"status":"running","message":null}},"model":"gpt-6.1-sol","reasoningEffort":"high","status":"completed"}}`

// @covers AC-AGENTS-CODEX-SUBAGENT-WIRE-001.1, AC-AGENTS-CODEX-SUBAGENT-WIRE-001.2
func TestCodexV2ActivityLifecycle(t *testing.T) {
	for _, terminalStatus := range []string{"completed", "interrupted"} {
		t.Run(terminalStatus, func(t *testing.T) {
			a := newCodexCorrelationTestAdapter()
			var start acpsdk.SessionUpdateToolCall
			if err := json.Unmarshal([]byte(codexV2ActivityStart), &start); err != nil {
				t.Fatal(err)
			}
			started := a.convertToolCallUpdate("session", &start)
			if started == nil || started.NormalizedPayload.Kind() != streams.ToolKindSubagentTask {
				t.Fatalf("activity must produce a subagent card: %+v", started)
			}
			child := started.NormalizedPayload.SubagentTask()
			if child.ChildSessionID != "child-1" || child.SubagentType != "reviewer" || child.Status != "started" {
				t.Fatalf("lost activity identity: %+v", child)
			}
			// The reporter sends a title-less update for the same activity.
			completed := acpsdk.ToolCallStatus("completed")
			update := &acpsdk.SessionToolCallUpdate{ToolCallId: start.ToolCallId, Status: &completed, RawInput: start.RawInput}
			if got := a.convertToolCallResultUpdate("session", update); got == nil || got.NormalizedPayload.SubagentTask().Status != "started" {
				t.Fatalf("activity completion lost child identity: %+v", got)
			}
			terminal := codexTerminalActivityToolCall("activity-terminal", "child-1", terminalStatus)
			terminal.Meta = nil
			ended := a.convertToolCallUpdate("session", terminal)
			if ended == nil || ended.Type != streams.EventTypeToolUpdate || ended.ToolCallID != started.ToolCallID {
				t.Fatalf("terminal activity must update the original card: %+v", ended)
			}
			if ended.ToolStatus != toolStatusComplete || ended.NormalizedPayload.SubagentTask().Status != terminalStatus {
				t.Fatalf("terminal state was lost: %+v", ended)
			}
			if child.Status != "started" || ended.NormalizedPayload.BackgroundWork() != nil {
				t.Fatal("must retain immutable snapshots and presentation-only provenance")
			}
		})
	}
}

func TestCodexV2SpawnResultEnrichesAndCorrelates(t *testing.T) {
	a := newCodexCorrelationTestAdapter()
	var start acpsdk.SessionUpdateToolCall
	var result acpsdk.SessionToolCallUpdate
	if err := json.Unmarshal([]byte(codexV2SpawnStart), &start); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(codexV2SpawnResultWithoutTitle), &result); err != nil {
		t.Fatal(err)
	}
	started := a.convertToolCallUpdate("session", &start)
	if started == nil || started.NormalizedPayload.Kind() != streams.ToolKindSubagentTask {
		t.Fatalf("spawn must produce a subagent card: %+v", started)
	}
	updated := a.convertToolCallResultUpdate("session", &result)
	if updated == nil || updated.ToolCallID != started.ToolCallID {
		t.Fatalf("spawn result must update original card: %+v", updated)
	}
	child := updated.NormalizedPayload.SubagentTask()
	if child.ChildSessionID != "child-1" || child.Model != "gpt-6.1-sol" || child.Status != "running" || child.Prompt != "Review the adapter" {
		t.Fatalf("spawn result lost child data: %+v", child)
	}
	terminal := codexTerminalActivityToolCall("terminal", "child-1", "completed")
	terminal.Meta = nil
	if got := a.convertToolCallUpdate("session", terminal); got == nil || got.ToolCallID != started.ToolCallID {
		t.Fatalf("child identified by the result must remain correlatable: %+v", got)
	}
}

func TestCodexV2TitlelessSpawnResultNeedsTrackedIdentity(t *testing.T) {
	var result acpsdk.SessionToolCallUpdate
	if err := json.Unmarshal([]byte(codexV2SpawnResultWithoutTitle), &result); err != nil {
		t.Fatal(err)
	}

	a := newCodexCorrelationTestAdapter()
	a.mu.Lock()
	_, _, _, inferredTitle, suppress := a.codexSubagentUpdateTargetLocked(
		"session",
		string(result.ToolCallId),
		nil,
		"",
		result.RawInput,
		result.Title == nil,
	)
	a.mu.Unlock()

	if inferredTitle != "" || suppress {
		t.Fatalf("an untracked title-less update must not acquire spawn identity: title=%q suppress=%v", inferredTitle, suppress)
	}
}

func TestCodexV2SiblingUpdatesUseChildIdentity(t *testing.T) {
	a := newCodexCorrelationTestAdapter()
	var second *AgentEvent
	for _, childID := range []string{"child-1", "child-2"} {
		start := codexStartedActivityToolCallForChild("shared-wire-id", childID)
		start.Meta = nil
		second = a.convertToolCallUpdate("session", start)
	}
	status := acpsdk.ToolCallStatus("completed")
	terminal := codexTerminalActivityToolCall("shared-wire-id", "child-2", "interrupted")
	update := &acpsdk.SessionToolCallUpdate{ToolCallId: terminal.ToolCallId, Status: &status, RawInput: terminal.RawInput}
	got := a.convertToolCallResultUpdate("session", update)
	if got == nil || got.NormalizedPayload.SubagentTask() == nil || got.ToolCallID != second.ToolCallID || got.ToolCallID == "shared-wire-id" {
		t.Fatalf("update must select the second child's emitted ID: %+v", got)
	}
	if got.NormalizedPayload.SubagentTask().Status != "interrupted" {
		t.Fatal("terminal child status was lost")
	}
	if first := a.activeToolCalls["shared-wire-id"]; first == nil || first.SubagentTask().Status != "started" {
		t.Fatalf("sibling was mutated: %+v", first)
	}
}

// @covers AC-AGENTS-CODEX-SUBAGENT-WIRE-001.4
func TestCodexV2CapacityEvidenceRetainsSubagentProvenance(t *testing.T) {
	for name, frame := range map[string]string{"activity": codexV2ActivityStart, "spawn result": codexV2SpawnResult} {
		t.Run(name, func(t *testing.T) {
			a, turn := newCapacityPromptTurn(t, 7)
			var update acpsdk.SessionUpdate
			if err := json.Unmarshal([]byte(frame), &update); err != nil {
				t.Fatal(err)
			}
			if update.ToolCallUpdate != nil {
				// The spawn identity can first arrive in an update.
				turn.capacityTools = map[string]capacityToolEvidence{"spawn": {status: "in_progress", kind: "other"}}
			}
			a.observeCapacityContinuation(makeNotification("capacity-session", update), 7)
			if snapshot := a.capacityContinuationSnapshot(turn, true); !snapshot.UnaccountedBackground {
				t.Fatal("structured Codex subagent evidence must fence capacity continuation")
			}
		})
	}
}

// @covers AC-AGENTS-CODEX-SUBAGENT-WIRE-001.3
func TestCodexV2RecognitionRejectsIncompleteAndUnrelatedFrames(t *testing.T) {
	cases := []struct {
		name, wire, provider, title string
		field                       string
		value                       any
	}{
		{name: "title only", wire: `{"title":"spawnAgent","kind":"other"}`},
		{name: "activity without thread", wire: codexV2ActivityStart, field: "agentThreadId"},
		{name: "activity blank thread", wire: codexV2ActivityStart, field: "agentThreadId", value: "  "},
		{name: "activity without path", wire: codexV2ActivityStart, field: "agentPath"},
		{name: "activity invalid path", wire: codexV2ActivityStart, field: "agentPath", value: "/"},
		{name: "activity without kind", wire: codexV2ActivityStart, field: "activityKind"},
		{name: "interaction is not spawn", wire: codexV2ActivityStart, field: "activityKind", value: "interacted"},
		{name: "unknown activity", wire: codexV2ActivityStart, field: "activityKind", value: "new-status"},
		{name: "other provider activity", wire: codexV2ActivityStart, provider: "claude-acp"},
		{name: "spawn without sender", wire: codexV2SpawnStart, field: "senderThreadId"},
		{name: "spawn without receivers", wire: codexV2SpawnStart, field: "receiverThreadIds"},
		{name: "spawn malformed receivers", wire: codexV2SpawnStart, field: "receiverThreadIds", value: []any{"child", 42}},
		{name: "spawn blank receiver", wire: codexV2SpawnStart, field: "receiverThreadIds", value: []string{" "}},
		{name: "spawn without states", wire: codexV2SpawnStart, field: "agentsStates"},
		{name: "spawn malformed states", wire: codexV2SpawnStart, field: "agentsStates", value: "running"},
		{name: "control operation", wire: codexV2SpawnStart, title: "sendInput"},
		{name: "other provider spawn", wire: codexV2SpawnStart, provider: "cursor-acp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var call acpsdk.SessionUpdateToolCall
			if err := json.Unmarshal([]byte(tc.wire), &call); err != nil {
				t.Fatal(err)
			}
			if tc.field != "" {
				call.RawInput.(map[string]any)[tc.field] = tc.value
			}
			if tc.title != "" {
				call.Title = tc.title
			}
			provider := tc.provider
			if provider == "" {
				provider = codexAgentID
			}
			payload := NewNormalizer(provider).NormalizeToolCall("other", map[string]any{
				argKeyTitle: call.Title, "raw_input": call.RawInput,
			})
			if payload.Kind() != streams.ToolKindGeneric {
				t.Fatalf("unrelated/incomplete frame became %q", payload.Kind())
			}
		})
	}
}

func TestCodexV2LegacyMetadataIsAuthoritative(t *testing.T) {
	for name, meta := range map[string]map[string]any{
		"legacy child": codexActivityMeta("started"),
		"control":      codexCollaborationMeta("sendInput", []any{"thread-child"}),
		"malformed":    {"codex": "invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			input := map[string]any{"agentThreadId": "conflicting-child", "agentPath": "/root/other", "activityKind": "started"}
			frame, ok := parseCodexSubagentFrame(meta, "Start subagent other", input)
			if name == "legacy child" {
				if !ok || frame.result.ChildSessionID != "thread-child" || frame.subagentType != "review_agent" {
					t.Fatalf("legacy identity was overridden: %+v", frame)
				}
			} else if ok {
				t.Fatal("raw input must not override explicit malformed/control metadata")
			}
		})
	}
}

func TestCodexV2NestedSpawnPreservesParentIdentity(t *testing.T) {
	a := newCodexCorrelationTestAdapter()
	parent := codexStartedActivityToolCallForChild("parent-call", "parent")
	parent.Meta = nil
	parentEvent := a.convertToolCallUpdate("session", parent)
	var child acpsdk.SessionUpdateToolCall
	if err := json.Unmarshal([]byte(codexV2SpawnStart), &child); err != nil {
		t.Fatal(err)
	}
	childEvent := a.convertToolCallUpdate("session", &child)
	if childEvent == nil || childEvent.ParentToolCallID != parentEvent.ToolCallID {
		t.Fatalf("nested spawn lost its parent: %+v", childEvent)
	}
	terminal := codexTerminalActivityToolCall("unmatched", "unknown-child", "completed")
	terminal.Meta = nil
	if event := a.convertToolCallUpdate("session", terminal); event != nil {
		t.Fatal("unmatched terminal activity must not create a card")
	}
}
