package acp

import (
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// EnrichFrame carries ACP-first tool-call fields passed into agent enrichers.
// Title is display-only at the UI layer and must not be parsed for file paths
// in the common normalizer.
type EnrichFrame struct {
	Title        string
	Meta         map[string]any
	RawInput     map[string]any
	Supplemental map[string]any
}

type agentEnrichFunc func(payload *streams.NormalizedPayload, frame EnrichFrame)

var agentEnrichers = map[string]agentEnrichFunc{
	"claude-acp":   enrichClaudePayload,
	"codex-acp":    enrichCodexPayload,
	"opencode-acp": enrichOpenCodePayload,
	"cursor-acp":   enrichCursorPayload,
}

func applyAgentEnrichment(agentID string, payload *streams.NormalizedPayload, frame EnrichFrame) {
	if payload == nil || agentID == "" {
		return
	}
	fn, ok := agentEnrichers[agentID]
	if !ok {
		return
	}
	fn(payload, frame)
}

func enrichFrameFromArgs(args map[string]any) EnrichFrame {
	frame := EnrichFrame{}
	if args == nil {
		return frame
	}
	frame.Title, _ = args[argKeyTitle].(string)
	if meta, ok := args[argKeyMeta].(map[string]any); ok {
		frame.Meta = meta
	}
	if raw, ok := args["raw_input"].(map[string]any); ok {
		frame.RawInput = raw
	}
	frame.Supplemental = supplementalFromArgs(args)
	return frame
}

func supplementalFromArgs(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}
	if _, ok := args[keyLocations]; ok {
		return map[string]any{keyLocations: args[keyLocations], keyPath: args[keyPath]}
	}
	if path, _ := args[keyPath].(string); path != "" {
		return map[string]any{keyPath: path}
	}
	return nil
}

func enrichFrameFromUpdate(title *string, meta map[string]any, rawInput any, supplemental map[string]any) EnrichFrame {
	frame := EnrichFrame{Meta: meta, Supplemental: supplemental}
	if title != nil {
		frame.Title = *title
	}
	if raw, ok := rawInput.(map[string]any); ok {
		frame.RawInput = raw
	}
	return frame
}

// fillIfEmpty assigns val to *dest when dest is empty and val is non-empty.
func fillIfEmpty(dest *string, val string) {
	if dest == nil || val == "" || *dest != "" {
		return
	}
	*dest = val
}

func claudeCodeMeta(meta map[string]any) map[string]any {
	if meta == nil {
		return nil
	}
	cc, _ := meta["claudeCode"].(map[string]any)
	return cc
}

func firstStructuredPath(rawInput, supplemental map[string]any) string {
	if p := pathFromStructuredInput(rawInput); p != "" {
		return p
	}
	return pathFromLocations(supplemental)
}

func pathFromStructuredInput(rawInput map[string]any) string {
	return stringFromMap(rawInput, "path", "file_path", "filePath")
}

func pathFromLocations(args map[string]any) string {
	if args == nil {
		return ""
	}
	return extractPathFromLocations(args)
}
