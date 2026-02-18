package adapter

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// exitedPattern matches the "<exited with exit code N>" suffix that Copilot appends to shell output.
var exitedPattern = regexp.MustCompile(`\n?<exited with exit code (\d+)>$`)

// Copilot tool name constants.
// Copilot typically uses lowercase snake_case tool names.
const (
	CopilotToolBash      = "bash"
	CopilotToolReadFile  = "read_file"
	CopilotToolWriteFile = "write_file"
	CopilotToolEditFile  = "edit_file"
	CopilotToolGlob      = "glob"
	CopilotToolGrep      = "grep"
	CopilotToolWebFetch  = "web_fetch"
	CopilotToolWebSearch = "web_search"
)

// CopilotNormalizer converts Copilot SDK tool data to NormalizedPayload.
type CopilotNormalizer struct{}

// NewCopilotNormalizer creates a new Copilot normalizer.
func NewCopilotNormalizer() *CopilotNormalizer {
	return &CopilotNormalizer{}
}

// NormalizeToolCall converts Copilot tool call data to NormalizedPayload.
func (n *CopilotNormalizer) NormalizeToolCall(toolName string, args any) *streams.NormalizedPayload {
	// Convert args to map if possible
	argsMap := n.toMap(args)

	// Normalize tool name to lowercase for comparison
	toolNameLower := strings.ToLower(toolName)

	switch toolNameLower {
	case CopilotToolBash, "shell", "execute", "run":
		return n.normalizeExecute(argsMap)
	case CopilotToolReadFile, "read":
		return n.normalizeRead(argsMap)
	case CopilotToolWriteFile, "write":
		return n.normalizeWrite(argsMap)
	case CopilotToolEditFile, "edit":
		return n.normalizeEdit(argsMap)
	case CopilotToolGlob, CopilotToolGrep, "search", "find":
		return n.normalizeCodeSearch(toolNameLower, argsMap)
	case CopilotToolWebFetch, CopilotToolWebSearch, "fetch", mcpServerTypeHTTP:
		return n.normalizeHttpRequest(toolNameLower, argsMap)
	default:
		return n.normalizeGeneric(toolName, argsMap)
	}
}

// NormalizeToolResult updates the payload with tool result data.
func (n *CopilotNormalizer) NormalizeToolResult(cachedPayload *streams.NormalizedPayload, result any, isError bool) *streams.NormalizedPayload {
	if cachedPayload == nil {
		return n.normalizeResultNoCached(result)
	}

	switch cachedPayload.Kind() {
	case streams.ToolKindShellExec:
		n.normalizeShellExecResult(cachedPayload, result, isError)
	case streams.ToolKindHttpRequest:
		n.normalizeHttpRequestResult(cachedPayload, result)
	case streams.ToolKindGeneric:
		n.normalizeGenericResult(cachedPayload, result)
	case streams.ToolKindReadFile:
		n.normalizeReadFileResult(cachedPayload, result)
	case streams.ToolKindCodeSearch:
		n.normalizeCodeSearchResult(cachedPayload, result)
	}

	return cachedPayload
}

// normalizeResultNoCached creates a generic payload when no cached payload exists.
func (n *CopilotNormalizer) normalizeResultNoCached(result any) *streams.NormalizedPayload {
	output := n.extractOutput(result)
	payload := streams.NewGeneric("unknown", nil)
	if payload.Generic() != nil {
		payload.Generic().Output = output
	}
	return payload
}

// normalizeShellExecResult populates ShellExec output from tool result.
func (n *CopilotNormalizer) normalizeShellExecResult(payload *streams.NormalizedPayload, result any, isError bool) {
	if payload.ShellExec() == nil {
		return
	}
	output, exitCode := n.parseShellResult(result)
	payload.ShellExec().Output = &streams.ShellExecOutput{
		Stdout:   output,
		ExitCode: exitCode,
	}
	if isError {
		payload.ShellExec().Output.Stderr = output
		payload.ShellExec().Output.Stdout = ""
	}
}

// normalizeHttpRequestResult populates HttpRequest response from tool result.
func (n *CopilotNormalizer) normalizeHttpRequestResult(payload *streams.NormalizedPayload, result any) {
	if payload.HttpRequest() == nil {
		return
	}
	if output, ok := n.extractOutput(result).(string); ok {
		payload.HttpRequest().Response = output
	}
}

