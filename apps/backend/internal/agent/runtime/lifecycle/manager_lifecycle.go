package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentctl/tracing"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	containerStateCreated = "created"
	containerStateExited  = "exited"
	containerStateRunning = "running"
)

// Start starts the lifecycle manager background tasks
func (m *Manager) Start(ctx context.Context) error {
	if m.executorRegistry == nil {
		m.logger.Warn("no runtime registry configured")
		return nil
	}

	runtimeNames := m.executorRegistry.List()
	m.logger.Info("starting lifecycle manager", zap.Int("runtimes", len(runtimeNames)))

	// Check health of all registered runtimes
	healthResults := m.executorRegistry.HealthCheckAll(ctx)
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
	recovered, err := m.executorRegistry.RecoverAll(ctx)
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
				metadata:             ri.Metadata,
				agentctl:             ri.Client,
				standaloneInstanceID: ri.StandaloneInstanceID,
				standalonePort:       ri.StandalonePort,
				promptDoneCh:         make(chan PromptCompletionSignal, 1),
			}
			// Create trace span for the recovered session
			_, recoverySpan := tracing.TraceSessionRecovered(
				context.Background(), execution.TaskID, execution.SessionID, execution.ID,
			)
			execution.SetSessionSpan(recoverySpan)
			if ri.Client != nil {
				ri.Client.SetTraceContext(execution.SessionTraceContext())
			}

			// Create short-lived init span so recovery-phase operations are visible
			_, initSpan := tracing.TraceSessionInit(
				execution.SessionTraceContext(), execution.TaskID, execution.SessionID, execution.ID,
			)

			if err := m.executionStore.Add(execution); err != nil {
				// Should not happen at startup — duplicate sessions in the recovery
				// list signal a DB consistency issue, not a normal race. Log loudly
				// and skip; the first one to land wins.
				m.logger.Error("skipping duplicate execution during recovery",
					zap.String("execution_id", execution.ID),
					zap.String("session_id", execution.SessionID),
					zap.Error(err))
				if ri.Client != nil {
					ri.Client.Close()
				}
				execution.EndSessionSpan()
				initSpan.End()
				continue
			}
			m.setRuntimeInterest(execution.SessionID, true)

			// Reconcile the persistence row to match the recovered in-memory ID.
			// If executors_running.agent_execution_id had drifted (e.g. from a
			// prior bug or manual edit), the recovered runtime instance is the
			// truth — overwrite the row to match. No-op if already in sync.
			m.persistExecutorRunning(ctx, execution)

			// Re-seed the base-branch map before reconnecting. A surviving
			// agentctl instance never passes through waitForAgentctlReady, so
			// without this its trackers keep whatever map they had — or none,
			// if it was created by a path that could not supply one — and
			// their diff stats stay pinned to an integration-branch fallback.
			if client, releaseClient := execution.AcquireAgentCtlClient(); client != nil {
				m.pushTaskBaseBranches(ctx, execution.TaskID, execution.ID, client)
				m.pushTaskComparisonTargets(ctx, execution.TaskID, execution.ID, client)
				releaseClient()
			}

			// Reconnect to workspace streams (shell, git, file changes) in background
			// This is needed so shell.input, git status, etc. work after backend restart
			go m.streamManager.ReconnectAll(execution)

			initSpan.End()
		}
		m.logger.Info("recovered executions", zap.Int("count", len(recovered)))
	}

	// Start remote status polling loop for runtimes exposing remote status.
	m.wg.Add(1)
	go m.remoteStatusLoop(ctx)
	m.logger.Info("remote status loop started")
	// Set up callbacks for passthrough mode (using standalone runtime)
	if standaloneRT, err := m.executorRegistry.GetBackend(executor.NameStandalone); err == nil {
		if interactiveRunner := standaloneRT.GetInteractiveRunner(); interactiveRunner != nil {
			// Turn complete callback
			interactiveRunner.SetTurnCompleteCallback(func(sessionID, processID string) {
				m.handlePassthroughTurnComplete(sessionID, processID)
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
			ExecutionID:        exec.ID,
			TaskID:             exec.TaskID,
			SessionID:          exec.SessionID,
			ContainerID:        exec.ContainerID,
			AgentProfileID:     exec.officeProfileID(),
			ExecutionProfileID: exec.AgentProfileID,
		})
	}
	return result
}

// IsShuttingDown reports whether graceful shutdown has begun. Set by
// StopAllAgents before it starts tearing down executions so concurrent
// handlers (e.g. passthrough exit auto-restart, agentctl HTTP calls) can
// skip or downgrade work that would otherwise race the teardown.
func (m *Manager) IsShuttingDown() bool {
	return m.shuttingDown.Load()
}

// closeStopCh closes the manager shutdown channel at most once.
func (m *Manager) closeStopCh() {
	m.stopOnce.Do(func() { close(m.stopCh) })
}

// Stop stops the lifecycle manager and releases resources held by executors.
func (m *Manager) Stop() error {
	m.logger.Info("stopping lifecycle manager")

	m.closeStopCh()
	if m.streamManager != nil {
		m.streamManager.Wait()
	}
	m.wg.Wait()

	// Close executor backends that hold resources (e.g., Docker SDK client).
	if m.executorRegistry != nil {
		m.executorRegistry.CloseAll()
	}

	return nil
}

