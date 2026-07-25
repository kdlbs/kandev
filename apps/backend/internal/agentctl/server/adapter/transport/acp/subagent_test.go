package acp

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// --- recognizeSubagent ---

// claudeAgentMeta builds the `_meta.claudeCode.toolName=Agent` payload
// Claude-acp attaches to subagent (Task) tool_call notifications.
func claudeAgentMeta() map[string]any {
	return map[string]any{"claudeCode": map[string]any{"toolName": "Agent"}}
}

func subagentRawInput() map[string]any {
	return map[string]any{
		"description":   "Investigate flaky test",
		"prompt":        "Find the root cause of the flaky test in foo_test.go",
		"subagent_type": "general-purpose",
	}
}

func TestRecognizeSubagent_ClaudeInitialEmpty(t *testing.T) {
	desc, prompt, st, ok := recognizeSubagent(claudeAgentMeta(), "", nil)
	if !ok {
		t.Fatal("expected Claude Agent meta to be recognized as subagent")
	}
	if desc != "" || prompt != "" || st != "" {
		t.Errorf("expected empty fields on initial call, got (%q,%q,%q)", desc, prompt, st)
	}
}

func TestRecognizeSubagent_ClaudeWithInput(t *testing.T) {
	desc, prompt, st, ok := recognizeSubagent(claudeAgentMeta(), "", subagentRawInput())
	if !ok {
		t.Fatal("expected recognition")
	}
	if desc != "Investigate flaky test" || st != "general-purpose" {
		t.Errorf("got (%q,%q,%q)", desc, prompt, st)
	}
	if prompt == "" {
		t.Errorf("expected prompt to be populated")
	}
}

func TestRecognizeSubagent_OpenCodeTitle(t *testing.T) {
	desc, _, st, ok := recognizeSubagent(nil, "task", nil)
	if !ok {
		t.Fatal("expected OpenCode title=task to be recognized")
	}
	if desc != "" || st != "" {
		t.Errorf("expected empty fields on initial OpenCode call, got (%q,%q)", desc, st)
	}
}

func TestRecognizeSubagent_OpenCodeTitleCaseInsensitive(t *testing.T) {
	if _, _, _, ok := recognizeSubagent(nil, "Task", nil); !ok {
		t.Fatal("expected case-insensitive match on title 'Task'")
	}
	if _, _, _, ok := recognizeSubagent(nil, "TASK", nil); !ok {
		t.Fatal("expected case-insensitive match on title 'TASK'")
	}
}

func TestRecognizeSubagent_OpenCodeWithInput(t *testing.T) {
	desc, prompt, st, ok := recognizeSubagent(nil, "task", subagentRawInput())
	if !ok {
		t.Fatal("expected recognition")
	}
	if desc != "Investigate flaky test" || prompt == "" || st != "general-purpose" {
		t.Errorf("got (%q,%q,%q)", desc, prompt, st)
	}
}

func TestRecognizeSubagent_CursorToolName(t *testing.T) {
	rawInput := map[string]any{"_toolName": "task"}
	_, _, _, ok := recognizeSubagent(nil, "Task: Subagent task", rawInput)
	if !ok {
		t.Fatal("expected Cursor rawInput._toolName=task to be recognized")
	}
}

func TestRecognizeSubagent_NegativeMonitor(t *testing.T) {
	meta := map[string]any{"claudeCode": map[string]any{"toolName": "Monitor"}}
	if _, _, _, ok := recognizeSubagent(meta, "Monitor foo", nil); ok {
		t.Error("Monitor meta must NOT be recognized as subagent")
	}
}

func TestRecognizeSubagent_NegativeScheduleWakeup(t *testing.T) {
	meta := map[string]any{"claudeCode": map[string]any{"toolName": "ScheduleWakeup"}}
	if _, _, _, ok := recognizeSubagent(meta, "", nil); ok {
		t.Error("ScheduleWakeup meta must NOT be recognized as subagent")
	}
}

func TestRecognizeSubagent_NegativePlainBash(t *testing.T) {
	if _, _, _, ok := recognizeSubagent(nil, "Bash", map[string]any{"command": "ls"}); ok {
		t.Error("plain bash must NOT be recognized as subagent")
	}
}

func TestRecognizeSubagent_NegativeEmpty(t *testing.T) {
	if _, _, _, ok := recognizeSubagent(nil, "", nil); ok {
		t.Error("empty inputs must NOT be recognized as subagent")
	}
}

// --- extractSubagentResult ---

func claudeResultMeta() map[string]any {
	return map[string]any{"claudeCode": map[string]any{"toolResponse": map[string]any{
		"agentId":           "agent_abc",
		"agentType":         "general-purpose",
		"status":            "completed",
		"totalDurationMs":   float64(12345),
		"totalTokens":       float64(6789),
		"totalToolUseCount": float64(11),
	}}}
}

func TestExtractSubagentResult_Claude(t *testing.T) {
	res, ok := extractSubagentResult(claudeResultMeta(), nil)
	if !ok {
		t.Fatal("expected Claude result to be extracted")
	}
	if res.AgentID != "agent_abc" {
		t.Errorf("AgentID = %q", res.AgentID)
	}
	if res.SubagentType != "general-purpose" {
		t.Errorf("SubagentType = %q", res.SubagentType)
	}
	if res.Status != "completed" {
		t.Errorf("Status = %q", res.Status)
	}
	if res.DurationMs != 12345 {
		t.Errorf("DurationMs = %d", res.DurationMs)
	}
	if res.TotalTokens != 6789 {
		t.Errorf("TotalTokens = %d", res.TotalTokens)
	}
	if res.ToolUseCount != 11 {
		t.Errorf("ToolUseCount = %d", res.ToolUseCount)
	}
}

func TestExtractSubagentResult_OpenCode(t *testing.T) {
	rawOutput := map[string]any{"metadata": map[string]any{
		"sessionId":       "ses_child",
		"parentSessionId": "ses_parent",
		"model": map[string]any{
			"providerID": "opencode",
			"modelID":    "big-pickle",
		},
	}}
	res, ok := extractSubagentResult(nil, rawOutput)
	if !ok {
		t.Fatal("expected OpenCode result to be extracted")
	}
	if res.ChildSessionID != "ses_child" {
		t.Errorf("ChildSessionID = %q", res.ChildSessionID)
	}
	if res.Model != "opencode/big-pickle" {
		t.Errorf("Model = %q, want opencode/big-pickle", res.Model)
	}
}

func TestExtractSubagentResult_Cursor(t *testing.T) {
	rawOutput := map[string]any{"durationMs": float64(4200), "isBackground": false}
	res, ok := extractSubagentResult(nil, rawOutput)
	if !ok {
		t.Fatal("expected Cursor result to be extracted")
	}
	if res.DurationMs != 4200 {
		t.Errorf("DurationMs = %d", res.DurationMs)
	}
}

