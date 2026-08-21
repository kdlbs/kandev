package lifecycle

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestLoadMCPAttachmentHistoryDecodesTypedAndJSONMetadata(t *testing.T) {
	want := streams.MCPAttachmentHistory{
		Version: streams.MCPAttachmentSchemaVersion,
		Current: streams.MCPAttachmentAttempt{
			AttemptID: "attempt-1",
			Servers: []streams.MCPServerAttachment{{
				Name:               "kandev",
				Status:             streams.MCPAttachmentStatusActive,
				ToolTokenEstimator: "o200k_base:mcp-tool-json-v1",
				Tools: []streams.MCPToolSummary{{
					Name:            "create_task_kandev",
					Description:     "Create a task",
					InputSchema:     json.RawMessage(`{"type":"object"}`),
					EstimatedTokens: 23,
				}},
			}},
		},
	}

	for _, raw := range []any{want, mustJSONMap(t, want)} {
		got, ok := LoadMCPAttachmentHistory(raw)
		if !ok {
			t.Fatalf("LoadMCPAttachmentHistory(%T) returned false", raw)
		}
		if len(got.Current.Servers) != 1 {
			t.Fatalf("history = %+v, want one server", got)
		}
		server := got.Current.Servers[0]
		if len(server.Tools) != 1 {
			t.Fatalf("history = %+v, want one tool", got)
		}
		if got.Current.AttemptID != want.Current.AttemptID || server.Status != streams.MCPAttachmentStatusActive || server.Tools[0].Name != "create_task_kandev" || server.Tools[0].EstimatedTokens != 23 || string(server.Tools[0].InputSchema) != `{"type":"object"}` || server.ToolTokenEstimator != "o200k_base:mcp-tool-json-v1" {
			t.Fatalf("history = %+v, want %+v", got, want)
		}
	}
}

func TestLoadMCPAttachmentHistoryRejectsMalformedMetadata(t *testing.T) {
	if _, ok := LoadMCPAttachmentHistory(map[string]any{"version": "not-a-number"}); ok {
		t.Fatal("LoadMCPAttachmentHistory() accepted malformed metadata")
	}
}

func mustJSONMap(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return result
}
