package opencode

import (
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
)

// Normalizer converts OpenCode protocol tool data to NormalizedPayload.
type Normalizer struct{}

// NewNormalizer creates a new OpenCode normalizer.
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeToolCall converts OpenCode tool call data to NormalizedPayload.
func (n *Normalizer) NormalizeToolCall(toolName string, args map[string]any) *streams.NormalizedPayload {
	// OpenCode tool names are typically lowercase
	switch toolName {
	case OpenCodeToolBash:
		return n.normalizeBash(args)
	case OpenCodeToolEdit:
		return n.normalizeEdit(args)
	case OpenCodeToolWebFetch:
		return n.normalizeWebFetch(args)
	default:
		return n.normalizeGeneric(toolName, args)
	}
}

// NormalizeToolResult updates the payload with tool result data.
func (n *Normalizer) NormalizeToolResult(payload *streams.NormalizedPayload, result any) {
	switch payload.Kind {
	case streams.ToolKindShellExec:
		if payload.ShellExec != nil {
			n.normalizeBashResult(payload.ShellExec, result)
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

// normalizeBash converts OpenCode bash tool data.
func (n *Normalizer) normalizeBash(args map[string]any) *streams.NormalizedPayload {
	command := shared.GetString(args, "command")

	// OpenCode tool state might have the command
	if command == "" {
		if state, ok := args["state"].(map[string]any); ok {
			command = shared.GetString(state, "command")
		}
	}

	return &streams.NormalizedPayload{
		Kind: streams.ToolKindShellExec,
		ShellExec: &streams.ShellExecPayload{
			Command: command,
		},
	}
}

// normalizeEdit converts OpenCode edit tool data.
func (n *Normalizer) normalizeEdit(args map[string]any) *streams.NormalizedPayload {
	// OpenCode edit args might be in "input" or directly in args
	input := args
	if inputMap, ok := args["input"].(map[string]any); ok {
		input = inputMap
	}

	filePath := shared.GetString(input, "path")
	if filePath == "" {
		filePath = shared.GetString(input, "file_path")
	}

	payload := &streams.ModifyFilePayload{
		FilePath:  filePath,
		Mutations: []streams.FileMutation{},
	}

	// OpenCode might have diff or content
	if diff, ok := input["diff"].(string); ok && diff != "" {
		payload.Mutations = append(payload.Mutations, streams.FileMutation{
			Type: streams.MutationPatch,
			Diff: diff,
		})
	} else {
		payload.Mutations = append(payload.Mutations, streams.FileMutation{
			Type: streams.MutationPatch,
		})
	}

	return &streams.NormalizedPayload{
		Kind:       streams.ToolKindModifyFile,
		ModifyFile: payload,
	}
}

// normalizeWebFetch converts OpenCode webfetch tool data.
func (n *Normalizer) normalizeWebFetch(args map[string]any) *streams.NormalizedPayload {
	url := shared.GetString(args, "url")

	return &streams.NormalizedPayload{
		Kind: streams.ToolKindHttpRequest,
		HttpRequest: &streams.HttpRequestPayload{
			URL:    url,
			Method: "GET",
		},
	}
}

// normalizeGeneric wraps unknown tools as generic.
func (n *Normalizer) normalizeGeneric(toolName string, args map[string]any) *streams.NormalizedPayload {
	return &streams.NormalizedPayload{
		Kind: streams.ToolKindGeneric,
		Generic: &streams.GenericPayload{
			Name:  toolName,
			Input: args,
		},
	}
}

// normalizeBashResult updates bash payload with result data.
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