// StopReasonBackendShutdown identifies the graceful backend-shutdown path.
// Executors may preserve resumable remote state for this reason only; user
// stops and rollback cleanup deliberately retain their destructive semantics.
const StopReasonBackendShutdown = "backend shutdown"

// StopAllAgents attempts a graceful shutdown of all active agents concurrently.
func (m *Manager) StopAllAgents(ctx context.Context) error {
	m.shuttingDown.Store(true)

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
			if err := m.StopAgentWithReason(ctx, e.ID, StopReasonBackendShutdown, false); err != nil {
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

const stopReasonStaleExecutionCleanup = "stale execution cleanup"

// CleanupStaleExecutionBySessionID cleans up a stale execution: stops the runtime
// instance, closes the client connection, and removes it from tracking.
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
//  1. Stops the runtime instance (workspace tracker, shell, etc.) via the executor backend
//  2. Closes the agentctl HTTP client connection
//  3. Removes the execution from the in-memory tracking store
//
// What this does NOT do:
//   - Clean up worktrees or containers (caller's responsibility)
//   - Update database session state (caller's responsibility)
//
// Safe to call even if the process is already stopped — StopInstance is idempotent.
//
// Returns nil if no execution exists for the session (idempotent).
func (m *Manager) CleanupStaleExecutionBySessionID(ctx context.Context, sessionID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil // No execution to clean up
	}
	return m.cleanupStaleExecution(ctx, execution)
}

// CleanupStaleExecutionBySessionIDIfCurrent cleans up only the execution that
// the caller observed. The operation shares the session-keyed singleflight
// used by launch paths, so a successor launch either wins before cleanup or
// runs after the stale execution has been removed. The persisted timestamp
// check closes the remaining same-ID workspace-promotion window.
func (m *Manager) CleanupStaleExecutionBySessionIDIfCurrent(
	ctx context.Context,
	sessionID, expectedExecutionID string,
	expectedUpdatedAt time.Time,
) error {
	if sessionID == "" || expectedExecutionID == "" {
		return nil
	}
	_, err := m.doCoalescedExecution(ctx, sessionID, func(sharedCtx context.Context) (interface{}, error) {
		execution, exists := m.executionStore.GetBySessionID(sessionID)
		if !exists || execution == nil || execution.ID != expectedExecutionID {
			return nil, nil
		}
		if reader, ok := m.runningWriter.(executorRunningReader); ok && !expectedUpdatedAt.IsZero() {
			current, readErr := reader.GetExecutorRunningBySessionID(sharedCtx, sessionID)
			if readErr != nil || current == nil || current.AgentExecutionID != expectedExecutionID ||
				!current.UpdatedAt.Equal(expectedUpdatedAt) {
				return nil, nil
			}
		}
		return nil, m.cleanupStaleExecution(sharedCtx, execution)
	})
	return err
}

func (m *Manager) cleanupStaleExecution(ctx context.Context, execution *AgentExecution) error {
	if execution == nil {
		return nil
	}
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	execution.agentctlLifecycleMu.Lock()
	defer execution.agentctlLifecycleMu.Unlock()
	sessionID := execution.SessionID

	m.logger.Info("cleaning up stale agent execution",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID))

	// Stop the runtime instance (workspace tracker, shell, etc.) to prevent leaked
	// goroutines. Without this, the old agentctl instance keeps running when a new
	// execution is created for the same session, causing git polling on deleted worktrees.
	// This is idempotent — returns success if the instance is already gone.
	if err := m.stopAgentViaBackend(ctx, execution.ID, execution, stopReasonStaleExecutionCleanup, false, false); err != nil {
		return fmt.Errorf("stop stale runtime %s: %w", execution.ID, err)
	}

	// End session trace span only after the runtime has stopped. A failed stop
	// remains retryable and should keep the execution's lifecycle intact.
	execution.EndSessionSpan()

	// Close agentctl connection if it exists
	if client := execution.currentAgentCtlClient(); client != nil { // protected by agentctlLifecycleMu
		client.Close()
	}

	// Remove from execution store
	m.RemoveExecution(execution.ID)

	// Delete the persistence row in lockstep with store removal so we never
	// leave a phantom executors_running row pointing at a non-existent
	// execution. Best-effort; "not found" is expected and silently swallowed.
	m.deleteExecutorRunning(ctx, sessionID, execution.ID)

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
//   - Does NOT close the agentctl client (use the execution lifecycle lock first)
//   - Does NOT clean up resources (worktrees, containers, etc.)
//
// After calling this, the executionID and sessionID can no longer be used to query
// or control the execution. Any references to this execution will become invalid.
//
// Typical usage: Called by cleanup loops or after successful StopAgent completion.
// For stale/dead executions, use CleanupStaleExecutionBySessionID instead.
func (m *Manager) RemoveExecution(executionID string) {
	m.releaseActivity(executionActivityKey(executionID))
	if execution, ok := m.executionStore.Get(executionID); ok {
		m.closeStreamCoalescer(execution)
		m.cleanupPassthroughMCPConfig(execution)
		m.setRuntimeInterest(execution.SessionID, false)
	}
	m.executionStore.Remove(executionID)
	m.logger.Debug("removed execution from tracking",
		zap.String("execution_id", executionID))
}
