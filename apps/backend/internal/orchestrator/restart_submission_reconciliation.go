package orchestrator

import (
	"context"
	"errors"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

func (s *Service) reconcileRestartSubmissionStates(ctx context.Context, inputs *silentRestoreInputs) error {
	lister, ok := s.repo.(nativeResumeDeliverySubmissionReader)
	if !ok {
		return errors.New("restart submission inventory is unavailable")
	}
	rows, err := lister.ListAgentDeliverySubmissions(ctx, inputs.session.ID)
	if err != nil {
		return err
	}
	var dispatching bool
	for _, row := range rows {
		dispatching = dispatching || row.State == models.DeliverySubmissionDispatching
	}
	if !dispatching {
		return nil
	}
	client, ok := s.agentManager.(deliveryRecordEvidenceClient)
	if !ok {
		return errors.New("restart journal evidence is unavailable")
	}
	evidence, err := client.InspectDeliveryRecordEvidence(ctx, agentruntime.DeliveryRecordEvidenceRequest{
		TaskID: inputs.task.ID, SessionID: inputs.session.ID, ExecutionID: inputs.recovery.AgentExecutionID,
		IncarnationID: inputs.recovery.IncarnationID, HarnessGeneration: uint64(inputs.recovery.HarnessGeneration),
		OriginalRuntime: &inputs.recovery.OriginalRuntime,
	})
	if err != nil {
		return err
	}
	if err := s.validateRestartSubmissionEvidence(ctx, inputs, rows, evidence); err != nil {
		return err
	}
	store, ok := s.repo.(taskrepo.AgentDeliveryRepository)
	if !ok {
		return errors.New("restart submission store is unavailable")
	}
	for _, row := range rows {
		if row.State != models.DeliverySubmissionDispatching {
			continue
		}
		changed, err := store.TransitionAgentDeliverySubmission(ctx, row.ID, row.State,
			models.DeliverySubmissionInterruptedUnknown, unknownPromptOutcome, time.Now().UTC())
		if err != nil {
			return err
		}
		if !changed {
			return errors.New("restart submission changed during reconciliation")
		}
	}
	return nil
}

func (s *Service) validateRestartSubmissionEvidence(ctx context.Context, inputs *silentRestoreInputs, rows []*models.AgentDeliverySubmission, evidence *agentruntime.DeliveryRecordEvidence) error {
	if evidence == nil || evidence.ProcessTerminated == nil || !*evidence.ProcessTerminated {
		return errors.New("restart source termination is not proven")
	}
	descriptor := evidence.Descriptor
	if !restartJournalOwnerMatches(inputs, descriptor) {
		return errors.New("restart journal owner changed")
	}

	retained := make(map[string]journal.SubmissionSummary, len(descriptor.Submissions))
	for _, peer := range descriptor.Submissions {
		if _, duplicate := retained[peer.ID]; duplicate {
			return errors.New("duplicate restart journal evidence")
		}
		retained[peer.ID] = peer
	}
	for _, row := range rows {
		if terminalDeliverySubmission(row.State) {
			continue
		}
		if err := s.validateRestartSubmissionRow(ctx, inputs, row, evidence, retained); err != nil {
			return err
		}
	}
	if len(retained) != 0 {
		return errors.New("restart journal has unassociated work")
	}
	return nil
}

func (s *Service) validateRestartSubmissionRow(ctx context.Context, inputs *silentRestoreInputs, row *models.AgentDeliverySubmission, evidence *agentruntime.DeliveryRecordEvidence, retained map[string]journal.SubmissionSummary) error {
	peer, found := retained[row.ID]
	if !found && row.State == models.DeliverySubmissionInterruptedUnknown && row.ID != inputs.recovery.SubmissionID {
		return s.verifyRetiredRestartHistory(ctx, inputs, row, evidence)
	}
	if !found || !matchingRestartSubmission(inputs, row, peer) {
		return errors.New("restart SQL and journal evidence disagree")
	}
	delete(retained, row.ID)
	return nil
}

func matchingRestartSubmission(inputs *silentRestoreInputs, row *models.AgentDeliverySubmission, peer journal.SubmissionSummary) bool {
	return restartSubmissionStatesMatch(row.State, peer.State) &&
		row.SessionID == inputs.session.ID && peer.SessionID == row.SessionID &&
		row.IncarnationID == inputs.recovery.IncarnationID && peer.IncarnationID == row.IncarnationID &&
		row.HarnessGeneration > 0 && row.HarnessGeneration <= inputs.recovery.HarnessGeneration &&
		peer.HarnessGeneration == uint64(row.HarnessGeneration) && peer.StreamID != "" &&
		(row.ID != inputs.recovery.SubmissionID || peer.StreamID == inputs.recovery.StreamID) &&
		peer.Hash != "" && peer.Hash == row.PayloadHash
}

func restartSubmissionStatesMatch(sqlState models.DeliverySubmissionState, peerState journal.SubmissionState) bool {
	return (sqlState == models.DeliverySubmissionDispatching || sqlState == models.DeliverySubmissionInterruptedUnknown) &&
		(peerState == journal.SubmissionDispatching || peerState == journal.SubmissionInterruptedUnknown)
}

func restartJournalOwnerMatches(inputs *silentRestoreInputs, descriptor journal.RecoveryDescriptor) bool {
	return descriptor.Durable && descriptor.Version == journal.CurrentVersion && !descriptor.SubmissionsTruncated &&
		descriptor.SessionID == inputs.session.ID && descriptor.IncarnationID == inputs.recovery.IncarnationID &&
		descriptor.HarnessGeneration == uint64(inputs.recovery.HarnessGeneration)
}

func (s *Service) verifyRetiredRestartHistory(ctx context.Context, inputs *silentRestoreInputs, row *models.AgentDeliverySubmission, evidence *agentruntime.DeliveryRecordEvidence) error {
	client, ok := s.agentManager.(deliveryRecordEvidenceClient)
	if !ok {
		return errors.New("retired restart evidence is unavailable")
	}
	retained, err := client.InspectDeliveryRecordEvidence(ctx, agentruntime.DeliveryRecordEvidenceRequest{
		TaskID: inputs.session.TaskID, SessionID: inputs.session.ID, ExecutionID: inputs.recovery.AgentExecutionID,
		IncarnationID: inputs.recovery.IncarnationID, HarnessGeneration: uint64(inputs.recovery.HarnessGeneration),
		OriginalRuntime: &inputs.recovery.OriginalRuntime, RetiredSubmissionID: row.ID})
	if err != nil {
		return err
	}
	if retained == nil || retained.ProcessTerminated == nil || !*retained.ProcessTerminated ||
		!sameRestartDescriptor(evidence.Descriptor, retained.Descriptor) {
		return errors.New("retired restart owner changed")
	}
	if !matchingRetiredRestartSubmission(inputs, row, retained.RetiredSubmission) {
		return errors.New("retired restart SQL and journal evidence disagree")
	}
	return nil
}

func matchingRetiredRestartSubmission(inputs *silentRestoreInputs, row *models.AgentDeliverySubmission, peer *journal.Submission) bool {
	return peer != nil && peer.Retired && peer.State == journal.SubmissionInterruptedUnknown && peer.ID == row.ID &&
		peer.StreamID != "" && peer.SessionID == inputs.session.ID && row.SessionID == peer.SessionID &&
		peer.IncarnationID == inputs.recovery.IncarnationID && row.IncarnationID == peer.IncarnationID &&
		row.HarnessGeneration > 0 && row.HarnessGeneration <= inputs.recovery.HarnessGeneration &&
		peer.HarnessGeneration == uint64(row.HarnessGeneration) && peer.Hash != "" && peer.Hash == row.PayloadHash
}

func sameRestartDescriptor(a, b journal.RecoveryDescriptor) bool {
	if a.SessionID != b.SessionID || a.IncarnationID != b.IncarnationID || a.HarnessGeneration != b.HarnessGeneration ||
		a.Durable != b.Durable || a.Version != b.Version || a.SubmissionCount != b.SubmissionCount ||
		a.SubmissionsTruncated != b.SubmissionsTruncated || len(a.Submissions) != len(b.Submissions) {
		return false
	}
	for i := range a.Submissions {
		if !journal.SubmissionSummariesEqual(a.Submissions[i], b.Submissions[i]) {
			return false
		}
	}
	return true
}
