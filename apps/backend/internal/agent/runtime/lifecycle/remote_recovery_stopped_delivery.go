package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (m *Manager) stoppedRemoteInterruptedTurn(
	ctx context.Context,
	snapshot *RemoteRecoverySnapshot,
	execution *AgentExecution,
	delivery *stoppedRemoteDeliveryReplay,
) (string, models.DeliverySubmissionState, error) {
	turns, ok := m.runningWriter.(activeSessionTurnReader)
	if !ok {
		return "", "", errors.New("stopped remote settlement cannot read the exact open turn")
	}
	turn, err := turns.GetActiveTurnBySessionID(ctx, execution.SessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return stoppedRemoteOutcomeWithoutOpenTurn(delivery)
		}
		return "", "", fmt.Errorf("read stopped remote open turn: %w", err)
	}
	if turn == nil {
		return stoppedRemoteOutcomeWithoutOpenTurn(delivery)
	}
	if delivery.terminal != nil {
		return stoppedRemoteTerminalTurn(turn, execution, delivery.terminal)
	}
	return stoppedRemoteInterruptedOpenTurn(turn, snapshot, execution, delivery)
}

func stoppedRemoteOutcomeWithoutOpenTurn(
	delivery *stoppedRemoteDeliveryReplay,
) (string, models.DeliverySubmissionState, error) {
	if delivery.terminal == nil {
		return "", "", nil
	}
	return "", models.DeliverySubmissionState(delivery.terminal.Outcome), nil
}

func stoppedRemoteTerminalTurn(
	turn *models.Turn,
	execution *AgentExecution,
	terminal *models.AgentDeliveryEffect,
) (string, models.DeliverySubmissionState, error) {
	terminalTurnID, err := stoppedRemoteTerminalTurnBinding(execution, terminal)
	if err != nil {
		return "", "", err
	}
	if terminalTurnID == "" || terminalTurnID != turn.ID {
		return "", "", fmt.Errorf("%w: terminal submission does not match the original open turn", ErrDurableAdoptionBlocked)
	}
	return turn.ID, models.DeliverySubmissionState(terminal.Outcome), nil
}

func stoppedRemoteTerminalTurnBinding(
	execution *AgentExecution,
	terminal *models.AgentDeliveryEffect,
) (string, error) {
	terminalTurnID := terminal.TurnID
	binding, exists := models.LoadAgentDeliveryPromptTurnBinding(execution.MetadataSnapshot())
	if !exists || binding.SubmissionID != terminal.SubmissionID {
		return terminalTurnID, nil
	}
	if binding.TurnID == "" || binding.TaskID != execution.TaskID || binding.SessionID != execution.SessionID ||
		binding.ExecutionID != execution.ID || binding.IncarnationID != execution.DeliveryIncarnationID ||
		binding.HarnessGeneration != execution.DeliveryHarnessGeneration || binding.StreamID != execution.DeliveryStreamID {
		return "", fmt.Errorf("%w: terminal submission does not match its saved owner binding", ErrDurableAdoptionIdentityMismatch)
	}
	if terminalTurnID != "" && terminalTurnID != binding.TurnID {
		return "", fmt.Errorf("%w: terminal event changed the original prompt turn", ErrDurableAdoptionIdentityMismatch)
	}
	return binding.TurnID, nil
}

func stoppedRemoteTerminalWorkflowTurnID(
	execution *AgentExecution,
	terminal *models.AgentDeliveryEffect,
	openTurnID string,
) (string, error) {
	if openTurnID != "" {
		return openTurnID, nil
	}
	return stoppedRemoteTerminalTurnBinding(execution, terminal)
}

