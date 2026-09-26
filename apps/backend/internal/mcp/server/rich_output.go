package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	richOutputToolName        = "show_rich_output_kandev"
	richOutputBlockTypeChart  = "chart"
	richOutputBlockTypeFile   = "file"
	richOutputFieldPath       = "path"
	richOutputFieldSessionID  = "session_id"
	maxRichOutputPayloadBytes = 64 * 1024
)

var richOutputInputSchema = json.RawMessage(`{
  "type": "object",
  "description": "One compact Kandev-native presentation. Use semantic data only; Kandev owns layout and styling.",
  "properties": {
    "version": {"type": "integer", "const": 1, "description": "Rich-output contract version. Must be 1."},
    "title": {"type": "string", "minLength": 1, "maxLength": 120, "description": "Plain-text presentation title."},
    "description": {"type": "string", "maxLength": 500, "description": "Optional plain-text context for the full presentation."},
    "blocks": {
      "type": "array",
      "minItems": 1,
      "maxItems": 4,
      "description": "One to four ordered file, chart, or metrics blocks.",
      "items": {
        "oneOf": [
          {
            "type": "object",
            "properties": {
              "type": {"const": "file", "description": "Workspace file preview block."},
              "path": {"type": "string", "minLength": 1, "maxLength": 1024, "description": "Workspace-relative file path."},
              "repo": {"type": "string", "maxLength": 255, "description": "Optional repository discriminator for multi-repository tasks."},
              "title": {"type": "string", "maxLength": 120, "description": "Optional plain-text file title."},
              "caption": {"type": "string", "maxLength": 500, "description": "Optional plain-text reason to inspect the file."},
              "mime_type": {"type": "string", "maxLength": 128, "description": "Optional media type hint."}
            },
            "required": ["type", "path"],
            "additionalProperties": false
          },
          {
            "type": "object",
            "properties": {
              "type": {"const": "chart", "description": "Native line or bar chart with host-owned axes, tooltip, and legend."},
              "chart_type": {"type": "string", "enum": ["line", "bar"], "description": "Use line for trends or time series; bar for category comparisons."},
              "title": {"type": "string", "minLength": 1, "maxLength": 120, "description": "Plain-text chart title."},
              "summary": {"type": "string", "minLength": 1, "maxLength": 500, "description": "Plain-text nonvisual interpretation of the chart."},
              "labels": {
                "type": "array",
                "minItems": 1,
                "maxItems": 100,
                "description": "Inline x-axis labels. Use with inline series, not csv.",
                "items": {"type": "string", "minLength": 1, "maxLength": 120}
              },
              "series": {
                "type": "array",
                "minItems": 1,
                "maxItems": 4,
                "description": "Inline numeric series. Each values array must match labels.",
                "items": {
                  "type": "object",
                  "properties": {
                    "label": {"type": "string", "minLength": 1, "maxLength": 120, "description": "Legend label; include the unit when useful."},
                    "values": {
                      "type": "array",
                      "minItems": 1,
                      "maxItems": 100,
                      "items": {"type": ["number", "null"]}
                    }
                  },
                  "required": ["label", "values"],
                  "additionalProperties": false
                }
              },
              "csv": {
                "type": "object",
                "description": "Load bounded chart data from one workspace-relative CSV instead of inlining rows.",
                "properties": {
                  "path": {"type": "string", "minLength": 1, "maxLength": 1024, "description": "Workspace-relative CSV path ending in .csv."},
                  "repo": {"type": "string", "maxLength": 255, "description": "Optional repository discriminator for a multi-repository task."},
                  "x_column": {"type": "string", "minLength": 1, "maxLength": 120, "description": "Exact CSV header used for x-axis category or time labels. Rows render in file order."},
                  "series": {
                    "type": "array",
                    "minItems": 1,
                    "maxItems": 4,
                    "description": "One to four CSV numeric column mappings.",
                    "items": {
                      "type": "object",
                      "properties": {
                        "column": {"type": "string", "minLength": 1, "maxLength": 120, "description": "Exact CSV header for a numeric column."},
                        "label": {"type": "string", "minLength": 1, "maxLength": 120, "description": "Optional legend label; include the unit when useful. Defaults to column."}
                      },
                      "required": ["column"],
                      "additionalProperties": false
                    }
                  }
                },
                "required": ["path", "x_column", "series"],
                "additionalProperties": false
              }
            },
            "required": ["type", "chart_type", "title", "summary"],
            "oneOf": [
              {
                "required": ["labels", "series"],
                "not": {"required": ["csv"]}
              },
              {
                "required": ["csv"],
                "not": {
                  "anyOf": [
                    {"required": ["labels"]},
                    {"required": ["series"]}
                  ]
                }
              }
            ],
            "additionalProperties": false
          },
          {
            "type": "object",
            "properties": {
              "type": {"const": "metrics", "description": "Compact metric group."},
              "items": {
                "type": "array",
                "minItems": 1,
                "maxItems": 6,
                "items": {
                  "type": "object",
                  "properties": {
                    "label": {"type": "string", "minLength": 1, "maxLength": 120},
                    "value": {"type": "string", "minLength": 1, "maxLength": 200},
                    "detail": {"type": "string", "maxLength": 500}
                  },
                  "required": ["label", "value"],
                  "additionalProperties": false
                }
              }
            },
            "required": ["type", "items"],
            "additionalProperties": false
          }
        ]
      }
    }
  },
  "examples": [
	{"version":1,"title":"Service failures","blocks":[{"type":"chart","chart_type":"bar","title":"Failures by service","summary":"Failed requests in the latest interval.","labels":["A","B"],"series":[{"label":"Count","values":[42,27]}]}]},
    {"version":1,"title":"Latency trend","blocks":[{"type":"chart","chart_type":"line","title":"p95 latency","summary":"Latency across recorded samples.","csv":{"path":"reports/latency.csv","x_column":"recorded_at","series":[{"column":"p95_ms","label":"p95 (ms)"}]}}]},
    {"version":1,"title":"Requests by route","blocks":[{"type":"chart","chart_type":"bar","title":"Route volume","summary":"Request count for each route.","csv":{"path":"reports/routes.csv","x_column":"route","series":[{"column":"requests"}]}}]}
  ],
  "required": ["version", "title", "blocks"],
  "additionalProperties": false
}`)

