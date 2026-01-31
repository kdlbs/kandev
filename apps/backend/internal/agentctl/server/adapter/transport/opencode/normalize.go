package opencode

import (
	"strings"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// OpenCode part type constants
const (
	OpenCodePartText      = "text"
	OpenCodePartReasoning = "reasoning"
	OpenCodePartTool      = "tool"
)

// OpenCode tool name constants
const (
	OpenCodeToolBash     = "bash"
	OpenCodeToolEdit     = "edit"
	OpenCodeToolWebFetch = "webfetch"
	OpenCodeToolGlob     = "glob"
	OpenCodeToolGrep     = "grep"
	OpenCodeToolRead     = "read"
)

// Normalizer converts OpenCode protocol tool data to NormalizedPayload.
type Normalizer struct{}

// NewNormalizer creates a new OpenCode normalizer.
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// ToolState contains the full tool state for normalization
type ToolState struct {
	Input    map[string]any
	Output   string
	Metadata map[string]any
	Title    string
}

// NormalizeToolCall converts OpenCode tool call data to NormalizedPayload.
func (n *Normalizer) NormalizeToolCall(toolName string, state *ToolState) *streams.NormalizedPayload {
	if state == nil {
		state = &ToolState{}
	}
	if state.Input == nil {
		state.Input = make(map[string]any)
	}

	// OpenCode tool names are typically lowercase
	switch toolName {
	case OpenCodeToolBash:
		return n.normalizeBash(state)
	case OpenCodeToolEdit:
		return n.normalizeEdit(state)
	case OpenCodeToolWebFetch:
		return n.normalizeWebFetch(state)
	case OpenCodeToolGlob:
		return n.normalizeGlob(state)
	case OpenCodeToolGrep:
		return n.normalizeGrep(state)
	case OpenCodeToolRead:
		return n.normalizeRead(state)
	default:
		return n.normalizeGeneric(toolName, state)
	}
}

// NormalizeToolResult updates the payload with tool result data.
// Deprecated: Result data is now handled in NormalizeToolCall via ToolState
func (n *Normalizer) NormalizeToolResult(payload *streams.NormalizedPayload, result any) {
	// Keep for backwards compatibility but normalization now happens in NormalizeToolCall
	switch payload.Kind() {
	case streams.ToolKindShellExec:
		if payload.ShellExec() != nil && payload.ShellExec().Output == nil {
			n.normalizeBashResult(payload.ShellExec(), result)
		}
	case streams.ToolKindHttpRequest:
		if payload.HttpRequest() != nil {
			if r, ok := result.(string); ok {
				payload.HttpRequest().Response = r
			}
		}
	case streams.ToolKindGeneric:
		if payload.Generic() != nil && payload.Generic().Output == nil {
			payload.Generic().Output = result
		}
	}
}

// normalizeBash converts OpenCode bash tool data.
func (n *Normalizer) normalizeBash(state *ToolState) *streams.NormalizedPayload {
	command := shared.GetString(state.Input, "command")
	description := shared.GetString(state.Input, "description")
	timeout := shared.GetInt(state.Input, "timeout")
	background := shared.GetBool(state.Input, "background")

	// Use factory function
	payload := streams.NewShellExec(command, "", description, timeout, background)

	// Add output if available
	if state.Output != "" || state.Metadata != nil {
		payload.ShellExec().Output = &streams.ShellExecOutput{
			Stdout: state.Output,
		}
		// Extract exit code from metadata
		if state.Metadata != nil {
			if exitCode, ok := state.Metadata["exit"].(float64); ok {
				payload.ShellExec().Output.ExitCode = int(exitCode)
			}
		}
	}

	return payload
}

// normalizeEdit converts OpenCode edit tool data.
func (n *Normalizer) normalizeEdit(state *ToolState) *streams.NormalizedPayload {
	// OpenCode edit args might be in "input" or directly in args
	input := state.Input
	if inputMap, ok := state.Input["input"].(map[string]any); ok {
		input = inputMap
	}

	filePath := shared.GetString(input, "path")
	if filePath == "" {
		filePath = shared.GetString(input, "file_path")
	}

	// Build mutations
	var mutations []streams.FileMutation
	if diff, ok := input["diff"].(string); ok && diff != "" {
		mutations = append(mutations, streams.FileMutation{
			Type: streams.MutationPatch,
			Diff: diff,
		})
	} else {
		mutations = append(mutations, streams.FileMutation{
			Type: streams.MutationPatch,
		})
	}

	// Use factory function
	return streams.NewModifyFile(filePath, mutations)
}

// normalizeWebFetch converts OpenCode webfetch tool data.
func (n *Normalizer) normalizeWebFetch(state *ToolState) *streams.NormalizedPayload {
	url := shared.GetString(state.Input, "url")

	// Use factory function
	payload := streams.NewHttpRequest(url, "GET")

	// Add response if available
	if state.Output != "" {
		payload.HttpRequest().Response = state.Output
	}

	return payload
}

// normalizeGlob converts OpenCode glob tool data to code_search.
func (n *Normalizer) normalizeGlob(state *ToolState) *streams.NormalizedPayload {
	pattern := shared.GetString(state.Input, "pattern")
	path := shared.GetString(state.Input, "path")

	// Use factory function: NewCodeSearch(query, pattern, path, glob)
	// For glob, pattern goes to both pattern and glob fields
	payload := streams.NewCodeSearch("", pattern, path, pattern)

	// Parse output (newline-separated file list) and metadata
	if state.Output != "" || state.Metadata != nil {
		payload.CodeSearch().Output = &streams.CodeSearchOutput{}

		// Parse file list from output
		if state.Output != "" {
			lines := strings.Split(strings.TrimSpace(state.Output), "\n")
			// Filter out empty lines and the truncation message
			var files []string
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "(Results are truncated") {
					files = append(files, line)
				}
			}
			payload.CodeSearch().Output.Files = files
		}

		// Extract metadata
		if state.Metadata != nil {
			if count, ok := state.Metadata["count"].(float64); ok {
				payload.CodeSearch().Output.FileCount = int(count)
			}
			if truncated, ok := state.Metadata["truncated"].(bool); ok {
				payload.CodeSearch().Output.Truncated = truncated
			}
		}
	}

	return payload
}

