package acp

import (
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func enrichOpenCodePayload(payload *streams.NormalizedPayload, frame EnrichFrame) {
	switch payload.Kind() {
	case streams.ToolKindReadFile:
		if rf := payload.ReadFile(); rf != nil {
			fillIfEmpty(&rf.FilePath, firstStructuredPath(frame.RawInput, frame.Supplemental))
		}
	case streams.ToolKindModifyFile:
		enrichOpenCodeModify(payload.ModifyFile(), frame)
	case streams.ToolKindCodeSearch:
		if cs := payload.CodeSearch(); cs != nil {
			fillIfEmpty(&cs.Query, stringFromMap(frame.RawInput, "pattern", "query"))
			fillIfEmpty(&cs.Path, pathFromStructuredInput(frame.RawInput))
		}
	}
}

func enrichOpenCodeModify(mf *streams.ModifyFilePayload, frame EnrichFrame) {
	if mf == nil || frame.RawInput == nil {
		return
	}
	fillIfEmpty(&mf.FilePath, firstStructuredPath(frame.RawInput, frame.Supplemental))
	oldStr, _ := frame.RawInput["oldString"].(string)
	newStr, _ := frame.RawInput["newString"].(string)
	if oldStr == "" && newStr == "" {
		return
	}
	if len(mf.Mutations) == 0 {
		mf.Mutations = []streams.FileMutation{{Type: streams.MutationPatch}}
	}
	mut := &mf.Mutations[0]
	if mut.Diff == "" {
		mut.Diff = shared.GenerateUnifiedDiff(oldStr, newStr, mf.FilePath, mut.StartLine)
	}
}
