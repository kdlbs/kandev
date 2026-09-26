package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRichOutputToolRegistration(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want bool
	}{
		{name: "task", mode: ModeTask, want: true},
		{name: "office", mode: ModeOffice, want: true},
		{name: "configuration", mode: ModeConfig, want: false},
		{name: "external", mode: ModeExternal, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(&testBackend{}, "session-1", "task-1", 10005, newTestLogger(t), "", false, tt.mode)
			_, got := s.mcpServer.ListTools()["show_rich_output_kandev"]
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRichOutputToolAcceptsValidPayload(t *testing.T) {
	s := New(&testBackend{}, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)

	result := callTool(t, s, richOutputToolName, validRichOutputArgs())

	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
}

func TestRichOutputToolResolvesCSVChartSnapshot(t *testing.T) {
	csv := "recorded_at,p50_ms,p95_ms\n2026-08-13T10:00:00Z,18.2,29.4\n2026-08-14T10:00:00Z,16.4,\n"
	backend := &testBackend{response: map[string]interface{}{
		"path": "reports/latency.csv", "content": csv, "size": len(csv), "is_binary": false,
	}}
	s := New(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)

	result := callTool(t, s, richOutputToolName, csvRichOutputArgs())

	require.False(t, result.IsError)
	assert.Equal(t, ws.ActionWorkspaceFileContentGet, backend.lastAction)
	assert.Equal(t, map[string]interface{}{
		"session_id": "session-1", "path": "reports/latency.csv", "repo": "api",
	}, backend.lastPayload)
	want := map[string]interface{}{
		"version": 1,
		"resolved_charts": []interface{}{
			map[string]interface{}{
				"block_index": 0,
				"labels":      []interface{}{"2026-08-13T10:00:00Z", "2026-08-14T10:00:00Z"},
				"series": []interface{}{
					map[string]interface{}{"label": "p50", "values": []interface{}{18.2, 16.4}},
					map[string]interface{}{"label": "p95_ms", "values": []interface{}{29.4, nil}},
				},
			},
		},
	}
	assertJSONEquivalent(t, want, result.StructuredContent)
	require.Len(t, result.Content, 1)
	textContent, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	var fallback map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textContent.Text), &fallback))
	assertJSONEquivalent(t, want, fallback)
}

func TestRichOutputToolRejectsInvalidCSVData(t *testing.T) {
	tooManyRows := "x,value\n"
	for i := 0; i < 101; i++ {
		tooManyRows += fmt.Sprintf("%d,%d\n", i, i)
	}
	tooLongLabel := strings.Repeat("x", 121)
	tests := []struct {
		name    string
		content string
		size    int
		binary  bool
		mutate  func(map[string]interface{})
		want    string
	}{
		{name: "binary", content: "AAEC", size: 3, binary: true, want: "CSV source must be UTF-8 text"},
		{name: "oversized", content: "x,value\na,1\n", size: 256*1024 + 1, want: "CSV source exceeds 256 KiB"},
		{name: "header only", content: "x,value\n", size: 8, want: "CSV source must contain 1 to 100 data rows"},
		{name: "too many rows", content: tooManyRows, size: len(tooManyRows), want: "CSV source must contain at most 100 data rows"},
		{name: "duplicate header", content: "x,value,value\na,1,2\n", size: 20, want: `CSV header "value" is duplicated`},
		{name: "missing x column", content: "when,value\na,1\n", size: 15, want: `CSV column "x" was not found`},
		{name: "missing series column", content: "x,other\na,1\n", size: 12, want: `CSV column "value" was not found`},
		{name: "blank x value", content: "x,value\n,1\n", size: 11, want: `CSV row 2 column "x" must not be blank`},
		{name: "long x value", content: "x,value\n" + tooLongLabel + ",1\n", size: 132, want: `CSV row 2 column "x" exceeds 120 characters`},
		{name: "bad numeric value", content: "x,value\na,fast\n", size: 15, want: `CSV row 2 column "value" must be a finite number or empty`},
		{name: "uneven row", content: "x,value\na,1,extra\n", size: 18, want: "CSV row 2"},
		{
			name: "unsafe path", content: "x,value\na,1\n", size: 12, want: "CSV path must be workspace relative",
			mutate: func(args map[string]interface{}) {
				csvChartSource(args)["path"] = "../report.csv"
			},
		},
		{
			name: "non CSV path", content: "x,value\na,1\n", size: 12, want: "CSV path must end in .csv",
			mutate: func(args map[string]interface{}) {
				csvChartSource(args)["path"] = "reports/report.txt"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &testBackend{response: map[string]interface{}{
				"path": "reports/latency.csv", "content": tt.content, "size": tt.size, "is_binary": tt.binary,
			}}
			s := New(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)
			args := csvRichOutputArgs()
			source := csvChartSource(args)
			source["x_column"] = "x"
			source["series"] = []interface{}{map[string]interface{}{"column": "value"}}
			if tt.mutate != nil {
				tt.mutate(args)
			}

			result := callTool(t, s, richOutputToolName, args)

			require.True(t, result.IsError)
			content, ok := result.Content[0].(mcp.TextContent)
			require.True(t, ok)
			assert.Contains(t, content.Text, tt.want)
		})
	}
}

