package lifecycle

import (
	"context"
	"database/sql"
	"errors"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

const retainedIdentityMismatchReason = "delivery_identity_mismatch"

func (m *Manager) recoverRetainedJournal(ctx context.Context, lease *agentctl.RuntimeLease, control *agentctl.ControlClient, requested AgentDeliveryRecoveryIdentity, identity DeliveryReconciliationIdentity) DeliveryReconciliationResult {
	fail := func(reason string, err error) DeliveryReconciliationResult {
		return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked, reason, err)
	}
	delivery := m.streamManager.deliveryRepository()
	if delivery == nil {
		return fail("delivery_projection_unavailable", ErrDeliveryRecoveryBlocked)
	}
	cursor, err := delivery.GetAgentDeliveryCursor(ctx, requested.StreamID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fail("delivery_projection_unavailable", err)
	}
	after := uint64(0)
	if cursor != nil {
		if !retainedCursorMatches(cursor, requested) {
			return fail(retainedIdentityMismatchReason, ErrDeliveryOwnerMismatch)
		}
		after = uint64(cursor.ProjectedSequence)
	}
	request := journal.RetainedRecoveryRequest{Root: m.dataDir, SessionID: requested.SessionID, ExecutionID: requested.ExecutionID, IncarnationID: requested.IncarnationID, HarnessGeneration: requested.HarnessGeneration, StreamID: requested.StreamID, SubmissionID: requested.SubmissionID, OriginalRuntime: requested.OriginalRuntime, After: after, Limit: 4}
	execution := &AgentExecution{ID: requested.ExecutionID, TaskID: requested.TaskID, SessionID: requested.SessionID, DeliveryMode: DurableDeliveryV1, DeliveryStreamID: requested.StreamID, DeliveryIncarnationID: requested.IncarnationID, DeliveryHarnessGeneration: requested.HarnessGeneration, DeliveryReplayCursor: after, promptGeneration: requested.PromptGeneration}
	execution.setDeliverySubmissionID(requested.SubmissionID)
	evidence, after, reason, replayErr := m.replayRetainedJournalPages(ctx, lease, control, request, execution)
	if replayErr != nil {
		return fail(reason, replayErr)
	}
	if after > 0 {
		if err = lease.CheckCurrent(); err != nil {
			return fail("runtime_owner_changed", err)
		}
		request.After = after
		request.Acknowledge = after
		if _, err = control.ReadRetainedDelivery(ctx, request); err != nil {
			return fail("delivery_evidence_unavailable", err)
		}
	}
	if !submissionHasRetainedTerminal(&evidence.Submission) {
		result := deliveryRecoveryResult(identity, DeliveryReconciliationUncertain, "terminal_outcome_missing", ErrUncertainPromptDelivery)
		result.ProcessTerminated = evidence.ProcessTerminated
		return result
	}
	if evidence.Submission.TerminalSequence > after {
		return fail("terminal_projection_incomplete", ErrDeliveryRecoveryBlocked)
	}
	settler, ok := delivery.(agentDeliveryTerminalSettler)
	if !ok {
		return fail("terminal_settlement_unavailable", ErrDeliveryRecoveryBlocked)
	}
	if err = lease.CheckCurrent(); err != nil {
		return fail("runtime_owner_changed", err)
	}
	if _, err = settler.SettleAgentDeliveryTerminal(ctx, requested.StreamID, int64(evidence.Submission.TerminalSequence), models.DeliverySubmissionState(evidence.Submission.State), evidence.Submission.UpdatedAt); err != nil {
		return fail("terminal_settlement_failed", err)
	}
	return deliveryRecoveryResult(identity, DeliveryReconciliationTerminalSettled, "", nil)
}

func (m *Manager) replayRetainedJournalPages(ctx context.Context, lease *agentctl.RuntimeLease, control *agentctl.ControlClient, request journal.RetainedRecoveryRequest, execution *AgentExecution) (*journal.RetainedRecoveryResponse, uint64, string, error) {
	after := request.After
	identity := deliveryReconciliationIdentity(AgentDeliveryRecoveryIdentity{SessionID: request.SessionID, ExecutionID: request.ExecutionID, SubmissionID: request.SubmissionID, StreamID: request.StreamID, IncarnationID: request.IncarnationID, HarnessGeneration: request.HarnessGeneration, PromptGeneration: execution.promptGeneration})
	var err error
	var target uint64
	var evidence *journal.RetainedRecoveryResponse
	for {
		if err = lease.CheckCurrent(); err != nil {
			return nil, after, "runtime_owner_changed", err
		}
		evidence, err = control.ReadRetainedDelivery(ctx, request)
		if err != nil {
			return nil, after, "delivery_evidence_unavailable", err
		}
		descriptor := &evidence.Descriptor
		if !matchesDeliveryReconciliationOwner(identity, descriptor, &evidence.Submission) {
			return nil, after, retainedIdentityMismatchReason, ErrDeliveryOwnerMismatch
		}
		if descriptor.Stream == nil {
			break
		}
		if target == 0 {
			target = descriptor.Stream.HighWater
		}
		if err = validateRecoveredReplayStream(*descriptor.Stream, execution, request.StreamID, target); err != nil {
			return nil, after, retainedIdentityMismatchReason, err
		}
		if after > target {
			return nil, after, retainedIdentityMismatchReason, ErrDeliveryRecoveryBlocked
		}
		execution.DeliveryDescriptor = descriptor
		previous := after
		var reason string
		after, reason, err = m.projectRetainedJournalPage(ctx, lease, execution, evidence.Events, after, target)
		if err != nil {
			return nil, after, reason, err
		}
		if after == target {
			break
		}
		if previous == after {
			return nil, after, "terminal_evidence_incomplete", ErrDeliveryRecoveryBlocked
		}
		request.After = after
	}
	return evidence, after, "", nil
}

func (m *Manager) projectRetainedJournalPage(ctx context.Context, lease *agentctl.RuntimeLease, execution *AgentExecution, events []journal.Event, after, target uint64) (uint64, string, error) {
	delivery := m.streamManager.deliveryRepository()
	var err error
	for _, event := range events {
		if event.Sequence > target {
			break
		}
		if event.Sequence != after+1 {
			return after, "terminal_evidence_incomplete", journal.ErrSequenceConflict
		}
		if err = lease.CheckCurrent(); err != nil {
			return after, "runtime_owner_changed", err
		}
		if err = m.streamManager.processRecoveredDeliveryEvent(ctx, execution, nil, delivery, event, 0, true); err != nil {
			return after, "retained_replay_failed", err
		}
		after = event.Sequence
	}
	return after, "", nil
}

func retainedCursorMatches(cursor *models.AgentDeliveryCursor, requested AgentDeliveryRecoveryIdentity) bool {
	return cursor.SessionID == requested.SessionID && cursor.IncarnationID == requested.IncarnationID && cursor.HarnessGeneration == int64(requested.HarnessGeneration) && cursor.ProjectedSequence >= 0
}
