package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tableArgs(t *testing.T) map[string]interface{} {
	t.Helper()
	var args map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(`{"version":2,"catalog_version":1,"title":"Services","spec":{"root":"services","elements":{"services":{"type":"Table","props":{"columns":[{"key":"service","label":"Service"},{"key":"cost","label":"Cost (USD)"}],"csv":{"path":"services.csv","repo":"api"}},"children":[]}}}}`), &args))
	return args
}

type tableReadBackend struct {
	*testBackend
	reads int
}

func (b *tableReadBackend) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	b.reads++
	return b.testBackend.RequestPayload(ctx, action, payload, result)
}

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.3, AC-AGENTS-AGENT-RICH-OUTPUT-003.4
func TestRichOutputTableLiteralSnapshot(t *testing.T) {
	for _, mode := range []string{ModeTask, ModeOffice} {
		t.Run(mode, func(t *testing.T) {
			csv := "\ufeffservice,cost,unused\nAPI,00420,secret\nPayments,,secret\n\"<script>\",\"=1+2\",secret\n"
			backend := &testBackend{response: map[string]interface{}{"content": csv, "size": len(csv)}}
			result := callTool(t, New(backend, "table-session", "task-1", 10005, newTestLogger(t), "", false, mode), richOutputToolName, tableArgs(t))
			require.False(t, result.IsError)
			assert.Equal(t, map[string]interface{}{"session_id": "table-session", "path": "services.csv", "repo": "api"}, backend.lastPayload)
			assertJSONEquivalent(t, map[string]interface{}{"version": 2, "catalog_version": 1, "resolved_tables": []interface{}{map[string]interface{}{"element_id": "services", "columns": []interface{}{map[string]interface{}{"key": "service", "label": "Service"}, map[string]interface{}{"key": "cost", "label": "Cost (USD)"}}, "rows": [][]string{{"API", "00420"}, {"Payments", ""}, {"<script>", "=1+2"}}}}}, result.StructuredContent)
		})
	}
}

func TestRichOutputTableCSVBounds(t *testing.T) {
	tests := []struct {
		name, content string
		valid         bool
	}{
		{"header only", "service,cost\n", true},
		{"quoted whitespace", "service,cost\n\" API \",\" 0420 \"\n", true},
		{"missing header", "", false},
		{"duplicate header", "service,cost,cost\n", false},
		{"blank header", "service,cost,\n", false},
		{"missing column", "service,price\nAPI,4\n", false},
		{"row width", "service,cost\nAPI\n", false},
		{"invalid UTF8", "service,cost\nAPI,\xff\n", false},
		{"unicode cell", "service,cost\nAPI," + strings.Repeat("😀", 500) + "\n", true},
		{"oversized cell", "service,cost\nAPI," + strings.Repeat("😀", 501) + "\n", false},
		{"too many rows", "service,cost\n" + strings.Repeat("API,4\n", 101), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &testBackend{response: map[string]interface{}{"content": tt.content, "size": len(tt.content), "is_binary": !utf8.ValidString(tt.content)}}
			result := callTool(t, New(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask), richOutputToolName, tableArgs(t))
			assert.Equal(t, !tt.valid, result.IsError)
			if !tt.valid {
				assert.Nil(t, result.StructuredContent)
			}
		})
	}
}

func TestRichOutputTableSharesChartSourceRead(t *testing.T) {
	args := tableArgs(t)
	spec := args["spec"].(map[string]interface{})
	elements := spec["elements"].(map[string]interface{})
	spec["root"] = "report"
	elements["report"] = map[string]interface{}{"type": "Stack", "props": map[string]interface{}{"direction": "vertical"}, "children": []string{"services", "costs"}}
	elements["costs"] = map[string]interface{}{"type": "Chart", "props": map[string]interface{}{"chart_type": "bar", "title": "Costs", "summary": "Costs in USD", "csv": map[string]interface{}{"path": "services.csv", "repo": "api", "x_column": "service", "series": []interface{}{map[string]interface{}{"column": "cost", "label": "Cost (USD)"}}}}, "children": []string{}}
	backend := &tableReadBackend{testBackend: &testBackend{response: map[string]interface{}{"content": "service,cost\nAPI,00420\n"}}}
	s := New(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)
	result := callTool(t, s, richOutputToolName, args)
	require.False(t, result.IsError)
	assert.Equal(t, 1, backend.reads)
	encoded, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"resolved_charts"`)
	assert.Contains(t, string(encoded), `"resolved_tables"`)
	backend.response = map[string]interface{}{"content": "service,cost\nAPI,not-a-number\n"}
	result = callTool(t, s, richOutputToolName, args)
	require.True(t, result.IsError)
	assert.Nil(t, result.StructuredContent)
}

func TestRichOutputTableReadAndSnapshotLimits(t *testing.T) {
	content := "service,cost\n" + strings.Repeat(strings.Repeat("x", 400)+","+strings.Repeat("y", 400)+"\n", 100)
	for _, response := range []map[string]interface{}{
		{"content": content},
		{"content": "service,cost\nAPI,4\n", "size": maxRichOutputCSVBytes + 1},
		{"is_binary": true, "content": "AP8="},
		{"error": "file unavailable"},
	} {
		backend := &testBackend{response: response}
		result := callTool(t, New(backend, "s1", "t1", 10005, newTestLogger(t), "", false, ModeTask), richOutputToolName, tableArgs(t))
		require.True(t, result.IsError)
		require.Nil(t, result.StructuredContent)
	}
}