func stoppedRemoteInterruptedOpenTurn(
	turn *models.Turn,
	snapshot *RemoteRecoverySnapshot,
	execution *AgentExecution,
	delivery *stoppedRemoteDeliveryReplay,
) (string, models.DeliverySubmissionState, error) {
	if execution.DeliveryMode == DurableDeliveryLegacy {
		return turn.ID, "", nil
	}
	if execution.DeliveryMode == DurableDeliveryV1 && !snapshot.RecoverySourcePresent {
		return "", "", fmt.Errorf("%w: stopped remote turn has no pinned interruption source", ErrDurableAdoptionBlocked)
	}
	if len(delivery.active) != 1 || delivery.active[0].State != models.DeliverySubmissionInterruptedUnknown {
		return "", "", fmt.Errorf("%w: stopped remote turn is not owned by one interrupted submission", ErrDurableAdoptionBlocked)
	}
	if binding, exists := models.LoadAgentDeliveryPromptTurnBinding(execution.MetadataSnapshot()); exists &&
		binding.SubmissionID == delivery.active[0].ID && binding.TurnID != turn.ID {
		return "", "", fmt.Errorf("%w: interrupted submission does not match its saved turn binding", ErrDurableAdoptionIdentityMismatch)
	}
	return turn.ID, "", nil
}

func (m *Manager) stoppedRemoteTerminalEvidence(
	ctx context.Context,
	snapshot *RemoteRecoverySnapshot,
	execution *AgentExecution,
	ri *ExecutorInstance,
	peer []journal.Submission,
) (*models.AgentDeliveryEffect, error) {
	if m.streamManager == nil {
		return nil, errors.New("stopped remote recovery stream manager is unavailable")
	}
	reader, ok := m.streamManager.deliveryRepository().(agentDeliverySubmissionReader)
	if !ok {
		return nil, fmt.Errorf("%w: stopped remote SQL submission reader is unavailable", ErrDurableAdoptionEvidenceUnavailable)
	}
	submissionID, err := stoppedRemoteTerminalSubmissionID(snapshot, execution)
	if err != nil {
		return nil, err
	}
	if submissionID == "" {
		return nil, nil
	}
	submission, err := m.readStoppedRemoteTerminalSubmission(ctx, reader, snapshot, execution, submissionID)
	if err != nil {
		return nil, err
	}
	if !stoppedRemoteSubmissionHasTerminalOutcome(submission) {
		return nil, nil
	}
	if submission.TerminalSequence > 0 && submission.TerminalStreamID != "" {
		return m.stoppedRemoteTerminalEffectForIdentity(ctx, submission, submission.TerminalStreamID, submission.TerminalSequence,
			submission.State)
	}
	effect, found, err := m.stoppedRemoteRetainedTerminalEffect(ctx, submission)
	if err != nil {
		return nil, err
	}
	if found {
		return effect, nil
	}
	return m.stoppedRemoteTerminalPeerEffect(ctx, execution, ri, peer, submission)
}

