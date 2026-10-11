package lifecycle

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/common/processidentity"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

// AgentDeliveryRecoveryIdentity is the durable owner identity needed to
// reconcile one submission after its in-memory execution has been removed.
type AgentDeliveryRecoveryIdentity struct {
	OriginalRuntime   processidentity.Identity
	TaskID            string
	SessionID         string
	ExecutionID       string
	SubmissionID      string
	StreamID          string
	IncarnationID     string
	HarnessGeneration uint64
	PromptGeneration  uint64
}

// RecoverAgentPromptStreamWithIdentity resolves a retained owner through the
// current authenticated runtime lease when the execution is no longer tracked.
// It reads and projects terminal evidence only; it never starts or prompts a
// harness.
func (m *Manager) RecoverAgentPromptStreamWithIdentity(
	ctx context.Context,
	requested AgentDeliveryRecoveryIdentity,
) DeliveryReconciliationResult {
	identity := deliveryReconciliationIdentity(requested)
	if !completeAgentDeliveryRecoveryIdentity(requested) {
		return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked,
			"recovery_identity_incomplete", ErrDeliveryRecoveryBlocked)
	}
	if execution, exists := m.GetExecutionBySessionID(requested.SessionID); exists {
		if !agentDeliveryRecoveryExecutionMatches(execution, requested) {
			return deliveryRecoveryResult(identity, DeliveryReconciliationOwnerMismatch,
				"execution_identity_changed", ErrDeliveryOwnerMismatch)
		}
		if m.isRetiredLocalExecution(execution) {
			return m.recoverRetainedAgentDeliveryWithoutExecution(ctx, requested, identity)
		}
		if m.streamManager == nil {
			return deliveryRecoveryResult(identity, DeliveryReconciliationTransportUnavailable,
				"delivery_recovery_unavailable", ErrDeliveryTransportUnavailable)
		}
		return m.streamManager.ReconcileAgentDelivery(ctx, execution)
	}
	return m.recoverRetainedAgentDeliveryWithoutExecution(ctx, requested, identity)
}

func (m *Manager) recoverRetainedAgentDeliveryWithoutExecution(
	ctx context.Context,
	requested AgentDeliveryRecoveryIdentity,
	identity DeliveryReconciliationIdentity,
) DeliveryReconciliationResult {
	if m.runtimeOwner == nil || m.streamManager == nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationTransportUnavailable,
			"runtime_unavailable", ErrDeliveryTransportUnavailable)
	}
	lease, err := m.runtimeOwner.Acquire(ctx)
	if err != nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationTransportUnavailable,
			"runtime_unavailable", err)
	}
	defer lease.Close()
	control := lease.NewControlClient(m.logger)
	if control == nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationTransportUnavailable,
			"runtime_unavailable", ErrDeliveryTransportUnavailable)
	}
	instance, err := control.GetInstance(ctx, requested.ExecutionID)
	if err != nil {
		if errors.Is(err, agentctl.ErrInstanceNotFound) {
			return m.recoverRetainedJournal(ctx, lease, control, requested, identity)
		}
		return deliveryRecoveryResult(identity, DeliveryReconciliationTransportUnavailable,
			"instance_unavailable", err)
	}
	if !retainedInstanceMatches(instance, requested) {
		return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked,
			"instance_identity_mismatch", ErrDeliveryOwnerMismatch)
	}
	if err := lease.CheckCurrent(); err != nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationOwnerMismatch,
			"runtime_owner_changed", err)
	}
	client := lease.NewBoundInstanceClient(instance.Port, m.logger)
	if client == nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationTransportUnavailable,
			"instance_unavailable", ErrDeliveryTransportUnavailable)
	}
	defer client.Close()
	return m.recoverRetainedInstance(ctx, lease, client, requested)
}

func (m *Manager) recoverRetainedInstance(ctx context.Context, lease *agentctl.RuntimeLease, client *agentctl.Client, requested AgentDeliveryRecoveryIdentity) DeliveryReconciliationResult {
	identity := deliveryReconciliationIdentity(requested)
	status, err := client.GetStatus(ctx)
	if err != nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationTransportUnavailable,
			"instance_unavailable", err)
	}
	descriptor, err := client.GetDeliveryStatus(ctx, requested.StreamID)
	if err != nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked,
			"delivery_evidence_unavailable", err)
	}
	submission, err := client.GetDeliverySubmission(ctx, requested.SubmissionID)
	if err != nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked,
			"submission_evidence_unavailable", err)
	}
	if !matchesDeliveryReconciliationOwner(identity, descriptor, submission) {
		return deliveryRecoveryResult(identity, DeliveryReconciliationOwnerMismatch,
			"delivery_identity_mismatch", ErrDeliveryOwnerMismatch)
	}
	if !submissionHasRetainedTerminal(submission) {
		if reason := deliveryPressureRecoveryReason(status); reason != "" {
			return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked, reason, ErrDeliveryRecoveryBlocked)
		}
		result := deliveryRecoveryResult(identity, DeliveryReconciliationUncertain,
			"terminal_outcome_missing", ErrUncertainPromptDelivery)
		return result
	}
	if descriptor.Stream == nil || submission.TerminalSequence > descriptor.Stream.HighWater {
		return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked,
			"terminal_evidence_incomplete", ErrDeliveryRecoveryBlocked)
	}
	if err := lease.CheckCurrent(); err != nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationOwnerMismatch,
			"runtime_owner_changed", err)
	}
	return m.projectRetainedInstance(ctx, lease, client, requested, descriptor, submission)
}

