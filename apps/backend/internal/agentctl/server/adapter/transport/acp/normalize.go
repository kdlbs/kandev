package acp

import (
	"strings"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// DetectToolOperationType determines the specific tool operation type from ACP tool data.
// Used for logging and backwards compatibility.
func DetectToolOperationType(toolKind string, args map[string]any) string {
	// Check Auggie's "kind" field first
	if kind, ok := args["kind"].(string); ok {
		switch kind {
		case "edit":
			return "tool_edit"
		case "read":
			return "tool_read"
		case "execute":
			return "tool_execute"
		}
	}

	// Fallback to tool kind/name matching
	switch strings.ToLower(toolKind) {
	case "edit":
		return "tool_edit"
	case "read", "view":
		return "tool_read"
	case "execute", "bash", "run":
		return "tool_execute"
	default:
		return "tool_call" // Generic fallback
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
	case "edit":
		return n.normalizeEdit(args)
	case "read", "view":
		return n.normalizeRead(args)
	case "execute", "bash", "run", "shell":
		return n.normalizeExecute(args)
	case "glob", "grep", "search":
		return n.normalizeCodeSearch(toolName, args)
	default:
		return n.normalizeGeneric(toolName, args)
	}
}

// NormalizeToolResult updates the payload with tool result data.
func (n *Normalizer) NormalizeToolResult(payload *streams.NormalizedPayload, result any) {
	switch payload.Kind {
	case streams.ToolKindShellExec:
		if payload.ShellExec != nil {
			shared.NormalizeShellResult(payload.ShellExec, result)
		}
	case streams.ToolKindGeneric:
		if payload.Generic != nil {
			payload.Generic.Output = result
		}
	}
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

	payload := &streams.NormalizedPayload{
		Kind: streams.ToolKindModifyFile,
		ModifyFile: &streams.ModifyFilePayload{
			FilePath:  path,
			Mutations: []streams.FileMutation{},
		},
	}

	// Check if this is file creation (has file_content) vs str_replace
	if fileContent, ok := rawInput["file_content"].(string); ok {
		payload.ModifyFile.Mutations = append(payload.ModifyFile.Mutations, streams.FileMutation{
			Type:    streams.MutationCreate,
			Content: fileContent,
		})
	} else {
		// str_replace operation
		oldStr, _ := rawInput["old_str_1"].(string)
		newStr, _ := rawInput["new_str_1"].(string)

		mutation := streams.FileMutation{
			Type:       streams.MutationPatch,
			OldContent: oldStr,
			NewContent: newStr,
		}

		// Add line numbers if available
		if startLine, ok := rawInput["old_str_start_line_number_1"].(float64); ok {
			mutation.StartLine = int(startLine)
		}
		if endLine, ok := rawInput["old_str_end_line_number_1"].(float64); ok {
			mutation.EndLine = int(endLine)
		}

		// Generate unified diff
		if oldStr != "" && newStr != "" {
			mutation.Diff = shared.GenerateUnifiedDiff(oldStr, newStr, path, mutation.StartLine)
		}

		payload.ModifyFile.Mutations = append(payload.ModifyFile.Mutations, mutation)
	}

	return payload
}

// normalizeRead converts ACP read tool data.
func (n *Normalizer) normalizeRead(args map[string]any) *streams.NormalizedPayload {
	rawInput, _ := args["raw_input"].(map[string]any)
	if rawInput == nil {
		rawInput = args
	}

	path := shared.GetString(rawInput, "path")
	if path == "" {
		path = extractPathFromLocations(args)
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
	case "glob":
		glob = pattern
	case "grep", "search":
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
