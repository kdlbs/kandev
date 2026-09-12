package handlers

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository/admission"
)

// composeInitialTaskBrief keeps the prepared session's original brief in the
// first direct prompt while preserving the user's instruction as a separate
// paragraph. Trimmed equality avoids repeating a brief that the user already
// supplied verbatim.
func composeInitialTaskBrief(brief, instruction string) string {
	brief = strings.TrimSpace(brief)
	instruction = strings.TrimSpace(instruction)
	if brief == "" {
		return instruction
	}
	if instruction == "" || brief == instruction {
		return brief
	}
	return brief + "\n\n" + instruction
}

func (h *MessageHandlers) prepareInitialTaskBriefCandidate(
	ctx context.Context,
	req wsAddMessageRequest,
	sessionResp *dto.GetTaskSessionResponse,
	task *models.Task,
	configMode, startCreatedSession, titleOwner, hasMessageContent bool,
) *admission.InitialTaskBriefCandidate {
	if !eligibleForInitialTaskBrief(task, sessionResp, configMode, startCreatedSession, hasMessageContent) {
		return nil
	}
	// Compose the raw brief and instruction before the server-owned preparer sees
	// either value. This keeps equality deduplication and saved-prompt expansion
	// on one acceptance-time snapshot.
	candidateRawContent := composeInitialTaskBrief(task.Description, req.Content)
	candidateContent, candidateTrustedPromptContext, promptReferencesPrepared := h.prepareDirectPrompt(
		ctx, candidateRawContent, sessionResp.Session.IsPassthrough,
	)
	if len(req.PlanCommentRefs) > 0 {
		candidateContent = plancomments.WithPlaceholder(candidateContent)
	}
	candidateContent = orchestrator.AppendEntityReferenceContext(candidateContent, req.EntityReferences)
	candidateContent = h.injectMessageContext(
		ctx, req, sessionResp, task, configMode, startCreatedSession, titleOwner,
		req.includeCanvasGuidance, candidateContent, candidateTrustedPromptContext,
	)
	return &admission.InitialTaskBriefCandidate{
		DescriptionSnapshot:      task.Description,
		Content:                  candidateContent,
		PromptReferenceContext:   candidateTrustedPromptContext,
		PromptReferencesPrepared: promptReferencesPrepared,
	}
}