// normalizeGenericResult populates Generic output from tool result.
func (n *CopilotNormalizer) normalizeGenericResult(payload *streams.NormalizedPayload, result any) {
	if payload.Generic() == nil {
		return
	}
	payload.Generic().Output = n.extractOutput(result)
}

// normalizeReadFileResult populates ReadFile output from tool result.
func (n *CopilotNormalizer) normalizeReadFileResult(payload *streams.NormalizedPayload, result any) {
	if payload.ReadFile() == nil {
		return
	}
	if output, ok := n.extractOutput(result).(string); ok {
		payload.ReadFile().Output = &streams.ReadFileOutput{
			Content: output,
		}
	}
}

// normalizeCodeSearchResult populates CodeSearch output from tool result.
func (n *CopilotNormalizer) normalizeCodeSearchResult(payload *streams.NormalizedPayload, result any) {
	if payload.CodeSearch() == nil {
		return
	}
	if output, ok := n.extractOutput(result).(string); ok {
		// Parse file list from newline-separated output
		files := n.parseFileList(output)
		payload.CodeSearch().Output = &streams.CodeSearchOutput{
			Files:     files,
			FileCount: len(files),
		}
	}
}

// toMap converts an argument to a map[string]any.
func (n *CopilotNormalizer) toMap(args any) map[string]any {
	if args == nil {
		return nil
	}
	if m, ok := args.(map[string]any); ok {
		return m
	}
	// Try JSON round-trip for struct types
	if data, err := json.Marshal(args); err == nil {
		var m map[string]any
		if err := json.Unmarshal(data, &m); err == nil {
			return m
		}
	}
	return nil
}

// extractStringFieldFromMap tries common output field names on a map.
func extractStringFieldFromMap(m map[string]any) (string, bool) {
	for _, key := range []string{"output", "stdout", "content", "text"} {
		if v, ok := m[key].(string); ok {
			return v, true
		}
	}
	return "", false
}

// extractOutput extracts a string output from various result formats.
func (n *CopilotNormalizer) extractOutput(result any) any {
	if result == nil {
		return nil
	}

	switch r := result.(type) {
	case string:
		return r
	case map[string]any:
		if v, ok := extractStringFieldFromMap(r); ok {
			return v
		}
		return r
	default:
		return n.extractOutputFromStruct(result)
	}
}

// extractOutputFromStruct handles struct types (like Copilot SDK's *Result) via JSON round-trip.
func (n *CopilotNormalizer) extractOutputFromStruct(result any) any {
	data, err := json.Marshal(result)
	if err != nil {
		return result
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return result
	}
	if v, ok := extractStringFieldFromMap(m); ok {
		return v
	}
	return result
}

// parseShellResult extracts output and exit code from shell execution result.
func (n *CopilotNormalizer) parseShellResult(result any) (string, int) {
	if result == nil {
		return "", 0
	}

	var output string
	var exitCode int

	switch r := result.(type) {
	case string:
		output = r
	case map[string]any:
		output, exitCode = n.parseShellResultFromMap(r)
	default:
		output, exitCode = n.parseShellResultFromStruct(result)
	}

	// Strip Copilot's "<exited with exit code N>" suffix and extract exit code
	output, exitCode = n.cleanShellOutput(output, exitCode)

	return output, exitCode
}

// parseShellResultFromMap extracts output and exit code from a map result.
func (n *CopilotNormalizer) parseShellResultFromMap(r map[string]any) (string, int) {
	output := shared.GetString(r, "output")
	if output == "" {
		output = shared.GetString(r, "stdout")
	}
	if output == "" {
		output = shared.GetString(r, "content")
	}
	if output == "" {
		output = shared.GetString(r, "text")
	}
	exitCode := shared.GetInt(r, "exitCode")
	if exitCode == 0 {
		exitCode = shared.GetInt(r, "exit_code")
	}
	return output, exitCode
}

// parseShellResultFromStruct handles struct types via JSON round-trip.
func (n *CopilotNormalizer) parseShellResultFromStruct(result any) (string, int) {
	data, err := json.Marshal(result)
	if err != nil {
		return "", 0
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return string(data), 0
	}
	if content, ok := m["content"].(string); ok {
		return content, 0
	}
	if out, ok := m["output"].(string); ok {
		return out, 0
	}
	if stdout, ok := m["stdout"].(string); ok {
		return stdout, 0
	}
	return string(data), 0
}

