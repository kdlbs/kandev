package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type stepCompleteRejectionStore interface {
	SetSessionMetadataKey(context.Context, string, string, interface{}) error
}

const stepCompleteRejectionReasonLimit = 400

func (h *Handlers) recordStepCompleteRejection(ctx context.Context, sessionID, stepID, reason string) {
	writer, ok := h.sessionRepo.(stepCompleteRejectionStore)
	if !ok || sessionID == "" || stepID == "" || strings.TrimSpace(reason) == "" {
		return
	}
	rejection := models.StepCompleteRejection{StepID: stepID, Reason: boundStepCompleteRejectionReason(reason), At: time.Now().UTC()}
	if err := writer.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyStepCompleteRejection, rejection.ToMap()); err != nil {
		h.logger.Debug("step_complete: failed to record refusal reason", zap.String("session_id", sessionID), zap.Error(err))
	}
}

func (h *Handlers) clearStepCompleteRejection(ctx context.Context, sessionID string) {
	writer, ok := h.sessionRepo.(stepCompleteRejectionStore)
	if !ok || sessionID == "" {
		return
	}
	if err := writer.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyStepCompleteRejection, nil); err != nil {
		h.logger.Debug("step_complete: failed to clear refusal reason", zap.String("session_id", sessionID), zap.Error(err))
	}
}

func boundStepCompleteRejectionReason(reason string) string {
	runes := []rune(strings.TrimSpace(reason))
	if len(runes) > stepCompleteRejectionReasonLimit {
		runes = runes[:stepCompleteRejectionReasonLimit]
	}
	return strings.TrimSpace(string(runes))
}
