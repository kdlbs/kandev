package acp

import (
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// Tool operation type constants.
const (
	toolKindEdit    = "edit"
	toolKindRead    = "read"
	toolKindExecute = "execute"
	toolKindGlob    = "glob"
	toolKindGrep    = "grep"
	toolKindSearch  = "search"

	toolTypeEdit    = "tool_edit"
	toolTypeRead    = "tool_read"
	toolTypeExecute = "tool_execute"
	toolTypeSearch  = "tool_search"
	toolTypeGeneric = "tool_call"

	toolStatusComplete = "complete"
	toolStatusError    = "error"
)

// DetectToolOperationType determines the specific tool operation type from ACP tool data.
// Used for logging and backwards compatibility.
func DetectToolOperationType(toolKind string, args map[string]any) string {
	// Check Auggie's "kind" field first
	if kind, ok := args["kind"].(string); ok {
		switch kind {
		case toolKindEdit:
			return toolTypeEdit
		case toolKindRead:
			// Check if this is a directory read (file listing)
			if rawInput, ok := args["raw_input"].(map[string]any); ok {
				if readType, ok := rawInput["type"].(string); ok && readType == "directory" {
					return toolTypeSearch
				}
			}
			return toolTypeRead
		case toolKindExecute:
			return toolTypeExecute
		}
	}

	// Fallback to tool kind/name matching
	switch strings.ToLower(toolKind) {
	case toolKindEdit:
		return toolTypeEdit
	case toolKindRead, "view":
		return toolTypeRead
	case toolKindExecute, "bash", "run":
		return toolTypeExecute
	case toolKindGlob, toolKindGrep, toolKindSearch:
		return toolTypeSearch
	default:
		return toolTypeGeneric // Generic fallback (intentional: different from tool type constants)
	}
}

// Normalizer converts ACP protocol tool data to NormalizedPayload.
type Normalizer struct{}

// NewNormalizer creates a new ACP normalizer.
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeToolCall converts ACP tool call data to NormalizedPayload.
func (n *Normalizer) NormalizeToolCall(toolName string, args map[string]any) *streams.NormalizedPayload {
	// ACP uses "kind" field to identify tool type
	kind, _ := args["kind"].(string)
	if kind == "" {
		kind = toolName
	}

	switch strings.ToLower(kind) {
	case toolKindEdit:
		return n.normalizeEdit(args)
	case toolKindRead, "view":
		return n.normalizeRead(args)
	case toolKindExecute, "bash", "run", "shell":
		return n.normalizeExecute(args)
	case toolKindGlob, toolKindGrep, toolKindSearch:
		return n.normalizeCodeSearch(toolName, args)
	default:
		return n.normalizeGeneric(toolName, args)
	}
}

// NormalizeToolResult updates the payload with tool result data.
func (n *Normalizer) NormalizeToolResult(payload *streams.NormalizedPayload, result any) {
	// Extract rawOutput.output if result is wrapped
	output := extractRawOutput(result)

	switch payload.Kind() {
	case streams.ToolKindReadFile:
		if payload.ReadFile() != nil && output != "" {
			lines := strings.Count(output, "\n")
			if !strings.HasSuffix(output, "\n") && len(output) > 0 {
				lines++ // Count the last line if it doesn't end with newline
			}
			payload.ReadFile().Output = &streams.ReadFileOutput{
				Content:   output,
				LineCount: lines,
			}
		}
	case streams.ToolKindCodeSearch:
		if payload.CodeSearch() != nil && output != "" {
			// Parse output as file listing (one file per line)
			files := parseFileList(output)
			payload.CodeSearch().Output = &streams.CodeSearchOutput{
				Files:     files,
				FileCount: len(files),
			}
		}
	case streams.ToolKindShellExec:
		if payload.ShellExec() != nil && output != "" {
			// Parse ACP's XML-like shell output format
			exitCode, stdout, stderr := parseShellOutput(output)
			payload.ShellExec().Output = &streams.ShellExecOutput{
				ExitCode: exitCode,
				Stdout:   stdout,
				Stderr:   stderr,
			}
		}
	case streams.ToolKindGeneric:
		if payload.Generic() != nil {
			payload.Generic().Output = result
		}
	}
}

// extractRawOutput gets the output string from ACP result data.
// ACP wraps results in {"rawOutput": {"output": "..."}}
func extractRawOutput(result any) string {
	if result == nil {
		return ""
	}

	// Try direct string
	if s, ok := result.(string); ok {
		return s
	}

	// Try rawOutput.output pattern
	resultMap, ok := result.(map[string]any)
	if !ok {
		return ""
	}

	// Check for rawOutput wrapper
	if rawOutput, ok := resultMap["rawOutput"].(map[string]any); ok {
		if output, ok := rawOutput["output"].(string); ok {
			return output
		}
	}

	// Check for direct output field
	if output, ok := resultMap["output"].(string); ok {
		return output
	}

	return ""
}

// parseFileList parses a newline-separated file listing into a slice of paths.
func parseFileList(output string) []string {
	var files []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip header lines that don't look like paths
		if strings.HasPrefix(line, "Here's") || strings.HasPrefix(line, "Files") {
			continue
		}
		files = append(files, line)
	}
	return files
}