// cleanShellOutput strips Copilot's "<exited with exit code N>" suffix from shell output
// and extracts the exit code if not already set.
func (n *CopilotNormalizer) cleanShellOutput(output string, exitCode int) (string, int) {
	if matches := exitedPattern.FindStringSubmatch(output); matches != nil {
		// Strip the suffix
		output = exitedPattern.ReplaceAllString(output, "")
		// Extract exit code from the suffix if we don't already have one
		if exitCode == 0 && len(matches) > 1 {
			if code, err := strconv.Atoi(matches[1]); err == nil {
				exitCode = code
			}
		}
	}
	return strings.TrimSuffix(output, "\n"), exitCode
}

// normalizeExecute converts shell execution tool data.
func (n *CopilotNormalizer) normalizeExecute(args map[string]any) *streams.NormalizedPayload {
	command := shared.GetString(args, "command")
	if command == "" {
		command = shared.GetString(args, "cmd")
	}
	description := shared.GetString(args, "description")
	timeout := shared.GetInt(args, "timeout")
	background := shared.GetBool(args, "background")

	return streams.NewShellExec(command, "", description, timeout, background)
}

// normalizeRead converts file read tool data.
func (n *CopilotNormalizer) normalizeRead(args map[string]any) *streams.NormalizedPayload {
	filePath := shared.GetString(args, "file_path")
	if filePath == "" {
		filePath = shared.GetString(args, "path")
	}
	offset := shared.GetInt(args, "offset")
	limit := shared.GetInt(args, "limit")

	return streams.NewReadFile(filePath, offset, limit)
}

// normalizeWrite converts file write tool data.
func (n *CopilotNormalizer) normalizeWrite(args map[string]any) *streams.NormalizedPayload {
	filePath := shared.GetString(args, "file_path")
	if filePath == "" {
		filePath = shared.GetString(args, "path")
	}
	content := shared.GetString(args, "content")

	return streams.NewModifyFile(filePath, []streams.FileMutation{
		{
			Type:    streams.MutationCreate,
			Content: content,
		},
	})
}

// normalizeEdit converts file edit tool data.
func (n *CopilotNormalizer) normalizeEdit(args map[string]any) *streams.NormalizedPayload {
	filePath := shared.GetString(args, "file_path")
	if filePath == "" {
		filePath = shared.GetString(args, "path")
	}
	oldContent := shared.GetString(args, "old_string")
	if oldContent == "" {
		oldContent = shared.GetString(args, "old_content")
	}
	newContent := shared.GetString(args, "new_string")
	if newContent == "" {
		newContent = shared.GetString(args, "new_content")
	}

	mutation := streams.FileMutation{
		Type:       streams.MutationPatch,
		OldContent: oldContent,
		NewContent: newContent,
	}

	// Generate unified diff
	if oldContent != "" && newContent != "" {
		mutation.Diff = shared.GenerateUnifiedDiff(oldContent, newContent, filePath, 0)
	}

	return streams.NewModifyFile(filePath, []streams.FileMutation{mutation})
}

// normalizeCodeSearch converts code search tool data.
func (n *CopilotNormalizer) normalizeCodeSearch(toolName string, args map[string]any) *streams.NormalizedPayload {
	path := shared.GetString(args, "path")
	if path == "" {
		path = shared.GetString(args, "directory")
	}
	pattern := shared.GetString(args, "pattern")
	query := shared.GetString(args, "query")
	glob := ""

	if toolName == CopilotToolGlob {
		glob = pattern
	}

	return streams.NewCodeSearch(query, pattern, path, glob)
}

// normalizeHttpRequest converts HTTP request tool data.
func (n *CopilotNormalizer) normalizeHttpRequest(toolName string, args map[string]any) *streams.NormalizedPayload {
	url := shared.GetString(args, "url")
	method := shared.GetString(args, "method")
	if method == "" {
		method = "GET"
	}
	if toolName == CopilotToolWebSearch {
		method = "SEARCH"
	}

	return streams.NewHttpRequest(url, method)
}

// normalizeGeneric wraps unknown tools as generic.
func (n *CopilotNormalizer) normalizeGeneric(toolName string, args map[string]any) *streams.NormalizedPayload {
	return streams.NewGeneric(toolName, args)
}

// parseFileList parses newline-separated file paths from grep/glob output.
func (n *CopilotNormalizer) parseFileList(output string) []string {
	if output == "" {
		return nil
	}
	lines := strings.Split(output, "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}
