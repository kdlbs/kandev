package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

func (m *Manager) settleRemoteRecoveryAbsence(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	attemptRecord *models.ExecutorRunning,
	outcome RecoveryCandidateOutcome,
) error {
	if !remoteRecoveryAbsenceEligible(entry, outcome) {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateRemoteRecoveryAbsenceAttempt(entry, attemptRecord); err != nil {
		return err
	}
	reader, writer, err := remoteRecoveryAbsencePersistence(m.runningWriter)
	if err != nil {
		return err
	}
	if !m.remoteRecoveryAbsenceDispositionAllowed(ctx, entry) {
		return nil
	}
	if err := m.validateRemoteRecoveryAbsenceCurrentOwner(ctx, entry, attemptRecord, reader); err != nil {
		return err
	}
	if !m.remoteRecoveryAbsenceDispositionAllowed(ctx, entry) {
		return nil
	}
	if err := writer.RepairExecutorRunningDeadIfCurrent(
		ctx, entry.record.SessionID, attemptRecord.AgentExecutionID, attemptRecord.UpdatedAt,
	); err != nil {
		return fmt.Errorf("repair confirmed absent remote executor row: %w", err)
	}

	m.completeRemoteRecoveryAbsence(entry, ctx)
	return nil
}

func remoteRecoveryAbsenceEligible(entry *remoteRecoveryRetry, outcome RecoveryCandidateOutcome) bool {
	return entry != nil && entry.record != nil && outcome == RecoveryOutcomeNoMatchingInstance &&
		remoteRecoveryNoMatchProvesStopped(entry.record.Runtime) && !remoteRecoveryOwnerUnsupported(entry.record)
}

func validateRemoteRecoveryAbsenceAttempt(entry *remoteRecoveryRetry, attemptRecord *models.ExecutorRunning) error {
	if !sameRemoteRecoveryRecordOwner(entry.record, attemptRecord) || attemptRecord.UpdatedAt.IsZero() {
		return errors.New("remote recovery attempt did not retain its observed runtime row")
	}
	return nil
}

func remoteRecoveryAbsencePersistence(writer ExecutorRunningWriter) (executorRunningReader, executorRunningCASWriter, error) {
	reader, hasReader := writer.(executorRunningReader)
	casWriter, hasWriter := writer.(executorRunningCASWriter)
	if !hasReader || !hasWriter {
		return nil, nil, errors.New("remote recovery absence repair requires runtime row read and compare-and-set support")
	}
	return reader, casWriter, nil
}

func (m *Manager) validateRemoteRecoveryAbsenceCurrentOwner(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	attemptRecord *models.ExecutorRunning,
	reader executorRunningReader,
) error {
	owner, err := m.remoteRecoverySnapshot(ctx, entry.record.SessionID)
	if err != nil {
		return fmt.Errorf("re-read remote recovery owner before absence repair: %w", err)
	}
	if !sameRemoteRecoveryOwner(entry.snapshot, owner, entry.record) {
		return errRemoteRecoveryFenced
	}
	if err := validateRemoteRecoverySnapshot(owner, entry.record); err != nil || remoteRecoverySnapshotFences(owner) {
		if err != nil {
			return fmt.Errorf("remote recovery owner is no longer eligible for absence repair: %w", err)
		}
		return errors.New("remote recovery owner is fenced from absence repair")
	}
	current, err := reader.GetExecutorRunningBySessionID(ctx, entry.record.SessionID)
	if err != nil {
		return fmt.Errorf("re-read executor row before remote absence repair: %w", err)
	}
	if !sameRemoteRecoveryRecordOwner(attemptRecord, current) || !current.UpdatedAt.Equal(attemptRecord.UpdatedAt) {
		return errRemoteRecoveryFenced
	}
	return nil
}

func (m *Manager) completeRemoteRecoveryAbsence(entry *remoteRecoveryRetry, ctx context.Context) {
	m.remoteRecoveryMu.Lock()
	defer m.remoteRecoveryMu.Unlock()
	if m.remoteRecoveryPending[entry.record.SessionID] != entry || !entry.inProgress || entry.stopRequested ||
		m.shuttingDown.Load() || ctx.Err() != nil || m.remoteRecoverySessionTracked(entry) {
		return
	}
	select {
	case <-m.stopCh:
		return
	default:
	}
	delete(m.remoteRecoveryPending, entry.record.SessionID)
	m.recoveryGuard.Release(entry.record.SessionID)
}

func (m *Manager) remoteRecoveryAbsenceAttemptCurrent(entry *remoteRecoveryRetry) bool {
	if entry == nil || entry.record == nil {
		return false
	}
	m.remoteRecoveryMu.Lock()
	defer m.remoteRecoveryMu.Unlock()
	if m.remoteRecoveryPending[entry.record.SessionID] != entry || !entry.inProgress || entry.stopRequested || m.shuttingDown.Load() {
		return false
	}
	select {
	case <-m.stopCh:
		return false
	default:
		return true
	}
}

func (m *Manager) remoteRecoveryAbsenceDispositionAllowed(ctx context.Context, entry *remoteRecoveryRetry) bool {
	return ctx.Err() == nil && m.remoteRecoveryAbsenceAttemptCurrent(entry) && !m.remoteRecoverySessionTracked(entry)
}

func (m *Manager) remoteRecoverySessionTracked(entry *remoteRecoveryRetry) bool {
	if entry == nil || entry.record == nil {
		return false
	}
	_, tracked := m.executionStore.GetBySessionID(entry.record.SessionID)
	return tracked
}
