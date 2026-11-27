package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// PromptAdmissionPersistenceError distinguishes failed durable admission from
// ownership or cancellation rejections that must leave the current owner intact.
type PromptAdmissionPersistenceError struct{ Err error }

func (e *PromptAdmissionPersistenceError) Error() string { return e.Err.Error() }
func (e *PromptAdmissionPersistenceError) Unwrap() error { return e.Err }

type initialDeliveryAdmissionStore interface {
	PrepareAgentDeliverySubmission(context.Context, *models.AgentDeliverySubmission) (bool, error)
	GetAgentDeliverySubmission(context.Context, string) (*models.AgentDeliverySubmission, error)
	TransitionAgentDeliverySubmission(context.Context, string, models.DeliverySubmissionState, models.DeliverySubmissionState, string, time.Time) (bool, error)
}

func (sm *SessionManager) persistInitialDeliveryBeforeDispatch(ctx context.Context, execution *AgentExecution, prompt string, attachments []v1.MessageAttachment, submissionID string) (*models.AgentDeliverySubmission, error) {
	initialID := initialPromptDeliverySubmissionID(execution)
	if initialID == "" || submissionID == "" || deliverySubmissionIdentityForPrompt(submissionID, "") != deliverySubmissionIdentityForPrompt(initialID, "") {
		return nil, nil
	}
	if sm.streamManager == nil {
		return nil, errors.New("initial durable delivery repository is unavailable")
	}
	store, ok := sm.streamManager.deliveryRepository().(initialDeliveryAdmissionStore)
	if !ok {
		return nil, errors.New("initial durable delivery admission is unavailable")
	}
	payload, err := json.Marshal(struct {
		Text        string                 `json:"text"`
		Attachments []v1.MessageAttachment `json:"attachments,omitempty"`
	}{Text: prompt, Attachments: attachments})
	if err != nil {
		return nil, err
	}
	id := deliverySubmissionIdentityForPrompt(submissionID, "")
	now := time.Now().UTC()
	submission := &models.AgentDeliverySubmission{
		ID: id, SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
		HarnessGeneration: int64(execution.DeliveryHarnessGeneration), OwnerGeneration: int64(execution.DeliveryHarnessGeneration),
		DispatchAttemptID: execution.ID, PayloadHash: journal.SubmissionHash(payload), Payload: payload,
		State: models.DeliverySubmissionDispatching, CreatedAt: now, UpdatedAt: now,
	}
	created, err := store.PrepareAgentDeliverySubmission(ctx, submission)
	if err != nil {
		return nil, fmt.Errorf("persist initial durable submission: %w", err)
	}
	if !created {
		stored, readErr := store.GetAgentDeliverySubmission(ctx, id)
		if readErr != nil || !callerAdmittedInitialSubmissionMatches(stored, execution, id, payload) {
			return nil, errors.New("initial durable submission admission identity changed")
		}
		submission = stored
	}
	execution.setMetadataValue(initialDeliverySubmissionIDMetadataKey, id)
	return submission, nil
}

func (sm *SessionManager) settleUnsentInitialDelivery(ctx context.Context, submission *models.AgentDeliverySubmission, cause error) error {
	var notSent *agentctl.RequestNotSentError
	if submission == nil || !errors.As(cause, &notSent) {
		return cause
	}
	writer, ok := sm.streamManager.deliveryRepository().(interface {
		FailUnsentAgentDeliverySubmission(context.Context, *models.AgentDeliverySubmission, time.Time) (bool, error)
	})
	if !ok {
		return errors.Join(cause, errors.New("initial unsent delivery settlement is unavailable"))
	}
	changed, err := writer.FailUnsentAgentDeliverySubmission(context.WithoutCancel(ctx), submission, time.Now().UTC())
	if err != nil {
		return errors.Join(cause, err)
	}
	if !changed {
		return errors.Join(cause, errors.New("initial unsent delivery reservation changed"))
	}
	return cause
}

func callerAdmittedInitialSubmissionMatches(stored *models.AgentDeliverySubmission, execution *AgentExecution, id string, payload []byte) bool {
	// The launch handoff identifies the canonical first instruction. Runtime-owned
	// records and an execution that already dispatched require reconciliation.
	initialID := execution.metadataString(initialDeliverySubmissionIDMetadataKey)
	return initialID != "" && deliverySubmissionIdentityForPrompt(initialID, "") == id &&
		execution.promptGenerationSnapshot() == 0 &&
		stored != nil && stored.ID == id && stored.SessionID == execution.SessionID &&
		stored.IncarnationID == execution.DeliveryIncarnationID &&
		stored.HarnessGeneration == int64(execution.DeliveryHarnessGeneration) &&
		stored.OwnerGeneration == int64(execution.DeliveryHarnessGeneration) &&
		stored.DispatchAttemptID != "" && deliverySubmissionIdentityForPrompt(stored.DispatchAttemptID, "") == id &&
		stored.State == models.DeliverySubmissionDispatching &&
		stored.PayloadHash == journal.SubmissionHash(payload) && bytes.Equal(stored.Payload, payload)
}
