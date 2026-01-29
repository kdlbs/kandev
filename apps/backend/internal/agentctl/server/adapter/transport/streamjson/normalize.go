package streamjson

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/claudecode"
)

// DetectStreamJSONToolType determines tool type from stream-json/Claude Code tool names.
// Used for logging and backwards compatibility.
func DetectStreamJSONToolType(toolName string) string {
	switch toolName {
	case claudecode.ToolEdit, claudecode.ToolWrite, claudecode.ToolNotebookEdit:
		return "tool_edit"
	case claudecode.ToolRead, claudecode.ToolGlob, claudecode.ToolGrep:
		return "tool_read"
	case claudecode.ToolBash:
		return "tool_execute"
	default:
		return "tool_call"
	}
}

// Tool name constants (matching Claude Code CLI)
const (
	ToolEdit         = "Edit"
	ToolWrite        = "Write"
	ToolNotebookEdit = "NotebookEdit"
	ToolRead         = "Read"
	ToolGlob         = "Glob"
	ToolGrep         = "Grep"
	ToolBash         = "Bash"
	ToolWebFetch     = "WebFetch"
	ToolWebSearch    = "WebSearch"
	ToolTask         = "Task"
	ToolTaskCreate   = "TaskCreate"
	ToolTaskUpdate   = "TaskUpdate"
	ToolTaskList     = "TaskList"
	ToolTodoWrite    = "TodoWrite"
)

// Normalizer converts stream-json protocol tool data to NormalizedPayload.
type Normalizer struct{}

// NewNormalizer creates a new stream-json normalizer.
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeToolCall converts stream-json tool call data to NormalizedPayload.
func (n *Normalizer) NormalizeToolCall(toolName string, args map[string]any) *streams.NormalizedPayload {
	switch toolName {
	case ToolEdit, ToolWrite, ToolNotebookEdit:
		return n.normalizeEdit(toolName, args)
	case ToolRead:
		return n.normalizeRead(args)
	case ToolGlob, ToolGrep:
		return n.normalizeCodeSearch(toolName, args)
	case ToolBash:
		return n.normalizeExecute(args)
	case ToolWebFetch, ToolWebSearch:
		return n.normalizeHttpRequest(toolName, args)
	case ToolTask:
		return n.normalizeSubagentTask(args)
	case ToolTaskCreate:
		return n.normalizeCreateTask(args)
	case ToolTaskUpdate, ToolTaskList:
		return n.normalizeManageTodos(toolName, args)
	case ToolTodoWrite:
		return n.normalizeManageTodos(toolName, args)
	default:
		return n.normalizeGeneric(toolName, args)
	}
}

// NormalizeToolResult updates the payload with tool result data.
func (n *Normalizer) NormalizeToolResult(payload *streams.NormalizedPayload, result any) {
	switch payload.Kind {
	case streams.ToolKindShellExec:
		if payload.ShellExec != nil {
			n.normalizeShellResult(payload.ShellExec, result)
		}
	case streams.ToolKindHttpRequest:
		if payload.HttpRequest != nil {
			if r, ok := result.(string); ok {
				payload.HttpRequest.Response = r
			}
		}
	case streams.ToolKindGeneric:
		if payload.Generic != nil {
			payload.Generic.Output = result
		}
	}
}

