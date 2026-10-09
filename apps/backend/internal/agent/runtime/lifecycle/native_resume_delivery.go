package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

// AcknowledgeNativeResumeDelivery seals uncertain prior prompts after an
// explicitly authorized native resume. It never dispatches their payloads.
func (m *Manager) AcknowledgeNativeResumeDelivery(ctx context.Context, sessionID string) error {
	execution, exists := m.GetExecutionBySessionID(sessionID)
	if !exists || execution == nil {
		return fmt.Errorf("native resume execution is unavailable")
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return fmt.Errorf("native resume agentctl is unavailable")
	}
	status, err := m.nativeResumeDeliveryStatus(ctx, execution, client)
	if err != nil || status == nil {
		return err
	}
	submissions, err := client.ListDeliverySubmissions(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("list native resume delivery submissions: %w", err)
	}
	if err := m.validateNativeResumeSQLHistory(ctx, execution, status, submissions, false); err != nil {
		return err
	}
	ids, err := nativeResumeInterruptedSubmissionIDs(execution, submissions)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := client.RetireDeliverySubmission(ctx, id); err != nil {
			return fmt.Errorf("acknowledge native resume delivery: %w", err)
		}
	}
	status, err = client.GetDeliveryStatus(ctx, execution.DeliveryStreamID)
	if err != nil {
		return fmt.Errorf("confirm native resume delivery: %w", err)
	}
	if err := validateNativeResumeDeliveryOwner(execution, status); err != nil {
		return err
	}
	if status.Unresolved {
		return fmt.Errorf("native resume delivery remains unresolved")
	}
	submissions, err = client.ListDeliverySubmissions(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("confirm native resume retirement evidence: %w", err)
	}
	return m.validateNativeResumeSQLHistory(ctx, execution, status, submissions, true)
}

func validateNativeResumeDeliveryOwner(execution *AgentExecution, status *agentctl.DeliveryStatus) error {
	if !execution.isSessionInitialized() || execution.ACPSessionID == "" {
		return fmt.Errorf("native conversation has not resumed")
	}
	if status == nil || !status.Durable || status.Version != journal.CurrentVersion {
		return fmt.Errorf("native resume durable storage is unavailable")
	}
	if status.SessionID != execution.SessionID || status.IncarnationID != execution.DeliveryIncarnationID ||
		status.HarnessGeneration != execution.DeliveryHarnessGeneration || status.StreamID != execution.DeliveryStreamID {
		return journal.ErrOwnerMismatch
	}
	return nil
}

func nativeResumeInterruptedSubmissionIDs(execution *AgentExecution, submissions []journal.Submission) ([]string, error) {
	var ids []string
	for _, submission := range submissions {
		switch submission.State {
		case journal.SubmissionPrepared, journal.SubmissionAccepted, journal.SubmissionDispatching:
			return nil, fmt.Errorf("native resume cannot acknowledge live delivery work")
		case journal.SubmissionCompleted:
			if !submission.TerminalEventRetained {
				return nil, fmt.Errorf("native resume cannot acknowledge incomplete terminal evidence")
			}
		case journal.SubmissionInterruptedUnknown:
			if submission.Retired {
				continue
			}
			if submission.SessionID != execution.SessionID || submission.IncarnationID != execution.DeliveryIncarnationID ||
				submission.HarnessGeneration == 0 || submission.HarnessGeneration > execution.DeliveryHarnessGeneration {
				return nil, journal.ErrOwnerMismatch
			}
			ids = append(ids, submission.ID)
		}
	}
	return ids, nil
}

func (m *Manager) checkNativeResumeLegacyHistory(ctx context.Context, sessionID string) error {
	if m.streamManager == nil || m.streamManager.deliveryRepository() == nil {
		return nil
	}
	active, err := activeBackendSubmissions(ctx, m.streamManager.deliveryRepository(), sessionID)
	if err != nil {
		return err
	}
	if len(active) != 0 {
		return fmt.Errorf("%w: legacy peer cannot acknowledge unresolved v1 history", ErrDurableAdoptionBlocked)
	}
	return nil
}

func (m *Manager) nativeResumeDeliveryStatus(ctx context.Context, execution *AgentExecution, client *agentctl.Client) (*agentctl.DeliveryStatus, error) {
	status, err := client.GetDeliveryStatus(ctx, execution.DeliveryStreamID)
	if err != nil {
		_, advertised := client.DurableDeliveryCapability()
		var httpError *agentctl.DeliveryHTTPError
		if !advertised && errors.As(err, &httpError) && httpError.StatusCode == http.StatusNotFound {
			return nil, m.checkNativeResumeLegacyHistory(ctx, execution.SessionID)
		}
		return nil, fmt.Errorf("read native resume delivery status: %w", err)
	}
	if !status.Durable && !status.Unresolved &&
		(status.Reason == durableDeliveryStorageNotDurable || status.Version == 0 && status.Reason == "") {
		return nil, m.checkNativeResumeLegacyHistory(ctx, execution.SessionID)
	}
	if err := validateNativeResumeDeliveryOwner(execution, status); err != nil {
		return nil, err
	}
	return status, nil
}
