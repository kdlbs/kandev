package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// PromptAgent sends a follow-up prompt to a running agent
// Attachments (images) are passed to the agent if provided
func (m *Manager) PromptAgent(ctx context.Context, executionID string, prompt string, attachments []v1.MessageAttachment) (*PromptResult, error) {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return nil, fmt.Errorf("execution %q not found: %w", executionID, ErrExecutionNotFound)
	}
	return m.sessionManager.SendPrompt(ctx, execution, prompt, true, attachments)
}

// CancelAgent interrupts the current agent turn without terminating the process,
// allowing the user to send a new prompt.
func (m *Manager) CancelAgent(ctx context.Context, executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	if execution.agentctl == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}

	m.logger.Info("cancelling agent turn",
		zap.String("execution_id", executionID),
		zap.String("task_id", execution.TaskID),
		zap.String("session_id", execution.SessionID))

	if err := execution.agentctl.Cancel(ctx); err != nil {
		m.logger.Error("failed to cancel agent turn",
			zap.String("execution_id", executionID),
			zap.Error(err))
		return fmt.Errorf("failed to cancel agent: %w", err)
	}

	// Clear streaming state after cancel to ensure clean state for next prompt
	execution.messageMu.Lock()
	execution.messageBuffer.Reset()
	execution.thinkingBuffer.Reset()
	execution.currentMessageID = ""
	execution.currentThinkingID = ""
	execution.messageMu.Unlock()

	// Mark as ready for follow-up prompts after successful cancel
	if err := m.MarkReady(executionID); err != nil {
		m.logger.Warn("failed to mark execution as ready after cancel",
			zap.String("execution_id", executionID),
			zap.Error(err))
	}

	m.logger.Info("agent turn cancelled successfully",
		zap.String("execution_id", executionID))

	return nil
}

// CancelAgentBySessionID cancels the current agent turn for a specific session
func (m *Manager) CancelAgentBySessionID(ctx context.Context, sessionID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent running for session %q", sessionID)
	}

	return m.CancelAgent(ctx, execution.ID)
}

// StopAgent stops an agent execution
func (m *Manager) StopAgent(ctx context.Context, executionID string, force bool) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	m.logger.Info("stopping agent",
		zap.String("execution_id", executionID),
		zap.Bool("force", force),
		zap.String("runtime", execution.RuntimeName))

	// Try to gracefully stop via agentctl first
	if execution.agentctl != nil && !force {
		if err := execution.agentctl.Stop(ctx); err != nil {
			m.logger.Warn("failed to stop agent via agentctl",
				zap.String("execution_id", executionID),
				zap.Error(err))
		}
		execution.agentctl.Close()
	}

	// Stop the agent execution via the runtime that created it
	m.stopAgentViaRuntime(ctx, executionID, execution, force)

	// Update execution status and remove from tracking
	_ = m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
		exec.Status = v1.AgentStatusStopped
		now := time.Now()
		exec.FinishedAt = &now
	})
	m.executionStore.Remove(executionID)

	m.logger.Info("agent stopped and removed from tracking",
		zap.String("execution_id", executionID),
		zap.String("task_id", execution.TaskID))

	// Publish stopped event
	m.eventPublisher.PublishAgentEvent(ctx, events.AgentStopped, execution)

	return nil
}

// StopBySessionID stops the agent for a specific session
func (m *Manager) StopBySessionID(ctx context.Context, sessionID string, force bool) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent running for session %q", sessionID)
	}

	return m.StopAgent(ctx, execution.ID, force)
}

// GetExecution returns an agent execution by ID.
//
// Returns (execution, true) if found, or (nil, false) if not found.
// The returned execution pointer should not be modified directly - use the Manager's
// methods to update execution state (MarkReady, MarkCompleted, UpdateStatus).
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func (m *Manager) GetExecution(executionID string) (*AgentExecution, bool) {
	return m.executionStore.Get(executionID)
}

// GetExecutionBySessionID returns the agent execution for a session.
//
// Returns (execution, true) if found, or (nil, false) if not found.
// A session can have at most one active execution at a time. If a session exists
// but has no active execution, this returns (nil, false).
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func (m *Manager) GetExecutionBySessionID(sessionID string) (*AgentExecution, bool) {
	return m.executionStore.GetBySessionID(sessionID)
}

// GetAvailableCommandsForSession returns the available slash commands for a session.
// Returns nil if the session doesn't exist or has no commands stored.
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func (m *Manager) GetAvailableCommandsForSession(sessionID string) []streams.AvailableCommand {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil
	}
	return execution.GetAvailableCommands()
}

