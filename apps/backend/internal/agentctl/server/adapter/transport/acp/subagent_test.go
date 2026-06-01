package acp

import (
	"encoding/json"
	"strings"
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