// normalizeGrep converts OpenCode grep tool data to code_search.
func (n *Normalizer) normalizeGrep(state *ToolState) *streams.NormalizedPayload {
	pattern := shared.GetString(state.Input, "pattern")
	path := shared.GetString(state.Input, "path")
	query := shared.GetString(state.Input, "query")

	// Use factory function: NewCodeSearch(query, pattern, path, glob)
	payload := streams.NewCodeSearch(query, pattern, path, "")

	// Parse output and metadata
	if state.Output != "" || state.Metadata != nil {
		payload.CodeSearch().Output = &streams.CodeSearchOutput{}

		// Parse file list from output (grep typically outputs file:line format)
		if state.Output != "" {
			lines := strings.Split(strings.TrimSpace(state.Output), "\n")
			fileSet := make(map[string]bool)
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				// Extract file path (before first colon for grep output)
				if idx := strings.Index(line, ":"); idx > 0 {
					fileSet[line[:idx]] = true
				} else {
					fileSet[line] = true
				}
			}
			var files []string
			for f := range fileSet {
				files = append(files, f)
			}
			payload.CodeSearch().Output.Files = files
			payload.CodeSearch().Output.FileCount = len(files)
		}

		// Extract metadata
		if state.Metadata != nil {
			if truncated, ok := state.Metadata["truncated"].(bool); ok {
				payload.CodeSearch().Output.Truncated = truncated
			}
		}
	}

	return payload
}

// normalizeRead converts OpenCode read tool data.
func (n *Normalizer) normalizeRead(state *ToolState) *streams.NormalizedPayload {
	filePath := shared.GetString(state.Input, "path")
	if filePath == "" {
		filePath = shared.GetString(state.Input, "file_path")
	}

	offset := shared.GetInt(state.Input, "offset")
	limit := shared.GetInt(state.Input, "limit")

	// Use factory function
	payload := streams.NewReadFile(filePath, offset, limit)

	// Add output if available
	if state.Output != "" || state.Metadata != nil {
		payload.ReadFile().Output = &streams.ReadFileOutput{
			Content: state.Output,
		}

		if state.Metadata != nil {
			if lineCount, ok := state.Metadata["line_count"].(float64); ok {
				payload.ReadFile().Output.LineCount = int(lineCount)
			}
			if truncated, ok := state.Metadata["truncated"].(bool); ok {
				payload.ReadFile().Output.Truncated = truncated
			}
		}

		// Calculate line count from content if not in metadata
		if payload.ReadFile().Output.LineCount == 0 && state.Output != "" {
			payload.ReadFile().Output.LineCount = strings.Count(state.Output, "\n") + 1
		}
	}

	return payload
}

// normalizeGeneric wraps unknown tools as generic.
func (n *Normalizer) normalizeGeneric(toolName string, state *ToolState) *streams.NormalizedPayload {
	// Use factory function
	payload := streams.NewGeneric(toolName, state.Input)

	// Add output if available
	if state.Output != "" {
		payload.Generic().Output = state.Output
	}

	return payload
}

// normalizeBashResult updates bash payload with result data.
// Deprecated: Use ToolState in NormalizeToolCall instead
func (n *Normalizer) normalizeBashResult(payload *streams.ShellExecPayload, result any) {
	if payload.Output == nil {
		payload.Output = &streams.ShellExecOutput{}
	}

	switch r := result.(type) {
	case string:
		payload.Output.Stdout = r
	case map[string]any:
		if output, ok := r["output"].(string); ok {
			payload.Output.Stdout = output
		}
		if errStr, ok := r["error"].(string); ok {
			payload.Output.Stderr = errStr
		}
	}
}
