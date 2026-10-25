package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.4, AC-AGENTS-AGENT-RICH-OUTPUT-001.16,
// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.2, AC-AGENTS-AGENT-RICH-OUTPUT-003.4
func TestRichOutputCSVHeaderBOMReports(t *testing.T) {
	for _, mode := range []string{ModeTask, ModeOffice} {
		for _, component := range []string{richOutputComponentChart, richOutputComponentTable} {
			for _, header := range []struct{ name, text string }{
				{"quoted BOM", "\ufeff\"service\",cost\n"},
				{"quoted no BOM", "\"service\",cost\n"},
				{"unquoted BOM", "\ufeffservice,cost\n"},
			} {
				t.Run(mode+"/"+component+"/"+header.name, func(t *testing.T) {
					content := header.text + "API,00420\nPayments,\n"
					backend := &testBackend{response: map[string]interface{}{"content": content, "size": len(content)}}
					s := New(backend, "bom-session", "task-1", 10005, newTestLogger(t), "", false, mode)
					result := callTool(t, s, richOutputToolName, csvBOMReportArgs(t, component))
					require.False(t, result.IsError, "valid CSV rejected: %v", result.Content)
					want := richOutputCSVSnapshot{Version: 2, CatalogVersion: 1}
					if component == richOutputComponentChart {
						want.ResolvedCharts = []richOutputResolvedChart{csvBOMExpectedChart()}
					} else {
						want.ResolvedTables = []richOutputResolvedTable{{ElementID: "services", Columns: csvBOMTableColumns(), Rows: [][]string{{"API", "00420"}, {"Payments", ""}}}}
					}
					assertCSVBOMResult(t, want, result)
				})
			}
		}
	}
}

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.5, AC-AGENTS-AGENT-RICH-OUTPUT-002.6,
// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.2, AC-AGENTS-AGENT-RICH-OUTPUT-003.3,
// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.10
func TestRichOutputCSVHeaderBOMSharedSourceSnapshot(t *testing.T) {
	for _, mode := range []string{ModeTask, ModeOffice} {
		t.Run(mode, func(t *testing.T) {
			args := csvBOMReportArgs(t, richOutputComponentTable)
			spec := args["spec"].(map[string]interface{})
			elements := spec["elements"].(map[string]interface{})
			elements["costs"] = csvBOMChartElement()
			elements["report"] = map[string]interface{}{"type": "Stack", "props": map[string]interface{}{"direction": "vertical"}, "children": []string{"services", "costs"}}
			spec["root"] = "report"
			columns := append(csvBOMTableColumns(), richOutputTableColumn{Key: "evidence", Label: "Evidence"})
			richOutputTestProps(args, "services")["columns"] = columns
			content := "\ufeff\"service\",cost,evidence,unused\n\" API \",00420,\"=1+2 <b>\"\"quoted\"\"</b>\",private-sentinel\nPayments,,\" before\ufeffafter \",private-sentinel\n"
			backend := &tableReadBackend{testBackend: &testBackend{response: map[string]interface{}{"content": content, "size": len(content)}}}
			s := New(backend, "bom-session", "task-1", 10005, newTestLogger(t), "", false, mode)
			result := callTool(t, s, richOutputToolName, args)
			require.False(t, result.IsError, "valid shared CSV rejected: %v", result.Content)
			assert.Equal(t, 1, backend.reads)
			assert.Equal(t, map[string]interface{}{"session_id": "bom-session", "path": "services.csv", "repo": "api"}, backend.lastPayload)
			want := richOutputCSVSnapshot{
				Version: 2, CatalogVersion: 1,
				ResolvedCharts: []richOutputResolvedChart{csvBOMExpectedChart()},
				ResolvedTables: []richOutputResolvedTable{{ElementID: "services", Columns: columns, Rows: [][]string{
					{" API ", "00420", "=1+2 <b>\"quoted\"</b>"},
					{"Payments", "", " before\ufeffafter "},
				}}},
			}
			assertCSVBOMResult(t, want, result)
		})
	}
}

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.16, AC-AGENTS-AGENT-RICH-OUTPUT-003.4
func TestRichOutputCSVHeaderBOMStrictControls(t *testing.T) {
	for _, component := range []string{richOutputComponentChart, richOutputComponentTable} {
		t.Run(component+"/header only", func(t *testing.T) {
			backend := &testBackend{response: map[string]interface{}{"content": "\ufeff\"service\",cost\n"}}
			s := New(backend, "bom-session", "task-1", 10005, newTestLogger(t), "", false, ModeTask)
			result := callTool(t, s, richOutputToolName, csvBOMReportArgs(t, component))
			if component == richOutputComponentChart {
				assertCSVBOMError(t, result, "must contain 1 to 100 data rows")
				return
			}
			require.False(t, result.IsError, "valid header-only CSV rejected: %v", result.Content)
			want := richOutputCSVSnapshot{Version: 2, CatalogVersion: 1, ResolvedTables: []richOutputResolvedTable{{ElementID: "services", Columns: csvBOMTableColumns(), Rows: [][]string{}}}}
			assertCSVBOMResult(t, want, result)
		})
		for _, fixture := range csvBOMRejectedFiles() {
			t.Run(component+"/"+fixture.name, func(t *testing.T) {
				backend := &csvBOMRawContentBackend{
					testBackend: &testBackend{response: map[string]interface{}{"size": fixture.size, "is_binary": fixture.binary}},
					content:     fixture.content,
				}
				s := New(backend, "bom-session", "task-1", 10005, newTestLogger(t), "", false, ModeTask)
				result := callTool(t, s, richOutputToolName, csvBOMReportArgs(t, component))
				assertCSVBOMError(t, result, fixture.want)
			})
		}
	}
	for _, value := range []string{"NaN", "+Inf", "-Inf", "1e9999", "\ufeff420"} {
		t.Run("Chart/nonfinite/"+value, func(t *testing.T) {
			backend := &testBackend{response: map[string]interface{}{"content": "\ufeff\"service\",cost\nAPI," + value + "\n"}}
			s := New(backend, "bom-session", "task-1", 10005, newTestLogger(t), "", false, ModeTask)
			result := callTool(t, s, richOutputToolName, csvBOMReportArgs(t, richOutputComponentChart))
			assertCSVBOMError(t, result, "must be a finite number or empty")
		})
	}
}