// ListExecutions returns all currently tracked agent executions.
//
// Returns a snapshot of all executions in memory at the time of call. The returned slice
// contains pointers to execution objects that may be modified by other goroutines after
// this method returns. Do not modify execution state directly - use Manager methods instead.
//
// The list includes executions in all states:
//   - Starting (process launching, agentctl initializing)
//   - Running (actively processing prompts)
//   - Ready (waiting for user input)
//   - Completed/Failed (finished but not yet removed)
//
// Thread-safe: Can be called concurrently. Returns a new slice on each call.
//
// Typical usage: Status endpoints, debugging, cleanup loops.
func (m *Manager) ListExecutions() []*AgentExecution {
	return m.executionStore.List()
}

// IsAgentRunningForSession checks if an agent process is running or starting for a session.
//
// This probes agentctl's status endpoint to verify the agent process state. Returns true if:
//   - Agent status is "running" (actively processing prompts)
//   - Agent status is "starting" (process launched but not yet ready)
//
// Returns false if:
//   - No execution exists for this session
//   - agentctl client is not available
//   - Status check fails (network/timeout error)
//   - Agent is in any other state (stopped, failed, etc.)
//
// Note: The name "IsAgentRunning" is slightly misleading - it includes "starting" state.
// Use this to check if an agent subprocess exists for the session, regardless of ready state.
func (m *Manager) IsAgentRunningForSession(ctx context.Context, sessionID string) bool {
	// First check if we have an execution tracked for this session
	execution, exists := m.GetExecutionBySessionID(sessionID)
	if !exists {
		return false
	}

	// Probe agentctl status to verify the agent process is running
	if execution.agentctl == nil {
		return false
	}

	status, err := execution.agentctl.GetStatus(ctx)
	if err != nil {
		m.logger.Debug("failed to get agentctl status",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false
	}

	return status.IsAgentRunning()
}

// UpdateStatus updates the status of an execution
func (m *Manager) UpdateStatus(executionID string, status v1.AgentStatus) error {
	if err := m.executionStore.WithLock(executionID, func(execution *AgentExecution) {
		execution.Status = status
	}); err != nil {
		if errors.Is(err, ErrExecutionNotFound) {
			return fmt.Errorf("execution %q not found", executionID)
		}
		return err
	}

	m.logger.Debug("updated execution status",
		zap.String("execution_id", executionID),
		zap.String("status", string(status)))

	return nil
}

// MarkReady marks an execution as ready for follow-up prompts.
//
// This transitions the execution to the "ready" state, indicating the agent has finished
// processing the current prompt and is waiting for user input. This is called:
//   - After agent initialization completes (session loaded, workspace ready)
//   - After agent finishes processing a prompt (via stream completion event)
//   - After cancelling an agent turn (to allow new prompts)
//
// State Machine Transitions:
//
//	Starting -> Ready (after initialization)
//	Running  -> Ready (after prompt completion)
//	Any      -> Ready (after cancel)
//
// Publishes an AgentReady event to notify subscribers (frontend, orchestrator).
//
// Returns error if execution not found.
func (m *Manager) MarkReady(executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	// Skip if already ready (prevents duplicate events)
	if execution.Status == v1.AgentStatusReady {
		return nil
	}

	m.executionStore.UpdateStatus(executionID, v1.AgentStatusReady)

	m.logger.Info("execution ready for follow-up prompts",
		zap.String("execution_id", executionID))

	// Publish ready event
	m.eventPublisher.PublishAgentEvent(context.Background(), events.AgentReady, execution)

	return nil
}

// MarkCompleted marks an execution as completed or failed.
//
// This is called when the agent process terminates, either successfully or with an error.
// The final status is determined by exit code and error message:
//
//   - exitCode == 0 && errorMessage == "" → AgentStatusCompleted (success)
//   - Otherwise                            → AgentStatusFailed (failure)
//
// Parameters:
//   - executionID: The execution to mark as completed
//   - exitCode: Process exit code (0 = success, non-zero = failure)
//   - errorMessage: Human-readable error description (empty string if no error)
//
// State Machine:
//
//	This is a terminal state transition - no further state changes are expected after this.
//	Typical flow: Starting -> Running -> Ready -> ... -> Completed/Failed
//
// Publishes either AgentCompleted or AgentFailed event depending on final status.
//
// Returns error if execution not found.
func (m *Manager) MarkCompleted(executionID string, exitCode int, errorMessage string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	_ = m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
		now := time.Now()
		exec.FinishedAt = &now
		exec.ExitCode = &exitCode
		exec.ErrorMessage = errorMessage

		if exitCode == 0 && errorMessage == "" {
			exec.Status = v1.AgentStatusCompleted
		} else {
			exec.Status = v1.AgentStatusFailed
		}
	})

	m.logger.Info("execution completed",
		zap.String("execution_id", executionID),
		zap.Int("exit_code", exitCode),
		zap.String("status", string(execution.Status)))

	// Publish completion event
	eventType := events.AgentCompleted
	if execution.Status == v1.AgentStatusFailed {
		eventType = events.AgentFailed
	}
	m.eventPublisher.PublishAgentEvent(context.Background(), eventType, execution)

	return nil
}

