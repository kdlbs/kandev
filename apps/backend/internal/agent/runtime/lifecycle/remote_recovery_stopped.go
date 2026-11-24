package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

func validateAttachedRemoteAgentctlIdentity(record *models.ExecutorRunning, instance *ExecutorInstance) error {
	if record == nil || instance == nil {
		return errors.New("attached remote owner checkpoint is incomplete")
	}
	recordedID, err := recordedAgentctlInstanceID(record.Runtime, record.AgentExecutionID, record.Metadata)
	if err != nil {
		return err
	}
	attachedID, err := recordedAgentctlInstanceID(instance.RuntimeName, instance.InstanceID, instance.Metadata)
	if err != nil {
		return err
	}
	if recordedID != attachedID {
		return fmt.Errorf("%w: attached remote checkpoint changed agentctl identity", ErrDurableAdoptionIdentityMismatch)
	}
	return nil
}

func (m *Manager) settleStoppedRemoteInstance(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	snapshot *RemoteRecoverySnapshot,
	ri *ExecutorInstance,
) error {
	settlement, err := m.prepareStoppedRemoteSettlement(ctx, entry, snapshot, ri)
	if err != nil {
		return err
	}
	stateChanged, updatedAt, err := settlement.writer.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, entry.record.SessionID, settlement.attemptRecord.AgentExecutionID, settlement.attemptRecord.UpdatedAt,
		snapshot.SessionState, settlement.turnID, settlement.terminalOutcome, settlement.workflowOutcome,
		remoteRecoveryOwnerLockIDs(snapshot),
		func(ownerCtx context.Context) error {
			return m.validateStoppedRemoteSettlementOwner(ownerCtx, entry)
		},
	)
	if err != nil {
		return fmt.Errorf("settle stopped remote execution: %w", err)
	}
	if stateChanged {
		m.publishStoppedRemoteSessionState(ctx, entry.record, snapshot.SessionState, updatedAt)
	}
	if !m.completeStoppedRemoteRecovery(entry, ctx) {
		return errRemoteRecoveryFenced
	}
	m.discardRemoteRecoveryInstance(ri)
	return nil
}

type stoppedRemoteSettlement struct {
	attemptRecord   *models.ExecutorRunning
	turnID          string
	terminalOutcome models.DeliverySubmissionState
	workflowOutcome *models.AgentDeliveryEffect
	writer          stoppedRemoteExecutionOwnerCASWriter
}

type stoppedRemoteDeliveryReplay struct {
	snapshot *RemoteRecoverySnapshot
	active   []*models.AgentDeliverySubmission
	terminal *models.AgentDeliveryEffect
}

func (m *Manager) prepareStoppedRemoteSettlement(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	snapshot *RemoteRecoverySnapshot,
	ri *ExecutorInstance,
) (*stoppedRemoteSettlement, error) {
	if !m.remoteRecoveryAbsenceDispositionAllowed(ctx, entry) {
		return nil, errRemoteRecoveryFenced
	}
	execution, err := m.prepareStoppedRemoteExecution(ctx, entry.record, snapshot, ri)
	if err != nil {
		return nil, fmt.Errorf("reconstruct stopped remote execution: %w", err)
	}
	delivery, err := m.replayStoppedRemoteDelivery(ctx, entry, snapshot, execution, ri)
	if err != nil {
		return nil, err
	}
	snapshot = delivery.snapshot
	turnID, terminalOutcome, err := m.stoppedRemoteInterruptedTurn(ctx, snapshot, execution, delivery)
	if err != nil {
		return nil, err
	}
	workflowOutcome, err := m.stoppedRemoteWorkflowOutcome(ctx, entry, execution, delivery.terminal, turnID)
	if err != nil {
		return nil, err
	}
	attemptRecord, rowPresent, err := m.remoteRecoveryAttemptRecord(ctx, entry)
	if err != nil {
		return nil, fmt.Errorf("refresh stopped remote executor row: %w", err)
	}
	if !rowPresent || !sameRemoteRecoveryRecordOwner(entry.record, attemptRecord) || attemptRecord.UpdatedAt.IsZero() {
		return nil, errors.New("stopped remote executor owner changed before settlement")
	}
	expectedAgentctlID, err := recordedAgentctlInstanceID(attemptRecord.Runtime, attemptRecord.AgentExecutionID, attemptRecord.Metadata)
	if err != nil || ri.agentSessionAssociation == nil || ri.agentSessionAssociation.InstanceID != expectedAgentctlID {
		return nil, fmt.Errorf("%w: stopped remote agentctl identity changed before settlement", ErrDurableAdoptionIdentityMismatch)
	}
	writer, ok := m.runningWriter.(stoppedRemoteExecutionOwnerCASWriter)
	if !ok {
		return nil, errors.New("stopped remote settlement requires owner-locked runtime and session compare-and-set support")
	}
	return &stoppedRemoteSettlement{
		attemptRecord: attemptRecord, turnID: turnID, terminalOutcome: terminalOutcome,
		workflowOutcome: workflowOutcome, writer: writer,
	}, nil
}