// parseShellOutput parses ACP's XML-like shell output format.
// Format: "...<return-code>N</return-code>...<output>...</output>..."
// Returns exit code, stdout, and stderr (stderr from <stderr> tag if present).
func parseShellOutput(output string) (exitCode int, stdout, stderr string) {
	// Extract return code
	if start := strings.Index(output, "<return-code>"); start != -1 {
		start += len("<return-code>")
		if end := strings.Index(output[start:], "</return-code>"); end != -1 {
			codeStr := strings.TrimSpace(output[start : start+end])
			if code, err := strconv.Atoi(codeStr); err == nil {
				exitCode = code
			}
		}
	}

	// Extract stdout from <output> tag
	if start := strings.Index(output, "<output>"); start != -1 {
		start += len("<output>")
		if end := strings.Index(output[start:], "</output>"); end != -1 {
			stdout = strings.TrimSpace(output[start : start+end])
		}
	}

	// Extract stderr from <stderr> tag if present
	if start := strings.Index(output, "<stderr>"); start != -1 {
		start += len("<stderr>")
		if end := strings.Index(output[start:], "</stderr>"); end != -1 {
			stderr = strings.TrimSpace(output[start : start+end])
		}
	}

	return exitCode, stdout, stderr
}

// normalizeEdit converts ACP edit tool data.
func (n *Normalizer) normalizeEdit(args map[string]any) *streams.NormalizedPayload {
	rawInput, _ := args["raw_input"].(map[string]any)
	if rawInput == nil {
		rawInput = args
	}

	// Get path from raw_input or locations
	path := shared.GetString(rawInput, "path")
	if path == "" {
		path = extractPathFromLocations(args)
	}

	var mutations []streams.FileMutation

	// Check if this is file creation (has file_content) vs str_replace
	if fileContent, ok := rawInput["file_content"].(string); ok {
		mutations = append(mutations, streams.FileMutation{
			Type:    streams.MutationCreate,
			Content: fileContent,
		})
	} else {
		// str_replace operation
		// Only include the diff (not old/new content) to reduce payload size
		oldStr, _ := rawInput["old_str_1"].(string)
		newStr, _ := rawInput["new_str_1"].(string)

		mutation := streams.FileMutation{
			Type: streams.MutationPatch,
		}

		// Add line numbers if available
		if startLine, ok := rawInput["old_str_start_line_number_1"].(float64); ok {
			mutation.StartLine = int(startLine)
		}
		if endLine, ok := rawInput["old_str_end_line_number_1"].(float64); ok {
			mutation.EndLine = int(endLine)
		}

		// Generate unified diff when at least one string is provided
		if oldStr != "" || newStr != "" {
			mutation.Diff = shared.GenerateUnifiedDiff(oldStr, newStr, path, mutation.StartLine)
		}

		mutations = append(mutations, mutation)
	}

	// Use factory function
	return streams.NewModifyFile(path, mutations)
}

// normalizeRead converts ACP read tool data.
// If rawInput.type is "directory", this becomes a code search (file listing) operation.
func (n *Normalizer) normalizeRead(args map[string]any) *streams.NormalizedPayload {
	rawInput, _ := args["raw_input"].(map[string]any)
	if rawInput == nil {
		rawInput = args
	}

	path := shared.GetString(rawInput, "path")
	if path == "" {
		path = extractPathFromLocations(args)
	}

	// Check if this is a directory read - treat as code search (file listing)
	if readType := shared.GetString(rawInput, "type"); readType == "directory" {
		return streams.NewCodeSearch("", "", path, "")
	}

	return streams.NewReadFile(path, 0, 0)
}

// normalizeExecute converts ACP execute/bash tool data.
func (n *Normalizer) normalizeExecute(args map[string]any) *streams.NormalizedPayload {
	rawInput, _ := args["raw_input"].(map[string]any)
	if rawInput == nil {
		rawInput = args
	}

	command := shared.GetString(rawInput, "command")
	workDir := shared.GetString(rawInput, "cwd")
	timeout := shared.GetInt(rawInput, "max_wait_seconds")

	// Background is true if wait is explicitly false
	background := false
	if wait, ok := rawInput["wait"].(bool); ok && !wait {
		background = true
	}

	return streams.NewShellExec(command, workDir, "", timeout, background)
}

// normalizeCodeSearch converts ACP search tool data.
func (n *Normalizer) normalizeCodeSearch(toolName string, args map[string]any) *streams.NormalizedPayload {
	rawInput, _ := args["raw_input"].(map[string]any)
	if rawInput == nil {
		rawInput = args
	}

	path := shared.GetString(rawInput, "path")
	pattern := shared.GetString(rawInput, "pattern")

	var query, glob string
	switch strings.ToLower(toolName) {
	case toolKindGlob:
		glob = pattern
	case toolKindGrep, toolKindSearch:
		query = shared.GetString(rawInput, "query")
	}

	return streams.NewCodeSearch(query, pattern, path, glob)
}

// normalizeGeneric wraps unknown tools as generic.
func (n *Normalizer) normalizeGeneric(toolName string, args map[string]any) *streams.NormalizedPayload {
	return streams.NewGeneric(toolName, args)
}

// --- Helper functions ---

func extractPathFromLocations(args map[string]any) string {
	locations, ok := args["locations"].([]any)
	if !ok || len(locations) == 0 {
		return ""
	}
	loc, ok := locations[0].(map[string]any)
	if !ok {
		return ""
	}
	path, _ := loc["path"].(string)
	return path
}