// RespondToPermission sends a response to an agent's permission request.
//
// When an agent requests permission (e.g., to run a bash command, modify files, etc.),
// it pauses execution and waits for user approval. This method sends the user's response.
//
// Parameters:
//   - executionID: The agent execution waiting for permission
//   - pendingID: Unique ID of the permission request (from permission request event)
//   - optionID: The user-selected option ID (from the permission request's options array)
//   - cancelled: If true, indicates user cancelled/rejected the permission request.
//     When cancelled=true, optionID is ignored.
//
// Response Semantics:
//   - cancelled=false, optionID="approve" → User approved the action
//   - cancelled=false, optionID="deny"    → User explicitly denied the action
//   - cancelled=true, optionID=""         → User cancelled/closed the dialog
//
// After receiving the response, the agent will either:
//   - Continue executing (if approved)
//   - Skip the action and report failure (if denied/cancelled)
//
// Timeout: 30 seconds for agentctl to acknowledge the response.
//
// Returns error if execution not found, agentctl unavailable, or communication fails.
func (m *Manager) RespondToPermission(executionID, pendingID, optionID string, cancelled bool) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("agent execution not found: %s", executionID)
	}

	if execution.agentctl == nil {
		return fmt.Errorf("agent execution has no agentctl client: %s", executionID)
	}

	m.logger.Info("responding to permission request",
		zap.String("execution_id", executionID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return execution.agentctl.RespondToPermission(ctx, pendingID, optionID, cancelled)
}

// RespondToPermissionBySessionID sends a response to a permission request using session ID.
//
// Convenience method that looks up the execution by session ID and delegates to RespondToPermission.
// See RespondToPermission for parameter semantics and behavior.
func (m *Manager) RespondToPermissionBySessionID(sessionID, pendingID, optionID string, cancelled bool) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent execution found for session: %s", sessionID)
	}

	return m.RespondToPermission(execution.ID, pendingID, optionID, cancelled)
}

// stopAgentViaRuntime stops the agent execution via the runtime that created it.
func (m *Manager) stopAgentViaRuntime(ctx context.Context, executionID string, execution *AgentExecution, force bool) {
	if execution.RuntimeName == "" || m.runtimeRegistry == nil {
		return
	}
	rt, err := m.runtimeRegistry.GetRuntime(runtime.Name(execution.RuntimeName))
	if err != nil {
		m.logger.Warn("failed to get runtime for stopping execution",
			zap.String("execution_id", executionID),
			zap.String("runtime", execution.RuntimeName),
			zap.Error(err))
		return
	}
	m.stopPassthroughProcess(ctx, executionID, execution, rt)
	runtimeInstance := &RuntimeInstance{
		InstanceID:           execution.ID,
		TaskID:               execution.TaskID,
		ContainerID:          execution.ContainerID,
		StandaloneInstanceID: execution.standaloneInstanceID,
		StandalonePort:       execution.standalonePort,
	}
	if err := rt.StopInstance(ctx, runtimeInstance, force); err != nil {
		m.logger.Warn("failed to stop runtime instance, continuing with cleanup",
			zap.String("execution_id", executionID),
			zap.Error(err))
	}
}

// stopPassthroughProcess stops the passthrough interactive process if one is running.
func (m *Manager) stopPassthroughProcess(ctx context.Context, executionID string, execution *AgentExecution, rt Runtime) {
	if execution.PassthroughProcessID == "" {
		return
	}
	interactiveRunner := rt.GetInteractiveRunner()
	if interactiveRunner == nil {
		return
	}
	if err := interactiveRunner.Stop(ctx, execution.PassthroughProcessID); err != nil {
		m.logger.Warn("failed to stop passthrough process",
			zap.String("execution_id", executionID),
			zap.String("process_id", execution.PassthroughProcessID),
			zap.Error(err))
		return
	}
	m.logger.Info("passthrough process stopped",
		zap.String("execution_id", executionID),
		zap.String("process_id", execution.PassthroughProcessID))
}