type csvBOMRejectedFile struct {
	name, content, want string
	size                int
	binary              bool
}

type csvBOMRawContentBackend struct {
	*testBackend
	content string
}

func (b *csvBOMRawContentBackend) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	if err := b.testBackend.RequestPayload(ctx, action, payload, result); err != nil {
		return err
	}
	// Raw content preserves invalid UTF-8 that a JSON fixture would replace.
	result.(*streams.FileContentResponse).Content = b.content
	return nil
}

func csvBOMRejectedFiles() []csvBOMRejectedFile {
	header := "\ufeff\"service\",cost\n"
	boundary := "\ufeff\"service\",cost,unused\nAPI,4,"
	boundary += strings.Repeat("x", maxRichOutputCSVBytes+1-len(boundary))
	return []csvBOMRejectedFile{
		{name: "malformed header", content: "\ufeff\"service,cost\n", want: "CSV header is invalid"},
		{name: "bare header quote", content: "\ufeffser\"vice,cost\n", want: "CSV header is invalid"},
		{name: "malformed row", content: header + "\"API,4\n", want: "CSV row 2 is invalid"},
		{name: "bare row quote", content: header + "AP\"I,4\n", want: "CSV row 2 is invalid"},
		{name: "nonleading BOM", content: " \ufeff\"service\",cost\nAPI,4\n", want: "CSV header is invalid"},
		{name: "repeated BOM", content: "\ufeff" + header + "API,4\n", want: "CSV header is invalid"},
		{name: "duplicate header", content: "\ufeff\"service\",cost,cost\n", want: "is duplicated"},
		{name: "blank header", content: "\ufeff\"service\",cost,\n", want: "must not be blank"},
		{name: "row width", content: header + "API\n", want: "CSV row 2 is invalid"},
		{name: "binary", content: header + "API,4\n", binary: true, want: "CSV source must be UTF-8 text"},
		{name: "invalid UTF8", content: header + "API,\xff\n", want: "CSV source must be UTF-8 text"},
		{name: "reported byte limit", content: header + "API,4\n", size: maxRichOutputCSVBytes + 1, want: "CSV source exceeds 256 KiB"},
		{name: "original content byte limit", content: boundary, want: "CSV source exceeds 256 KiB"},
	}
}

func csvBOMReportArgs(t *testing.T, component string) map[string]interface{} {
	t.Helper()
	args := tableArgs(t)
	if component == richOutputComponentChart {
		spec := args["spec"].(map[string]interface{})
		spec["root"] = "costs"
		spec["elements"] = map[string]interface{}{"costs": csvBOMChartElement()}
	}
	return args
}

func csvBOMChartElement() map[string]interface{} {
	return map[string]interface{}{
		"type": "Chart", "children": []string{},
		"props": map[string]interface{}{
			"chart_type": "bar", "title": "Costs", "summary": "Costs in USD",
			"csv": map[string]interface{}{
				"path": "services.csv", "repo": "api", "x_column": "service",
				"series": []interface{}{map[string]interface{}{"column": "cost", "label": "Cost (USD)"}},
			},
		},
	}
}

func csvBOMTableColumns() []richOutputTableColumn {
	return []richOutputTableColumn{{Key: "service", Label: "Service"}, {Key: "cost", Label: "Cost (USD)"}}
}

func csvBOMExpectedChart() richOutputResolvedChart {
	value := 420.0
	return richOutputResolvedChart{
		ElementID: "costs", Labels: []string{"API", "Payments"},
		Series: []richOutputResolvedSeries{{Label: "Cost (USD)", Values: []*float64{&value, nil}}},
	}
}

func assertCSVBOMResult(t *testing.T, want richOutputCSVSnapshot, result *mcplib.CallToolResult) {
	t.Helper()
	assertJSONEquivalent(t, want, result.StructuredContent)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	var fallback interface{}
	require.NoError(t, json.Unmarshal([]byte(text.Text), &fallback))
	assertJSONEquivalent(t, want, fallback)
}

func assertCSVBOMError(t *testing.T, result *mcplib.CallToolResult, want string) {
	t.Helper()
	require.True(t, result.IsError)
	assert.Nil(t, result.StructuredContent)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, want)
}
