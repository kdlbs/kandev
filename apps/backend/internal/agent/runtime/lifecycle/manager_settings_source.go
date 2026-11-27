package lifecycle

import (
	"context"
	"fmt"
	"math"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

// SessionSettingsSnapshotWriter reserves recovered settings-source epochs in
// task-session metadata before adopted provider streams reconnect.
type SessionSettingsSnapshotWriter interface {
	SetSessionMetadataKey(ctx context.Context, sessionID, key string, value interface{}) error
}

// SetSessionSettingsSnapshotWriter wires the atomic task-session metadata
// writer used while adopting surviving executions.
func (m *Manager) SetSessionSettingsSnapshotWriter(writer SessionSettingsSnapshotWriter) {
	m.sessionSettingsSnapshotWriter = writer
}

func (m *Manager) restoreRecoveredSessionSettingsSource(ctx context.Context, execution *AgentExecution) error {
	if execution == nil {
		return fmt.Errorf("recovered settings source requires a session")
	}
	if execution.SessionID == "" {
		execution.restoreSessionSettingsSource(0, "", SessionSettingsPolicyStrict)
		return nil
	}
	snapshot, hasDurableSource, err := m.loadRecoveredSessionSettingsSnapshot(ctx, execution.SessionID)
	if err != nil {
		return err
	}
	if m.sessionSettingsSnapshotWriter == nil && hasDurableSource {
		return fmt.Errorf("cannot reserve recovered settings source without a snapshot writer")
	}
	if snapshot.SettingsSourceExecutionID != "" && snapshot.SettingsSourceExecutionID != execution.ID {
		snapshot = SessionModelsSnapshot{}
	}
	if m.sessionSettingsSnapshotWriter == nil {
		execution.restoreSessionSettingsSource(0, snapshot.SettingsAttemptID, lifecycleSettingsPolicy(snapshot.SettingsPolicy))
		return nil
	}

	nextGeneration := uint64(1)
	if snapshot.SettingsSourceGeneration > 0 {
		if snapshot.SettingsSourceGeneration == math.MaxUint64 {
			return fmt.Errorf("recovered settings source generation is exhausted")
		}
		nextGeneration = snapshot.SettingsSourceGeneration + 1
	}
	snapshot.SettingsSourceExecutionID = execution.ID
	snapshot.SettingsSourceGeneration = nextGeneration
	snapshot.CurrentModelGeneration = 0
	snapshot.CurrentModeGeneration = 0
	if err := m.sessionSettingsSnapshotWriter.SetSessionMetadataKey(
		ctx,
		execution.SessionID,
		models.SessionMetaKeyACPModelState,
		snapshot,
	); err != nil {
		return fmt.Errorf("reserve recovered session settings source: %w", err)
	}

	execution.restoreSessionSettingsSource(
		nextGeneration,
		snapshot.SettingsAttemptID,
		lifecycleSettingsPolicy(snapshot.SettingsPolicy),
	)
	return nil
}

func (m *Manager) loadRecoveredSessionSettingsSnapshot(
	ctx context.Context,
	sessionID string,
) (SessionModelsSnapshot, bool, error) {
	if m.executorProfileReader == nil {
		if m.sessionSettingsSnapshotWriter != nil {
			return SessionModelsSnapshot{}, false, fmt.Errorf("recovered settings source reader is not configured")
		}
		return SessionModelsSnapshot{}, false, nil
	}

	session, err := m.executorProfileReader.GetTaskSession(ctx, sessionID)
	if err != nil {
		return SessionModelsSnapshot{}, false, fmt.Errorf("read recovered session settings snapshot: %w", err)
	}
	if session == nil {
		return SessionModelsSnapshot{}, false, fmt.Errorf("recovered task session %q is missing", sessionID)
	}

	snapshot, _ := LoadSessionModelsSnapshot(session.Metadata[models.SessionMetaKeyACPModelState])
	hasDurableSource := snapshot.SettingsSourceGeneration > 0 || snapshot.SettingsSourceExecutionID != ""
	return snapshot, hasDurableSource, nil
}

func lifecycleSettingsPolicy(policy streams.SessionSettingsPolicy) SessionSettingsPolicy {
	if policy == streams.SessionSettingsPolicyProviderRestored {
		return SessionSettingsPolicyProviderRestored
	}
	return SessionSettingsPolicyStrict
}

func sessionSettingsProjectionPolicy(policy SessionSettingsPolicy) streams.SessionSettingsPolicy {
	if policy == SessionSettingsPolicyProviderRestored {
		return streams.SessionSettingsPolicyProviderRestored
	}
	return ""
}
