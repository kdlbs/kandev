package mcp

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	richOutputToolName            = "show_rich_output_kandev"
	richOutputFieldPath           = "path"
	richOutputFieldSessionID      = "session_id"
	maxRichOutputPayloadBytes     = 64 * 1024
	richOutputComponentChart      = "Chart"
	richOutputComponentFile       = "FilePreview"
	richOutputComponentTable      = "Table"
	richOutputStaleAuthoringError = "Rich output requires version 2 and catalog_version 1. Refresh tool discovery and send spec.root/spec.elements instead of blocks."
)

//go:embed rich_output_catalog_v1.guidance.txt
var richOutputDiscoveryDescription string

//go:embed rich_output_catalog_v1.schema.json
var richOutputCatalogSchema []byte
var richOutputInputSchema = json.RawMessage(richOutputCatalogSchema)
var richOutputSchema, richOutputSchemaError = compileToolArgumentSchema(richOutputToolName, mcplib.NewToolWithRawSchema(richOutputToolName, "", richOutputInputSchema))

type richOutputComposition struct {
	Spec struct {
		Root     string                       `json:"root"`
		Elements map[string]richOutputElement `json:"elements"`
	} `json:"spec"`
}
type richOutputElement struct {
	Type     string          `json:"type"`
	Props    json.RawMessage `json:"props"`
	Children []string        `json:"children"`
}
type richOutputChartProperties struct {
	Labels []string                   `json:"labels"`
	Series []richOutputResolvedSeries `json:"series"`
	CSV    *richOutputCSVSourceInput  `json:"csv,omitempty"`
}

func (s *Server) registerRichOutputTool() {
	tool := mcplib.NewToolWithRawSchema(richOutputToolName, strings.TrimSpace(richOutputDiscoveryDescription), richOutputInputSchema)
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
	encoded, err := encodeRichOutputPayload(args)
	if err != nil {
		return fmt.Errorf("rich output payload is invalid")
	}
	if len(encoded) > maxRichOutputPayloadBytes {
		return fmt.Errorf("rich output exceeds 64 KiB")
	}
	if args["version"] == float64(1) || args["version"] == 1 {
		return fmt.Errorf("%s", richOutputStaleAuthoringError)
	}
	if richOutputSchemaError != nil {
		return fmt.Errorf("rich output catalog schema is invalid")
	}
	if err := richOutputSchema.Validate(args); err != nil {
		return sanitizedToolArgumentError(richOutputToolName, richOutputSchema, args, err)
	}
	var input richOutputComposition
	if err := json.Unmarshal(encoded, &input); err != nil {
		return fmt.Errorf("rich output composition is invalid")
	}
	return validateRichOutputTree(input)
}
func validateRichOutputTree(input richOutputComposition) error {
	seen := make(map[string]bool)
	contentCount := 0
	var visit func(string, int) error
	visit = func(id string, depth int) error {
		element, exists := input.Spec.Elements[id]
		if !exists {
			return fmt.Errorf("element %s: child reference is missing", id)
		}
		if seen[id] {
			return fmt.Errorf("element %s: repeated reference or cycle", id)
		}
		if depth > 8 {
			return fmt.Errorf("element %s: composition exceeds 8 levels", id)
		}
		seen[id] = true
		if element.Type == richOutputComponentChart || element.Type == richOutputComponentFile || element.Type == "Metrics" || element.Type == richOutputComponentTable || element.Type == "StatusList" {
			contentCount++
			if contentCount > 4 {
				return fmt.Errorf("composition exceeds 4 Chart/Metrics/FilePreview/Table/StatusList elements")
			}
		}
		if err := validateRichOutputContent(element); err != nil {
			return fmt.Errorf("element %s: %w", id, err)
		}
		for _, child := range element.Children {
			if err := visit(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(input.Spec.Root, 1); err != nil {
		return err
	}
	if len(seen) != len(input.Spec.Elements) {
		return fmt.Errorf("composition contains unreachable elements")
	}
	return nil
}
func validateRichOutputContent(element richOutputElement) error {
	if element.Type == richOutputComponentTable {
		return validateRichOutputTable(element)
	}
	if element.Type == richOutputComponentFile {
		var file struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(element.Props, &file); err != nil {
			return err
		}
		if !isWorkspaceRelativeRichOutputPath(file.Path) {
			return fmt.Errorf("file path must be workspace relative")
		}
	}
	if element.Type != richOutputComponentChart {
		return nil
	}
	var chart richOutputChartProperties
	if err := json.Unmarshal(element.Props, &chart); err != nil {
		return err
	}
	if chart.CSV != nil {
		if !isWorkspaceRelativeRichOutputPath(chart.CSV.Path) {
			return fmt.Errorf("CSV path must be workspace relative")
		}
		if !strings.EqualFold(path.Ext(chart.CSV.Path), ".csv") {
			return fmt.Errorf("CSV path must end in .csv")
		}
		return nil
	}
	for _, series := range chart.Series {
		if len(series.Values) != len(chart.Labels) {
			return fmt.Errorf("chart series values must match labels")
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
	meaningful := false
	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return false
		}
		if segment != "" && segment != "." {
			meaningful = true
		}
	}
	return meaningful
}
func isASCIILetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

// Payload limits measure JSON data without optional HTML escaping.
func encodeRichOutputPayload(value interface{}) ([]byte, error) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(encoded.Bytes(), []byte("\n")), nil
}
