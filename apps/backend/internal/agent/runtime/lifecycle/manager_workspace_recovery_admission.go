package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

type workspaceSessionRecoveryReader interface {
	GetCurrentHarnessSessionGeneration(context.Context, string, string) (*models.HarnessSessionGeneration, error)
	GetOpenSessionRecoveryBlock(context.Context, string, string, int64) (*models.SessionRecoveryBlock, error)
}

func (m *Manager) ensureWorkspaceRecoveryResolved(ctx context.Context, sessionID string) error {
	if m.executorProfileReader == nil || sessionID == "" {
		return nil
	}
	session, err := m.readWorkspaceRecoverySession(ctx, sessionID)
	if err != nil {
		return err
	}
	hasRecovery, err := workspaceRecoveryMetadata(session)
	if err != nil {
		return err
	}
	reader, ok := m.executorProfileReader.(workspaceSessionRecoveryReader)
	if !ok {
		if hasRecovery {
			return workspaceRecoveryRequired("session recovery reader is unavailable", nil)
		}
		return nil
	}
	if session.QueueIncarnationID == "" {
		if hasRecovery {
			return workspaceRecoveryRequired("current session incarnation is unavailable", nil)
		}
		return nil
	}

	generation, err := readWorkspaceRecoveryGeneration(ctx, reader, session, hasRecovery)
	if err != nil {
		return err
	}
	if generation == nil {
		return nil
	}
	return ensureWorkspaceRecoveryBlockAbsent(ctx, reader, session, generation)
}

func (m *Manager) readWorkspaceRecoverySession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	session, err := m.executorProfileReader.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, workspaceRecoveryRequired("read session recovery state", err)
	}
	if session == nil || session.ID != sessionID {
		return nil, workspaceRecoveryRequired("session recovery identity is unavailable", nil)
	}
	return session, nil
}

func workspaceRecoveryMetadata(session *models.TaskSession) (bool, error) {
	recovery, valid := models.LoadAgentDeliveryRecovery(session.Metadata)
	_, markerPresent := session.Metadata[models.SessionMetaKeyAgentDeliveryRecovery]
	if markerPresent && !valid {
		return true, workspaceRecoveryRequired("session recovery metadata is malformed", nil)
	}
	if !valid {
		return false, nil
	}

	switch recovery.Phase {
	case models.AgentDeliveryRecoveryReconnecting, models.AgentDeliveryRecoveryUncertain:
		return true, workspaceRecoveryRequired("session recovery is unresolved", nil)
	case models.AgentDeliveryRecoveryRecovered,
		models.AgentDeliveryRecoverySettled,
		models.AgentDeliveryRecoveryContinued,
		models.AgentDeliveryRecoveryRestored:
		return true, nil
	default:
		return true, workspaceRecoveryRequired("session recovery phase is unknown", nil)
	}
}

func readWorkspaceRecoveryGeneration(
	ctx context.Context,
	reader workspaceSessionRecoveryReader,
	session *models.TaskSession,
	hasRecovery bool,
) (*models.HarnessSessionGeneration, error) {
	generation, err := reader.GetCurrentHarnessSessionGeneration(ctx, session.ID, session.QueueIncarnationID)
	if err != nil {
		if isNoRows(err) || errors.Is(err, models.ErrTaskSessionNotFound) {
			if hasRecovery {
				return nil, workspaceRecoveryRequired("current session generation is unavailable", err)
			}
			return nil, nil
		}
		return nil, workspaceRecoveryRequired("read current session generation", err)
	}
	if generation == nil {
		if hasRecovery {
			return nil, workspaceRecoveryRequired("current session generation is unavailable", nil)
		}
		return nil, nil
	}
	if generation.SessionID != session.ID || generation.IncarnationID != session.QueueIncarnationID || generation.Generation < 1 {
		return nil, workspaceRecoveryRequired("current session generation is inconsistent", nil)
	}
	return generation, nil
}

func ensureWorkspaceRecoveryBlockAbsent(
	ctx context.Context,
	reader workspaceSessionRecoveryReader,
	session *models.TaskSession,
	generation *models.HarnessSessionGeneration,
) error {
	block, err := reader.GetOpenSessionRecoveryBlock(ctx, session.ID, session.QueueIncarnationID, generation.Generation)
	if err != nil {
		if isNoRows(err) {
			return nil
		}
		return workspaceRecoveryRequired("read current session recovery block", err)
	}
	if block == nil {
		return workspaceRecoveryRequired("session recovery block result is unavailable", nil)
	}
	if block.SessionID != session.ID || block.IncarnationID != session.QueueIncarnationID ||
		block.ExpectedGeneration != generation.Generation || block.State != models.RecoveryBlockOpen {
		return workspaceRecoveryRequired("session recovery block is inconsistent", nil)
	}
	return &SessionRecoveryBlockedError{Block: block}
}

func workspaceRecoveryRequired(reason string, cause error) error {
	if cause != nil {
		return fmt.Errorf("%w: %s: %w", ErrSessionRecoveryRequired, reason, cause)
	}
	return fmt.Errorf("%w: %s", ErrSessionRecoveryRequired, reason)
}
