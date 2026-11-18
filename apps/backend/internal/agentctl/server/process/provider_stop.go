package process

import (
	"context"
	"errors"
	"go.uber.org/zap"
)

func (m *Manager) stopWithWorkspace(ctx context.Context, workspace bool) error {
	m.stopInProgress.Add(1)
	defer m.stopInProgress.Add(-1)
	if err := m.passthrough.StopAll(ctx); err != nil {
		return err
	}
	defer m.closeOpenCodePlugin(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop intentionally serializes the full teardown under m.mu. The shutdown
	// path closes adapters, stdin, shell sessions, and process groups as one
	// lifecycle transition; concurrent readers may briefly block while process
	// group reaping finishes.

	// Stop trackers before the status guard: passthrough never calls Start() so the early return below would otherwise leak them.
	var comparisonStopErr error
	if workspace {
		m.stopWorkspaceTrackers()
		var drained bool
		comparisonStopErr, drained = m.stopComparisonTargetOperations(ctx)
		if drained {
			defer m.reopenComparisonTargetOperations()
		}
	}

	status := m.Status()
	if status == StatusStopped || status == StatusStopping {
		m.logger.Info("Stop called but already stopped/stopping",
			zap.String("status", string(status)),
			zap.Int("pid", m.agentPID()))
		if status == StatusStopped {
			return m.finishStoppedProvider(ctx, workspace, comparisonStopErr)
		}
		return nil
	}

	m.logger.Info("stopping agent process - START",
		zap.Int("pid", m.agentPID()),
		zap.String("protocol", m.agentProtocol()))
	m.logger.Debug("agent process stop requested",
		zap.Int("pid", m.agentPID()),
		zap.String("protocol", m.agentProtocol()))
	m.status.Store(StatusStopping)
	if stopping := m.shutdownIntent.Load(); stopping != nil {
		stopping.Store(true)
	}

	auxiliaryStopErr := errors.Join(comparisonStopErr, m.stopWorkspaceProcessesIfRequested(ctx, workspace))
	m.closeAdapterAndStdin()
	m.killProcessGroupIfRequired()
	mainStopErr := m.waitForProcessExit(ctx)

	m.status.Store(StatusStopped)
	m.logger.Info("stopping agent process - COMPLETE")
	if stopErr := errors.Join(auxiliaryStopErr, mainStopErr); stopErr != nil {
		m.mainReapPending.Store(mainStopErr != nil)
		return stopErr
	}
	m.mainReapPending.Store(false)
	return nil
}

func (m *Manager) finishStoppedProvider(ctx context.Context, workspace bool, comparisonStopErr error) error {
	// The agent process reaches StatusStopped on its own whenever the
	// child exits without an explicit Stop (waitForExit stores it). The
	// adapter and stopCh were never torn down in that case, so do it
	// here — otherwise the adapter's update worker and forwardUpdates
	// stay alive for the lifetime of the manager. The call is a no-op
	// after a normal stop, in which case behaviour is unchanged.
	tornDown := m.closeAdapterAndStdin()
	if err := m.stopWorkspaceProcessesIfRequested(ctx, workspace); err != nil {
		return errors.Join(comparisonStopErr, err)
	}
	switch {
	case m.mainReapPending.Load():
		if err := m.waitForProcessExit(ctx); err != nil {
			return errors.Join(comparisonStopErr, err)
		}
		m.mainReapPending.Store(false)
	case tornDown:
		m.drainAfterLateTeardown(ctx)
	}
	return comparisonStopErr
}

// ProviderTerminationConfirmed requires the main group to be fully reaped.
func (m *Manager) ProviderTerminationConfirmed() bool {
	return m.Status() == StatusStopped && !m.mainReapPending.Load() && m.AgentProcessIdentity() == nil
}
