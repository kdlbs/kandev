package codex

import (
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// Codex item type constants
const (
	CodexItemCommandExecution = "commandExecution"
	CodexItemFileChange       = "fileChange"
	CodexItemReasoning        = "reasoning"
	CodexItemUserMessage      = "userMessage"
	CodexItemAgentMessage     = "agentMessage"
	CodexItemMcpToolCall      = "mcpToolCall"
)

// Normalizer converts Codex protocol tool data to NormalizedPayload.
type Normalizer struct{}

// NewNormalizer creates a new Codex normalizer.
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeToolCall converts Codex tool call data to NormalizedPayload.
// Codex uses item types rather than explicit tool names.
func (n *Normalizer) NormalizeToolCall(toolName string, args map[string]any) *streams.NormalizedPayload {
	// Codex toolName is actually the item type
	switch toolName {
	case CodexItemCommandExecution:
		return n.normalizeCommand(args)
	case CodexItemFileChange:
		return n.normalizeFileChange(args)
	case CodexItemMcpToolCall:
		return n.normalizeMcpToolCall(args)
	default:
		return n.normalizeGeneric(toolName, args)
	}
}

// NormalizeToolResult updates the payload with tool result data.
func (n *Normalizer) NormalizeToolResult(payload *streams.NormalizedPayload, result any) {
	switch payload.Kind() {
	case streams.ToolKindShellExec:
		if payload.ShellExec() != nil {
			n.normalizeCommandResult(payload.ShellExec(), result)
		}
	case streams.ToolKindModifyFile:
		// File changes are typically completed with a diff in the update
		if payload.ModifyFile() != nil && len(payload.ModifyFile().Mutations) > 0 {
			if diffStr, ok := result.(string); ok && diffStr != "" {
				payload.ModifyFile().Mutations[0].Diff = diffStr
			}
		}
	case streams.ToolKindGeneric:
		if payload.Generic() != nil {
			payload.Generic().Output = result
		}
	}
}

// normalizeCommand converts Codex commandExecution item data.
func (n *Normalizer) normalizeCommand(args map[string]any) *streams.NormalizedPayload {
	command := shared.GetString(args, "command")
	workDir := shared.GetString(args, "cwd")

	// Use factory function
	return streams.NewShellExec(command, workDir, "", 0, false)
}

// normalizeFileChange converts Codex fileChange item data.
func (n *Normalizer) normalizeFileChange(args map[string]any) *streams.NormalizedPayload {
	// Codex sends changes as an array in the item
	changes, _ := args["changes"].([]any)

	var filePath string
	var mutations []streams.FileMutation

	for _, change := range changes {
		changeMap, ok := change.(map[string]any)
		if !ok {
			continue
		}

		path := shared.GetString(changeMap, "path")
		if filePath == "" {
			filePath = path
		}

		mutation := streams.FileMutation{
			Type: streams.MutationPatch,
		}

		// Extract diff if available
		if diff, ok := changeMap["diff"].(string); ok {
			mutation.Diff = diff
		}

		mutations = append(mutations, mutation)
	}

	// If no changes array, try single file fields
	if len(mutations) == 0 {
		filePath = shared.GetString(args, "path")
		mutations = append(mutations, streams.FileMutation{
			Type: streams.MutationPatch,
		})
	}

	// Use factory function
	return streams.NewModifyFile(filePath, mutations)
}

// normalizeMcpToolCall converts Codex mcpToolCall item data.
// MCP tool calls have server, tool, arguments, result, and error fields.
func (n *Normalizer) normalizeMcpToolCall(args map[string]any) *streams.NormalizedPayload {
	server := shared.GetString(args, "server")
	tool := shared.GetString(args, "tool")

	// Create a generic payload with the MCP tool info
	// The tool name is formatted as "server/tool" for display
	toolName := tool
	if server != "" {
		toolName = server + "/" + tool
	}

	// Include the arguments in the payload for display
	displayArgs := map[string]any{
		"server": server,
		"tool":   tool,
	}
	if arguments, ok := args["arguments"]; ok {
		displayArgs["arguments"] = arguments
	}
	if result, ok := args["result"]; ok {
		displayArgs["result"] = result
	}
	if toolError, ok := args["error"]; ok {
		displayArgs["error"] = toolError
	}

	return streams.NewGeneric(toolName, displayArgs)
}

// normalizeGeneric wraps unknown items as generic.
func (n *Normalizer) normalizeGeneric(toolName string, args map[string]any) *streams.NormalizedPayload {
	// Use factory function
	return streams.NewGeneric(toolName, args)
}

// normalizeCommandResult updates command payload with result data.
func (n *Normalizer) normalizeCommandResult(payload *streams.ShellExecPayload, result any) {
	if payload.Output == nil {
		payload.Output = &streams.ShellExecOutput{}
	}

	switch r := result.(type) {
	case string:
		payload.Output.Stdout = r
	case map[string]any:
		if output, ok := r["aggregatedOutput"].(string); ok {
			payload.Output.Stdout = output
		}
		if status, ok := r["status"].(string); ok && status == "failed" {
			payload.Output.ExitCode = 1
		}
	}
}

