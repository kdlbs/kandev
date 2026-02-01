package adapter

import (
	"encoding/json"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// Amp tool name constants (matching Claude Code tool names).
const (
	AmpToolBash         = "Bash"
	AmpToolRead         = "Read"
	AmpToolWrite        = "Write"
	AmpToolEdit         = "Edit"
	AmpToolGlob         = "Glob"
	AmpToolGrep         = "Grep"
	AmpToolWebFetch     = "WebFetch"
	AmpToolWebSearch    = "WebSearch"
	AmpToolTask         = "Task"
	AmpToolTaskCreate   = "TaskCreate"
	AmpToolTaskUpdate   = "TaskUpdate"
	AmpToolTaskList     = "TaskList"
	AmpToolNotebookEdit = "NotebookEdit"
	AmpToolTodoWrite    = "TodoWrite"
)

// AmpNormalizer converts Amp protocol tool data to NormalizedPayload.
type AmpNormalizer struct{}

// NewAmpNormalizer creates a new Amp normalizer.
func NewAmpNormalizer() *AmpNormalizer {
	return &AmpNormalizer{}
}

// NormalizeToolCall converts Amp tool call data to NormalizedPayload.
func (n *AmpNormalizer) NormalizeToolCall(toolName string, args map[string]any) *streams.NormalizedPayload {
	switch toolName {
	case AmpToolEdit, AmpToolWrite, AmpToolNotebookEdit:
		return n.normalizeEdit(toolName, args)
	case AmpToolRead:
		return n.normalizeRead(args)
	case AmpToolGlob, AmpToolGrep:
		return n.normalizeCodeSearch(toolName, args)
	case AmpToolBash:
		return n.normalizeExecute(args)
	case AmpToolWebFetch, AmpToolWebSearch:
		return n.normalizeHttpRequest(toolName, args)
	case AmpToolTask:
		return n.normalizeSubagentTask(args)
	case AmpToolTaskCreate:
		return n.normalizeCreateTask(args)
	case AmpToolTaskUpdate, AmpToolTaskList:
		return n.normalizeManageTodos(toolName, args)
	case AmpToolTodoWrite:
		return n.normalizeManageTodos(toolName, args)
	default:
		return n.normalizeGeneric(toolName, args)
	}
}

// NormalizeToolResult updates the payload with tool result data from Amp.
// Amp's tool results are JSON-wrapped strings like "{\"output\":\"...\",\"exitCode\":0}".
func (n *AmpNormalizer) NormalizeToolResult(cachedPayload *streams.NormalizedPayload, content any, isError bool) *streams.NormalizedPayload {
	if cachedPayload == nil {
		// No cached payload, create a generic one with the result
		output, _ := n.parseToolResultContent(content)
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
			output, exitCode := n.parseToolResultContent(content)
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
			output, _ := n.parseToolResultContent(content)
			cachedPayload.HttpRequest().Response = output
		}
	case streams.ToolKindGeneric:
		if cachedPayload.Generic() != nil {
			output, _ := n.parseToolResultContent(content)
			cachedPayload.Generic().Output = output
		}
	case streams.ToolKindReadFile:
		// Parse Amp's read result format: {absolutePath, content, isDirectory, directoryEntries}
		if cachedPayload.ReadFile() != nil {
			result := n.parseAmpReadResult(content)
			if result != nil {
				// If it's a directory, convert to code_search
				if result.IsDirectory {
					codeSearch := streams.NewCodeSearch(
						"",                  // query
						"",                  // pattern
						result.AbsolutePath, // path
						"*",                 // glob - all entries
					)
					if codeSearch.CodeSearch() != nil {
						codeSearch.CodeSearch().Output = &streams.CodeSearchOutput{
							Files:     result.DirectoryEntries,
							FileCount: len(result.DirectoryEntries),
						}
					}
					return codeSearch
				}
				// Regular file read
				output := &streams.ReadFileOutput{
					Content: result.Content,
				}
				if result.Content != "" {
					output.LineCount = strings.Count(result.Content, "\n") + 1
				}
				cachedPayload.ReadFile().Output = output
			}
		}
	case streams.ToolKindCodeSearch:
		if cachedPayload.CodeSearch() != nil {
			output, _ := n.parseToolResultContent(content)
			shared.NormalizeCodeSearchResult(cachedPayload.CodeSearch(), output)
		}
	}

	return cachedPayload
}

