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
	case CopilotToolWebFetch, CopilotToolWebSearch, "fetch", "http":
		return n.normalizeHttpRequest(toolNameLower, argsMap)
	default:
		return n.normalizeGeneric(toolName, argsMap)
	}
}

// NormalizeToolResult updates the payload with tool result data.
func (n *CopilotNormalizer) NormalizeToolResult(cachedPayload *streams.NormalizedPayload, result any, isError bool) *streams.NormalizedPayload {
	if cachedPayload == nil {
		// No cached payload, create a generic one with the result
		output := n.extractOutput(result)
		payload := streams.NewGeneric("unknown", nil)
		if payload.Generic() != nil {
			payload.Generic().Output = output
		}
		return payload
	}

	// Update the cached payload with the result
	switch cachedPayload.Kind() {
	case streams.ToolKindShellExec:
		if cachedPayload.ShellExec() != nil {
			output, exitCode := n.parseShellResult(result)
			cachedPayload.ShellExec().Output = &streams.ShellExecOutput{
				Stdout:   output,
				ExitCode: exitCode,
			}
			if isError {
				cachedPayload.ShellExec().Output.Stderr = output
				cachedPayload.ShellExec().Output.Stdout = ""
			}
		}
	case streams.ToolKindHttpRequest:
		if cachedPayload.HttpRequest() != nil {
			if output, ok := n.extractOutput(result).(string); ok {
				cachedPayload.HttpRequest().Response = output
			}
		}
	case streams.ToolKindGeneric:
		if cachedPayload.Generic() != nil {
			cachedPayload.Generic().Output = n.extractOutput(result)
		}
	case streams.ToolKindReadFile:
		if cachedPayload.ReadFile() != nil {
			if output, ok := n.extractOutput(result).(string); ok {
				cachedPayload.ReadFile().Output = &streams.ReadFileOutput{
					Content: output,
				}
			}
		}
	case streams.ToolKindCodeSearch:
		if cachedPayload.CodeSearch() != nil {
			if output, ok := n.extractOutput(result).(string); ok {
				// Parse file list from newline-separated output
				files := n.parseFileList(output)
				cachedPayload.CodeSearch().Output = &streams.CodeSearchOutput{
					Files:     files,
					FileCount: len(files),
				}
			}
		}
	}

	return cachedPayload
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

// extractOutput extracts a string output from various result formats.
func (n *CopilotNormalizer) extractOutput(result any) any {
	if result == nil {
		return nil
	}

	switch r := result.(type) {
	case string:
		return r
	case map[string]any:
		// Try common output field names
		if output, ok := r["output"].(string); ok {
			return output
		}
		if stdout, ok := r["stdout"].(string); ok {
			return stdout
		}
		if content, ok := r["content"].(string); ok {
			return content
		}
		if text, ok := r["text"].(string); ok {
			return text
		}
		return r
	default:
		// Handle struct types (like Copilot SDK's *Result) via JSON round-trip
		if data, err := json.Marshal(result); err == nil {
			var m map[string]any
			if err := json.Unmarshal(data, &m); err == nil {
				// Try to extract content from the unmarshaled map
				if content, ok := m["content"].(string); ok {
					return content
				}
				if output, ok := m["output"].(string); ok {
					return output
				}
				if stdout, ok := m["stdout"].(string); ok {
					return stdout
				}
				if text, ok := m["text"].(string); ok {
					return text
				}
			}
		}
		return result
	}
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
		// Try various output field names used by different tools
		output = shared.GetString(r, "output")
		if output == "" {
			output = shared.GetString(r, "stdout")
		}
		if output == "" {
			output = shared.GetString(r, "content")
		}
		if output == "" {
			output = shared.GetString(r, "text")
		}
		exitCode = shared.GetInt(r, "exitCode")
		if exitCode == 0 {
			exitCode = shared.GetInt(r, "exit_code")
		}
	default:
		// Handle struct types (like Copilot SDK's *Result) via JSON round-trip
		if data, err := json.Marshal(result); err == nil {
			var m map[string]any
			if err := json.Unmarshal(data, &m); err == nil {
				// Try to extract content from the unmarshaled map
				if content, ok := m["content"].(string); ok {
					output = content
				} else if out, ok := m["output"].(string); ok {
					output = out
				} else if stdout, ok := m["stdout"].(string); ok {
					output = stdout
				} else {
					// Fallback: return the JSON string
					output = string(data)
				}
			} else {
				output = string(data)
			}
		}
	}

	// Strip Copilot's "<exited with exit code N>" suffix and extract exit code
	output, exitCode = n.cleanShellOutput(output, exitCode)

	return output, exitCode
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