func (m *Manager) projectRetainedInstance(ctx context.Context, lease *agentctl.RuntimeLease, client *agentctl.Client, requested AgentDeliveryRecoveryIdentity, descriptor *agentctl.DeliveryStatus, submission *journal.Submission) DeliveryReconciliationResult {
	identity := deliveryReconciliationIdentity(requested)
	delivery := m.streamManager.deliveryRepository()
	if delivery == nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked,
			"delivery_projection_unavailable", ErrDeliveryRecoveryBlocked)
	}
	after, err := m.streamManager.deliveryReplayCursor(ctx, delivery, requested.StreamID)
	if err != nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked,
			"delivery_projection_unavailable", err)
	}
	execution := &AgentExecution{
		ID: requested.ExecutionID, TaskID: requested.TaskID, SessionID: requested.SessionID,
		DeliveryMode: DurableDeliveryV1, DeliveryStreamID: requested.StreamID,
		DeliveryIncarnationID:     requested.IncarnationID,
		DeliveryHarnessGeneration: requested.HarnessGeneration,
		DeliveryDescriptor:        descriptor, DeliveryReplayCursor: after,
		runtimeEpoch: lease.Epoch(), promptGeneration: requested.PromptGeneration,
		agentctl: client,
	}
	execution.setDeliverySubmissionID(requested.SubmissionID)
	if err := m.streamManager.replayRecoveredDelivery(ctx, execution, true, lease.CheckCurrent); err != nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked,
			"retained_replay_failed", err)
	}
	if err := lease.CheckCurrent(); err != nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationOwnerMismatch,
			"runtime_owner_changed", err)
	}
	if execution.DeliveryReplayCursor < submission.TerminalSequence {
		return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked,
			"terminal_projection_incomplete", ErrDeliveryRecoveryBlocked)
	}
	settler, ok := delivery.(agentDeliveryTerminalSettler)
	if !ok {
		return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked,
			"terminal_settlement_unavailable", ErrDeliveryRecoveryBlocked)
	}
	if _, err := settler.SettleAgentDeliveryTerminal(
		ctx, requested.StreamID, int64(submission.TerminalSequence),
		models.DeliverySubmissionState(submission.State), submission.UpdatedAt,
	); err != nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked,
			"terminal_settlement_failed", err)
	}
	return deliveryRecoveryResult(identity, DeliveryReconciliationTerminalSettled, "", nil)
}

func completeAgentDeliveryRecoveryIdentity(identity AgentDeliveryRecoveryIdentity) bool {
	return identity.TaskID != "" && identity.SessionID != "" && identity.ExecutionID != "" &&
		identity.SubmissionID != "" && identity.StreamID != "" && identity.IncarnationID != "" &&
		identity.HarnessGeneration > 0 && identity.PromptGeneration > 0
}

func agentDeliveryRecoveryExecutionMatches(
	execution *AgentExecution,
	identity AgentDeliveryRecoveryIdentity,
) bool {
	return execution != nil && execution.ID == identity.ExecutionID && execution.TaskID == identity.TaskID &&
		execution.SessionID == identity.SessionID && execution.DeliveryMode == DurableDeliveryV1 &&
		execution.DeliveryStreamID == identity.StreamID && execution.DeliveryIncarnationID == identity.IncarnationID &&
		execution.DeliveryHarnessGeneration == identity.HarnessGeneration &&
		execution.promptGenerationSnapshot() == identity.PromptGeneration &&
		currentDeliverySubmissionID(execution) == identity.SubmissionID
}

func deliveryReconciliationIdentity(identity AgentDeliveryRecoveryIdentity) DeliveryReconciliationIdentity {
	return DeliveryReconciliationIdentity{
		OriginalRuntime: identity.OriginalRuntime, SessionID: identity.SessionID, ExecutionID: identity.ExecutionID,
		IncarnationID: identity.IncarnationID, HarnessGeneration: identity.HarnessGeneration,
		StreamID: identity.StreamID, SubmissionID: identity.SubmissionID,
		PromptGeneration: identity.PromptGeneration,
	}
}

func deliveryRecoveryResult(
	identity DeliveryReconciliationIdentity,
	outcome DeliveryReconciliationOutcome,
	reason string,
	err error,
) DeliveryReconciliationResult {
	return DeliveryReconciliationResult{Identity: identity, Outcome: outcome, Reason: reason, Err: err}
}

func retainedInstanceMatches(instance *agentctl.InstanceInfo, requested AgentDeliveryRecoveryIdentity) bool {
	return instance != nil && instance.ID == requested.ExecutionID && instance.TaskID == requested.TaskID && instance.SessionID == requested.SessionID && instance.Port > 0
}