// ampReadResult holds parsed read result data from Amp.
type ampReadResult struct {
	AbsolutePath     string
	Content          string
	IsDirectory      bool
	DirectoryEntries []string
}

// parseAmpReadResult parses Amp's read result format.
func (n *AmpNormalizer) parseAmpReadResult(content any) *ampReadResult {
	switch c := content.(type) {
	case string:
		var result ampReadResult
		if err := json.Unmarshal([]byte(c), &result); err == nil {
			return &result
		}
		// Not JSON, treat as raw file content
		return &ampReadResult{Content: c}

	case map[string]any:
		result := &ampReadResult{}
		if v, ok := c["absolutePath"].(string); ok {
			result.AbsolutePath = v
		}
		if v, ok := c["content"].(string); ok {
			result.Content = v
		}
		if v, ok := c["isDirectory"].(bool); ok {
			result.IsDirectory = v
		}
		if v, ok := c["directoryEntries"].([]any); ok {
			for _, entry := range v {
				if s, ok := entry.(string); ok {
					result.DirectoryEntries = append(result.DirectoryEntries, s)
				}
			}
		}
		return result

	default:
		return &ampReadResult{}
	}
}


// parseToolResultContent extracts the output string and exit code from Amp's JSON-wrapped content.
// Amp sends tool results in formats like:
// - String: "{\"output\":\"actual content\",\"exitCode\":0}"
// - Direct string (non-JSON)
// - Map with output/exitCode keys
// - Any other JSON structure (returned as-is)
func (n *AmpNormalizer) parseToolResultContent(content any) (string, int) {
	switch c := content.(type) {
	case string:
		// Try to parse as JSON-wrapped content with {output, exitCode} structure
		var wrapped struct {
			Output   string `json:"output"`
			ExitCode int    `json:"exitCode"`
		}
		if err := json.Unmarshal([]byte(c), &wrapped); err == nil && wrapped.Output != "" {
			// Successfully parsed AND has output field
			return wrapped.Output, wrapped.ExitCode
		}
		// Either not JSON, or JSON without "output" field - use as-is
		return c, 0

	case map[string]any:
		// Handle map format (already parsed JSON)
		output := shared.GetString(c, "output")
		exitCode := shared.GetInt(c, "exitCode")
		if output == "" {
			// Try alternative field names
			output = shared.GetString(c, "stdout")
			if stderr := shared.GetString(c, "stderr"); stderr != "" && output == "" {
				output = stderr
			}
			if ec := shared.GetInt(c, "exit_code"); ec != 0 {
				exitCode = ec
			}
		}
		return output, exitCode

	case []any:
		// Handle array of content blocks (some tools return multiple blocks)
		var sb strings.Builder
		for _, item := range c {
			if str, ok := item.(string); ok {
				sb.WriteString(str)
			} else if m, ok := item.(map[string]any); ok {
				if text := shared.GetString(m, "text"); text != "" {
					sb.WriteString(text)
				}
			}
		}
		return sb.String(), 0

	default:
		// Unknown format, try JSON marshaling for debugging
		if data, err := json.Marshal(content); err == nil {
			return string(data), 0
		}
		return "", 0
	}
}

// normalizeEdit converts Amp Edit/Write tool data.
func (n *AmpNormalizer) normalizeEdit(toolName string, args map[string]any) *streams.NormalizedPayload {
	filePath := shared.GetString(args, "file_path")
	oldString := shared.GetString(args, "old_string")
	newString := shared.GetString(args, "new_string")
	content := shared.GetString(args, "content")

	var mutations []streams.FileMutation

	if toolName == AmpToolWrite {
		// Write is a full file creation/replacement
		mutations = append(mutations, streams.FileMutation{
			Type:    streams.MutationCreate,
			Content: content,
		})
	} else {
		// Edit is a patch operation
		mutation := streams.FileMutation{
			Type:       streams.MutationPatch,
			OldContent: oldString,
			NewContent: newString,
		}

		// Generate unified diff
		if oldString != "" && newString != "" {
			mutation.Diff = shared.GenerateUnifiedDiff(oldString, newString, filePath, 0)
		}

		mutations = append(mutations, mutation)
	}

	return streams.NewModifyFile(filePath, mutations)
}