// normalizeEdit converts stream-json Edit/Write tool data.
func (n *Normalizer) normalizeEdit(toolName string, args map[string]any) *streams.NormalizedPayload {
	filePath := shared.GetString(args, "file_path")
	oldString := shared.GetString(args, "old_string")
	newString := shared.GetString(args, "new_string")
	content := shared.GetString(args, "content")

	payload := &streams.ModifyFilePayload{
		FilePath:  filePath,
		Mutations: []streams.FileMutation{},
	}

	if toolName == ToolWrite {
		// Write is a full file creation/replacement
		payload.Mutations = append(payload.Mutations, streams.FileMutation{
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
			mutation.Diff = generateUnifiedDiff(oldString, newString, filePath, 0, 0)
		}

		payload.Mutations = append(payload.Mutations, mutation)
	}

	return &streams.NormalizedPayload{
		Kind:       streams.ToolKindModifyFile,
		ModifyFile: payload,
	}
}

// normalizeRead converts stream-json Read tool data.
func (n *Normalizer) normalizeRead(args map[string]any) *streams.NormalizedPayload {
	filePath := shared.GetString(args, "file_path")
	offset := shared.GetInt(args, "offset")
	limit := shared.GetInt(args, "limit")

	return streams.NewReadFile(filePath, offset, limit)
}

// normalizeCodeSearch converts stream-json Glob/Grep tool data.
func (n *Normalizer) normalizeCodeSearch(toolName string, args map[string]any) *streams.NormalizedPayload {
	path := shared.GetString(args, "path")
	pattern := shared.GetString(args, "pattern")

	var query, glob string
	switch toolName {
	case ToolGlob:
		glob = pattern
	case ToolGrep:
		query = shared.GetString(args, "query")
	}

	return streams.NewCodeSearch(query, pattern, path, glob)
}

// normalizeExecute converts stream-json Bash tool data.
func (n *Normalizer) normalizeExecute(args map[string]any) *streams.NormalizedPayload {
	command := shared.GetString(args, "command")
	description := shared.GetString(args, "description")
	timeout := shared.GetInt(args, "timeout")
	background := shared.GetBool(args, "run_in_background")

	return streams.NewShellExec(command, "", description, timeout, background)
}

// normalizeHttpRequest converts stream-json WebFetch/WebSearch tool data.
func (n *Normalizer) normalizeHttpRequest(toolName string, args map[string]any) *streams.NormalizedPayload {
	url := shared.GetString(args, "url")
	if url == "" {
		url = shared.GetString(args, "query")
	}

	method := "GET"
	if toolName == ToolWebSearch {
		method = "SEARCH"
	}

	return streams.NewHttpRequest(url, method)
}

// normalizeSubagentTask converts stream-json Task tool data (subagent invocation).
func (n *Normalizer) normalizeSubagentTask(args map[string]any) *streams.NormalizedPayload {
	description := shared.GetString(args, "description")
	prompt := shared.GetString(args, "prompt")
	subagentType := shared.GetString(args, "subagent_type")

	return streams.NewSubagentTask(description, prompt, subagentType)
}

// normalizeCreateTask converts stream-json TaskCreate tool data.
func (n *Normalizer) normalizeCreateTask(args map[string]any) *streams.NormalizedPayload {
	title := shared.GetString(args, "subject")
	description := shared.GetString(args, "description")

	return streams.NewCreateTask(title, description)
}

// normalizeManageTodos converts stream-json TaskUpdate/TaskList/TodoWrite tool data.
func (n *Normalizer) normalizeManageTodos(toolName string, args map[string]any) *streams.NormalizedPayload {
	operation := "update"
	switch toolName {
	case ToolTaskList:
		operation = "list"
	case ToolTodoWrite:
		operation = "write"
	}

	payload := &streams.ManageTodosPayload{
		Operation: operation,
	}

	// Extract items if present
	if items, ok := args["items"].([]any); ok {
		for _, item := range items {
			if itemMap, ok := item.(map[string]any); ok {
				payload.Items = append(payload.Items, streams.TodoItem{
					ID:          shared.GetString(itemMap, "id"),
					Description: shared.GetString(itemMap, "description"),
					Status:      shared.GetString(itemMap, "status"),
				})
			}
		}
	}

	return &streams.NormalizedPayload{
		Kind:        streams.ToolKindManageTodos,
		ManageTodos: payload,
	}
}

// normalizeGeneric wraps unknown tools as generic.
func (n *Normalizer) normalizeGeneric(toolName string, args map[string]any) *streams.NormalizedPayload {
	return streams.NewGeneric(toolName, args)
}

// normalizeShellResult updates shell payload with result data.
func (n *Normalizer) normalizeShellResult(payload *streams.ShellExecPayload, result any) {
	if payload.Output == nil {
		payload.Output = &streams.ShellExecOutput{}
	}

	switch r := result.(type) {
	case string:
		payload.Output.Stdout = r
	case map[string]any:
		if stdout, ok := r["stdout"].(string); ok {
			payload.Output.Stdout = stdout
		}
		if stderr, ok := r["stderr"].(string); ok {
			payload.Output.Stderr = stderr
		}
		if exitCode, ok := r["exit_code"].(float64); ok {
			payload.Output.ExitCode = int(exitCode)
		}
	}
}

// --- Helper functions ---

func generateUnifiedDiff(oldStr, newStr, path string, startLine, _ int) string {
	if oldStr == "" || newStr == "" {
		return ""
	}

	oldLines := splitLines(oldStr)
	newLines := splitLines(newStr)

	if startLine == 0 {
		startLine = 1
	}

	// Build diff header
	var sb strings.Builder
	fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", path, path)
	sb.WriteString("index 0000000..0000000 100644\n")
	fmt.Fprintf(&sb, "--- a/%s\n", path)
	fmt.Fprintf(&sb, "+++ b/%s\n", path)
	fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", startLine, len(oldLines), startLine, len(newLines))

	// Add removed lines
	for _, line := range oldLines {
		sb.WriteString("-")
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Add added lines
	for _, line := range newLines {
		sb.WriteString("+")
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

// detectLanguage maps file extension to language identifier.
func detectLanguage(path string) string {
	if path == "" {
		return "plaintext"
	}

	// Find last dot
	lastDot := -1
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			lastDot = i
			break
		}
		if path[i] == '/' {
			break // No extension found
		}
	}

	if lastDot == -1 || lastDot == len(path)-1 {
		return "plaintext"
	}

	ext := path[lastDot+1:]

	langMap := map[string]string{
		"ts":   "typescript",
		"tsx":  "typescript",
		"js":   "javascript",
		"jsx":  "javascript",
		"py":   "python",
		"go":   "go",
		"rs":   "rust",
		"java": "java",
		"cpp":  "cpp",
		"c":    "c",
		"h":    "c",
		"hpp":  "cpp",
		"css":  "css",
		"html": "html",
		"json": "json",
		"md":   "markdown",
		"yaml": "yaml",
		"yml":  "yaml",
		"sh":   "bash",
		"bash": "bash",
	}

	if lang, ok := langMap[ext]; ok {
		return lang
	}
	return "plaintext"
}
