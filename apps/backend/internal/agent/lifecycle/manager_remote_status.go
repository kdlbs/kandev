package lifecycle

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"go.uber.org/zap"
)

func (m *Manager) remoteStatusLoop(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(m.remoteStatusPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pollRemoteStatuses(ctx)
		}
	}
}

func (m *Manager) pollRemoteStatuses(ctx context.Context) {
	for _, execution := range m.executionStore.List() {
		m.pollOneRemoteStatus(ctx, execution)
	}
}

func (m *Manager) pollOneRemoteStatus(ctx context.Context, execution *AgentExecution) {
	if execution == nil || execution.SessionID == "" || m.executorRegistry == nil {
		return
	}
	rt, err := m.executorRegistry.GetBackend(executor.Name(execution.RuntimeName))
	if err != nil {
		return
	}
	provider, ok := rt.(RemoteStatusProvider)
	if !ok {
		return
	}

	instance := &ExecutorInstance{
		InstanceID:           execution.ID,
		TaskID:               execution.TaskID,
		SessionID:            execution.SessionID,
		RuntimeName:          execution.RuntimeName,
		ContainerID:          execution.ContainerID,
		ContainerIP:          execution.ContainerIP,
		WorkspacePath:        execution.WorkspacePath,
		StandaloneInstanceID: execution.standaloneInstanceID,
		StandalonePort:       execution.standalonePort,
		Metadata:             execution.Metadata,
	}
	status, statusErr := provider.GetRemoteStatus(ctx, instance)
	if statusErr != nil {
		m.storeRemoteStatus(execution.SessionID, &RemoteStatus{
			RuntimeName:   execution.RuntimeName,
			LastCheckedAt: time.Now().UTC(),
			ErrorMessage:  statusErr.Error(),
		})
		m.logger.Debug("remote status poll failed",
			zap.String("session_id", execution.SessionID),
			zap.String("execution_id", execution.ID),
			zap.String("runtime", execution.RuntimeName),
			zap.Error(statusErr))
		return
	}

	if status == nil {
		return
	}
	if status.RuntimeName == "" {
		status.RuntimeName = execution.RuntimeName
	}
	if status.LastCheckedAt.IsZero() {
		status.LastCheckedAt = time.Now().UTC()
	}
	m.storeRemoteStatus(execution.SessionID, status)
}

func (m *Manager) storeRemoteStatus(sessionID string, status *RemoteStatus) {
	if sessionID == "" || status == nil {
		return
	}
	m.remoteStatusMu.Lock()
	defer m.remoteStatusMu.Unlock()
	copyStatus := *status
	m.remoteStatusBySession[sessionID] = &copyStatus
}

func (m *Manager) clearRemoteStatus(sessionID string) {
	if sessionID == "" {
		return
	}
	m.remoteStatusMu.Lock()
	defer m.remoteStatusMu.Unlock()
	delete(m.remoteStatusBySession, sessionID)
}

// GetRemoteStatusBySession returns the last known remote status for a session, if any.
func (m *Manager) GetRemoteStatusBySession(sessionID string) (*RemoteStatus, bool) {
	m.remoteStatusMu.RLock()
	defer m.remoteStatusMu.RUnlock()
	status, ok := m.remoteStatusBySession[sessionID]
	if !ok || status == nil {
		return nil, false
	}
	copyStatus := *status
	return &copyStatus, true
}

// GetRemoteStatusBySessionID returns remote status and refreshes it opportunistically
// when a currently tracked execution exists for the session.
func (m *Manager) GetRemoteStatusBySessionID(ctx context.Context, sessionID string) (*RemoteStatus, bool) {
	if execution, ok := m.executionStore.GetBySessionID(sessionID); ok && execution != nil {
		m.pollOneRemoteStatus(ctx, execution)
	}
	return m.GetRemoteStatusBySession(sessionID)
}

// RemoteStatusPollRecord contains the minimal information needed to poll remote status
// for a session that is not currently tracked in the in-memory execution store.
type RemoteStatusPollRecord struct {
	SessionID        string
	Runtime          string
	AgentExecutionID string
	ContainerID      string
}

// PollRemoteStatusForRecords performs a one-time remote status poll for executor records
// that are not in the in-memory execution store. Used during startup to populate the
// remote status cache before any sessions are lazily resumed.
func (m *Manager) PollRemoteStatusForRecords(ctx context.Context, records []RemoteStatusPollRecord) {
	if m.executorRegistry == nil {
		return
	}
	for _, rec := range records {
		if rec.SessionID == "" || rec.Runtime == "" {
			continue
		}
		rt, err := m.executorRegistry.GetBackend(executor.Name(rec.Runtime))
		if err != nil {
			continue
		}
		provider, ok := rt.(RemoteStatusProvider)
		if !ok {
			continue
		}

		instance := &ExecutorInstance{
			InstanceID:  rec.AgentExecutionID,
			SessionID:   rec.SessionID,
			RuntimeName: rec.Runtime,
			ContainerID: rec.ContainerID,
		}
		status, statusErr := provider.GetRemoteStatus(ctx, instance)
		if statusErr != nil {
			m.storeRemoteStatus(rec.SessionID, &RemoteStatus{
				RuntimeName:   rec.Runtime,
				LastCheckedAt: time.Now().UTC(),
				ErrorMessage:  statusErr.Error(),
			})
			m.logger.Debug("startup remote status poll failed",
				zap.String("session_id", rec.SessionID),
				zap.String("runtime", rec.Runtime),
				zap.Error(statusErr))
			continue
		}
		if status == nil {
			continue
		}
		if status.RuntimeName == "" {
			status.RuntimeName = rec.Runtime
		}
		if status.LastCheckedAt.IsZero() {
			status.LastCheckedAt = time.Now().UTC()
		}
		m.storeRemoteStatus(rec.SessionID, status)
		m.logger.Debug("startup remote status polled",
			zap.String("session_id", rec.SessionID),
			zap.String("runtime", rec.Runtime),
			zap.String("state", status.State))
	}
}