func (m *Manager) stoppedRemoteWorkflowOutcome(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	execution *AgentExecution,
	terminal *models.AgentDeliveryEffect,
	openTurnID string,
) (*models.AgentDeliveryEffect, error) {
	if terminal == nil {
		return nil, nil
	}
	turnID, err := stoppedRemoteTerminalWorkflowTurnID(execution, terminal, openTurnID)
	if err != nil {
		return nil, err
	}
	effect, _, err := m.stoppedRemoteWorkflowEffect(ctx, entry, terminal, turnID)
	return effect, err
}

func (m *Manager) validateStoppedRemoteSettlementOwner(
	ctx context.Context,
	entry *remoteRecoveryRetry,
) error {
	if !m.remoteRecoveryAbsenceDispositionAllowed(ctx, entry) {
		return errRemoteRecoveryFenced
	}
	current, err := m.remoteRecoverySnapshot(ctx, entry.record.SessionID)
	if err != nil {
		return fmt.Errorf("re-read stopped remote owner before settlement: %w", err)
	}
	if !sameRemoteRecoveryOwner(entry.snapshot, current, entry.record) {
		return errRemoteRecoveryFenced
	}
	if err := validateRemoteRecoverySnapshot(current, entry.record); err != nil {
		return err
	}
	return nil
}

func (m *Manager) replayStoppedRemoteDelivery(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	snapshot *RemoteRecoverySnapshot,
	execution *AgentExecution,
	ri *ExecutorInstance,
) (*stoppedRemoteDeliveryReplay, error) {
	switch execution.DeliveryMode {
	case DurableDeliveryV1:
		return m.replayStoppedRemoteDurableDelivery(ctx, entry, snapshot, execution, ri)
	case DurableDeliveryLegacy:
		return &stoppedRemoteDeliveryReplay{snapshot: snapshot}, nil
	default:
		return nil, fmt.Errorf("%w: stopped remote delivery evidence is unavailable", ErrDurableAdoptionBlocked)
	}
}

func (m *Manager) replayStoppedRemoteDurableDelivery(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	snapshot *RemoteRecoverySnapshot,
	execution *AgentExecution,
	ri *ExecutorInstance,
) (*stoppedRemoteDeliveryReplay, error) {
	if m.streamManager == nil {
		return nil, errors.New("stopped remote recovery stream manager is unavailable")
	}
	if err := m.streamManager.ReplayRecoveredDeliveryWithoutLifecycleCallbacks(ctx, execution); err != nil {
		return nil, fmt.Errorf("replay stopped remote delivery: %w", err)
	}
	peer, err := ri.Client.ListDeliverySubmissions(ctx, execution.SessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: list stopped remote submissions: %v", ErrDurableAdoptionEvidenceUnavailable, err)
	}
	missingPeer, err := m.reconcileNativeResumeSQLDispatch(ctx, execution, ri.DeliveryStatus, peer)
	if err != nil {
		return nil, fmt.Errorf("reconcile stopped remote dispatch: %w", err)
	}
	if err := validateStoppedRemoteMissingPeerSubmissions(snapshot, missingPeer); err != nil {
		return nil, err
	}
	active, err := activeBackendSubmissions(ctx, m.streamManager.deliveryRepository(), execution.SessionID)
	if err != nil {
		return nil, err
	}
	if !snapshot.RecoverySourcePresent && len(active) > 0 {
		snapshot, err = m.persistStoppedRemoteInterruptedNotice(ctx, entry, snapshot, execution, ri, peer, active)
		if err != nil {
			return nil, err
		}
	}
	terminal, err := m.stoppedRemoteTerminalEvidence(ctx, snapshot, execution, ri, peer)
	if err != nil {
		return nil, err
	}
	return &stoppedRemoteDeliveryReplay{snapshot: snapshot, active: active, terminal: terminal}, nil
}

func validateStoppedRemoteMissingPeerSubmissions(
	snapshot *RemoteRecoverySnapshot,
	missing []*models.AgentDeliverySubmission,
) error {
	for _, submission := range missing {
		if submission.ID != snapshot.RecoverySubmissionID || submission.State != models.DeliverySubmissionInterruptedUnknown {
			return fmt.Errorf("%w: stopped remote session has separate unverifiable delivery work", ErrDurableAdoptionBlocked)
		}
	}
	return nil
}

