package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

type persistedStandaloneCleanupStore interface {
	ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error)
}

// liveStandaloneExecutionRow returns the authoritative standalone row for a
// session whose execution is no longer tracked in memory. Failed adoption
// detaches intentionally leave this row so Stop can still reach the peer.
func (m *Manager) liveStandaloneExecutionRow(ctx context.Context, sessionID string) *models.ExecutorRunning {
	if m == nil || sessionID == "" {
		return nil
	}
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return nil
	}
	row, err := reader.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || row == nil || row.Runtime != agentruntime.RuntimeStandalone ||
		row.AgentExecutionID == "" || isTerminalExecutorRunningStatus(row.Status) {
		return nil
	}
	return row
}

// stopPersistedStandaloneExecution stops the exact peer identified by a
// detached execution's authoritative runtime row. Healthy backend shutdown with
// agent survival enabled keeps the peer for the next backend.
func (m *Manager) stopPersistedStandaloneExecution(
	ctx context.Context,
	executionID string,
	reason string,
	force bool,
) (bool, error) {
	if reason == StopReasonBackendShutdown && m.agentSurvivalEnabled {
		return false, nil
	}
	store, ok := m.runningWriter.(persistedStandaloneCleanupStore)
	if !ok {
		return false, nil
	}
	rows, err := store.ListExecutorsRunning(ctx)
	if err != nil {
		return true, fmt.Errorf("load persisted standalone runtime inventory: %w", err)
	}
	row, err := uniquePersistedStandaloneExecution(rows, executionID)
	if err != nil {
		return true, err
	}
	if row == nil {
		return false, nil
	}
	return true, m.stopPersistedStandaloneRecord(ctx, row, reason, force)
}

func (m *Manager) stopPersistedStandaloneRecord(
	ctx context.Context,
	row *models.ExecutorRunning,
	reason string,
	force bool,
) error {
	activityLease, err := m.acquireActivity(ctx, activity.KindExecutionStopping)
	if err != nil {
		return err
	}
	defer activityLease.Release()

	if m.executorRegistry == nil {
		return errors.New("standalone runtime registry is unavailable")
	}
	backend, err := m.executorRegistry.GetBackend(executor.NameStandalone)
	if err != nil {
		return fmt.Errorf("get standalone runtime: %w", err)
	}
	instance := &ExecutorInstance{
		InstanceID:           row.AgentExecutionID,
		TaskID:               row.TaskID,
		SessionID:            row.SessionID,
		RuntimeName:          agentruntime.RuntimeStandalone,
		StandaloneInstanceID: row.AgentExecutionID,
		StandalonePort:       row.AgentctlPort,
		Metadata:             row.Metadata,
		StopReason:           reason,
	}
	if err := m.stopWorkspaceRuntime(ctx, backend, instance, force); err != nil {
		return fmt.Errorf("stop persisted standalone runtime: %w", err)
	}
	if err := m.repairPersistedStandaloneRow(ctx, row); err != nil {
		return fmt.Errorf("repair persisted standalone runtime row: %w", err)
	}
	m.clearRemoteStatus(row.SessionID)
	return nil
}

// repairPersistedStandaloneRow keeps the row resumable for a later explicit
// continuation while clearing its live process handle.
func (m *Manager) repairPersistedStandaloneRow(ctx context.Context, row *models.ExecutorRunning) error {
	if cas, ok := m.runningWriter.(interface {
		RepairExecutorRunningDeadIfCurrent(context.Context, string, string, time.Time) error
	}); ok {
		return cas.RepairExecutorRunningDeadIfCurrent(ctx, row.SessionID, row.AgentExecutionID, row.UpdatedAt)
	}
	if repair, ok := m.runningWriter.(interface {
		RepairExecutorRunningDead(context.Context, string) error
	}); ok {
		return repair.RepairExecutorRunningDead(ctx, row.SessionID)
	}
	return nil
}

func uniquePersistedStandaloneExecution(
	rows []*models.ExecutorRunning,
	executionID string,
) (*models.ExecutorRunning, error) {
	var found *models.ExecutorRunning
	for _, row := range rows {
		if row == nil || row.Runtime != agentruntime.RuntimeStandalone ||
			strings.TrimSpace(row.AgentExecutionID) != executionID {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("multiple persisted standalone rows match execution %q", executionID)
		}
		found = row
	}
	return found, nil
}
