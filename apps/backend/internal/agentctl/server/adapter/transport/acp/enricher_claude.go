package acp

import (
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func enrichClaudePayload(payload *streams.NormalizedPayload, frame EnrichFrame) {
	cc := claudeCodeMeta(frame.Meta)
	if cc == nil && frame.RawInput == nil {
		return
	}

	switch payload.Kind() {
	case streams.ToolKindReadFile:
		enrichClaudeRead(payload.ReadFile(), frame, cc)
	case streams.ToolKindModifyFile:
		enrichClaudeModify(payload.ModifyFile(), frame)
	case streams.ToolKindCodeSearch:
		enrichClaudeSearch(payload.CodeSearch(), frame)
	}
}

func enrichClaudeRead(rf *streams.ReadFilePayload, frame EnrichFrame, cc map[string]any) {
	if rf == nil {
		return
	}
	fillIfEmpty(&rf.FilePath, firstStructuredPath(frame.RawInput, frame.Supplemental))
	if resp, ok := cc["toolResponse"].(map[string]any); ok {
		if file, ok := resp["file"].(map[string]any); ok {
			if fp, _ := file["filePath"].(string); fp != "" {
				fillIfEmpty(&rf.FilePath, fp)
			}
		}
	}
}

func enrichClaudeModify(mf *streams.ModifyFilePayload, frame EnrichFrame) {
	if mf == nil || frame.RawInput == nil {
		return
	}
	fillIfEmpty(&mf.FilePath, firstStructuredPath(frame.RawInput, frame.Supplemental))
	oldStr, _ := frame.RawInput["old_string"].(string)
	newStr, _ := frame.RawInput["new_string"].(string)
	if oldStr == "" && newStr == "" {
		return
	}
	if len(mf.Mutations) == 0 {
		mf.Mutations = []streams.FileMutation{{Type: streams.MutationPatch}}
	}
	mut := &mf.Mutations[0]
	if mut.Diff == "" && (oldStr != "" || newStr != "") {
		mut.Diff = shared.GenerateUnifiedDiff(oldStr, newStr, mf.FilePath, mut.StartLine)
	}
}

func enrichClaudeSearch(cs *streams.CodeSearchPayload, frame EnrichFrame) {
	if cs == nil || frame.RawInput == nil {
		return
	}
	fillIfEmpty(&cs.Query, stringFromMap(frame.RawInput, "pattern", "query"))
	fillIfEmpty(&cs.Path, pathFromStructuredInput(frame.RawInput))
}
