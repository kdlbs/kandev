package streams

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMCPAttachmentCatalogNormalizesEntriesBeforeStorage(t *testing.T) {
	tools := make([]map[string]string, MaxMCPAttachmentTools+2)
	for i := range tools {
		tools[i] = map[string]string{
			"name":        string(rune('z' - i%26)),
			"description": "short",
		}
	}
	tools[0] = map[string]string{"name": "", "description": "ignored"}
	tools[1] = map[string]string{"name": "alpha", "description": strings.Repeat("界", 600)}
	tools[2] = map[string]string{"name": "beta", "description": "second"}
	tools[len(tools)-1] = map[string]string{"name": "aardvark", "description": "late but sorted first"}

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
	var aardvark map[string]any
	for _, value := range stored {
		entry := value.(map[string]any)
		if entry["name"] == "aardvark" {
			aardvark = entry
			break
		}
	}
	if aardvark == nil {
		t.Fatal("stored catalog did not retain the alphabetically first late entry")
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
	if len(history.Current.Evidence) != 1 || len(history.Current.Evidence[0].Tools) != 0 {
		t.Fatalf("evidence retained tool summaries: %+v", history.Current.Evidence)
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

func TestMCPAttachmentCatalogRetainsBoundedSchemaAndEstimatorMetadata(t *testing.T) {
	history := MCPAttachmentHistory{}
	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-1"})
	raw := []byte(`{
		"attachment_attempt_id":"attempt-1",
		"server_name":"kandev",
		"kind":"tools_list_observed",
		"tool_count":1,
		"tool_token_estimator":"o200k_base:mcp-tool-json-v1",
		"tools":[{
			"name":"create_task_kandev",
			"input_schema":{"type":"object","properties":{"title":{"type":"string"}}},
			"estimated_tokens":42
		}]
	}`)
	var evidence MCPAttachmentEvidence
	if err := json.Unmarshal(raw, &evidence); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	if !history.Apply(evidence) {
		t.Fatal("Apply() rejected catalog evidence")
	}

	server, _ := history.CurrentServer("kandev")
	encoded, err := json.Marshal(server)
	if err != nil {
		t.Fatalf("marshal server: %v", err)
	}
	for _, expected := range []string{
		`"tool_token_estimator":"o200k_base:mcp-tool-json-v1"`,
		`"input_schema":{"type":"object","properties":{"title":{"type":"string"}}}`,
		`"estimated_tokens":42`,
	} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("server JSON = %s, want %s", encoded, expected)
		}
	}

	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-2"})
	previous, err := json.Marshal(history.Previous[0])
	if err != nil {
		t.Fatalf("marshal previous attempt: %v", err)
	}
	for _, forbidden := range []string{"input_schema", "estimated_tokens", "tool_token_estimator"} {
		if strings.Contains(string(previous), forbidden) {
			t.Fatalf("historical attempt retained %q: %s", forbidden, previous)
		}
	}
}

func TestMCPAttachmentCatalogOmitsSchemasOverPerToolAndCombinedLimits(t *testing.T) {
	history := MCPAttachmentHistory{}
	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-1"})
	largeValue := strings.Repeat("x", 64*1024)
	tools := []map[string]any{{
		"name":         "oversized",
		"input_schema": map[string]any{"type": "object", "description": largeValue},
	}}
	for index := 0; index < 9; index++ {
		tools = append(tools, map[string]any{
			"name":         "combined-" + string(rune('a'+index)),
			"input_schema": map[string]any{"type": "object", "description": strings.Repeat("y", 60*1024)},
		})
	}
	raw, err := json.Marshal(map[string]any{
		"attachment_attempt_id": history.Current.AttemptID,
		"server_name":           "kandev",
		"kind":                  MCPAttachmentEvidenceToolsListObserved,
		"tool_count":            len(tools),
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

	server, _ := history.CurrentServer("kandev")
	encoded, err := json.Marshal(server)
	if err != nil {
		t.Fatalf("marshal server: %v", err)
	}
	var decoded struct {
		Tools []struct {
			Name                 string          `json:"name"`
			InputSchema          json.RawMessage `json:"input_schema"`
			InputSchemaTruncated bool            `json:"input_schema_truncated"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal server: %v", err)
	}
	var storedBytes int
	var omitted int
	for _, tool := range decoded.Tools {
		storedBytes += len(tool.InputSchema)
		if tool.InputSchemaTruncated {
			omitted++
			if len(tool.InputSchema) != 0 {
				t.Fatalf("truncated tool %q retained partial schema", tool.Name)
			}
		}
	}
	if omitted < 2 {
		t.Fatalf("omitted schemas = %d, want per-tool and combined omissions", omitted)
	}
	if storedBytes > 512*1024 {
		t.Fatalf("stored schema bytes = %d, want at most %d", storedBytes, 512*1024)
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
