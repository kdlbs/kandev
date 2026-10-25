package handlers

import "context"

type builtinPromptPreparer interface {
	PrepareTaskSessionMCPPrompt(context.Context, string, string) (context.Context, error)
}

func (h *MessageHandlers) prepareBuiltinMessageContext(ctx context.Context, taskID, sessionID string) (context.Context, error) {
	preparer, ok := h.orchestrator.(builtinPromptPreparer)
	if !ok {
		return ctx, nil
	}
	return preparer.PrepareTaskSessionMCPPrompt(ctx, taskID, sessionID)
}
