package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/appctx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

// StartAgentProcess configures and starts the agent subprocess for an execution.
// This must be called after Launch() to actually start the agent (e.g., auggie, codex).
// The command is built internally based on the execution's agent profile.
func (m *Manager) StartAgentProcess(ctx context.Context, executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	// Check if this execution should use passthrough mode
	if execution.AgentProfileID != "" && m.profileResolver != nil {
		profileInfo, err := m.profileResolver.ResolveProfile(ctx, execution.AgentProfileID)
		if err == nil && profileInfo.CLIPassthrough {
			// On resume (e.g., after backend restart), use the resume command path
			// so the agent receives resume flags (-c / --resume).
			if execution.isResumedSession {
				return m.ResumePassthroughSession(ctx, execution.SessionID)
			}
			return m.startPassthroughSession(ctx, execution, profileInfo)
		}
	}

	if execution.agentctl == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}

	// Check if we're reconnecting to an existing running agent process.
	// When the existing process is still alive inside a remote executor (e.g., Sprites),
	// we skip subprocess launch and go directly to ACP session initialization.
	reuseExisting, _ := execution.Metadata["reuse_existing_process"].(bool)

	if !reuseExisting && execution.AgentCommand == "" {
		return fmt.Errorf("execution %q has no agent command configured", executionID)
	}

	// Wait for agentctl to be ready
	if err := execution.agentctl.WaitForReady(ctx, 60*time.Second); err != nil {
		m.updateExecutionError(executionID, "agentctl not ready: "+err.Error())
		return fmt.Errorf("agentctl not ready: %w", err)
	}

	taskDescription := getTaskDescriptionFromMetadata(execution)
	approvalPolicy, agentDisplayName := m.resolveApprovalPolicyAndDisplayName(ctx, execution)

	var bootCommand string
	if reuseExisting {
		// Agent subprocess is already running inside the remote executor.
		// Skip configureAndStartAgent (which would spawn a conflicting subprocess)
		// and go directly to ACP session initialization.
		bootCommand = "reconnecting to running agent"
		m.logger.Info("reusing existing agent process, skipping subprocess launch",
			zap.String("execution_id", executionID),
			zap.String("task_id", execution.TaskID))
	} else {
		m.logger.Info("StartAgentProcess: starting subprocess",
			zap.String("execution_id", executionID),
			zap.String("task_id", execution.TaskID),
			zap.String("agent_command", execution.AgentCommand),
			zap.String("acp_session_id", execution.ACPSessionID))

		var err error
		bootCommand, err = m.configureAndStartAgent(ctx, execution, taskDescription, approvalPolicy)
		if err != nil {
			return err
		}

		m.logger.Info("agent process started",
			zap.String("execution_id", executionID),
			zap.String("task_id", execution.TaskID),
			zap.String("command", bootCommand))
	}

	return m.initializeAgentSession(ctx, execution, bootCommand, agentDisplayName, taskDescription)
}

// pollAgentStderr polls the agent's stderr buffer every 2 seconds and updates the boot message.
func (m *Manager) pollAgentStderr(execution *AgentExecution, client *agentctl.Client, msg *models.Message, stopCh chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastLineCount int

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			baseCtx := execution.SessionTraceContext()
			ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
			lines, err := client.GetAgentStderr(ctx)
			cancel()
			if err != nil {
				m.logger.Debug("failed to poll agent stderr", zap.Error(err))
				continue
			}

			if len(lines) > lastLineCount {
				lastLineCount = len(lines)
				msg.Content = strings.Join(lines, "\n")
				ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
				if updateErr := m.bootMessageService.UpdateMessage(ctx2, msg); updateErr != nil {
					m.logger.Debug("failed to update boot message with stderr",
						zap.String("message_id", msg.ID),
						zap.Error(updateErr))
				}
				cancel2()
			}
		}
	}
}

