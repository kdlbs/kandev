package mcp

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compositionArgs(t *testing.T, csv bool) map[string]interface{} {
	t.Helper()
	raw := `{"version":2,"catalog_version":1,"title":"Latency","spec":{"root":"latency","elements":{"latency":{"type":"Chart","props":{"chart_type":"line","title":"p95","summary":"Accepted samples","labels":["a"],"series":[{"label":"ms","values":[29.4]}]},"children":[]}}}}`
	if csv {
		raw = `{"version":2,"catalog_version":1,"title":"Latency","spec":{"root":"latency","elements":{"latency":{"type":"Chart","props":{"chart_type":"line","title":"p95","summary":"Accepted samples","csv":{"path":"reports/latency.csv","x_column":"time","series":[{"column":"ms"}]}},"children":[]}}}}`
	}
	var args map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(raw), &args))
	return args
}

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.1, AC-AGENTS-AGENT-RICH-OUTPUT-002.5
func TestRichOutputCompositionAuthoring(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"content": "time,ms\na,29.4\n", "size": 15, "is_binary": false}}
	s := New(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)
	result := callTool(t, s, richOutputToolName, compositionArgs(t, false))
	require.False(t, result.IsError)
	assert.Empty(t, backend.lastAction)
	result = callTool(t, s, richOutputToolName, compositionArgs(t, true))
	require.False(t, result.IsError)
	var snapshot struct {
		Version        int `json:"version"`
		CatalogVersion int `json:"catalog_version"`
		ResolvedCharts []struct {
			ElementID string   `json:"element_id"`
			Labels    []string `json:"labels"`
		} `json:"resolved_charts"`
	}
	encoded, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &snapshot))
	assert.Equal(t, 2, snapshot.Version)
	assert.Equal(t, 1, snapshot.CatalogVersion)
	require.Len(t, snapshot.ResolvedCharts, 1)
	assert.Equal(t, "latency", snapshot.ResolvedCharts[0].ElementID)
	assert.Equal(t, []string{"a"}, snapshot.ResolvedCharts[0].Labels)
}

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.2, AC-AGENTS-AGENT-RICH-OUTPUT-002.3
func TestRichOutputSharedCompositionContract(t *testing.T) {
	encoded, err := os.ReadFile("testdata/rich_output_composition_cases.json")
	require.NoError(t, err)
	var cases []struct {
		Name  string                 `json:"name"`
		Input map[string]interface{} `json:"input"`
		Valid bool                   `json:"valid"`
	}
	require.NoError(t, json.Unmarshal(encoded, &cases))
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			backend := &testBackend{response: map[string]interface{}{"content": "time,ms\na,1\n"}}
			s := New(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)
			err := validateRichOutput(c.Input)
			assert.Equal(t, c.Valid, err == nil)
			if !c.Valid {
				result := callTool(t, s, richOutputToolName, c.Input)
				assert.True(t, result.IsError)
				assert.Empty(t, backend.lastAction)
				if c.Name == "StatusList content limit 5" {
					content := result.Content[0].(mcp.TextContent)
					for _, component := range []string{"Chart", "Metrics", "FilePreview", "Table", "StatusList"} {
						assert.Contains(t, content.Text, component)
					}
				}
			}
		})
	}
}

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.4
func TestRichOutputRejectsStaleAuthoringBeforeCSVRead(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"content": "time,ms\na,1\n"}}
	s := New(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)
	result := callTool(t, s, richOutputToolName, map[string]interface{}{"version": 1, "title": "Cached", "blocks": []interface{}{}})
	require.True(t, result.IsError)
	content := result.Content[0].(mcp.TextContent)
	assert.Contains(t, content.Text, "version 2")
	assert.Empty(t, backend.lastAction)
}

func TestRichOutputDiscoveryExamples(t *testing.T) {
	var schema struct {
		Examples []map[string]interface{} `json:"examples"`
	}
	require.NoError(t, json.Unmarshal(richOutputCatalogSchema, &schema))
	require.GreaterOrEqual(t, len(schema.Examples), 5)
	components := map[string]bool{}
	for _, example := range schema.Examples {
		require.NoError(t, validateRichOutput(example))
		data, err := json.Marshal(example)
		require.NoError(t, err)
		var input richOutputComposition
		require.NoError(t, json.Unmarshal(data, &input))
		for _, element := range input.Spec.Elements {
			components[element.Type] = true
		}
	}
	for _, component := range []string{"Table", "StatusList", "Callout"} {
		assert.True(t, components[component], component)
	}
}

type richOutputCountingBackend struct {
	testBackend
	reads int
}

func (b *richOutputCountingBackend) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	b.reads++
	return b.testBackend.RequestPayload(ctx, action, payload, result)
}

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.5
func TestRichOutputCSVCompositionReadsSharedSourceOnce(t *testing.T) {
	for _, mode := range []string{ModeTask, ModeOffice} {
		t.Run(mode, func(t *testing.T) {
			backend := &richOutputCountingBackend{testBackend: testBackend{response: map[string]interface{}{"content": "time,ms\na,29.4\n", "size": 15, "is_binary": false}}}
			args := compositionArgs(t, true)
			spec := args["spec"].(map[string]interface{})
			elements := spec["elements"].(map[string]interface{})
			elements["second"] = elements["latency"]
			elements["report"] = map[string]interface{}{"type": "Stack", "props": map[string]interface{}{"direction": "vertical"}, "children": []string{"latency", "second"}}
			spec["root"] = "report"
			s := New(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, mode)
			result := callTool(t, s, richOutputToolName, args)
			require.False(t, result.IsError)
			assert.Equal(t, 1, backend.reads)
			encoded, err := json.Marshal(result.StructuredContent)
			require.NoError(t, err)
			var snapshot richOutputCSVSnapshot
			require.NoError(t, json.Unmarshal(encoded, &snapshot))
			require.Len(t, snapshot.ResolvedCharts, 2)
			assert.Equal(t, "latency", snapshot.ResolvedCharts[0].ElementID)
			assert.Equal(t, "second", snapshot.ResolvedCharts[1].ElementID)
			assert.Equal(t, snapshot.ResolvedCharts[0].Labels, snapshot.ResolvedCharts[1].Labels)
		})
	}
}
