package streams

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMCPAttachmentCatalogNormalizesEntriesBeforeStorage(t *testing.T) {
	tools := make([]map[string]string, MaxMCPAttachmentTools+1)
	for i := range tools {
		tools[i] = map[string]string{
			"name":        string(rune('z' - i%26)),
			"description": "short",
		}
	}
	tools[0] = map[string]string{"name": "", "description": "ignored"}
	tools[1] = map[string]string{"name": "alpha", "description": strings.Repeat("界", 600)}
	tools[2] = map[string]string{"name": "beta", "description": "second"}

	history := MCPAttachmentHistory{}
	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-1"})
	applyCatalogEvidenceJSON(t, history.Current.AttemptID, len(tools), tools, &history)

	server, ok := history.CurrentServer("kandev")
	if !ok {
		t.Fatal("CurrentServer() did not retain the catalog server")
	}
	encoded, err := json.Marshal(server)
	if err != nil {
		t.Fatalf("marshal server: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal server: %v", err)
	}
	stored, ok := got["tools"].([]any)
	if !ok {
		t.Fatalf("stored tools = %#v, want catalog", got["tools"])
	}
	if len(stored) != MaxMCPAttachmentTools {
		t.Fatalf("stored tool count = %d, want %d", len(stored), MaxMCPAttachmentTools)
	}
	if got["tool_catalog_truncated"] != true {
		t.Fatalf("truncated marker = %#v, want true", got["tool_catalog_truncated"])
	}
	if got["tool_count"] != float64(len(tools)) {
		t.Fatalf("tool count = %#v, want %d", got["tool_count"], len(tools))
	}
	var alpha map[string]any
	for _, value := range stored {
		entry := value.(map[string]any)
		if entry["name"] == "alpha" {
			alpha = entry
			break
		}
	}
	if alpha == nil {
		t.Fatal("stored catalog did not contain alpha")
	}
	description, ok := alpha["description"].(string)
	if !ok || len(description) > MaxMCPToolDescriptionBytes || !utf8.ValidString(description) {
		t.Fatalf("description was not safely bounded: bytes=%d valid=%v", len(description), utf8.ValidString(description))
	}
	for _, value := range stored {
		entry := value.(map[string]any)
		if entry["name"] == "" {
			t.Fatal("empty tool name was stored")
		}
	}
}

func TestMCPAttachmentHistorySupersededAttemptDropsCatalogButKeepsCount(t *testing.T) {
	history := MCPAttachmentHistory{}
	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-1"})
	applyCatalogEvidenceJSON(t, history.Current.AttemptID, MaxMCPAttachmentTools+1, []map[string]string{{
		"name": "create_task_kandev", "description": "Create a task",
	}}, &history)

	current, ok := history.CurrentServer("kandev")
	if !ok || len(current.Tools) != 1 {
		t.Fatalf("current catalog = %+v, want one tool", current)
	}
	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-2"})
	if len(history.Previous) != 1 {
		t.Fatalf("previous attempts = %+v, want one attempt", history.Previous)
	}
	previousIndex := mcpAttachmentServerIndex(history.Previous[0].Servers, "kandev")
	if previousIndex == -1 {
		t.Fatalf("previous servers = %+v, want kandev", history.Previous[0].Servers)
	}
	previous := history.Previous[0].Servers[previousIndex]
	if previous.ToolCount != MaxMCPAttachmentTools+1 {
		t.Fatalf("previous server = %+v, want count %d", previous, MaxMCPAttachmentTools+1)
	}
	if len(previous.Tools) != 0 || previous.ToolCatalogTruncated {
		t.Fatalf("previous catalog = %+v, want no entries and no truncation marker", previous)
	}
}

func applyCatalogEvidenceJSON(t *testing.T, attemptID string, toolCount int, tools []map[string]string, history *MCPAttachmentHistory) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"attachment_attempt_id": attemptID,
		"server_name":           "kandev",
		"kind":                  MCPAttachmentEvidenceToolsListObserved,
		"tool_count":            toolCount,
		"tools":                 tools,
	})
	if err != nil {
		t.Fatalf("marshal evidence: %v", err)
	}
	var evidence MCPAttachmentEvidence
	if err := json.Unmarshal(raw, &evidence); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	if !history.Apply(evidence) {
		t.Fatal("Apply() rejected catalog evidence")
	}
}