// normalizeRead converts Amp Read tool data.
// Note: Amp uses "path" for the file path, while Claude Code uses "file_path".
func (n *AmpNormalizer) normalizeRead(args map[string]any) *streams.NormalizedPayload {
	// Amp uses "path", Claude Code uses "file_path" - check both
	filePath := shared.GetString(args, "path")
	if filePath == "" {
		filePath = shared.GetString(args, "file_path")
	}
	offset := shared.GetInt(args, "offset")
	limit := shared.GetInt(args, "limit")

	return streams.NewReadFile(filePath, offset, limit)
}

// normalizeCodeSearch converts Amp Glob/Grep tool data.
func (n *AmpNormalizer) normalizeCodeSearch(toolName string, args map[string]any) *streams.NormalizedPayload {
	path := shared.GetString(args, "path")
	pattern := shared.GetString(args, "pattern")

	var query, glob string
	switch toolName {
	case AmpToolGlob:
		glob = pattern
	case AmpToolGrep:
		query = shared.GetString(args, "query")
		if query == "" {
			query = pattern
		}
	}

	return streams.NewCodeSearch(query, pattern, path, glob)
}

// normalizeExecute converts Amp Bash tool data.
// Note: Amp uses "cmd" for the command field, while Claude Code uses "command".
func (n *AmpNormalizer) normalizeExecute(args map[string]any) *streams.NormalizedPayload {
	// Amp uses "cmd", Claude Code uses "command" - check both
	command := shared.GetString(args, "cmd")
	if command == "" {
		command = shared.GetString(args, "command")
	}
	description := shared.GetString(args, "description")
	timeout := shared.GetInt(args, "timeout")
	background := shared.GetBool(args, "run_in_background")

	return streams.NewShellExec(command, "", description, timeout, background)
}

// normalizeHttpRequest converts Amp WebFetch/WebSearch tool data.
func (n *AmpNormalizer) normalizeHttpRequest(toolName string, args map[string]any) *streams.NormalizedPayload {
	url := shared.GetString(args, "url")
	if url == "" {
		url = shared.GetString(args, "query")
	}

	method := "GET"
	if toolName == AmpToolWebSearch {
		method = "SEARCH"
	}

	return streams.NewHttpRequest(url, method)
}

// normalizeSubagentTask converts Amp Task tool data (subagent invocation).
func (n *AmpNormalizer) normalizeSubagentTask(args map[string]any) *streams.NormalizedPayload {
	description := shared.GetString(args, "description")
	prompt := shared.GetString(args, "prompt")
	subagentType := shared.GetString(args, "subagent_type")

	return streams.NewSubagentTask(description, prompt, subagentType)
}

// normalizeCreateTask converts Amp TaskCreate tool data.
func (n *AmpNormalizer) normalizeCreateTask(args map[string]any) *streams.NormalizedPayload {
	title := shared.GetString(args, "subject")
	description := shared.GetString(args, "description")

	return streams.NewCreateTask(title, description)
}

// normalizeManageTodos converts Amp TaskUpdate/TaskList/TodoWrite tool data.
func (n *AmpNormalizer) normalizeManageTodos(toolName string, args map[string]any) *streams.NormalizedPayload {
	operation := "update"
	switch toolName {
	case AmpToolTaskList:
		operation = "list"
	case AmpToolTodoWrite:
		operation = "write"
	}

	var items []streams.TodoItem

	// Extract items if present
	if rawItems, ok := args["items"].([]any); ok {
		for _, item := range rawItems {
			if itemMap, ok := item.(map[string]any); ok {
				items = append(items, streams.TodoItem{
					ID:          shared.GetString(itemMap, "id"),
					Description: shared.GetString(itemMap, "description"),
					Status:      shared.GetString(itemMap, "status"),
				})
			}
		}
	}

	return streams.NewManageTodos(operation, items)
}

// normalizeGeneric wraps unknown tools as generic.
func (n *AmpNormalizer) normalizeGeneric(toolName string, args map[string]any) *streams.NormalizedPayload {
	return streams.NewGeneric(toolName, args)
}