func TestRichOutputToolReportsCSVReadFailure(t *testing.T) {
	backend := &testBackend{err: fmt.Errorf("workspace offline")}
	s := New(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)

	result := callTool(t, s, richOutputToolName, csvRichOutputArgs())

	require.True(t, result.IsError)
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, content.Text, "could not read CSV source reports/latency.csv")
}

func TestRichOutputToolDescriptionStaysFocusedAndDiscoverable(t *testing.T) {
	s := New(&testBackend{}, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)
	tool := s.mcpServer.ListTools()[richOutputToolName].Tool

	for _, phrase := range []string{"file preview", "line chart", "bar chart", "CSV chart", "graph", "plot", "KPI", "metrics"} {
		assert.Contains(t, tool.Description, phrase)
	}
	for _, phrase := range []string{
		"explicit chart, graph, plot, preview, KPI, or metrics request with data, call directly",
		"Do not implement the display as ASCII, SVG, HTML, or with another app",
		"Kandev owns axes, legends, tooltips, layout, and styling",
		"task-workspace-relative",
		"Markdown only for small text tables",
		"Label series with units",
	} {
		assert.Contains(t, tool.Description, phrase)
	}
	assert.NotContains(t, tool.Description, "Inline chart recipe")
	assert.NotContains(t, tool.Description, "CSV chart recipe")
	assert.NotContains(t, tool.Description, `"labels":["A","B"]`)
	assert.NotContains(t, tool.Description, `"x_column":"recorded_at"`)

	encoded := tool.RawInputSchema
	require.NotEmpty(t, encoded)
	for _, phrase := range []string{"workspace-relative CSV", "x-axis", "numeric column"} {
		assert.Contains(t, string(encoded), phrase)
	}
	assert.Contains(t, string(encoded), `"examples"`)
	assert.Contains(t, string(encoded), `"chart_type":"bar"`)
	assert.Contains(t, string(encoded), `"chart_type":"line"`)
	assert.Contains(t, string(encoded), `"labels":["A","B"]`)
	assert.Contains(t, string(encoded), `"series":[{"label":"Count","values":[42,27]}]`)
	require.NotNil(t, tool.Annotations.ReadOnlyHint)
	assert.True(t, *tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestRichOutputToolRejectsMismatchedChartSeries(t *testing.T) {
	s := New(&testBackend{}, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)
	args := validRichOutputArgs()
	chart := args["blocks"].([]interface{})[1].(map[string]interface{})
	series := chart["series"].([]interface{})[0].(map[string]interface{})
	series["values"] = []interface{}{18.2}

	result := callTool(t, s, richOutputToolName, args)

	require.True(t, result.IsError)
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, content.Text, "chart series values must match labels")
}

func TestRichOutputToolRejectsUnsafeFilePaths(t *testing.T) {
	tests := []string{
		"/etc/passwd",
		"../secrets.txt",
		"reports/../../secrets.txt",
		"https://example.com/report.json",
		"data:text/plain,secret",
		`C:\\Users\\person\\secret.txt`,
		`\\\\server\\share\\secret.txt`,
	}

	for _, filePath := range tests {
		t.Run(filePath, func(t *testing.T) {
			s := New(&testBackend{}, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)
			args := validRichOutputArgs()
			file := args["blocks"].([]interface{})[2].(map[string]interface{})
			file["path"] = filePath

			result := callTool(t, s, richOutputToolName, args)

			require.True(t, result.IsError)
			content, ok := result.Content[0].(mcp.TextContent)
			require.True(t, ok)
			assert.Contains(t, content.Text, "file path must be workspace relative")
		})
	}
}

func TestRichOutputToolRejectsOversizedPayload(t *testing.T) {
	labels := make([]interface{}, 100)
	values := make([]interface{}, 100)
	for i := range labels {
		labels[i] = strings.Repeat("a", 120)
		values[i] = 123456789012345.5
	}
	blocks := make([]interface{}, 4)
	for i := range blocks {
		series := make([]interface{}, 4)
		for j := range series {
			series[j] = map[string]interface{}{
				"label":  strings.Repeat("s", 120),
				"values": values,
			}
		}
		blocks[i] = map[string]interface{}{
			"type":       "chart",
			"chart_type": "bar",
			"title":      "Large chart",
			"summary":    "Bounded presentation",
			"labels":     labels,
			"series":     series,
		}
	}
	args := map[string]interface{}{"version": 1, "title": "Too large", "blocks": blocks}
	encoded, err := json.Marshal(args)
	require.NoError(t, err)
	require.Greater(t, len(encoded), 64*1024)
	s := New(&testBackend{}, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)

	result := callTool(t, s, richOutputToolName, args)

	require.True(t, result.IsError)
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, content.Text, "rich output exceeds 64 KiB")
}

