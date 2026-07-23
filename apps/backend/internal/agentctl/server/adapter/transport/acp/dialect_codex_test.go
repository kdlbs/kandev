package acp

import "testing"

func TestParseCodexSubagentFrameCollaboration(t *testing.T) {
	meta := codexCollaborationMeta(codexCollaborationSpawnAgent, []any{"thread-child"})
	rawInput := map[string]any{
		"prompt":          "Inspect the flaky test",
		"model":           "gpt-5.2-codex",
		"reasoningEffort": "high",
		"agentsStates": map[string]any{
			"thread-child": map[string]any{
				"status":  "running",
				"message": "Reading the test suite",
			},
		},
	}

	frame, ok := parseCodexSubagentFrame(meta, "spawnAgent", rawInput)
	if !ok {
		t.Fatal("expected spawnAgent metadata to be recognized")
	}
	if frame.description != "Reading the test suite" {
		t.Errorf("description = %q", frame.description)
	}
	if frame.prompt != "Inspect the flaky test" {
		t.Errorf("prompt = %q", frame.prompt)
	}
	if frame.result.ChildSessionID != "thread-child" {
		t.Errorf("ChildSessionID = %q", frame.result.ChildSessionID)
	}
	if frame.result.Status != "running" {
		t.Errorf("Status = %q", frame.result.Status)
	}
	if frame.result.Model != "gpt-5.2-codex" {
		t.Errorf("Model = %q", frame.result.Model)
	}
}

func TestParseCodexSubagentFrameSpawnFallbacks(t *testing.T) {
	meta := codexCollaborationMeta(codexCollaborationSpawnAgent, nil)
	rawInput := map[string]any{
		"prompt":            "Review the adapter",
		"receiverThreadIds": []string{"thread-from-input"},
		"status":            "inProgress",
	}
	frame, ok := parseCodexSubagentFrame(meta, "", rawInput)
	if !ok {
		t.Fatal("expected partial spawnAgent metadata to be recognized")
	}
	if frame.description != "Review the adapter" {
		t.Errorf("description = %q, want prompt fallback", frame.description)
	}
	if frame.result.ChildSessionID != "thread-from-input" {
		t.Errorf("ChildSessionID = %q", frame.result.ChildSessionID)
	}
	if frame.result.Status != "inProgress" {
		t.Errorf("Status = %q", frame.result.Status)
	}
}

func TestParseCodexSubagentFrameStartedActivity(t *testing.T) {
	meta := map[string]any{"codex": map[string]any{"subagent": map[string]any{
		"threadId": "thread-child",
		"path":     "/root/review_agent",
		"activity": codexSubagentStarted,
	}}}
	frame, ok := parseCodexSubagentFrame(meta, "Start subagent review_agent", nil)
	if !ok {
		t.Fatal("expected started activity to be recognized")
	}
	if frame.result.ChildSessionID != "thread-child" {
		t.Errorf("ChildSessionID = %q", frame.result.ChildSessionID)
	}
	if frame.result.Status != codexSubagentStarted {
		t.Errorf("Status = %q", frame.result.Status)
	}
	if frame.subagentType != "review_agent" || frame.description != "review_agent" {
		t.Errorf("type/description = %q/%q", frame.subagentType, frame.description)
	}
}

func TestParseCodexSubagentFrameRejectsControlsAndMalformedMeta(t *testing.T) {
	tests := []struct {
		name string
		meta map[string]any
	}{
		{"interact", codexCollaborationMeta("interact", []any{"thread-child"})},
		{"wait", codexCollaborationMeta("wait", []any{"thread-child"})},
		{"close", codexCollaborationMeta("close", []any{"thread-child"})},
		{"interacted activity", codexActivityMeta("interacted")},
		{"interrupted activity", codexActivityMeta("interrupted")},
		{"missing codex", map[string]any{"other": map[string]any{}}},
		{"wrong codex type", map[string]any{"codex": "invalid"}},
		{"wrong collaboration type", map[string]any{"codex": map[string]any{"collaboration": 42}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := parseCodexSubagentFrame(test.meta, "", nil); ok {
				t.Fatal("metadata must not create a subagent")
			}
		})
	}
}

func TestParseCodexSubagentFrameMalformedCollectionsAreSafe(t *testing.T) {
	meta := codexCollaborationMeta(codexCollaborationSpawnAgent, "not-an-array")
	rawInput := map[string]any{
		"receiverThreadIds": []any{42, nil},
		"agentsStates":      []any{"not-an-object"},
	}
	frame, ok := parseCodexSubagentFrame(meta, "", rawInput)
	if !ok {
		t.Fatal("the explicit spawnAgent signal should still be recognized")
	}
	if frame.result.ChildSessionID != "" || frame.result.Status != "" {
		t.Errorf("malformed fields leaked into result: %+v", frame.result)
	}
}

func codexCollaborationMeta(tool string, receivers any) map[string]any {
	return codexCollaborationMetaFrom(tool, "thread-main", receivers)
}

func codexCollaborationMetaFrom(tool, sender string, receivers any) map[string]any {
	return map[string]any{"codex": map[string]any{"collaboration": map[string]any{
		"tool":              tool,
		"senderThreadId":    sender,
		"receiverThreadIds": receivers,
	}}}
}

func codexActivityMeta(activity string) map[string]any {
	return map[string]any{"codex": map[string]any{"subagent": map[string]any{
		"threadId": "thread-child",
		"path":     "/root/review_agent",
		"activity": activity,
	}}}
}
