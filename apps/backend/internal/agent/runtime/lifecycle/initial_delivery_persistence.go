package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type initialDeliveryAdmissionStore interface {
	PrepareAgentDeliverySubmission(context.Context, *models.AgentDeliverySubmission) (bool, error)
	GetAgentDeliverySubmission(context.Context, string) (*models.AgentDeliverySubmission, error)
	TransitionAgentDeliverySubmission(context.Context, string, models.DeliverySubmissionState, models.DeliverySubmissionState, string, time.Time) (bool, error)
}

func (sm *SessionManager) persistInitialDeliveryBeforeDispatch(ctx context.Context, execution *AgentExecution, prompt string, attachments []v1.MessageAttachment, submissionID string) error {
	initialID := initialPromptDeliverySubmissionID(execution)
	if initialID == "" || submissionID == "" || deliverySubmissionIdentityForPrompt(submissionID, "") != deliverySubmissionIdentityForPrompt(initialID, "") {
		return nil
	}
	if sm.streamManager == nil {
		return errors.New("initial durable delivery repository is unavailable")
	}
	store, ok := sm.streamManager.deliveryRepository().(initialDeliveryAdmissionStore)
	if !ok {
		return errors.New("initial durable delivery admission is unavailable")
	}
	payload, err := json.Marshal(struct {
		Text        string                 `json:"text"`
		Attachments []v1.MessageAttachment `json:"attachments,omitempty"`
	}{Text: prompt, Attachments: attachments})
	if err != nil {
		return err
	}
	id := deliverySubmissionIdentityForPrompt(submissionID, "")
	now := time.Now().UTC()
	created, err := store.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: id, SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
		HarnessGeneration: int64(execution.DeliveryHarnessGeneration), OwnerGeneration: int64(execution.DeliveryHarnessGeneration),
		DispatchAttemptID: execution.ID, PayloadHash: journal.SubmissionHash(payload), Payload: payload,
		State: models.DeliverySubmissionDispatching, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return fmt.Errorf("persist initial durable submission: %w", err)
	}
	if !created {
		stored, readErr := store.GetAgentDeliverySubmission(ctx, id)
		if readErr != nil || !callerAdmittedInitialSubmissionMatches(stored, execution, id, payload) {
			return errors.New("initial durable submission admission identity changed")
		}
	}
	execution.setMetadataValue(initialDeliverySubmissionIDMetadataKey, id)
	return nil
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