func TestRichOutputToolRejectsSchemaViolations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]interface{})
	}{
		{
			name: "unsupported version",
			mutate: func(args map[string]interface{}) {
				args["version"] = 2
			},
		},
		{
			name: "unknown top-level field",
			mutate: func(args map[string]interface{}) {
				args["theme"] = "neon"
			},
		},
		{
			name: "no blocks",
			mutate: func(args map[string]interface{}) {
				args["blocks"] = []interface{}{}
			},
		},
		{
			name: "unknown block type",
			mutate: func(args map[string]interface{}) {
				args["blocks"] = []interface{}{map[string]interface{}{"type": "table"}}
			},
		},
		{
			name: "too many metrics",
			mutate: func(args map[string]interface{}) {
				items := make([]interface{}, 7)
				for i := range items {
					items[i] = map[string]interface{}{"label": "Metric", "value": "1"}
				}
				args["blocks"] = []interface{}{map[string]interface{}{"type": "metrics", "items": items}}
			},
		},
		{
			name: "too many chart labels",
			mutate: func(args map[string]interface{}) {
				labels := make([]interface{}, 101)
				values := make([]interface{}, 101)
				for i := range labels {
					labels[i] = "run"
					values[i] = i
				}
				args["blocks"] = []interface{}{map[string]interface{}{
					"type":       "chart",
					"chart_type": "line",
					"title":      "Runs",
					"summary":    "Run history",
					"labels":     labels,
					"series": []interface{}{
						map[string]interface{}{"label": "Seconds", "values": values},
					},
				}}
			},
		},
		{
			name: "CSV chart with stray inline labels",
			mutate: func(args map[string]interface{}) {
				args["blocks"] = csvRichOutputArgs()["blocks"]
				chart := args["blocks"].([]interface{})[0].(map[string]interface{})
				chart["labels"] = []interface{}{"run-1"}
			},
		},
		{
			name: "inline chart with stray CSV source",
			mutate: func(args map[string]interface{}) {
				chart := args["blocks"].([]interface{})[1].(map[string]interface{})
				chart["csv"] = csvChartSource(csvRichOutputArgs())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &testBackend{}
			s := New(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)
			args := validRichOutputArgs()
			tt.mutate(args)

			result := callTool(t, s, richOutputToolName, args)

			require.True(t, result.IsError)
			assert.Empty(t, backend.lastAction)
		})
	}
}

func validRichOutputArgs() map[string]interface{} {
	return map[string]interface{}{
		"version":     1,
		"title":       "Build health",
		"description": "Latest local verification",
		"blocks": []interface{}{
			map[string]interface{}{
				"type": "metrics",
				"items": []interface{}{
					map[string]interface{}{"label": "Passed", "value": "38"},
				},
			},
			map[string]interface{}{
				"type":       "chart",
				"chart_type": "line",
				"title":      "Runtime by run",
				"summary":    "Runtime fell across the last three runs.",
				"labels":     []interface{}{"1", "2", "3"},
				"series": []interface{}{
					map[string]interface{}{"label": "Seconds", "values": []interface{}{18.2, 16.4, nil}},
				},
			},
			map[string]interface{}{
				"type":      "file",
				"path":      "reports/build.json",
				"repo":      "kandev",
				"title":     "Raw report",
				"caption":   "Generated by verification",
				"mime_type": "application/json",
			},
		},
	}
}

func csvRichOutputArgs() map[string]interface{} {
	return map[string]interface{}{
		"version": 1,
		"title":   "API latency",
		"blocks": []interface{}{
			map[string]interface{}{
				"type":       "chart",
				"chart_type": "line",
				"title":      "Latency over time",
				"summary":    "p50 and p95 latency from the workspace report.",
				"csv": map[string]interface{}{
					"path":     "reports/latency.csv",
					"repo":     "api",
					"x_column": "recorded_at",
					"series": []interface{}{
						map[string]interface{}{"column": "p50_ms", "label": "p50"},
						map[string]interface{}{"column": "p95_ms"},
					},
				},
			},
		},
	}
}

func csvChartSource(args map[string]interface{}) map[string]interface{} {
	chart := args["blocks"].([]interface{})[0].(map[string]interface{})
	return chart["csv"].(map[string]interface{})
}

func assertJSONEquivalent(t *testing.T, want, got interface{}) {
	t.Helper()
	normalize := func(value interface{}) interface{} {
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		var normalized interface{}
		require.NoError(t, json.Unmarshal(encoded, &normalized))
		return normalized
	}
	assert.Equal(t, normalize(want), normalize(got))
}