// TestExtractSubagentResult_ClaudeAsyncLaunched covers the claude-acp
// envelope for `run_in_background: true`: status=async_launched plus the
// isAsync/outputFile/canReadOutputFile fields.
func TestExtractSubagentResult_ClaudeAsyncLaunched(t *testing.T) {
	meta := map[string]any{"claudeCode": map[string]any{"toolResponse": map[string]any{
		"agentId":           "agent_async_1",
		"description":       "Run sleep",
		"isAsync":           true,
		"outputFile":        "/tmp/tasks/abc.output",
		"canReadOutputFile": true,
		"status":            "async_launched",
	}}}
	res, ok := extractSubagentResult(meta, nil)
	if !ok {
		t.Fatal("expected extraction to succeed")
	}
	if res.Status != "async_launched" {
		t.Errorf("Status = %q, want async_launched", res.Status)
	}
	if !res.IsAsync {
		t.Error("IsAsync = false, want true")
	}
	if res.OutputFile != "/tmp/tasks/abc.output" {
		t.Errorf("OutputFile = %q", res.OutputFile)
	}
	if !res.CanReadOutputFile {
		t.Error("CanReadOutputFile = false, want true")
	}
}

func TestIsSubagentAsyncLaunched(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta map[string]any
		want bool
	}{
		{"async_launched", map[string]any{"claudeCode": map[string]any{"toolResponse": map[string]any{"status": "async_launched"}}}, true},
		{"completed", map[string]any{"claudeCode": map[string]any{"toolResponse": map[string]any{"status": "completed"}}}, false},
		{"nil meta", nil, false},
		{"no claudeCode", map[string]any{"other": 1}, false},
		{"no toolResponse", map[string]any{"claudeCode": map[string]any{"toolName": "Agent"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSubagentAsyncLaunched(tc.meta); got != tc.want {
				t.Errorf("isSubagentAsyncLaunched = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExtractSubagentResult_Empty(t *testing.T) {
	if _, ok := extractSubagentResult(nil, nil); ok {
		t.Error("expected no result from empty inputs")
	}
}

// --- Subagent (Task) normalization ---

func TestNormalizeToolCall_ClaudeSubagent(t *testing.T) {
	n := NewNormalizer("")
	args := map[string]any{
		"meta": map[string]any{"claudeCode": map[string]any{"toolName": "Agent"}},
	}
	payload := n.NormalizeToolCall("Agent", args)
	if payload.Kind() != streams.ToolKindSubagentTask {
		t.Fatalf("Kind = %q, want subagent_task", payload.Kind())
	}
}

func TestNormalizeToolCall_OpenCodeSubagent(t *testing.T) {
	n := NewNormalizer("")
	args := map[string]any{"title": "task"}
	payload := n.NormalizeToolCall("", args)
	if payload.Kind() != streams.ToolKindSubagentTask {
		t.Fatalf("Kind = %q, want subagent_task", payload.Kind())
	}
}

func TestNormalizeToolCall_CursorSubagent(t *testing.T) {
	n := NewNormalizer("")
	args := map[string]any{
		"title":     "Task: Subagent task",
		"raw_input": map[string]any{"_toolName": "task"},
	}
	payload := n.NormalizeToolCall("", args)
	if payload.Kind() != streams.ToolKindSubagentTask {
		t.Fatalf("Kind = %q, want subagent_task", payload.Kind())
	}
}

func TestNormalizeToolCall_CodexSpawnSubagent(t *testing.T) {
	n := NewNormalizer(codexAgentID)
	payload := n.NormalizeToolCall("other", map[string]any{
		"kind": toolTypeGeneric,
		"meta": codexCollaborationMeta(codexCollaborationSpawnAgent, []any{"thread-child"}),
		"raw_input": map[string]any{
			"prompt": "Audit the ACP adapter",
			"model":  "gpt-5.2-codex",
			"agentsStates": map[string]any{
				"thread-child": map[string]any{"status": "running"},
			},
		},
	})
	if payload.Kind() != streams.ToolKindSubagentTask {
		t.Fatalf("Kind = %q, want subagent_task", payload.Kind())
	}
	sa := payload.SubagentTask()
	if sa.ChildSessionID != "thread-child" || sa.Prompt != "Audit the ACP adapter" {
		t.Errorf("child/prompt = %q/%q", sa.ChildSessionID, sa.Prompt)
	}
	if sa.Model != "gpt-5.2-codex" || sa.Status != "running" {
		t.Errorf("model/status = %q/%q", sa.Model, sa.Status)
	}
}

func TestNormalizeToolCall_CodexStartedActivity(t *testing.T) {
	n := NewNormalizer(codexAgentID)
	payload := n.NormalizeToolCall("other", map[string]any{
		"kind": toolTypeGeneric,
		"meta": codexActivityMeta(codexSubagentStarted),
	})
	if payload.Kind() != streams.ToolKindSubagentTask {
		t.Fatalf("Kind = %q, want subagent_task", payload.Kind())
	}
	sa := payload.SubagentTask()
	if sa.ChildSessionID != "thread-child" || sa.Status != "started" {
		t.Errorf("child/status = %q/%q", sa.ChildSessionID, sa.Status)
	}
}

func TestNormalizeToolCall_CodexControlsStayGeneric(t *testing.T) {
	for _, tool := range []string{"interact", "wait", "close"} {
		t.Run(tool, func(t *testing.T) {
			n := NewNormalizer(codexAgentID)
			payload := n.NormalizeToolCall("other", map[string]any{
				"kind": toolTypeGeneric,
				"meta": codexCollaborationMeta(tool, []any{"thread-child"}),
			})
			if payload.Kind() != streams.ToolKindGeneric {
				t.Fatalf("Kind = %q, want generic", payload.Kind())
			}
		})
	}
}

func TestNormalizeToolCall_CodexMetaIsDialectScoped(t *testing.T) {
	payload := NewNormalizer("cursor-acp").NormalizeToolCall("other", map[string]any{
		"kind": toolTypeGeneric,
		"meta": codexCollaborationMeta(codexCollaborationSpawnAgent, []any{"thread-child"}),
	})
	if payload.Kind() != streams.ToolKindGeneric {
		t.Fatalf("Kind = %q, want generic for a non-Codex agent", payload.Kind())
	}
}

func TestCodexSubagentSequenceUsesImmutableSnapshots(t *testing.T) {
	a := newTestAdapter()
	a.agentID = codexAgentID
	a.normalizer = NewNormalizer(codexAgentID)
	meta := codexCollaborationMeta(codexCollaborationSpawnAgent, []any{"thread-child"})
	start := &acp.SessionUpdateToolCall{
		ToolCallId: "call-spawn",
		Kind:       "other",
		Title:      codexCollaborationSpawnAgent,
		Status:     toolStatusInProgress,
		RawInput: map[string]any{
			"prompt": "Audit the adapter",
			"agentsStates": map[string]any{
				"thread-child": map[string]any{"status": "running"},
			},
		},
		Meta: meta,
	}
	startEvent := a.convertToolCallUpdate("session-1", start)
	if startEvent.NormalizedPayload.Kind() != streams.ToolKindSubagentTask {
		t.Fatalf("start kind = %q", startEvent.NormalizedPayload.Kind())
	}
	startSnapshot := startEvent.NormalizedPayload

	completeStatus := acp.ToolCallStatus(toolStatusCompleted)
	complete := &acp.SessionToolCallUpdate{
		ToolCallId: "call-spawn",
		Status:     &completeStatus,
		RawInput: map[string]any{
			"prompt": "Audit the adapter",
			"model":  "gpt-5.2-codex",
			"agentsStates": map[string]any{
				"thread-child": map[string]any{"status": "completed"},
			},
		},
		Meta: meta,
	}
	completeEvent := a.convertToolCallResultUpdate("session-1", complete)
	if completeEvent.ToolCallID != startEvent.ToolCallID {
		t.Fatalf("tool call IDs differ: %q/%q", startEvent.ToolCallID, completeEvent.ToolCallID)
	}
	sa := completeEvent.NormalizedPayload.SubagentTask()
	if sa.Status != "completed" || sa.Model != "gpt-5.2-codex" {
		t.Errorf("status/model = %q/%q", sa.Status, sa.Model)
	}
	if got := startSnapshot.SubagentTask(); got.Status != "running" || got.Model != "" {
		t.Errorf("start event snapshot mutated after completion: status/model = %q/%q", got.Status, got.Model)
	}
	if _, active := a.activeToolCalls["call-spawn"]; active {
		t.Error("terminal completion must remove the active tool call")
	}

	// codex-acp may deliver the matching started activity after the terminal
	// collaboration update. It must update the existing card, not create a
	// second one or regress the terminal state back to running.
	activityEvent := a.convertToolCallUpdate("session-1", codexStartedActivityToolCall("call-spawn"))
	if activityEvent.Type != streams.EventTypeToolUpdate {
		t.Fatalf("reordered activity type = %q, want tool_update", activityEvent.Type)
	}
	if activityEvent.ToolStatus != toolStatusComplete || activityEvent.NormalizedPayload.SubagentTask().Status != "completed" {
		t.Errorf("reordered activity regressed status: event=%q payload=%q", activityEvent.ToolStatus, activityEvent.NormalizedPayload.SubagentTask().Status)
	}
	if got := completeEvent.NormalizedPayload.SubagentTask(); got.SubagentType != "" {
		t.Errorf("completion event snapshot mutated after activity: subagent_type = %q", got.SubagentType)
	}
	if got := activityEvent.NormalizedPayload.SubagentTask(); got.SubagentType != "review_agent" {
		t.Errorf("activity snapshot subagent_type = %q, want review_agent", got.SubagentType)
	}
}

func TestCloneSubagentPayloadDeepCopiesMutableFields(t *testing.T) {
	payload := streams.NewSubagentTask("description", "prompt", "reviewer")
	count := 7
	source := payload.SubagentTask()
	source.Status = "completed"
	source.AgentID = "agent-1"
	source.Model = "gpt-5.2-codex"
	source.ChildSessionID = "child-1"
	source.DurationMs = 123
	source.TotalTokens = 456
	source.ToolUseCount = &count
	source.ResultText = "done"
	source.IsAsync = true
	source.OutputFile = "/tmp/result"
	source.CanReadOutputFile = true
	source.SetIsAuggie(true)

	clone := cloneSubagentPayload(payload).SubagentTask()
	source.Description = "mutated"
	*source.ToolUseCount = 99
	if clone.Description != "description" || clone.ToolUseCount == nil || *clone.ToolUseCount != 7 {
		t.Fatalf("clone changed with source: description/count = %q/%v", clone.Description, clone.ToolUseCount)
	}
	if !clone.IsAuggie() || !clone.IsAsync || clone.OutputFile != "/tmp/result" || !clone.CanReadOutputFile {
		t.Fatalf("clone lost adapter fields: %+v", clone)
	}
}

func TestCodexSubagentActivityDeduplicatesWhileRunning(t *testing.T) {
	a := newTestAdapter()
	a.agentID = codexAgentID
	a.normalizer = NewNormalizer(codexAgentID)
	meta := codexCollaborationMeta(codexCollaborationSpawnAgent, []any{"thread-child"})
	start := &acp.SessionUpdateToolCall{
		ToolCallId: "call-spawn",
		Kind:       "other",
		Title:      codexCollaborationSpawnAgent,
		Status:     toolStatusInProgress,
		RawInput: map[string]any{
			"prompt": "Audit the adapter",
			"agentsStates": map[string]any{
				"thread-child": map[string]any{"status": "running"},
			},
		},
		Meta: meta,
	}
	startEvent := a.convertToolCallUpdate("session-1", start)
	activityEvent := a.convertToolCallUpdate("session-1", codexStartedActivityToolCall("call-spawn"))
	if startEvent.Type != streams.EventTypeToolCall || activityEvent.Type != streams.EventTypeToolUpdate {
		t.Fatalf("event types = %q/%q, want tool_call/tool_update", startEvent.Type, activityEvent.Type)
	}
	if activityEvent.ToolStatus != toolStatusInProgress {
		t.Errorf("activity ToolStatus = %q, want in_progress", activityEvent.ToolStatus)
	}
	if got := activityEvent.NormalizedPayload.SubagentTask().Status; got != "running" {
		t.Errorf("payload status = %q, want running (started must not downgrade it)", got)
	}
	if got := startEvent.NormalizedPayload.SubagentTask().SubagentType; got != "" {
		t.Errorf("start event snapshot mutated after activity: subagent_type = %q", got)
	}
	if got := activityEvent.NormalizedPayload.SubagentTask().SubagentType; got != "review_agent" {
		t.Errorf("activity subagent_type = %q, want review_agent", got)
	}
}

func TestCodexSubagentCorrelationPressureRetainsDelayedMatch(t *testing.T) {
	a := newTestAdapter()
	a.agentID = codexAgentID
	a.normalizer = NewNormalizer(codexAgentID)
	delayed := codexCollaborationToolCall("delayed", "delayed-child", "running")
	if event := a.convertToolCallUpdate("session-1", delayed); event.Type != streams.EventTypeToolCall {
		t.Fatalf("delayed first event type = %q", event.Type)
	}
	for i := 0; i < maxCodexCompletedSubagentCorrelations+20; i++ {
		id := "live-" + strconv.Itoa(i)
		a.convertToolCallUpdate("session-1", codexCollaborationToolCall(id, "child-"+strconv.Itoa(i), "running"))
	}
	wantLive := maxCodexCompletedSubagentCorrelations + 21
	if got := len(a.codexSubagentCorrelations); got != wantLive {
		t.Fatalf("live correlation count = %d, want %d retained until completion", got, wantLive)
	}
	delayedEvent := a.convertToolCallUpdate("session-1", codexStartedActivityToolCallForChild("delayed", "delayed-child"))
	if delayedEvent.Type != streams.EventTypeToolUpdate {
		t.Fatalf("delayed matching event type = %q, want tool_update after cache pressure", delayedEvent.Type)
	}
	key := codexSubagentCorrelationKey{
		sessionID:      "session-1",
		toolCallID:     "delayed",
		childSessionID: "delayed-child",
	}
	correlation := a.codexSubagentCorrelations[key]
	if correlation == nil || !correlation.collaborationSeen || !correlation.activitySeen {
		t.Fatalf("delayed correlation signal state = %+v", correlation)
	}
}

func TestCodexCompletedTombstonesAreBoundedOldestFirst(t *testing.T) {
	a := newCodexCorrelationTestAdapter()
	total := maxCodexCompletedSubagentCorrelations + 20
	for i := 0; i < total; i++ {
		id := "paired-" + strconv.Itoa(i)
		childID := "child-" + strconv.Itoa(i)
		a.convertToolCallUpdate("session-1", codexCollaborationToolCall(id, childID, "running"))
		a.convertToolCallUpdate("session-1", codexStartedActivityToolCallForChild(id, childID))
	}
	if got := a.codexCompletedCorrelationCountLocked(); got != maxCodexCompletedSubagentCorrelations {
		t.Fatalf("completed tombstones = %d, want %d", got, maxCodexCompletedSubagentCorrelations)
	}
	oldest := codexSubagentCorrelationKey{sessionID: "session-1", toolCallID: "paired-0", childSessionID: "child-0"}
	if a.codexSubagentCorrelations[oldest] != nil {
		t.Fatal("oldest completed tombstone survived deterministic pruning")
	}
	newest := codexSubagentCorrelationKey{
		sessionID:      "session-1",
		toolCallID:     "paired-" + strconv.Itoa(total-1),
		childSessionID: "child-" + strconv.Itoa(total-1),
	}
	if a.codexSubagentCorrelations[newest] == nil {
		t.Fatal("newest completed tombstone was pruned")
	}
}

func TestCodexCorrelationSameToolIDDifferentChildCreatesNewCard(t *testing.T) {
	a := newCodexCorrelationTestAdapter()
	first := a.convertToolCallUpdate("session-1", codexCollaborationToolCall("reused", "child-a", "running"))
	firstActivity := a.convertToolCallUpdate("session-1", codexStartedActivityToolCallForChild("reused", "child-a"))
	second := a.convertToolCallUpdate("session-1", codexCollaborationToolCall("reused", "child-b", "running"))
	if first.Type != streams.EventTypeToolCall || firstActivity.Type != streams.EventTypeToolUpdate {
		t.Fatalf("first child event types = %q/%q", first.Type, firstActivity.Type)
	}
	if second.Type != streams.EventTypeToolCall {
		t.Fatalf("different child event type = %q, want tool_call", second.Type)
	}
	if first.ToolCallID == second.ToolCallID {
		t.Fatalf("sibling emitted IDs collide at %q", first.ToolCallID)
	}
	if first.ToolCallID != "reused" {
		t.Fatalf("first child ID = %q, want unchanged wire ID", first.ToolCallID)
	}
	if got := second.NormalizedPayload.SubagentTask().ChildSessionID; got != "child-b" {
		t.Fatalf("different child ID = %q", got)
	}

	lateFirst := a.convertToolCallUpdate("session-1", codexStartedActivityToolCallForChild("reused", "child-a"))
	if lateFirst.Type != streams.EventTypeToolUpdate {
		t.Fatalf("same-child replay type = %q, want tool_update", lateFirst.Type)
	}
	if lateFirst.ToolCallID != first.ToolCallID {
		t.Fatalf("late first child ID = %q, want %q", lateFirst.ToolCallID, first.ToolCallID)
	}
	secondActivity := a.convertToolCallUpdate("session-1", codexStartedActivityToolCallForChild("reused", "child-b"))
	if secondActivity.ToolCallID != second.ToolCallID {
		t.Fatalf("second activity ID = %q, want %q", secondActivity.ToolCallID, second.ToolCallID)
	}
	for id, childID := range map[string]string{first.ToolCallID: "child-a", second.ToolCallID: "child-b"} {
		if active := a.activeToolCalls[id]; codexSubagentChildID(active) != childID {
			t.Fatalf("active payload %q child = %q, want %q", id, codexSubagentChildID(active), childID)
		}
	}
	if got := a.codexCorrelationSiblingCountLocked("session-1", "reused"); got != 2 {
		t.Fatalf("correlation sibling count = %d, want 2", got)
	}

	firstResult := a.convertToolCallResultUpdate(
		"session-1",
		codexCollaborationResultUpdate("reused", "child-a", toolStatusCompleted),
	)
	secondResult := a.convertToolCallResultUpdate(
		"session-1",
		codexCollaborationResultUpdate("reused", "child-b", toolStatusCompleted),
	)
	if firstResult.ToolCallID != first.ToolCallID || secondResult.ToolCallID != second.ToolCallID {
		t.Fatalf(
			"result IDs = %q/%q, want %q/%q",
			firstResult.ToolCallID,
			secondResult.ToolCallID,
			first.ToolCallID,
			second.ToolCallID,
		)
	}
	if a.activeToolCalls[first.ToolCallID] != nil || a.activeToolCalls[second.ToolCallID] != nil {
		t.Fatal("terminal sibling results did not clear their independent active entries")
	}
}

func TestCodexAmbiguousChildlessResultsAreSuppressed(t *testing.T) {
	a := newCodexCorrelationTestAdapter()
	first := a.convertToolCallUpdate("session-1", codexCollaborationToolCall("reused", "child-a", "running"))
	second := a.convertToolCallUpdate("session-1", codexCollaborationToolCall("reused", "child-b", "running"))
	if first.ToolCallID == second.ToolCallID {
		t.Fatalf("persistence-facing sibling IDs collide at %q", first.ToolCallID)
	}
	firstBefore := mustMarshalPayload(t, a.activeToolCalls[first.ToolCallID])
	secondBefore := mustMarshalPayload(t, a.activeToolCalls[second.ToolCallID])

	running := a.convertToolCallResultUpdate(
		"session-1",
		codexChildlessCollaborationResultUpdate("reused", toolStatusInProgress),
	)
	if running != nil {
		t.Fatalf("ambiguous running update emitted persistence event %+v", running)
	}
	terminal := a.convertToolCallResultUpdate(
		"session-1",
		codexChildlessCollaborationResultUpdate("reused", toolStatusCompleted),
	)
	if terminal != nil {
		t.Fatalf("ambiguous terminal update emitted persistence event %+v", terminal)
	}
	lateReplay := a.convertToolCallResultUpdate(
		"session-1",
		codexChildlessCollaborationResultUpdate("reused", toolStatusCompleted),
	)
	if lateReplay != nil {
		t.Fatalf("ambiguous late replay emitted persistence event %+v", lateReplay)
	}

	assertActivePayloadUnchanged(t, a, first.ToolCallID, firstBefore)
	assertActivePayloadUnchanged(t, a, second.ToolCallID, secondBefore)

	firstResult := a.convertToolCallResultUpdate(
		"session-1",
		codexCollaborationResultUpdate("reused", "child-a", toolStatusCompleted),
	)
	if firstResult == nil || firstResult.ToolCallID != first.ToolCallID {
		t.Fatalf("known child-a result = %+v, want emitted ID %q", firstResult, first.ToolCallID)
	}
	if a.activeToolCalls[first.ToolCallID] != nil {
		t.Fatal("known terminal child-a result did not delete child-a active state")
	}
	assertActivePayloadUnchanged(t, a, second.ToolCallID, secondBefore)

	// Both sibling correlations still exist, so a childless replay remains
	// ambiguous even though only child-b is active. It must not complete or
	// delete child-b by falling back to the bare wire ID.
	lateAfterFirstCompletion := a.convertToolCallResultUpdate(
		"session-1",
		codexChildlessCollaborationResultUpdate("reused", toolStatusCompleted),
	)
	if lateAfterFirstCompletion != nil {
		t.Fatalf("post-completion ambiguous replay emitted event %+v", lateAfterFirstCompletion)
	}
	assertActivePayloadUnchanged(t, a, second.ToolCallID, secondBefore)

	secondResult := a.convertToolCallResultUpdate(
		"session-1",
		codexCollaborationResultUpdate("reused", "child-b", toolStatusCompleted),
	)
	if secondResult == nil || secondResult.ToolCallID != second.ToolCallID {
		t.Fatalf("known child-b result = %+v, want emitted ID %q", secondResult, second.ToolCallID)
	}
	if a.activeToolCalls[second.ToolCallID] != nil {
		t.Fatal("known terminal child-b result did not delete child-b active state")
	}
	if got := mustMarshalPayload(t, first.NormalizedPayload); got != firstBefore {
		t.Fatalf("persisted first event snapshot mutated:\nbefore: %s\nafter:  %s", firstBefore, got)
	}
	if got := mustMarshalPayload(t, second.NormalizedPayload); got != secondBefore {
		t.Fatalf("persisted second event snapshot mutated:\nbefore: %s\nafter:  %s", secondBefore, got)
	}
}

func TestCodexSoleChildlessResultReplayUsesCompletedCorrelation(t *testing.T) {
	a := newCodexCorrelationTestAdapter()
	start := a.convertToolCallUpdate("session-1", codexCollaborationToolCall("sole", "child-a", "running"))
	a.convertToolCallUpdate("session-1", codexStartedActivityToolCallForChild("sole", "child-a"))
	completed := a.convertToolCallResultUpdate(
		"session-1",
		codexCollaborationResultUpdate("sole", "child-a", toolStatusCompleted),
	)
	if completed == nil || completed.ToolCallID != start.ToolCallID {
		t.Fatalf("known completion = %+v, want ID %q", completed, start.ToolCallID)
	}
	completedSnapshot := mustMarshalPayload(t, completed.NormalizedPayload)

	replay := a.convertToolCallResultUpdate(
		"session-1",
		codexChildlessCollaborationResultUpdate("sole", toolStatusCompleted),
	)
	if replay == nil || replay.ToolCallID != start.ToolCallID {
		t.Fatalf("sole childless replay = %+v, want ID %q", replay, start.ToolCallID)
	}
	if got := mustMarshalPayload(t, completed.NormalizedPayload); got != completedSnapshot {
		t.Fatalf("completed event snapshot mutated:\nbefore: %s\nafter:  %s", completedSnapshot, got)
	}
}

func TestCodexCorrelationMissingChildFallbackIsUnambiguousOnly(t *testing.T) {
	a := newCodexCorrelationTestAdapter()
	a.convertToolCallUpdate("session-1", codexCollaborationToolCall("fallback", "child-a", "running"))
	missingActivity := a.convertToolCallUpdate("session-1", codexStartedActivityToolCallForChild("fallback", ""))
	if missingActivity.Type != streams.EventTypeToolUpdate {
		t.Fatalf("single incomplete fallback type = %q, want tool_update", missingActivity.Type)
	}
	if got := missingActivity.NormalizedPayload.SubagentTask().ChildSessionID; got != "child-a" {
		t.Fatalf("fallback lost known child ID: %q", got)
	}

	// The correlation is now fully matched. A later overlapping or replayed
	// child-less frame still belongs to the sole compatible child.
	replayed := a.convertToolCallUpdate("session-1", codexChildlessCollaborationToolCall("fallback"))
	if replayed.Type != streams.EventTypeToolUpdate {
		t.Fatalf("child-less frame after completed tombstone = %q, want tool_update", replayed.Type)
	}
	if replayed.ToolCallID != missingActivity.ToolCallID {
		t.Fatalf("replayed child-less ID = %q, want %q", replayed.ToolCallID, missingActivity.ToolCallID)
	}

	// The reverse ordering is also safe: a sole incomplete child-less spawn
	// adopts the child identity from its matching activity and is re-keyed.
	childless := a.convertToolCallUpdate("session-1", codexChildlessCollaborationToolCall("reverse"))
	reverse := a.convertToolCallUpdate("session-1", codexStartedActivityToolCallForChild("reverse", "child-c"))
	if reverse.Type != streams.EventTypeToolUpdate {
		t.Fatalf("reverse fallback type = %q, want tool_update", reverse.Type)
	}
	if reverse.ToolCallID != childless.ToolCallID {
		t.Fatalf("re-keyed child ID changed from %q to %q", childless.ToolCallID, reverse.ToolCallID)
	}
	knownKey := codexSubagentCorrelationKey{
		sessionID:      "session-1",
		toolCallID:     "reverse",
		childSessionID: "child-c",
	}
	if a.codexSubagentCorrelations[knownKey] == nil {
		t.Fatal("reverse fallback was not re-keyed to the discovered child")
	}
}

func TestCodexSiblingNestedChildrenUseEmittedParentIDs(t *testing.T) {
	a := newCodexCorrelationTestAdapter()
	first := a.convertToolCallUpdate("session-1", codexCollaborationToolCall("reused", "child-a", "running"))
	second := a.convertToolCallUpdate("session-1", codexCollaborationToolCall("reused", "child-b", "running"))

	nestedA := codexCollaborationToolCallFrom("nested-a", "child-a", "grandchild-a", "running")
	nestedAEvent := a.convertToolCallUpdate("session-1", nestedA)
	if nestedAEvent.ParentToolCallID != first.ToolCallID {
		t.Fatalf("nested child-a parent = %q, want %q", nestedAEvent.ParentToolCallID, first.ToolCallID)
	}
	nestedB := codexCollaborationToolCallFrom("nested-b", "child-b", "grandchild-b", "running")
	nestedBEvent := a.convertToolCallUpdate("session-1", nestedB)
	if nestedBEvent.ParentToolCallID != second.ToolCallID {
		t.Fatalf("nested child-b parent = %q, want %q", nestedBEvent.ParentToolCallID, second.ToolCallID)
	}

	nestedBActivity := a.convertToolCallUpdate(
		"session-1",
		codexStartedActivityToolCallForChild("nested-b", "grandchild-b"),
	)
	if nestedBActivity.ToolCallID != nestedBEvent.ToolCallID {
		t.Fatalf("nested child update ID = %q, want %q", nestedBActivity.ToolCallID, nestedBEvent.ToolCallID)
	}
	if nestedBActivity.ParentToolCallID != second.ToolCallID {
		t.Fatalf("nested child update parent = %q, want %q", nestedBActivity.ParentToolCallID, second.ToolCallID)
	}
}

func TestCodexCorrelationMissingChildDoesNotChooseAmongChildren(t *testing.T) {
	a := newCodexCorrelationTestAdapter()
	a.convertToolCallUpdate("session-1", codexCollaborationToolCall("ambiguous", "child-a", "running"))
	second := a.convertToolCallUpdate("session-1", codexCollaborationToolCall("ambiguous", "child-b", "running"))
	if second.Type != streams.EventTypeToolCall {
		t.Fatalf("second known child type = %q, want tool_call", second.Type)
	}
	missing := a.convertToolCallUpdate("session-1", codexStartedActivityToolCallForChild("ambiguous", ""))
	if missing.Type != streams.EventTypeToolCall {
		t.Fatalf("ambiguous child-less activity type = %q, want tool_call", missing.Type)
	}
	if got := a.codexCorrelationSiblingCountLocked("session-1", "ambiguous"); got != 3 {
		t.Fatalf("ambiguous correlation count = %d, want 3", got)
	}
}

func TestCodexSiblingEmittedIDsAvoidEncodedAndArbitraryCollisions(t *testing.T) {
	a := newCodexCorrelationTestAdapter()
	first := a.convertToolCallUpdate(
		"session-1",
		codexCollaborationToolCall("wire~codex-subagent~", "child/雪", "running"),
	)
	second := a.convertToolCallUpdate(
		"session-1",
		codexCollaborationToolCall("wire~codex-subagent~", "child~two", "running"),
	)
	if first.ToolCallID == second.ToolCallID {
		t.Fatalf("encoded sibling IDs collide at %q", first.ToolCallID)
	}

	// A later arbitrary ACP wire ID may equal an ID generated for an earlier
	// sibling. The session reservation prevents the two persisted cards from
	// sharing an emitted ID, and that fallback remains stable for updates.
	collidingWireID := second.ToolCallID
	third := a.convertToolCallUpdate(
		"session-1",
		codexCollaborationToolCall(collidingWireID, "child-three", "running"),
	)
	if third.ToolCallID == collidingWireID || third.ToolCallID == first.ToolCallID {
		t.Fatalf("arbitrary wire collision was not escaped: %q", third.ToolCallID)
	}
	thirdUpdate := a.convertToolCallUpdate(
		"session-1",
		codexStartedActivityToolCallForChild(collidingWireID, "child-three"),
	)
	if thirdUpdate.ToolCallID != third.ToolCallID {
		t.Fatalf("escaped ID changed on update: %q/%q", third.ToolCallID, thirdUpdate.ToolCallID)
	}
}

func TestCodexSubagentCorrelationCleanup(t *testing.T) {
	a := newTestAdapter()
	a.agentID = codexAgentID
	a.normalizer = NewNormalizer(codexAgentID)
	a.convertToolCallUpdate("session-1", codexCollaborationToolCall("first", "child-1", "running"))

	a.convertToolCallUpdate("session-2", &acp.SessionUpdateToolCall{
		ToolCallId: "other-session",
		Kind:       "other",
		Status:     toolStatusInProgress,
		Meta:       codexCollaborationMeta(codexCollaborationSpawnAgent, []any{"other-child"}),
	})
	a.mu.Lock()
	a.clearCodexSubagentCorrelationsLocked("session-1")
	remaining := len(a.codexSubagentCorrelations)
	a.mu.Unlock()
	if remaining != 1 {
		t.Fatalf("session cleanup left %d entries, want only the other session", remaining)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := len(a.codexSubagentCorrelations); got != 0 {
		t.Fatalf("Close left %d correlation entries", got)
	}
}

func TestCodexEmittedPayloadIsImmutableDuringQueuedSerialization(t *testing.T) {
	a := newTestAdapter()
	a.agentID = codexAgentID
	a.normalizer = NewNormalizer(codexAgentID)
	first := a.convertToolCallUpdate("session-1", codexCollaborationToolCall("call-race", "child", "running"))
	before, err := json.Marshal(first.NormalizedPayload)
	if err != nil {
		t.Fatalf("marshal before updates: %v", err)
	}

	errs := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2000; i++ {
			if _, marshalErr := json.Marshal(first.NormalizedPayload); marshalErr != nil {
				errs <- marshalErr
				return
			}
		}
	}()
	for i := 0; i < 2000; i++ {
		status := "completed"
		if i%2 == 0 {
			status = "errored"
		}
		a.convertToolCallUpdate("session-1", codexCollaborationToolCall("call-race", "child", status))
	}
	<-done
	select {
	case marshalErr := <-errs:
		t.Fatalf("concurrent marshal: %v", marshalErr)
	default:
	}
	after, err := json.Marshal(first.NormalizedPayload)
	if err != nil {
		t.Fatalf("marshal after updates: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("queued event payload mutated:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestCodexCorrelationDoesNotChangeOtherAgents(t *testing.T) {
	a := newTestAdapter()
	tc := &acp.SessionUpdateToolCall{
		ToolCallId: "same-id",
		Kind:       "other",
		Status:     toolStatusInProgress,
		Meta:       codexCollaborationMeta(codexCollaborationSpawnAgent, []any{"child"}),
	}
	first := a.convertToolCallUpdate("session-1", tc)
	second := a.convertToolCallUpdate("session-1", tc)
	if first.Type != streams.EventTypeToolCall || second.Type != streams.EventTypeToolCall {
		t.Fatalf("non-Codex event types = %q/%q, want unchanged tool_call behavior", first.Type, second.Type)
	}
	if first.ToolCallID != "same-id" || second.ToolCallID != "same-id" {
		t.Fatalf("non-Codex tool call IDs changed: %q/%q", first.ToolCallID, second.ToolCallID)
	}
	if len(a.codexSubagentCorrelations) != 0 {
		t.Fatal("non-Codex tool calls must not populate Codex correlation state")
	}
}

func codexStartedActivityToolCall(toolCallID string) *acp.SessionUpdateToolCall {
	return codexStartedActivityToolCallForChild(toolCallID, "thread-child")
}

func codexStartedActivityToolCallForChild(toolCallID, childID string) *acp.SessionUpdateToolCall {
	return &acp.SessionUpdateToolCall{
		ToolCallId: acp.ToolCallId(toolCallID),
		Kind:       "other",
		Title:      "Start subagent review_agent",
		Status:     toolStatusCompleted,
		RawInput: map[string]any{
			"agentThreadId": childID,
			"agentPath":     "/root/review_agent",
			"activityKind":  codexSubagentStarted,
		},
		Meta: map[string]any{"codex": map[string]any{"subagent": map[string]any{
			"threadId": childID,
			"path":     "/root/review_agent",
			"activity": codexSubagentStarted,
		}}},
	}
}

func codexCollaborationToolCall(toolCallID, childID, status string) *acp.SessionUpdateToolCall {
	return codexCollaborationToolCallFrom(toolCallID, "thread-main", childID, status)
}

func codexCollaborationToolCallFrom(toolCallID, senderID, childID, status string) *acp.SessionUpdateToolCall {
	return &acp.SessionUpdateToolCall{
		ToolCallId: acp.ToolCallId(toolCallID),
		Kind:       "other",
		Title:      codexCollaborationSpawnAgent,
		Status:     toolStatusInProgress,
		RawInput: map[string]any{
			"prompt": "Audit the adapter",
			"agentsStates": map[string]any{
				childID: map[string]any{"status": status},
			},
		},
		Meta: codexCollaborationMetaFrom(codexCollaborationSpawnAgent, senderID, []any{childID}),
	}
}

func codexCollaborationResultUpdate(toolCallID, childID, status string) *acp.SessionToolCallUpdate {
	toolStatus := acp.ToolCallStatus(status)
	return &acp.SessionToolCallUpdate{
		ToolCallId: acp.ToolCallId(toolCallID),
		Status:     &toolStatus,
		RawInput: map[string]any{
			"agentsStates": map[string]any{
				childID: map[string]any{"status": status},
			},
		},
		Meta: codexCollaborationMeta(codexCollaborationSpawnAgent, []any{childID}),
	}
}

func codexChildlessCollaborationResultUpdate(toolCallID, status string) *acp.SessionToolCallUpdate {
	toolStatus := acp.ToolCallStatus(status)
	return &acp.SessionToolCallUpdate{
		ToolCallId: acp.ToolCallId(toolCallID),
		Status:     &toolStatus,
		RawInput:   map[string]any{"prompt": "Unknown child"},
		Meta:       codexCollaborationMeta(codexCollaborationSpawnAgent, nil),
	}
}

func mustMarshalPayload(t *testing.T, payload *streams.NormalizedPayload) string {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(encoded)
}

func assertActivePayloadUnchanged(t *testing.T, a *Adapter, emittedID, want string) {
	t.Helper()
	payload := a.activeToolCalls[emittedID]
	if payload == nil {
		t.Fatalf("active payload %q was deleted", emittedID)
	}
	if got := mustMarshalPayload(t, payload); got != want {
		t.Fatalf("active payload %q mutated:\nbefore: %s\nafter:  %s", emittedID, want, got)
	}
}

func codexChildlessCollaborationToolCall(toolCallID string) *acp.SessionUpdateToolCall {
	return &acp.SessionUpdateToolCall{
		ToolCallId: acp.ToolCallId(toolCallID),
		Kind:       "other",
		Title:      codexCollaborationSpawnAgent,
		Status:     toolStatusInProgress,
		RawInput:   map[string]any{"prompt": "Unknown child"},
		Meta:       codexCollaborationMeta(codexCollaborationSpawnAgent, nil),
	}
}

func newCodexCorrelationTestAdapter() *Adapter {
	a := newTestAdapter()
	a.agentID = codexAgentID
	a.normalizer = NewNormalizer(codexAgentID)
	return a
}

func TestNormalizeToolCall_PlainBashNotSubagent(t *testing.T) {
	n := NewNormalizer("")
	args := map[string]any{
		"meta":      map[string]any{"claudeCode": map[string]any{"toolName": "Bash"}},
		"raw_input": map[string]any{"command": "ls"},
	}
	payload := n.NormalizeToolCall("bash", args)
	if payload.Kind() == streams.ToolKindSubagentTask {
		t.Fatal("plain bash must not normalize as subagent")
	}
}

func TestUpdatePayloadInput_FillsSubagentFields(t *testing.T) {
	n := NewNormalizer("")
	payload := streams.NewSubagentTask("", "", "")
	n.UpdatePayloadInput(payload, map[string]any{
		"description":   "Investigate flaky test",
		"prompt":        "Find the root cause",
		"subagent_type": "general-purpose",
	}, nil)
	sa := payload.SubagentTask()
	if sa.Description != "Investigate flaky test" {
		t.Errorf("Description = %q", sa.Description)
	}
	if sa.Prompt != "Find the root cause" {
		t.Errorf("Prompt = %q", sa.Prompt)
	}
	if sa.SubagentType != "general-purpose" {
		t.Errorf("SubagentType = %q", sa.SubagentType)
	}
}

func TestParentToolUseID(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta map[string]any
		want string
	}{
		{"present", map[string]any{"claudeCode": map[string]any{"parentToolUseId": "toolu_parent"}}, "toolu_parent"},
		{"absent (top-level call)", map[string]any{"claudeCode": map[string]any{"toolName": "Bash"}}, ""},
		{"nil meta", nil, ""},
		{"no claudeCode", map[string]any{"other": 1}, ""},
		{"wrong type", map[string]any{"claudeCode": map[string]any{"parentToolUseId": 123}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parentToolUseID(tc.meta); got != tc.want {
				t.Errorf("parentToolUseID = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEnrichSubagentResult_Claude(t *testing.T) {
	n := NewNormalizer("")
	payload := streams.NewSubagentTask("Investigate", "do it", "")
	meta := map[string]any{"claudeCode": map[string]any{"toolResponse": map[string]any{
		"agentId":           "agent_abc",
		"agentType":         "general-purpose",
		"status":            "completed",
		"totalDurationMs":   float64(12345),
		"totalTokens":       float64(6789),
		"totalToolUseCount": float64(11),
	}}}
	n.EnrichSubagentResult(payload, meta, nil)
	sa := payload.SubagentTask()
	if sa.Status != "completed" || sa.AgentID != "agent_abc" {
		t.Errorf("status/agentId = %q/%q", sa.Status, sa.AgentID)
	}
	if sa.ToolUseCount == nil || *sa.ToolUseCount != 11 {
		t.Errorf("ToolUseCount = %v, want 11", sa.ToolUseCount)
	}
	if sa.DurationMs != 12345 || sa.TotalTokens != 6789 {
		t.Errorf("metrics = %d/%d", sa.DurationMs, sa.TotalTokens)
	}
	if sa.SubagentType != "general-purpose" {
		t.Errorf("SubagentType = %q", sa.SubagentType)
	}
}

// A completed subagent that ran zero tools must serialize tool_use_count: 0
// (not drop it), so the UI can render the "0 tools" chip. Regression test for
// the omitempty + non-zero-guard bug.
func TestEnrichSubagentResult_ClaudeZeroToolUses(t *testing.T) {
	n := NewNormalizer("")
	payload := streams.NewSubagentTask("Investigate", "do it", "")
	meta := map[string]any{"claudeCode": map[string]any{"toolResponse": map[string]any{
		"status":            "completed",
		"totalToolUseCount": float64(0),
	}}}
	n.EnrichSubagentResult(payload, meta, nil)
	sa := payload.SubagentTask()
	if sa.ToolUseCount == nil || *sa.ToolUseCount != 0 {
		t.Fatalf("ToolUseCount = %v, want a non-nil pointer to 0", sa.ToolUseCount)
	}
	out, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"tool_use_count":0`) {
		t.Errorf("expected tool_use_count:0 in JSON, got %s", out)
	}
}

// Agents that don't report a tool count (OpenCode/Cursor) must leave the field
// omitted, not emit a misleading "0 tools".
func TestEnrichSubagentResult_OmitsUnknownToolUseCount(t *testing.T) {
	n := NewNormalizer("")
	payload := streams.NewSubagentTask("Investigate", "do it", "general-purpose")
	out := map[string]any{"metadata": map[string]any{"sessionId": "child_1"}}
	n.EnrichSubagentResult(payload, nil, out)
	if payload.SubagentTask().ToolUseCount != nil {
		t.Errorf("ToolUseCount = %v, want nil (unknown)", payload.SubagentTask().ToolUseCount)
	}
	j, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(j), "tool_use_count") {
		t.Errorf("did not expect tool_use_count in JSON, got %s", j)
	}
}

func TestEnrichSubagentResult_OpenCode(t *testing.T) {
	n := NewNormalizer("")
	payload := streams.NewSubagentTask("Investigate", "do it", "general-purpose")
	rawOutput := map[string]any{"metadata": map[string]any{
		"sessionId":       "ses_child",
		"parentSessionId": "ses_parent",
		"model":           map[string]any{"providerID": "opencode", "modelID": "big-pickle"},
	}}
	n.EnrichSubagentResult(payload, nil, rawOutput)
	sa := payload.SubagentTask()
	if sa.ChildSessionID != "ses_child" {
		t.Errorf("ChildSessionID = %q", sa.ChildSessionID)
	}
	if sa.Model != "opencode/big-pickle" {
		t.Errorf("Model = %q", sa.Model)
	}
}

// Model must not carry a leading/trailing slash when only one of
// providerID/modelID is present.
func TestEnrichSubagentResult_OpenCodePartialModel(t *testing.T) {
	for _, tc := range []struct {
		name, provider, modelID, want string
	}{
		{"modelOnly", "", "big-pickle", "big-pickle"},
		{"providerOnly", "opencode", "", "opencode"},
		{"both", "opencode", "big-pickle", "opencode/big-pickle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := NewNormalizer("")
			payload := streams.NewSubagentTask("d", "p", "general-purpose")
			rawOutput := map[string]any{"metadata": map[string]any{
				"model": map[string]any{"providerID": tc.provider, "modelID": tc.modelID},
			}}
			n.EnrichSubagentResult(payload, nil, rawOutput)
			if got := payload.SubagentTask().Model; got != tc.want {
				t.Errorf("Model = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEnrichSubagentResult_Cursor(t *testing.T) {
	n := NewNormalizer("")
	payload := streams.NewSubagentTask("", "", "")
	n.EnrichSubagentResult(payload, nil, map[string]any{"durationMs": float64(4200), "isBackground": false})
	if payload.SubagentTask().DurationMs != 4200 {
		t.Errorf("DurationMs = %d", payload.SubagentTask().DurationMs)
	}
}

// --- Auggie subagent detection ---

func TestAuggieSubagentTitleFields(t *testing.T) {
	for _, tc := range []struct {
		name, title, wantType, wantDesc string
		wantOK                          bool
	}{
		{"simple", "sub-agent-explore: find the bug", "explore", "find the bug", true},
		{"hyphenated type", "sub-agent-code-review: look at PR 12", "code-review", "look at PR 12", true},
		{"truncated description", "sub-agent-explore: a very long thing (und...", "explore", "a very long thing (und...", true},
		{"no description", "sub-agent-qa:", "qa", "", true},
		{"missing colon", "sub-agent-explore find the bug", "", "", false},
		{"missing prefix", "explore: find the bug", "", "", false},
		{"empty type after prefix", "sub-agent-: foo", "", "", false},
		{"empty title", "", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotDesc, gotOK := auggieSubagentTitleFields(tc.title)
			if gotOK != tc.wantOK || gotType != tc.wantType || gotDesc != tc.wantDesc {
				t.Errorf("got (%q,%q,%v) want (%q,%q,%v)", gotType, gotDesc, gotOK, tc.wantType, tc.wantDesc, tc.wantOK)
			}
		})
	}
}

func TestRecognizeSubagent_AuggieTitleNoInput(t *testing.T) {
	desc, prompt, st, ok := recognizeSubagent(nil, "sub-agent-explore: find the bug", nil)
	if !ok {
		t.Fatal("expected Auggie title to be recognized as subagent")
	}
	if st != "explore" {
		t.Errorf("SubagentType = %q, want explore", st)
	}
	if desc != "find the bug" {
		t.Errorf("Description = %q, want find the bug", desc)
	}
	if prompt != "" {
		t.Errorf("Prompt = %q, want empty (Auggie does not provide it)", prompt)
	}
}

func TestRecognizeSubagent_AuggieDoesNotOverrideRawInput(t *testing.T) {
	// Hypothetical: title looks Auggie-shaped but rawInput carries fuller
	// fields (e.g. a future agent that adopts both). rawInput must win for
	// fields it provides; the title fills only the gaps.
	rawInput := map[string]any{"description": "from rawInput", "subagent_type": "research"}
	desc, _, st, ok := recognizeSubagent(nil, "sub-agent-explore: from title", rawInput)
	if !ok {
		t.Fatal("expected recognition")
	}
	if desc != "from rawInput" {
		t.Errorf("Description = %q, want from rawInput (rawInput wins)", desc)
	}
	if st != "research" {
		t.Errorf("SubagentType = %q, want research (rawInput wins)", st)
	}
}

func TestAuggieSubagentResult_Output(t *testing.T) {
	var res SubagentTaskResult
	if !auggieSubagentResult(map[string]any{"output": "Subagent found the issue: foo.go:42 is wrong."}, &res) {
		t.Fatal("expected Auggie rawOutput.output to be extracted")
	}
	if res.ResultText != "Subagent found the issue: foo.go:42 is wrong." {
		t.Errorf("ResultText = %q", res.ResultText)
	}
}

func TestAuggieSubagentResult_EmptyOrWrongType(t *testing.T) {
	var res SubagentTaskResult
	if auggieSubagentResult(map[string]any{"output": ""}, &res) {
		t.Error("empty output must not register as a result")
	}
	if auggieSubagentResult(map[string]any{"output": 42}, &res) {
		t.Error("non-string output must not register as a result")
	}
}

// extractSubagentResult must NOT pick up Auggie's generic `output` field on
// its own — that path is gated on the payload's IsAuggie flag inside
// EnrichSubagentResult so a future agent emitting `{output: "..."}` alongside
// its own structured metadata can't silently surface as Auggie text.
func TestExtractSubagentResult_IgnoresAuggieOutput(t *testing.T) {
	rawOutput := map[string]any{"output": "this should not be picked up by extract"}
	res, ok := extractSubagentResult(nil, rawOutput)
	if ok {
		t.Error("extractSubagentResult must not recognize bare rawOutput.output")
	}
	if res.ResultText != "" {
		t.Errorf("ResultText = %q, want empty", res.ResultText)
	}
}

func TestEnrichSubagentResult_Auggie(t *testing.T) {
	n := NewNormalizer("")
	payload := streams.NewSubagentTask("find the bug", "", "explore")
	payload.SubagentTask().SetIsAuggie(true)
	n.EnrichSubagentResult(payload, nil, map[string]any{"output": "Done. Bug is at foo.go:42."})
	sa := payload.SubagentTask()
	if sa.ResultText != "Done. Bug is at foo.go:42." {
		t.Errorf("ResultText = %q", sa.ResultText)
	}
	// JSON must surface result_text for the frontend.
	out, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"result_text":"Done. Bug is at foo.go:42."`) {
		t.Errorf("expected result_text in JSON, got %s", out)
	}
}

// EnrichSubagentResult must NOT populate ResultText for non-Auggie payloads
// even when rawOutput happens to carry an `output` string. Guards against the
// regression Greptile/Claude flagged on the original Auggie PR.
func TestEnrichSubagentResult_NonAuggieIgnoresOutput(t *testing.T) {
	n := NewNormalizer("")
	payload := streams.NewSubagentTask("Investigate", "do it", "general-purpose")
	n.EnrichSubagentResult(payload, nil, map[string]any{"output": "stray string from some other agent"})
	if got := payload.SubagentTask().ResultText; got != "" {
		t.Errorf("ResultText = %q, want empty (non-Auggie payload)", got)
	}
}

func TestNormalizeToolCall_AuggieSubagent(t *testing.T) {
	n := NewNormalizer("")
	args := map[string]any{
		"kind":  "other",
		"title": "sub-agent-explore: investigate flaky test",
	}
	payload := n.NormalizeToolCall("other", args)
	if payload.Kind() != streams.ToolKindSubagentTask {
		t.Fatalf("Kind = %q, want subagent_task", payload.Kind())
	}
	sa := payload.SubagentTask()
	if sa.SubagentType != "explore" {
		t.Errorf("SubagentType = %q, want explore", sa.SubagentType)
	}
	if sa.Description != "investigate flaky test" {
		t.Errorf("Description = %q, want investigate flaky test", sa.Description)
	}
	if !sa.IsAuggie() {
		t.Error("IsAuggie() = false, want true (title carries Auggie prefix)")
	}
}

func TestNormalizeToolCall_NonAuggieSubagentNotMarked(t *testing.T) {
	n := NewNormalizer("")
	// OpenCode-style title=task subagent must NOT be flagged as Auggie.
	args := map[string]any{
		"kind":  "other",
		"title": "task",
		"raw_input": map[string]any{
			"description":   "Investigate",
			"prompt":        "Find the bug",
			"subagent_type": "general-purpose",
		},
	}
	payload := n.NormalizeToolCall("other", args)
	if payload.Kind() != streams.ToolKindSubagentTask {
		t.Fatalf("Kind = %q, want subagent_task", payload.Kind())
	}
	if payload.SubagentTask().IsAuggie() {
		t.Error("IsAuggie() = true, want false (OpenCode-style title=task)")
	}
}
