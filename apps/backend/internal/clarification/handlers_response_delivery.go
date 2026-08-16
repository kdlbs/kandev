package clarification

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

func (h *Handlers) confirmLiveClarificationResponseDelivery(
	ctx context.Context,
	pendingID string,
	claim *clarificationResponseClaim,
) error {
	if !h.finalizeClarificationResponseDelivery(ctx, pendingID, claim) {
		return errors.New("clarification delivery confirmation failed")
	}
	return nil
}

func (h *Handlers) finalizeClarificationResponseDelivery(
	ctx context.Context,
	pendingID string,
	claim *clarificationResponseClaim,
) bool {
	persistenceCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	finalizedMessages, finalized, err := h.messageCreator.FinalizeClarificationResponseDelivery(
		persistenceCtx,
		pendingID,
		claim.terminalStatus,
		claim.messages,
	)
	if err != nil {
		h.logger.Error("failed to finalize clarification response delivery",
			zap.String("pending_id", pendingID),
			zap.Error(err))
		return false
	}
	if !finalized {
		h.logger.Warn("clarification response delivery was not finalizable",
			zap.String("pending_id", pendingID))
		return false
	}
	claim.messages = finalizedMessages
	return true
}

func clarificationClaimTurnID(messages []*taskmodels.Message) string {
	turnID, err := clarificationClaimTurnIdentity(messages)
	if err != nil {
		return ""
	}
	return turnID
}

func clarificationClaimTurnIdentity(messages []*taskmodels.Message) (string, error) {
	turnID := ""
	turnMessageID := ""
	for _, message := range messages {
		if message == nil || message.ID == "" || message.TurnID == "" {
			continue
		}
		if turnID == "" {
			turnID = message.TurnID
			turnMessageID = message.ID
			continue
		}
		if message.TurnID != turnID {
			return "", fmt.Errorf(
				"clarification bundle mixes turn %q from message %s with turn %q from message %s",
				turnID,
				turnMessageID,
				message.TurnID,
				message.ID,
			)
		}
	}
	return turnID, nil
}

func (h *Handlers) publishPrimaryAnsweredEvent(
	ctx context.Context,
	pendingID string,
	answers []Answer,
	rejected bool,
	rejectReason, clarificationTurnID string,
) {
	if h.eventBus == nil {
		return
	}
	persistenceCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	clarificationCtx, err := h.resolveClarificationEventContext(persistenceCtx, pendingID)
	if err != nil {
		h.logger.Warn("failed to resolve context for primary clarification event",
			zap.String("pending_id", pendingID),
			zap.Error(err))
		return
	}
	if clarificationCtx.SessionID == "" || clarificationCtx.TaskID == "" {
		h.logger.Warn("missing session/task for primary clarification event",
			zap.String("pending_id", pendingID),
			zap.String("session_id", clarificationCtx.SessionID),
			zap.String("task_id", clarificationCtx.TaskID))
		return
	}

	answerText := buildAnswerSummary(clarificationCtx.Questions, answers, rejected, rejectReason)
	eventData := map[string]any{
		metaSessionIDKey:        clarificationCtx.SessionID,
		metaTaskIDKey:           clarificationCtx.TaskID,
		metaPendingIDKey:        pendingID,
		"clarification_turn_id": clarificationTurnID,
		metaQuestionKey:         clarificationCtx.QuestionSummary,
		"answer_text":           answerText,
		metaRejectedKey:         rejected,
		"reject_reason":         rejectReason,
	}
	if err := h.eventBus.Publish(persistenceCtx, events.ClarificationPrimaryAnswered, bus.NewEvent(
		events.ClarificationPrimaryAnswered,
		"clarification-handlers",
		eventData,
	)); err != nil {
		h.logger.Warn("failed to publish primary clarification event",
			zap.String("pending_id", pendingID),
			zap.String("session_id", clarificationCtx.SessionID),
			zap.Error(err))
	}
}
