package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

const conversationForkModelLimitSource = "models.dev"

// ConversationForkModelLimitLookup resolves optional context-window metadata
// for a model selected by the caller. A missing model remains an unknown limit.
type ConversationForkModelLimitLookup interface {
	LookupConversationForkContextLimit(context.Context, string) (int64, bool)
}

// SetConversationForkModelLimitLookup installs the cached model metadata used
// by informational fork estimates. It is wired once during service startup.
func (s *Service) SetConversationForkModelLimitLookup(lookup ConversationForkModelLimitLookup) {
	s.conversationForkModelLimitLookup = lookup
}

func (s *Service) estimateConversationFork(
	ctx context.Context,
	text, modelID string,
	attachmentsUnmeasured bool,
) (models.ConversationForkEstimate, error) {
	tokens, err := estimateConversationForkTokens(text)
	if err != nil {
		return models.ConversationForkEstimate{}, fmt.Errorf("estimate conversation fork tokens: %w", err)
	}
	estimate := models.ConversationForkEstimate{
		EstimatedTokens: int64(tokens), Method: conversationForkCompilerMethod,
		ModelID: modelID, AttachmentsUnmeasured: attachmentsUnmeasured,
	}
	if lookup := s.conversationForkModelLimitLookup; lookup != nil && modelID != "" {
		if limit, ok := lookup.LookupConversationForkContextLimit(ctx, modelID); ok && limit > 0 {
			estimate.ContextLimit = &limit
			estimate.LimitSource = conversationForkModelLimitSource
		}
	}
	return estimate, nil
}
