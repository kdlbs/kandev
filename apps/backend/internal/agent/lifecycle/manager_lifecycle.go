package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const containerStateExited = "exited"

// Start starts the lifecycle manager background tasks
func (m *Manager) Start(ctx context.Context) error {
	if m.runtimeRegistry == nil {
		m.logger.Warn("no runtime registry configured")
		return nil
	}

	runtimeNames := m.runtimeRegistry.List()
	m.logger.Info("starting lifecycle manager", zap.Int("runtimes", len(runtimeNames)))

	// Check health of all registered runtimes
	healthResults := m.runtimeRegistry.HealthCheckAll(ctx)
	for name, err := range healthResults {
		if err != nil {
			m.logger.Warn("runtime health check failed",
				zap.String("runtime", string(name)),
				zap.Error(err))
		} else {
			m.logger.Info("runtime is healthy", zap.String("runtime", string(name)))
		}
	}

	// Try to recover executions from all runtimes
	recovered, err := m.runtimeRegistry.RecoverAll(ctx)
	if err != nil {
		m.logger.Warn("failed to recover executions from some runtimes", zap.Error(err))
	}
	if len(recovered) > 0 {
		for _, ri := range recovered {
			execution := &AgentExecution{
				ID:                   ri.InstanceID,
				TaskID:               ri.TaskID,
				SessionID:            ri.SessionID,
				ContainerID:          ri.ContainerID,
				ContainerIP:          ri.ContainerIP,
				WorkspacePath:        ri.WorkspacePath,
				RuntimeName:          ri.RuntimeName,
				Status:               v1.AgentStatusRunning,
				StartedAt:            time.Now(),
				Metadata:             ri.Metadata,
				agentctl:             ri.Client,
				standaloneInstanceID: ri.StandaloneInstanceID,
				standalonePort:       ri.StandalonePort,
				promptDoneCh:         make(chan PromptCompletionSignal, 1),
			}
			m.executionStore.Add(execution)

			// Reconnect to workspace streams (shell, git, file changes) in background
			// This is needed so shell.input, git status, etc. work after backend restart
			go m.streamManager.ReconnectAll(execution)
		}
		m.logger.Info("recovered executions", zap.Int("count", len(recovered)))
	}

	// Start cleanup loop when container manager is available (Docker mode)
	if m.containerManager != nil {
		m.wg.Add(1)
		go m.cleanupLoop(ctx)
		m.logger.Info("cleanup loop started")
	}

	// Set up callbacks for passthrough mode (using standalone runtime)
	if standaloneRT, err := m.runtimeRegistry.GetRuntime(runtime.NameStandalone); err == nil {
		if interactiveRunner := standaloneRT.GetInteractiveRunner(); interactiveRunner != nil {
			// Turn complete callback
			interactiveRunner.SetTurnCompleteCallback(func(sessionID string) {
				m.handlePassthroughTurnComplete(sessionID)
			})

			// Output callback for standalone passthrough (no WorkspaceTracker)
			interactiveRunner.SetOutputCallback(func(output *agentctltypes.ProcessOutput) {
				m.handlePassthroughOutput(output)
			})

			// Status callback for standalone passthrough (no WorkspaceTracker)
			interactiveRunner.SetStatusCallback(func(status *agentctltypes.ProcessStatusUpdate) {
				m.handlePassthroughStatus(status)
			})

			m.logger.Info("passthrough callbacks configured")
		}
	}

	return nil
}

// GetRecoveredExecutions returns a snapshot of all currently tracked executions
// This can be used by the orchestrator to sync with the database
func (m *Manager) GetRecoveredExecutions() []RecoveredExecution {
	executions := m.executionStore.List()
	result := make([]RecoveredExecution, 0, len(executions))
	for _, exec := range executions {
		result = append(result, RecoveredExecution{
			ExecutionID:    exec.ID,
			TaskID:         exec.TaskID,
			SessionID:      exec.SessionID,
			ContainerID:    exec.ContainerID,
			AgentProfileID: exec.AgentProfileID,
		})
	}
	return result
}

// Stop stops the lifecycle manager
func (m *Manager) Stop() error {
	m.logger.Info("stopping lifecycle manager")

	close(m.stopCh)
	m.wg.Wait()

	return nil
}

// StopAllAgents attempts a graceful shutdown of all active agents concurrently.
func (m *Manager) StopAllAgents(ctx context.Context) error {
	executions := m.executionStore.List()
	if len(executions) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(executions))

	for _, exec := range executions {
		wg.Add(1)
		go func(e *AgentExecution) {
			defer wg.Done()
			if err := m.StopAgent(ctx, e.ID, false); err != nil {
				errCh <- err
				m.logger.Warn("failed to stop agent during shutdown",
					zap.String("execution_id", e.ID),
					zap.Error(err))
			}
		}(exec)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// cleanupLoop runs periodic cleanup of stale containers
func (m *Manager) cleanupLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("cleanup loop stopped (context cancelled)")
			return
		case <-m.stopCh:
			m.logger.Info("cleanup loop stopped")
			return
		case <-ticker.C:
			m.performCleanup(ctx)
		}
	}
}

