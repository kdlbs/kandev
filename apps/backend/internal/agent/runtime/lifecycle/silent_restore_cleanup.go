package lifecycle

import (
	"context"
	"errors"
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
	cleanup := func(sharedCtx context.Context) (interface{}, error) {
		sharedCtx, cancel := context.WithCancel(sharedCtx)
		defer cancel()
		stop := context.AfterFunc(ctx, cancel)
		defer stop()
		return nil, m.cleanupOwnedFailedRestore(sharedCtx, sessionID, executionID, updatedAt, checkpoint)
	}
	_, err := m.doSerializedExecution(ctx, sessionID, cleanup)
	return err
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
