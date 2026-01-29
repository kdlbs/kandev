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
	default:
		return n.normalizeGeneric(toolName, args)
	}
}

// NormalizeToolResult updates the payload with tool result data.
func (n *Normalizer) NormalizeToolResult(payload *streams.NormalizedPayload, result any) {
	switch payload.Kind {
	case streams.ToolKindShellExec:
		if payload.ShellExec != nil {
			n.normalizeCommandResult(payload.ShellExec, result)
		}
	case streams.ToolKindModifyFile:
		// File changes are typically completed with a diff in the update
		if payload.ModifyFile != nil && len(payload.ModifyFile.Mutations) > 0 {
			if diffStr, ok := result.(string); ok && diffStr != "" {
				payload.ModifyFile.Mutations[0].Diff = diffStr
			}
		}
	case streams.ToolKindGeneric:
		if payload.Generic != nil {
			payload.Generic.Output = result
		}
	}
}

// normalizeCommand converts Codex commandExecution item data.
func (n *Normalizer) normalizeCommand(args map[string]any) *streams.NormalizedPayload {
	command := shared.GetString(args, "command")
	workDir := shared.GetString(args, "cwd")

	return &streams.NormalizedPayload{
		Kind: streams.ToolKindShellExec,
		ShellExec: &streams.ShellExecPayload{
			Command: command,
			WorkDir: workDir,
		},
	}
}

// normalizeFileChange converts Codex fileChange item data.
func (n *Normalizer) normalizeFileChange(args map[string]any) *streams.NormalizedPayload {
	// Codex sends changes as an array in the item
	changes, _ := args["changes"].([]any)

	payload := &streams.ModifyFilePayload{
		Mutations: []streams.FileMutation{},
	}

	for _, change := range changes {
		changeMap, ok := change.(map[string]any)
		if !ok {
			continue
		}

		path := shared.GetString(changeMap, "path")
		if payload.FilePath == "" {
			payload.FilePath = path
		}

		mutation := streams.FileMutation{
			Type: streams.MutationPatch,
		}

		// Extract diff if available
		if diff, ok := changeMap["diff"].(string); ok {
			mutation.Diff = diff
		}

		payload.Mutations = append(payload.Mutations, mutation)
	}

	// If no changes array, try single file fields
	if len(payload.Mutations) == 0 {
		path := shared.GetString(args, "path")
		payload.FilePath = path
		payload.Mutations = append(payload.Mutations, streams.FileMutation{
			Type: streams.MutationPatch,
		})
	}

	return &streams.NormalizedPayload{
		Kind:       streams.ToolKindModifyFile,
		ModifyFile: payload,
	}
}

// normalizeGeneric wraps unknown items as generic.
func (n *Normalizer) normalizeGeneric(toolName string, args map[string]any) *streams.NormalizedPayload {
	return &streams.NormalizedPayload{
		Kind: streams.ToolKindGeneric,
		Generic: &streams.GenericPayload{
			Name:  toolName,
			Input: args,
		},
	}
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

