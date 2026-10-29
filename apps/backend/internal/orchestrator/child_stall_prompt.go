package orchestrator

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

// childStallAlertPromptContent returns the prompt text of a queued child-turn
// stall alert. Every folded candidate is rechecked at the dispatch boundary,
// and only the candidates that still qualify contribute text, so a candidate
// that stopped qualifying never reaches the parent prompt. It reports false
// when no folded candidate qualifies, so the queued alert is discarded without
// creating a model turn.
func (s *Service) childStallAlertPromptContent(
	ctx context.Context,
	queuedMsg *messagequeue.QueuedMessage,
) (string, bool) {
	if queuedMsg == nil {
		return "", true
	}
	if !messagequeue.IsChildStallAlert(queuedMsg) {
		return queuedMessagePromptContent(queuedMsg), true
	}
	producer := s.childStallProducer
	if producer == nil {
		// Without the producer no candidate can be rechecked, so the alert is
		// delivered exactly as it was admitted.
		return queuedMessagePromptContent(queuedMsg), true
	}
	descriptors := messagequeue.ChildStallAlertDescriptors(queuedMsg)
	if len(descriptors) == 0 {
		// An alert without candidate provenance keeps its queued text.
		return queuedMessagePromptContent(queuedMsg), true
	}
	eligible := producer.eligibleChildStallDescriptors(ctx, queuedMsg)
	if len(eligible) == 0 {
		return "", false
	}
	if len(eligible) == len(descriptors) {
		return queuedMessagePromptContent(queuedMsg), true
	}
	parts := make([]string, 0, len(eligible))
	for _, descriptor := range eligible {
		parts = append(parts, formatChildStallPromptText(
			descriptor.ChildTaskTitle,
			descriptor.ChildTaskID,
			descriptor.TurnID,
			descriptor.StepName,
			descriptor.Cause,
			descriptor.QuestionID,
		))
	}
	// Folding concatenates the candidate texts in descriptor order, so the
	// surviving candidates rebuild the same prompt without the stale ones.
	return queuedMessagePromptContentFrom(queuedMsg, strings.Join(parts, "\n\n")), true
}
