package acp

import (
	"testing"

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

func TestExtractSubagentResult_Empty(t *testing.T) {
	if _, ok := extractSubagentResult(nil, nil); ok {
		t.Error("expected no result from empty inputs")
	}
}

// --- Subagent (Task) normalization ---

func TestNormalizeToolCall_ClaudeSubagent(t *testing.T) {
	n := NewNormalizer()
	args := map[string]any{
		"meta": map[string]any{"claudeCode": map[string]any{"toolName": "Agent"}},
	}
	payload := n.NormalizeToolCall("Agent", args)
	if payload.Kind() != streams.ToolKindSubagentTask {
		t.Fatalf("Kind = %q, want subagent_task", payload.Kind())
	}
}

func TestNormalizeToolCall_OpenCodeSubagent(t *testing.T) {
	n := NewNormalizer()
	args := map[string]any{"title": "task"}
	payload := n.NormalizeToolCall("", args)
	if payload.Kind() != streams.ToolKindSubagentTask {
		t.Fatalf("Kind = %q, want subagent_task", payload.Kind())
	}
}

func TestNormalizeToolCall_CursorSubagent(t *testing.T) {
	n := NewNormalizer()
	args := map[string]any{
		"title":     "Task: Subagent task",
		"raw_input": map[string]any{"_toolName": "task"},
	}
	payload := n.NormalizeToolCall("", args)
	if payload.Kind() != streams.ToolKindSubagentTask {
		t.Fatalf("Kind = %q, want subagent_task", payload.Kind())
	}
}

func TestNormalizeToolCall_PlainBashNotSubagent(t *testing.T) {
	n := NewNormalizer()
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
	n := NewNormalizer()
	payload := streams.NewSubagentTask("", "", "")
	n.UpdatePayloadInput(payload, map[string]any{
		"description":   "Investigate flaky test",
		"prompt":        "Find the root cause",
		"subagent_type": "general-purpose",
	})
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

func TestEnrichSubagentResult_Claude(t *testing.T) {
	n := NewNormalizer()
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
	if sa.DurationMs != 12345 || sa.TotalTokens != 6789 || sa.ToolUseCount != 11 {
		t.Errorf("metrics = %d/%d/%d", sa.DurationMs, sa.TotalTokens, sa.ToolUseCount)
	}
	if sa.SubagentType != "general-purpose" {
		t.Errorf("SubagentType = %q", sa.SubagentType)
	}
}

func TestEnrichSubagentResult_OpenCode(t *testing.T) {
	n := NewNormalizer()
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

func TestEnrichSubagentResult_Cursor(t *testing.T) {
	n := NewNormalizer()
	payload := streams.NewSubagentTask("", "", "")
	n.EnrichSubagentResult(payload, nil, map[string]any{"durationMs": float64(4200), "isBackground": false})
	if payload.SubagentTask().DurationMs != 4200 {
		t.Errorf("DurationMs = %d", payload.SubagentTask().DurationMs)
	}
}