func (m *Manager) stoppedRemoteRetainedTerminalEffect(
	ctx context.Context,
	submission *models.AgentDeliverySubmission,
) (*models.AgentDeliveryEffect, bool, error) {
	if reader, ok := m.streamManager.deliveryRepository().(interface {
		GetAgentDeliveryTerminalEffectBySubmission(context.Context, string) (*models.AgentDeliveryEffect, error)
	}); ok {
		effect, err := reader.GetAgentDeliveryTerminalEffectBySubmission(ctx, submission.ID)
		if err == nil {
			if !stoppedRemoteTerminalEffectMatchesSubmission(effect, submission) {
				return nil, false, fmt.Errorf("%w: stopped remote retained SQL terminal identity changed", ErrDurableAdoptionIdentityMismatch)
			}
			resolved, err := m.stoppedRemoteTerminalEffectForIdentity(ctx, submission, effect.StreamID, effect.Sequence, submission.State)
			return resolved, true, err
		}
		if errors.Is(err, repoerrors.ErrAgentDeliveryEffectNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("%w: read stopped remote terminal effect by submission: %v", ErrDurableAdoptionEvidenceUnavailable, err)
	}
	return nil, false, nil
}

func stoppedRemoteTerminalEffectMatchesSubmission(
	effect *models.AgentDeliveryEffect,
	submission *models.AgentDeliverySubmission,
) bool {
	return effect != nil && effect.SubmissionID == submission.ID && effect.StreamID != "" && effect.Sequence > 0 &&
		effect.SessionID == submission.SessionID && effect.IncarnationID == submission.IncarnationID &&
		effect.HarnessGeneration == submission.HarnessGeneration && effect.Outcome == string(submission.State)
}

func (m *Manager) readStoppedRemoteTerminalSubmission(
	ctx context.Context,
	reader agentDeliverySubmissionReader,
	snapshot *RemoteRecoverySnapshot,
	execution *AgentExecution,
	submissionID string,
) (*models.AgentDeliverySubmission, error) {
	submission, err := reader.GetAgentDeliverySubmission(ctx, submissionID)
	if err != nil {
		return nil, fmt.Errorf("%w: read stopped remote SQL submission: %v", ErrDurableAdoptionEvidenceUnavailable, err)
	}
	if !stoppedRemoteSubmissionMatchesExecution(submission, execution, submissionID) {
		return nil, fmt.Errorf("%w: stopped remote SQL submission identity changed", ErrDurableAdoptionIdentityMismatch)
	}
	if snapshot.RecoverySourcePresent {
		if err := validateStoppedRemoteSQLSubmission(snapshot, submission); err != nil {
			return nil, err
		}
	}
	return submission, nil
}

func stoppedRemoteSubmissionMatchesExecution(
	submission *models.AgentDeliverySubmission,
	execution *AgentExecution,
	submissionID string,
) bool {
	return submission != nil && submission.ID == submissionID && submission.SessionID == execution.SessionID &&
		submission.IncarnationID == execution.DeliveryIncarnationID &&
		submission.HarnessGeneration == int64(execution.DeliveryHarnessGeneration) && submission.PayloadHash != ""
}

func (m *Manager) stoppedRemoteTerminalPeerEffect(
	ctx context.Context,
	execution *AgentExecution,
	ri *ExecutorInstance,
	peer []journal.Submission,
	submission *models.AgentDeliverySubmission,
) (*models.AgentDeliveryEffect, error) {
	var matched *journal.Submission
	for i := range peer {
		if peer[i].ID != submission.ID {
			continue
		}
		if matched != nil {
			return nil, fmt.Errorf("%w: duplicate stopped remote terminal submission evidence", ErrDurableAdoptionBlocked)
		}
		matched = &peer[i]
	}
	if matched == nil || !retainedPeerTerminal(ri.DeliveryStatus, peerSubmissionSummary(matched)) ||
		matched.State != journal.SubmissionState(submission.State) || matched.StreamID != execution.DeliveryStreamID ||
		matched.Hash != submission.PayloadHash {
		return nil, fmt.Errorf("%w: stopped remote terminal outcome lacks exact retained peer evidence", ErrDurableAdoptionBlocked)
	}
	if matched.TerminalSequence > 1<<63-1 {
		return nil, fmt.Errorf("%w: terminal sequence is outside the SQL range", ErrDurableAdoptionIdentityMismatch)
	}
	return m.stoppedRemoteTerminalEffectForIdentity(ctx, submission, matched.StreamID, int64(matched.TerminalSequence),
		submission.State)
}

func stoppedRemoteTerminalSubmissionID(
	snapshot *RemoteRecoverySnapshot,
	execution *AgentExecution,
) (string, error) {
	if snapshot == nil || execution == nil {
		return "", nil
	}
	submissionID := execution.deliverySubmissionIDSnapshot()
	if snapshot.RecoverySourcePresent {
		if submissionID != "" && submissionID != snapshot.RecoverySubmissionID {
			return "", fmt.Errorf("%w: stopped remote submission changed from its pinned source", ErrDurableAdoptionIdentityMismatch)
		}
		return snapshot.RecoverySubmissionID, nil
	}
	if submissionID != "" {
		return submissionID, nil
	}
	binding, exists := models.LoadAgentDeliveryPromptTurnBinding(execution.MetadataSnapshot())
	if !exists || binding.SubmissionID == "" || binding.TaskID != execution.TaskID ||
		binding.SessionID != execution.SessionID || binding.ExecutionID != execution.ID ||
		binding.IncarnationID != execution.DeliveryIncarnationID ||
		binding.HarnessGeneration != execution.DeliveryHarnessGeneration || binding.StreamID != execution.DeliveryStreamID {
		return "", nil
	}
	return binding.SubmissionID, nil
}

func (m *Manager) stoppedRemoteTerminalEffectForIdentity(
	ctx context.Context,
	submission *models.AgentDeliverySubmission,
	streamID string,
	sequence int64,
	outcome models.DeliverySubmissionState,
) (*models.AgentDeliveryEffect, error) {
	reader, ok := m.streamManager.deliveryRepository().(agentDeliveryEffectReader)
	if !ok {
		return nil, fmt.Errorf("%w: stopped remote terminal effect reader is unavailable", ErrDurableAdoptionEvidenceUnavailable)
	}
	effect, err := reader.GetAgentDeliveryEffect(ctx, fmt.Sprintf("agent_delivery.terminal:%s:%d", streamID, sequence))
	if err != nil {
		return nil, fmt.Errorf("%w: read stopped remote terminal effect: %v", ErrDurableAdoptionEvidenceUnavailable, err)
	}
	if effect == nil || effect.EffectType != models.DeliveryTerminalEffectType || effect.StreamID != streamID ||
		effect.Sequence != sequence || effect.SessionID != submission.SessionID || effect.IncarnationID != submission.IncarnationID ||
		effect.HarnessGeneration != submission.HarnessGeneration || effect.SubmissionID != submission.ID ||
		effect.Outcome != string(outcome) {
		return nil, fmt.Errorf("%w: stopped remote terminal effect changed identity or outcome", ErrDurableAdoptionIdentityMismatch)
	}
	copy := *effect
	return &copy, nil
}

func (m *Manager) persistStoppedRemoteInterruptedNotice(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	snapshot *RemoteRecoverySnapshot,
	execution *AgentExecution,
	ri *ExecutorInstance,
	peer []journal.Submission,
	active []*models.AgentDeliverySubmission,
) (*RemoteRecoverySnapshot, error) {
	submission, err := exactStoppedRemoteInterruptedSubmission(active, execution)
	if err != nil {
		return nil, err
	}
	if err = validateExactStoppedRemoteInterruptedPeer(peer, execution, ri.DeliveryStatus, submission); err != nil {
		return nil, err
	}
	binding, err := stoppedRemotePromptTurnBinding(execution, submission.ID)
	if err != nil {
		return nil, err
	}
	return m.writeStoppedRemoteInterruptedNotice(ctx, entry, snapshot, execution, submission, binding)
}

func exactStoppedRemoteInterruptedSubmission(
	active []*models.AgentDeliverySubmission,
	execution *AgentExecution,
) (*models.AgentDeliverySubmission, error) {
	if len(active) != 1 || active[0] == nil || active[0].State != models.DeliverySubmissionInterruptedUnknown ||
		active[0].ID != execution.deliverySubmissionIDSnapshot() || active[0].IncarnationID != execution.DeliveryIncarnationID ||
		active[0].HarnessGeneration != int64(execution.DeliveryHarnessGeneration) {
		return nil, fmt.Errorf("%w: stopped remote interruption is not one exact abandoned submission", ErrDurableAdoptionBlocked)
	}
	return active[0], nil
}

func validateExactStoppedRemoteInterruptedPeer(
	peer []journal.Submission,
	execution *AgentExecution,
	status *agentctl.DeliveryStatus,
	submission *models.AgentDeliverySubmission,
) error {
	var matched *journal.Submission
	for i := range peer {
		if peer[i].ID != submission.ID {
			continue
		}
		if matched != nil {
			return fmt.Errorf("%w: duplicate stopped remote interruption evidence", ErrDurableAdoptionBlocked)
		}
		matched = &peer[i]
	}
	if matched == nil || !peerMatchesUnknownHistory(execution, status, *matched, submission) {
		return fmt.Errorf("%w: stopped remote interruption lacks one exact retained payload identity", ErrDurableAdoptionBlocked)
	}
	return nil
}

func stoppedRemotePromptTurnBinding(
	execution *AgentExecution,
	submissionID string,
) (models.AgentDeliveryPromptTurnBinding, error) {
	binding, exists := models.LoadAgentDeliveryPromptTurnBinding(execution.MetadataSnapshot())
	if !exists || binding.TurnID == "" || binding.PromptGeneration == 0 || binding.TaskID != execution.TaskID ||
		binding.SessionID != execution.SessionID || binding.ExecutionID != execution.ID ||
		binding.IncarnationID != execution.DeliveryIncarnationID || binding.HarnessGeneration != execution.DeliveryHarnessGeneration ||
		binding.StreamID != execution.DeliveryStreamID || binding.SubmissionID != submissionID {
		return models.AgentDeliveryPromptTurnBinding{}, fmt.Errorf("%w: stopped remote interruption has no matching saved prompt turn binding", ErrDurableAdoptionIdentityMismatch)
	}
	return binding, nil
}

func (m *Manager) writeStoppedRemoteInterruptedNotice(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	snapshot *RemoteRecoverySnapshot,
	execution *AgentExecution,
	submission *models.AgentDeliverySubmission,
	binding models.AgentDeliveryPromptTurnBinding,
) (*RemoteRecoverySnapshot, error) {
	if !m.remoteRecoveryAbsenceDispositionAllowed(ctx, entry) {
		return nil, errRemoteRecoveryFenced
	}
	recoveryWriter, ok := m.streamManager.deliveryRepository().(interface {
		UpsertAgentDeliveryRecovery(context.Context, *models.AgentDeliveryRecovery, *models.SessionRecoveryBlock) (bool, error)
	})
	if !ok {
		return nil, fmt.Errorf("%w: stopped remote recovery notice writer is unavailable", ErrDurableAdoptionEvidenceUnavailable)
	}
	recovery := &models.AgentDeliveryRecovery{
		OriginalRuntime: binding.OriginalRuntime, Phase: models.AgentDeliveryRecoveryUncertain,
		SessionID: execution.SessionID, AgentExecutionID: execution.ID,
		SubmissionID: submission.ID, StreamID: execution.DeliveryStreamID,
		IncarnationID:     execution.DeliveryIncarnationID,
		HarnessGeneration: int64(execution.DeliveryHarnessGeneration), PromptGeneration: binding.PromptGeneration,
		Message: "The prompt outcome is uncertain.",
	}
	block := &models.SessionRecoveryBlock{
		SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
		ExpectedGeneration: int64(execution.DeliveryHarnessGeneration), Reason: "unknown_prompt_outcome",
		State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery",
		DeliverySubmissionID: submission.ID, DeliveryStreamID: execution.DeliveryStreamID,
	}
	stored, err := recoveryWriter.UpsertAgentDeliveryRecovery(ctx, recovery, block)
	if err != nil {
		return nil, fmt.Errorf("persist stopped remote interruption notice: %w", err)
	}
	if !stored {
		return nil, fmt.Errorf("%w: stopped remote interruption notice lost its exact owner", ErrDurableAdoptionBlocked)
	}
	current, err := m.remoteRecoverySnapshot(ctx, execution.SessionID)
	if err != nil {
		return nil, fmt.Errorf("read stopped remote owner after interruption notice: %w", err)
	}
	if !stoppedRemoteOwnerMatchesNewInterruptionNotice(snapshot, current, entry.record, execution, submission) {
		return nil, errRemoteRecoveryFenced
	}
	m.remoteRecoveryMu.Lock()
	if m.remoteRecoveryPending[entry.record.SessionID] != entry || !entry.inProgress || entry.stopRequested {
		m.remoteRecoveryMu.Unlock()
		return nil, errRemoteRecoveryFenced
	}
	entry.snapshot = current
	m.remoteRecoveryMu.Unlock()
	return current, nil
}

func stoppedRemoteOwnerMatchesNewInterruptionNotice(
	previous, current *RemoteRecoverySnapshot,
	record *models.ExecutorRunning,
	execution *AgentExecution,
	submission *models.AgentDeliverySubmission,
) bool {
	if previous == nil || current == nil || previous.RecoverySourcePresent || !current.RecoverySourcePresent ||
		current.RecoveryExecutionID != execution.ID || current.RecoverySubmissionID != submission.ID ||
		current.RecoveryStreamID != execution.DeliveryStreamID || current.RecoveryIncarnationID != execution.DeliveryIncarnationID ||
		current.RecoveryHarnessGeneration != int64(execution.DeliveryHarnessGeneration) ||
		current.RecoveryRevision <= previous.RecoveryRevision {
		return false
	}
	base := remoteRecoverySnapshotWithNotice(previous, current)
	return sameRemoteRecoveryOwner(&base, current, record) && validateRemoteRecoverySnapshot(current, record) == nil
}

func remoteRecoverySnapshotWithNotice(previous, current *RemoteRecoverySnapshot) RemoteRecoverySnapshot {
	base := *previous
	base.RecoverySourcePresent = current.RecoverySourcePresent
	base.RecoverableDeliveryPending = current.RecoverableDeliveryPending
	base.RecoveryExecutionID = current.RecoveryExecutionID
	base.RecoverySubmissionID = current.RecoverySubmissionID
	base.RecoveryStreamID = current.RecoveryStreamID
	base.RecoveryIncarnationID = current.RecoveryIncarnationID
	base.RecoveryHarnessGeneration = current.RecoveryHarnessGeneration
	base.RecoveryRevision = current.RecoveryRevision
	return base
}

func stoppedRemoteOwnerMatchesSavedInterruptionNotice(
	previous, current *RemoteRecoverySnapshot,
	record *models.ExecutorRunning,
) bool {
	if !stoppedRemoteNoticeRevisionMatches(previous, current, record) ||
		!stoppedRemoteNoticeMatchesSavedBinding(current, record) {
		return false
	}
	base := remoteRecoverySnapshotWithNotice(previous, current)
	return sameRemoteRecoveryOwner(&base, current, record) && validateRemoteRecoverySnapshot(current, record) == nil
}

func stoppedRemoteNoticeRevisionMatches(
	previous, current *RemoteRecoverySnapshot,
	record *models.ExecutorRunning,
) bool {
	return previous != nil && current != nil && record != nil && !previous.RecoverySourcePresent &&
		current.RecoverySourcePresent && current.RecoverableDeliveryPending &&
		current.RecoveryExecutionID == record.AgentExecutionID &&
		current.RecoveryRevision > previous.RecoveryRevision
}

func stoppedRemoteNoticeMatchesSavedBinding(current *RemoteRecoverySnapshot, record *models.ExecutorRunning) bool {
	if current == nil || record == nil {
		return false
	}
	binding, exists := models.LoadAgentDeliveryPromptTurnBinding(record.Metadata)
	return exists && binding.SubmissionID != "" && binding.TurnID != "" && binding.TaskID == record.TaskID &&
		binding.SessionID == record.SessionID && binding.ExecutionID == record.AgentExecutionID &&
		binding.StreamID == current.RecoveryStreamID && binding.SubmissionID == current.RecoverySubmissionID &&
		binding.IncarnationID == current.RecoveryIncarnationID &&
		int64(binding.HarnessGeneration) == current.RecoveryHarnessGeneration
}

func (m *Manager) stoppedRemoteWorkflowEffect(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	terminal *models.AgentDeliveryEffect,
	turnID string,
) (*models.AgentDeliveryEffect, string, error) {
	if err := validateStoppedRemoteTerminalOwner(entry, terminal); err != nil {
		return nil, "", err
	}
	resolved, err := stoppedRemoteTerminalTurnEffect(terminal, turnID)
	if err != nil {
		return nil, "", err
	}
	task, err := m.stoppedRemoteTerminalTask(ctx, entry.record.TaskID, resolved.TaskID)
	if err != nil {
		return nil, "", err
	}
	workflow := *resolved
	workflow.EffectKey = "workflow.on_turn_complete:" + resolved.TurnID
	workflow.EffectType = "workflow.on_turn_complete"
	workflow.State = models.DeliveryEffectCompleted
	workflow.Outcome = models.DeliveryWorkflowOutcomeStoppedRecovery
	workflow.TaskID = task.ID
	workflow.WorkflowID = task.WorkflowID
	workflow.WorkflowStepID = task.WorkflowStepID
	now := time.Now().UTC()
	workflow.CreatedAt, workflow.CompletedAt = now, &now
	return &workflow, task.ID, nil
}

func validateStoppedRemoteTerminalOwner(entry *remoteRecoveryRetry, terminal *models.AgentDeliveryEffect) error {
	if entry == nil || entry.record == nil || terminal == nil {
		return fmt.Errorf("%w: stopped remote terminal owner is incomplete", ErrDurableAdoptionIdentityMismatch)
	}
	if terminal.TaskID != "" && terminal.TaskID != entry.record.TaskID || terminal.SessionID != entry.record.SessionID {
		return fmt.Errorf("%w: stopped remote terminal owner is incomplete", ErrDurableAdoptionIdentityMismatch)
	}
	return nil
}

func stoppedRemoteTerminalTurnEffect(
	terminal *models.AgentDeliveryEffect,
	turnID string,
) (*models.AgentDeliveryEffect, error) {
	resolved := *terminal
	if turnID != "" && resolved.TurnID != "" && resolved.TurnID != turnID {
		return nil, fmt.Errorf("%w: stopped remote terminal no longer owns the original turn", ErrDurableAdoptionIdentityMismatch)
	}
	if resolved.TurnID == "" {
		resolved.TurnID = turnID
	}
	if resolved.TurnID == "" {
		return nil, fmt.Errorf("%w: stopped remote terminal has no original turn identity", ErrDurableAdoptionIdentityMismatch)
	}
	return &resolved, nil
}

func (m *Manager) stoppedRemoteTerminalTask(
	ctx context.Context,
	taskID, terminalTaskID string,
) (*models.Task, error) {
	tasks, ok := m.runningWriter.(interface {
		GetTask(context.Context, string) (*models.Task, error)
	})
	if !ok {
		return nil, fmt.Errorf("%w: stopped remote task reader is unavailable", ErrDurableAdoptionEvidenceUnavailable)
	}
	task, err := tasks.GetTask(ctx, taskID)
	if err != nil || task == nil || task.ID != taskID || terminalTaskID != "" && task.ID != terminalTaskID {
		return nil, fmt.Errorf("%w: read stopped remote task for terminal settlement: %v", ErrDurableAdoptionEvidenceUnavailable, err)
	}
	return task, nil
}

func stoppedRemoteWorkflowOutcomeMatches(existing, expected *models.AgentDeliveryEffect) bool {
	if existing == nil || expected == nil || existing.EffectKey != expected.EffectKey ||
		existing.EffectType != expected.EffectType || existing.State != models.DeliveryEffectCompleted ||
		existing.StreamID != expected.StreamID || existing.Sequence != expected.Sequence ||
		existing.SessionID != expected.SessionID || existing.IncarnationID != expected.IncarnationID ||
		existing.HarnessGeneration != expected.HarnessGeneration || existing.SubmissionID != expected.SubmissionID ||
		existing.TurnID != expected.TurnID {
		return false
	}
	switch existing.Outcome {
	case models.DeliveryWorkflowOutcomeApplied, models.DeliveryWorkflowOutcomeNoAutomaticTransition,
		models.DeliveryWorkflowOutcomeNotRequired, models.DeliveryWorkflowOutcomeStoppedRecovery:
		return true
	default:
		return false
	}
}

func validateStoppedRemoteSQLSubmission(snapshot *RemoteRecoverySnapshot, submission *models.AgentDeliverySubmission) error {
	if submission == nil || submission.ID != snapshot.RecoverySubmissionID || submission.SessionID != snapshot.SessionID ||
		submission.IncarnationID != snapshot.RecoveryIncarnationID || submission.HarnessGeneration != snapshot.RecoveryHarnessGeneration ||
		submission.PayloadHash == "" {
		return fmt.Errorf("%w: stopped remote SQL submission identity changed", ErrDurableAdoptionIdentityMismatch)
	}
	return nil
}

func stoppedRemoteSubmissionHasTerminalOutcome(submission *models.AgentDeliverySubmission) bool {
	return submission != nil && (submission.State == models.DeliverySubmissionCompleted ||
		submission.State == models.DeliverySubmissionFailed || submission.State == models.DeliverySubmissionCancelled)
}