// finalizeBootMessage stops the polling goroutine and updates the boot message with final status.
func (m *Manager) finalizeBootMessage(execution *AgentExecution, msg *models.Message, stopCh chan struct{}, client *agentctl.Client, status string) {
	if msg == nil || m.bootMessageService == nil {
		return
	}

	// Stop the polling goroutine
	if stopCh != nil {
		close(stopCh)
	}

	// Final stderr fetch
	if client != nil && execution != nil {
		baseCtx := execution.SessionTraceContext()
		ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
		lines, err := client.GetAgentStderr(ctx)
		cancel()
		if err == nil && len(lines) > 0 {
			msg.Content = strings.Join(lines, "\n")
		}
	}

	msg.Metadata["status"] = status
	msg.Metadata["completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	if status == containerStateExited {
		msg.Metadata["exit_code"] = 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if updateErr := m.bootMessageService.UpdateMessage(ctx, msg); updateErr != nil {
		m.logger.Warn("failed to update boot message with final status",
			zap.String("message_id", msg.ID),
			zap.Error(updateErr))
	}
}

// buildEnvForExecution builds environment variables for any runtime.
// This is the unified method used by the runtime interface.
func (m *Manager) buildEnvForExecution(executionID string, req *LaunchRequest, agentConfig agents.Agent) map[string]string {
	env := make(map[string]string)

	// Copy request environment
	for k, v := range req.Env {
		env[k] = v
	}

	// Add standard variables for recovery after backend restart
	env["KANDEV_INSTANCE_ID"] = executionID
	env["KANDEV_TASK_ID"] = req.TaskID
	env["KANDEV_SESSION_ID"] = req.SessionID
	env["KANDEV_AGENT_PROFILE_ID"] = req.AgentProfileID
	env["TASK_DESCRIPTION"] = req.TaskDescription

	// Add agent runtime default env vars (e.g., MCP_TIMEOUT for Claude Code)
	if agentConfig != nil {
		if rt := agentConfig.Runtime(); rt != nil {
			for k, v := range rt.Env {
				if _, exists := env[k]; !exists {
					env[k] = v
				}
			}
		}
	}

	// Add required credentials from agent config
	if m.credsMgr != nil && agentConfig != nil {
		ctx := context.Background()
		for _, credKey := range agentConfig.Runtime().RequiredEnv {
			if value, err := m.credsMgr.GetCredentialValue(ctx, credKey); err == nil && value != "" {
				env[credKey] = value
			}
		}
	}

	return env
}

// waitForAgentctlReady waits for the agentctl HTTP server to be ready.
// This enables shell/workspace features without starting the agent process.
func (m *Manager) waitForAgentctlReady(execution *AgentExecution) {
	opStart := time.Now()
	// Use detached context that respects stopCh for graceful shutdown
	ctx, cancel := appctx.Detached(context.Background(), m.stopCh, 60*time.Second)
	defer cancel()

	m.logger.Debug("waiting for agentctl to be ready",
		zap.String("execution_id", execution.ID),
		zap.String("url", execution.agentctl.BaseURL()))

	if err := execution.agentctl.WaitForReady(ctx, 60*time.Second); err != nil {
		m.logger.Error("agentctl not ready",
			zap.String("execution_id", execution.ID),
			zap.Duration("duration", time.Since(opStart)),
			zap.Error(err))
		m.updateExecutionError(execution.ID, "agentctl not ready: "+err.Error())
		// Use the timeout context for event publishing instead of a fresh Background context
		m.eventPublisher.PublishAgentctlEvent(ctx, events.AgentctlError, execution, err.Error())
		return
	}

	elapsed := time.Since(opStart)
	if elapsed > 10*time.Second {
		m.logger.Warn("agentctl ready took longer than expected",
			zap.String("execution_id", execution.ID),
			zap.Duration("duration", elapsed))
	} else {
		m.logger.Debug("agentctl ready - shell/workspace access available",
			zap.String("execution_id", execution.ID),
			zap.Duration("duration", elapsed))
	}
	// Flush any session mode the gateway cached before this execution was
	// ready (pre-execution-focus race). Without this, agentctl stays in its
	// default slow poll mode even though the frontend already sent focus,
	// and git state updates take up to 30s to reach the UI.
	m.flushCachedPollMode(execution.SessionID)
	// Use the timeout context for event publishing instead of a fresh Background context
	m.eventPublisher.PublishAgentctlEvent(ctx, events.AgentctlReady, execution, "")
}