func (s *Server) registerRichOutputTool() {
	tool := mcplib.NewToolWithRawSchema(
		richOutputToolName,
		`Present one native file preview, metric group, line chart, bar chart, or CSV chart. For an explicit chart, graph, plot, preview, KPI, or metrics request with data, call directly. Do not implement the display as ASCII, SVG, HTML, or with another app. Use Markdown only for small text tables and prose otherwise. Supply semantic data; Kandev owns axes, legends, tooltips, layout, and styling. CSV paths are task-workspace-relative. Label series with units.`,
		richOutputInputSchema,
	)
	tool.Annotations.ReadOnlyHint = mcplib.ToBoolPtr(true)
	tool.Annotations.DestructiveHint = mcplib.ToBoolPtr(false)
	s.mcpServer.AddTool(tool, s.wrapHandler(richOutputToolName, s.showRichOutputHandler()))
}

func (s *Server) showRichOutputHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		if err := validateRichOutput(req.GetArguments()); err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}
		snapshot, err := s.resolveRichOutputCSV(ctx, req.GetArguments())
		if err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}
		if snapshot != nil {
			return mcplib.NewToolResultStructuredOnly(snapshot), nil
		}
		return mcplib.NewToolResultText("Rich output accepted."), nil
	}
}

func validateRichOutput(args map[string]interface{}) error {
	encoded, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("rich output payload is invalid")
	}
	if len(encoded) > maxRichOutputPayloadBytes {
		return fmt.Errorf("rich output exceeds 64 KiB")
	}
	blocks, ok := args["blocks"].([]interface{})
	if !ok {
		return fmt.Errorf("rich output blocks are invalid")
	}
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]interface{})
		if !ok {
			continue
		}
		switch block["type"] {
		case richOutputBlockTypeChart:
			if err := validateRichOutputChart(block); err != nil {
				return err
			}
		case richOutputBlockTypeFile:
			filePath, _ := block[richOutputFieldPath].(string)
			if !isWorkspaceRelativeRichOutputPath(filePath) {
				return fmt.Errorf("file path must be workspace relative")
			}
		}
	}
	return nil
}

func isWorkspaceRelativeRichOutputPath(value string) bool {
	if strings.TrimSpace(value) == "" || strings.ContainsRune(value, '\x00') {
		return false
	}
	normalized := strings.ReplaceAll(value, `\`, "/")
	lower := strings.ToLower(normalized)
	if strings.HasPrefix(normalized, "/") || strings.Contains(lower, "://") || strings.HasPrefix(lower, "data:") {
		return false
	}
	if len(normalized) >= 2 && isASCIILetter(normalized[0]) && normalized[1] == ':' {
		return false
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return false
		}
	}
	return path.Clean(normalized) != "."
}

func isASCIILetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func validateRichOutputChart(block map[string]interface{}) error {
	if rawCSV, exists := block["csv"]; exists {
		csvSource, ok := rawCSV.(map[string]interface{})
		if !ok {
			return fmt.Errorf("chart CSV source is invalid")
		}
		filePath, _ := csvSource[richOutputFieldPath].(string)
		if !isWorkspaceRelativeRichOutputPath(filePath) {
			return fmt.Errorf("CSV path must be workspace relative")
		}
		if !strings.EqualFold(path.Ext(filePath), ".csv") {
			return fmt.Errorf("CSV path must end in .csv")
		}
		return nil
	}
	labels, ok := block["labels"].([]interface{})
	if !ok {
		return fmt.Errorf("chart labels are invalid")
	}
	series, ok := block["series"].([]interface{})
	if !ok {
		return fmt.Errorf("chart series are invalid")
	}
	for _, rawSeries := range series {
		item, ok := rawSeries.(map[string]interface{})
		if !ok {
			return fmt.Errorf("chart series are invalid")
		}
		values, ok := item["values"].([]interface{})
		if !ok || len(values) != len(labels) {
			return fmt.Errorf("chart series values must match labels")
		}
	}
	return nil
}