// performCleanup checks for and cleans up stale containers (Docker mode only)
func (m *Manager) performCleanup(ctx context.Context) {
	m.logger.Debug("running cleanup check")

	// Skip cleanup if container manager is not available
	if m.containerManager == nil {
		m.logger.Debug("skipping cleanup - no container manager")
		return
	}

	// List all kandev-managed containers
	containers, err := m.containerManager.ListManagedContainers(ctx)
	if err != nil {
		m.logger.Error("failed to list containers for cleanup", zap.Error(err))
		return
	}

	for _, container := range containers {
		if container.State == containerStateExited {
			m.cleanupExitedContainer(ctx, container.ID)
		}
	}
}

// cleanupExitedContainer handles cleanup for a single exited container.
func (m *Manager) cleanupExitedContainer(ctx context.Context, containerID string) {
	execution, tracked := m.executionStore.GetByContainerID(containerID)
	if !tracked {
		return
	}

	// Get container info to get exit code
	info, err := m.containerManager.GetContainerInfo(ctx, containerID)
	if err != nil {
		m.logger.Warn("failed to get container info during cleanup",
			zap.String("container_id", containerID),
			zap.Error(err))
		return
	}

	// Mark execution as completed
	errorMsg := ""
	if info.ExitCode != 0 {
		errorMsg = fmt.Sprintf("container exited with code %d", info.ExitCode)
	}
	_ = m.MarkCompleted(execution.ID, info.ExitCode, errorMsg)

	// Remove the container
	if err := m.containerManager.RemoveContainer(ctx, containerID, false); err != nil {
		m.logger.Warn("failed to remove container during cleanup",
			zap.String("container_id", containerID),
			zap.Error(err))
	}

	// Remove the execution from tracking so new agents can be launched
	m.RemoveExecution(execution.ID)
}

// CleanupStaleExecutionBySessionID removes a stale execution from tracking without stopping it.
//
// A "stale" execution is one where the agent process has stopped externally (crashed, killed,
// or terminated outside of our control) but the execution is still tracked in memory.
//
// When to use this:
//   - After detecting the agentctl HTTP server is unreachable
//   - When the agent container no longer exists (Docker runtime)
//   - After server restart when recovering persisted state
//   - When IsAgentRunningForSession returns false but execution exists
//
// This method performs cleanup:
//  1. Closes the agentctl HTTP client connection
//  2. Removes the execution from the in-memory tracking store
//
// What this does NOT do:
//   - Stop the agent process (assumed already stopped)
//   - Clean up worktrees or containers (caller's responsibility)
//   - Update database session state (caller's responsibility)
//
// This is safe to call even if the process is still running - it won't send kill signals.
// Use StopAgent if you need to actively terminate a running agent.
//
// Returns nil if no execution exists for the session (idempotent).
func (m *Manager) CleanupStaleExecutionBySessionID(ctx context.Context, sessionID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil // No execution to clean up
	}

	m.logger.Info("cleaning up stale agent execution",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID))

	// Close agentctl connection if it exists
	if execution.agentctl != nil {
		execution.agentctl.Close()
	}

	// Remove from execution store
	m.executionStore.Remove(execution.ID)

	return nil
}

// RemoveExecution removes an execution from tracking.
//
// ⚠️  WARNING: This is a potentially dangerous operation that should only be called when:
//  1. The agent process has been fully stopped (via StopAgent)
//  2. All cleanup operations have completed (worktree cleanup, container removal)
//  3. The execution is in a terminal state (Completed, Failed, or Cancelled)
//
// This method:
//   - Removes the execution from the in-memory store
//   - Makes the sessionID available for new executions
//   - Does NOT stop the agent process (call StopAgent first)
//   - Does NOT close the agentctl client (call execution.agentctl.Close() first)
//   - Does NOT clean up resources (worktrees, containers, etc.)
//
// After calling this, the executionID and sessionID can no longer be used to query
// or control the execution. Any references to this execution will become invalid.
//
// Typical usage: Called by cleanup loops or after successful StopAgent completion.
// For stale/dead executions, use CleanupStaleExecutionBySessionID instead.
func (m *Manager) RemoveExecution(executionID string) {
	m.executionStore.Remove(executionID)
	m.logger.Debug("removed execution from tracking",
		zap.String("execution_id", executionID))
}
