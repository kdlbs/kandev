package lifecycle

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// CleanupFailedRestoreIfCurrent records confirmed termination before removing
// the runtime identity. A failed checkpoint keeps inventory available for retry.
func (m *Manager) CleanupFailedRestoreIfCurrent(
	ctx context.Context, sessionID, executionID string, updatedAt time.Time,
	checkpoint func(context.Context) error,
) error {
	if sessionID == "" || executionID == "" || updatedAt.IsZero() || checkpoint == nil {
		return errors.New("failed restore cleanup requires exact identity and a checkpoint")
	}
	var ownsCleanup atomic.Bool
	cleanup := func(sharedCtx context.Context) (interface{}, error) {
		ownsCleanup.Store(true)
		sharedCtx, cancel := context.WithCancel(sharedCtx)
		defer cancel()
		stop := context.AfterFunc(ctx, cancel)
		defer stop()
		return nil, m.cleanupOwnedFailedRestore(sharedCtx, sessionID, executionID, updatedAt, checkpoint)
	}
	// A workspace operation can own the shared slot. Its result does not prove
	// that this cleanup ran, so retry admission after that operation completes.
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := m.doCoalescedExecution(ctx, sessionID, cleanup)
		if ownsCleanup.Load() {
			return err
		}
	}
	return errors.New("failed restore cleanup could not acquire runtime ownership")
}

func (m *Manager) cleanupOwnedFailedRestore(
	ctx context.Context, sessionID, executionID string, updatedAt time.Time,
	checkpoint func(context.Context) error,
) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists || execution == nil || execution.ID != executionID {
		return models.ErrExecutionRotated
	}
	if execution.RuntimeName == "" || m.executorRegistry == nil {
		return errors.New("failed restore cleanup cannot confirm runtime termination")
	}
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return errors.New("failed restore cleanup requires durable runtime inventory")
	}
	row, err := reader.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		return err
	}
	if row == nil || row.AgentExecutionID != executionID || !row.UpdatedAt.Equal(updatedAt) {
		return models.ErrExecutionRotated
	}
	execution.cancelStartupRecovery()
	return m.cleanupStaleExecutionWithCheckpoint(ctx, execution, checkpoint)
}