func (m *Manager) publishStoppedRemoteSessionState(
	ctx context.Context,
	record *models.ExecutorRunning,
	oldState models.TaskSessionState,
	updatedAt time.Time,
) {
	if m.eventBus == nil || record == nil {
		return
	}
	data := map[string]interface{}{
		"task_id": record.TaskID, "session_id": record.SessionID,
		"old_state": string(oldState), "new_state": string(models.TaskSessionStateWaitingForInput),
		"error_message": "", "updated_at": updatedAt.Format(time.RFC3339Nano),
	}
	_ = m.eventBus.Publish(ctx, events.TaskSessionStateChanged,
		bus.NewEvent(events.TaskSessionStateChanged, "task-session", data))
}

func (m *Manager) processRecoveredRemoteInstance(ctx context.Context, entry *remoteRecoveryRetry, ri *ExecutorInstance) error {
	if err := m.validateAttachedRemoteRecoveryCheckpoint(ctx, entry, ri); err != nil {
		return err
	}
	if err := m.proveRecoveredRemoteInstance(ctx, entry, ri); err != nil {
		return err
	}
	if err := validateFailedRemoteRecoveryAgentProof(entry.snapshot, ri); err != nil {
		return err
	}
	current, err := m.remoteRecoverySnapshot(ctx, entry.record.SessionID)
	if err != nil {
		return fmt.Errorf("re-read remote owner before tracking: %w", err)
	}
	if !sameRemoteRecoveryOwner(entry.snapshot, current, entry.record) {
		m.removeRemoteRecovery(entry, true)
		return errRemoteRecoveryFenced
	}
	if err := validateRemoteRecoverySnapshot(current, entry.record); err != nil {
		return err
	}
	if remoteRecoveryOwnerUnsupported(entry.record) {
		return errors.New("run-owned remote recovery is unsupported by this task-session path")
	}
	if ri.agentSessionAssociation != nil && ri.agentSessionAssociation.AgentStatus == agentProcessStatusStopped {
		return m.settleStoppedRemoteInstance(ctx, entry, current, ri)
	}
	execution, err := m.prepareRecoveredRemoteExecution(ctx, entry.record, current, ri)
	if err != nil {
		return fmt.Errorf("reconstruct recovered remote execution: %w", err)
	}
	return m.trackRecoveredRemoteExecution(ctx, entry, execution, ri)
}

func (m *Manager) validateAttachedRemoteRecoveryCheckpoint(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	ri *ExecutorInstance,
) error {
	if entry.record.Runtime != agentruntime.RuntimeRemoteDocker && entry.record.Runtime != agentruntime.RuntimePluginRemote {
		return nil
	}
	attemptRecord, rowPresent, err := m.remoteRecoveryAttemptRecord(ctx, entry)
	if err != nil {
		return fmt.Errorf("refresh attached remote owner checkpoint: %w", err)
	}
	if !rowPresent || !sameRemoteRecoveryRecordOwner(entry.record, attemptRecord) || attemptRecord.UpdatedAt.IsZero() {
		return errors.New("attached remote owner checkpoint changed")
	}
	return validateAttachedRemoteAgentctlIdentity(attemptRecord, ri)
}

func (m *Manager) proveRecoveredRemoteInstance(ctx context.Context, entry *remoteRecoveryRetry, ri *ExecutorInstance) error {
	if !remoteRecoveryCandidateMatchesOwner(entry, ri) {
		return errors.New("runtime returned a different persisted remote execution identity")
	}
	if entry.snapshot.IsPassthrough && ri.RuntimeName == agentruntime.RuntimeSSH {
		return m.captureRemotePassthroughEvidence(ctx, ri)
	}
	if err := pinRemoteRecoveryDeliveryStream(entry.snapshot, ri); err != nil {
		return err
	}
	if err := m.captureAuthenticatedAgentSessionEvidence(ctx, ri); err != nil {
		return fmt.Errorf("capture authenticated agent-session evidence: %w", err)
	}
	if err := validateRemoteRecoveryDeliveryEvidence(entry.snapshot, ri); err != nil {
		return err
	}
	return nil
}

func remoteRecoveryCandidateMatchesOwner(entry *remoteRecoveryRetry, ri *ExecutorInstance) bool {
	return entry != nil && entry.record != nil && entry.snapshot != nil && ri != nil &&
		ri.RuntimeName == entry.record.Runtime && ri.SessionID == entry.record.SessionID &&
		ri.InstanceID == entry.record.AgentExecutionID && ri.TaskID == entry.record.TaskID
}

func pinRemoteRecoveryDeliveryStream(snapshot *RemoteRecoverySnapshot, ri *ExecutorInstance) error {
	if !snapshot.RecoverySourcePresent {
		return nil
	}
	if ri.expectedDeliveryStreamID != "" && ri.expectedDeliveryStreamID != snapshot.RecoveryStreamID {
		return errors.New("remote recovery stream expectation changed")
	}
	ri.expectedDeliveryStreamID = snapshot.RecoveryStreamID
	return nil
}
