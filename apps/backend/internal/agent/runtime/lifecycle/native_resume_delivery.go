package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
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
	if m.streamManager == nil || m.streamManager.deliveryRepository() == nil {
		return fmt.Errorf("%w: %w: SQL submission repository is unavailable", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable)
	}
	if err := m.reconcileNativeResumeSQLDispatch(ctx, execution, status, submissions); err != nil {
		return err
	}
	if _, err := authenticatedDeliveryReplayCursor(ctx, execution, m.streamManager.deliveryRepository(), status); err != nil {
		return err
	}
	// Dispatch reconciliation can change SQL state; pin one fresh inventory for validation and retirement selection.
	rows, err := activeBackendSubmissions(ctx, m.streamManager.deliveryRepository(), sessionID)
	if err != nil {
		return err
	}
	if err := validateNativeResumeSQLRows(execution, status, submissions, rows, false); err != nil {
		return err
	}
	ids, err := nativeResumeRetirementIDs(execution, status, submissions, rows)
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

func nativeResumeRetirementIDs(execution *AgentExecution, status *agentctl.DeliveryStatus, submissions []journal.Submission, rows []*models.AgentDeliverySubmission) ([]string, error) {
	var ids []string
	backend := make(map[string]*models.AgentDeliverySubmission, len(rows))
	for _, row := range rows {
		backend[row.ID] = row
	}
	for _, submission := range submissions {
		if submission.Retired {
			continue
		}
		switch submission.State {
		case journal.SubmissionPrepared, journal.SubmissionAccepted, journal.SubmissionDispatching:
			return nil, fmt.Errorf("native resume cannot acknowledge live delivery work")
		case journal.SubmissionInterruptedUnknown, journal.SubmissionCompleted, journal.SubmissionFailed, journal.SubmissionCancelled:
			if !journal.HasUnprovenOutcome(submission) {
				continue
			}
			sqlSubmission, found := backend[submission.ID]
			if !found || sqlSubmission.State != models.DeliverySubmissionInterruptedUnknown {
				if submission.State == journal.SubmissionFailed || submission.State == journal.SubmissionCancelled {
					continue
				}
				return nil, fmt.Errorf("%w: native resume requires uncertain SQL history", ErrDurableAdoptionBlocked)
			}
			if !peerMatchesUnknownHistory(execution, status, submission, sqlSubmission) {
				return nil, fmt.Errorf("%w: native resume submission identity changed", ErrDurableAdoptionBlocked)
			}
			ids = append(ids, submission.ID)
		default:
			return nil, fmt.Errorf("%w: unsupported native resume submission state", ErrDurableAdoptionBlocked)
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
