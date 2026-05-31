package acp

import (
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func enrichCodexPayload(payload *streams.NormalizedPayload, frame EnrichFrame) {
	switch payload.Kind() {
	case streams.ToolKindReadFile:
		enrichCodexRead(payload.ReadFile(), frame)
	case streams.ToolKindCodeSearch:
		enrichCodexSearch(payload.CodeSearch(), frame)
	case streams.ToolKindModifyFile:
		enrichCodexModify(payload.ModifyFile(), frame)
	}
}

func enrichCodexRead(rf *streams.ReadFilePayload, frame EnrichFrame) {
	if rf == nil {
		return
	}
	fillIfEmpty(&rf.FilePath, firstStructuredPath(frame.RawInput, frame.Supplemental))
	fillIfEmpty(&rf.FilePath, codexParsedCmdPath(frame.RawInput))
	fillIfEmpty(&rf.FilePath, codexReadTitleHint(frame.Title))
}

func enrichCodexSearch(cs *streams.CodeSearchPayload, frame EnrichFrame) {
	if cs == nil {
		return
	}
	query, path := codexParsedCmdSearch(frame.RawInput)
	fillIfEmpty(&cs.Query, query)
	fillIfEmpty(&cs.Path, path)
	if frame.Title != "" && (cs.Query == "" || cs.Path == "") {
		titleQuery, titlePath := codexSearchTitleHints(frame.Title)
		fillIfEmpty(&cs.Query, titleQuery)
		fillIfEmpty(&cs.Path, titlePath)
	}
}

func enrichCodexModify(mf *streams.ModifyFilePayload, frame EnrichFrame) {
	if mf == nil || frame.RawInput == nil {
		return
	}
	changes, ok := frame.RawInput["changes"].(map[string]any)
	if !ok || len(changes) == 0 {
		return
	}
	for path, changeAny := range changes {
		if mf.FilePath == "" && path != "" {
			mf.FilePath = path
		}
		change, ok := changeAny.(map[string]any)
		if !ok {
			continue
		}
		diff, _ := change["unified_diff"].(string)
		if diff == "" {
			continue
		}
		if len(mf.Mutations) == 0 {
			mf.Mutations = []streams.FileMutation{{Type: streams.MutationPatch, Diff: diff}}
			continue
		}
		if mf.Mutations[0].Diff == "" {
			mf.Mutations[0].Diff = diff
		}
	}
}

func codexParsedCmdPath(rawInput map[string]any) string {
	for _, cmd := range codexParsedCommands(rawInput) {
		if path, _ := cmd["path"].(string); path != "" {
			return path
		}
	}
	return ""
}

func codexParsedCmdSearch(rawInput map[string]any) (query, path string) {
	for _, cmd := range codexParsedCommands(rawInput) {
		cmdType, _ := cmd["type"].(string)
		if cmdType != toolKindSearch && cmdType != toolKindGrep {
			continue
		}
		if q, _ := cmd["query"].(string); q != "" {
			query = q
		}
		if p, _ := cmd["path"].(string); p != "" {
			path = p
		}
	}
	return query, path
}

func codexParsedCommands(rawInput map[string]any) []map[string]any {
	if rawInput == nil {
		return nil
	}
	items, ok := rawInput["parsed_cmd"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		cmd, ok := item.(map[string]any)
		if ok {
			out = append(out, cmd)
		}
	}
	return out
}

// codexReadTitleHint parses codex-acp shell read titles — codex-specific, not ACP.
func codexReadTitleHint(title string) string {
	for _, prefix := range []string{"Read ", "Write ", "Edit "} {
		if strings.HasPrefix(title, prefix) {
			target := strings.TrimSpace(strings.TrimPrefix(title, prefix))
			if target != "" && !isGenericPathLabel(target) {
				return target
			}
			return ""
		}
	}
	return ""
}

func isGenericPathLabel(target string) bool {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case genericLabelFile, readTypeDirectory, genericLabelFolder:
		return true
	default:
		return false
	}
}

// codexSearchTitleHints parses codex-acp shell titles — codex-specific, not ACP.
func codexSearchTitleHints(title string) (query, path string) {
	switch {
	case strings.HasPrefix(title, "Search "):
		rest := strings.TrimPrefix(title, "Search ")
		if idx := strings.LastIndex(rest, " in "); idx >= 0 {
			return strings.TrimSpace(rest[:idx]), strings.TrimSpace(rest[idx+4:])
		}
		return strings.TrimSpace(rest), ""
	case strings.HasPrefix(title, "List "):
		return "", strings.TrimSpace(strings.TrimPrefix(title, "List "))
	default:
		return "", ""
	}
}
