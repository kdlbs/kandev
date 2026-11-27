package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unionReport(t *testing.T, element string) map[string]interface{} {
	t.Helper()
	var args map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(`{"version":2,"catalog_version":1,"title":"Evidence","spec":{"root":"services","elements":{"services":`+element+`}}}`), &args))
	return args
}

func unionErrorText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.True(t, result.IsError)
	require.NotEmpty(t, result.Content)
	return result.Content[0].(mcp.TextContent).Text
}

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.12
func TestToolArgumentValidationSelectsRelevantUnion(t *testing.T) {
	tests := []struct {
		name, element string
		want, absent  []string
	}{
		{"inline table column type", `{"type":"Table","props":{"title":"private-value","columns":["Service"],"rows":[["API"]]},"children":[]}`, []string{"/spec/elements/services/props/columns/0", "expected object", "key", "label"}, []string{"chart_type", "direction", "unknown arguments", "private-value", "missing: \"csv\""}},
		{"csv table column type", `{"type":"Table","props":{"columns":["Service"],"csv":{"path":"services.csv"}},"children":[]}`, []string{"/props/columns/0", "expected object"}, []string{"chart_type", "missing: \"rows\"", "unknown arguments"}},
		{"inline chart labels type", `{"type":"Chart","props":{"chart_type":"line","title":"Latency","summary":"Evidence","labels":"private-value","series":[{"label":"ms","values":[1]}]},"children":[]}`, []string{"/props/labels", "expected array"}, []string{"missing: \"csv\"", "unknown arguments", "private-value"}},
		{"csv chart mapping type", `{"type":"Chart","props":{"chart_type":"line","title":"Latency","summary":"Evidence","csv":{"path":"data.csv","x_column":"x","series":"private-value"}},"children":[]}`, []string{"/props/csv/series", "expected array"}, []string{"missing: \"labels\"", "private-value"}},
		{"stack children bound", `{"type":"Stack","props":{"direction":"vertical"},"children":[]}`, []string{"/services/children", "minItems"}, []string{"unknown arguments", "chart_type", "missing:"}},
		{"status item type", `{"type":"StatusList","props":{"items":["private-value"]},"children":[]}`, []string{"/props/items/0", "expected object", "label", "status"}, []string{"chart_type", "private-value"}},
		{"status enum", `{"type":"StatusList","props":{"items":[{"label":"Build","status":"private-value"}]},"children":[]}`, []string{"/items/0/status", "passed", "failed"}, []string{"chart_type", "private-value"}},
		{"unknown component", `{"type":"private-value","props":{},"children":[]}`, []string{"/services/type", "Table", "Stack", "choose"}, []string{"chart_type", "private-value"}},
		{"non-scalar component", `{"type":[],"props":{},"children":[]}`, []string{"/services/type", "choose", "Table"}, []string{"chart_type"}},
		{"missing component", `{"props":{},"children":[]}`, []string{"/services/type", "choose", "Table"}, []string{"chart_type", "missing: \"csv\""}},
		{"both sources", `{"type":"Table","props":{"columns":[{"key":"service","label":"Service"}],"rows":[],"csv":{"path":"services.csv"}},"children":[]}`, []string{"alternative 1", "alternative 2"}, []string{"chart_type", "direction"}},
		{"neither source", `{"type":"Table","props":{"columns":[{"key":"service","label":"Service"}]},"children":[]}`, []string{"alternative 1", "alternative 2", "rows", "csv"}, []string{"chart_type", "direction"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTaskModeServer(t, &testBackend{}, "task-current")
			msg := unionErrorText(t, callTool(t, s, richOutputToolName, unionReport(t, tt.element)))
			for _, want := range tt.want {
				assert.Contains(t, msg, want)
			}
			for _, absent := range tt.absent {
				assert.NotContains(t, msg, absent)
			}
		})
	}
}

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.13
func TestRichOutputRejectedCallRecovery(t *testing.T) {
	for _, mode := range []string{ModeTask, ModeOffice} {
		t.Run(mode, func(t *testing.T) {
			csv := "service,cost\nAPI,00420\n"
			backend := &tableReadBackend{testBackend: &testBackend{response: map[string]interface{}{"content": csv, "size": len(csv)}}}
			s := New(backend, "recovery-session", "task-1", 10005, newTestLogger(t), "", false, mode)
			bad := tableArgs(t)
			props := bad["spec"].(map[string]interface{})["elements"].(map[string]interface{})["services"].(map[string]interface{})["props"].(map[string]interface{})
			props["columns"] = []interface{}{"private-value"}
			msg := unionErrorText(t, callTool(t, s, richOutputToolName, bad))
			assert.Contains(t, msg, "expected object")
			assert.NotContains(t, msg, "private-value")
			require.Zero(t, backend.reads)
			good := tableArgs(t)
			result := callTool(t, s, richOutputToolName, good)
			require.False(t, result.IsError)
			require.Equal(t, 1, backend.reads)
			assert.Equal(t, "recovery-session", backend.lastPayload.(map[string]interface{})["session_id"])
			assert.Contains(t, string(mustJSON(t, result.StructuredContent)), "00420")
			inline := unionReport(t, `{"type":"Table","props":{"columns":[{"key":"service","label":"Service"}],"rows":[["API"]]},"children":[]}`)
			require.False(t, callTool(t, s, richOutputToolName, inline).IsError)
			assert.Equal(t, 1, backend.reads)
		})
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestToolArgumentUnionIndependentFailures(t *testing.T) {
	args := unionReport(t, `{"type":"Table","props":{"columns":["bad"],"rows":[]},"children":[]}`)
	args["unexpected"] = "private-value"
	elements := args["spec"].(map[string]interface{})["elements"].(map[string]interface{})
	elements["note"] = map[string]interface{}{"type": "Text", "props": map[string]interface{}{}, "children": []interface{}{}}
	msg := unionErrorText(t, callTool(t, newTaskModeServer(t, &testBackend{}, "t1"), richOutputToolName, args))
	assert.Contains(t, msg, "columns/0")
	assert.Contains(t, msg, `missing: "text"`)
	assert.Contains(t, msg, `unknown arguments: "unexpected" at $`)
	assert.NotContains(t, msg, "private-value")
}

func TestToolArgumentUnionGenericAlternatives(t *testing.T) {
	const toolName = "union_test"
	tests := []struct {
		name, schema string
		args         map[string]interface{}
		want         []string
	}{
		{"unresolved", `{"type":"object","properties":{"choice":{"oneOf":[{"type":"object","required":["a"]},{"type":"object","required":["b"]}]}}}`, map[string]interface{}{"choice": map[string]interface{}{}}, []string{"alternative 1", "alternative 2", `"a"`, `"b"`}},
		{"overlap", `{"type":"object","properties":{"choice":{"oneOf":[{"type":"string"},{"type":"string"}]}}}`, map[string]interface{}{"choice": "private-value"}, []string{"oneOf", "multiple alternatives match"}},
		{"allOf", `{"type":"object","properties":{"choice":{"allOf":[{"type":"object","required":["a"]},{"type":"object","required":["b"]}]}}}`, map[string]interface{}{"choice": map[string]interface{}{}}, []string{`"a"`, `"b"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTaskModeServer(t, &testBackend{}, "t1")
			s.mcpServer.AddTool(mcp.NewToolWithRawSchema(toolName, "Test", json.RawMessage(tt.schema)), s.wrapHandler(toolName, s.listWorkspacesHandler()))
			s.rebuildToolArgumentValidators()
			msg := unionErrorText(t, callTool(t, s, toolName, tt.args))
			for _, want := range tt.want {
				assert.Contains(t, msg, want)
			}
			assert.NotContains(t, msg, "private-value")
		})
	}
}

func TestToolArgumentUnionFeedbackBound(t *testing.T) {
	args := unionReport(t, `{"type":"StatusList","props":{"items":[]},"children":[]}`)
	props := args["spec"].(map[string]interface{})["elements"].(map[string]interface{})["services"].(map[string]interface{})["props"].(map[string]interface{})
	items := make([]interface{}, 20)
	for i := range items {
		items[i] = map[string]interface{}{"label": "Check", "status": strings.Repeat("private-value", 100)}
	}
	props["items"] = items
	msg := unionErrorText(t, callTool(t, newTaskModeServer(t, &testBackend{}, "t1"), richOutputToolName, args))
	assert.LessOrEqual(t, len(msg), 4096)
	assert.Contains(t, msg, "omitted")
	assert.NotContains(t, msg, "private-value")
}
