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
	"github.com/kandev/kandev/internal/worktree"
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
			return m.startPassthroughSession(ctx, execution, profileInfo)
		}
	}

	if execution.agentctl == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}
	if execution.AgentCommand == "" {
		return fmt.Errorf("execution %q has no agent command configured", executionID)
	}

	// Wait for agentctl to be ready
	if err := execution.agentctl.WaitForReady(ctx, 60*time.Second); err != nil {
		m.updateExecutionError(executionID, "agentctl not ready: "+err.Error())
		return fmt.Errorf("agentctl not ready: %w", err)
	}

	taskDescription := getTaskDescriptionFromMetadata(execution)

	m.logger.Warn("StartAgentProcess: task description resolved",
		zap.String("execution_id", executionID),
		zap.String("task_id", execution.TaskID),
		zap.Int("task_description_length", len(taskDescription)),
		zap.String("agent_command", execution.AgentCommand),
		zap.String("acp_session_id", execution.ACPSessionID))

	approvalPolicy, agentDisplayName := m.resolveApprovalPolicyAndDisplayName(ctx, execution)

	bootCommand, err := m.configureAndStartAgent(ctx, execution, taskDescription, approvalPolicy)
	if err != nil {
		return err
	}

	m.logger.Info("agent process started",
		zap.String("execution_id", executionID),
		zap.String("task_id", execution.TaskID),
		zap.String("command", bootCommand))

	return m.initializeAgentSession(ctx, execution, bootCommand, agentDisplayName, taskDescription)
}

// pollAgentStderr polls the agent's stderr buffer every 2 seconds and updates the boot message.
func (m *Manager) pollAgentStderr(client *agentctl.Client, msg *models.Message, stopCh chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastLineCount int

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
func (m *Manager) finalizeBootMessage(msg *models.Message, stopCh chan struct{}, client *agentctl.Client, status string) {
	if msg == nil || m.bootMessageService == nil {
		return
	}

	// Stop the polling goroutine
	if stopCh != nil {
		close(stopCh)
	}

	// Final stderr fetch
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

// buildEnvForRuntime builds environment variables for any runtime.
// This is the unified method used by the runtime interface.
func (m *Manager) buildEnvForRuntime(executionID string, req *LaunchRequest, agentConfig agents.Agent) map[string]string {
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

// getOrCreateWorktree creates a new worktree or reuses an existing one for session resumption.
// If worktree_id is in metadata, it tries to reuse that specific worktree.
// Otherwise, creates a new worktree with a unique random suffix.
func (m *Manager) getOrCreateWorktree(ctx context.Context, req *LaunchRequest) (*worktree.Worktree, error) {
	// Check if we have a worktree_id in metadata for session resumption
	var worktreeID string
	if req.Metadata != nil {
		if id, ok := req.Metadata["worktree_id"].(string); ok && id != "" {
			worktreeID = id
		}
	}

	// Create request with optional WorktreeID for resumption
	createReq := worktree.CreateRequest{
		TaskID:               req.TaskID,
		SessionID:            req.SessionID,
		TaskTitle:            req.TaskTitle,
		RepositoryID:         req.RepositoryID,
		RepositoryPath:       req.RepositoryPath,
		BaseBranch:           req.BaseBranch,
		WorktreeBranchPrefix: req.WorktreeBranchPrefix,
		PullBeforeWorktree:   req.PullBeforeWorktree,
		WorktreeID:           worktreeID, // If set, will try to reuse this worktree
	}

	wt, err := m.worktreeMgr.Create(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	if worktreeID != "" && wt.ID == worktreeID {
		m.logger.Debug("reusing existing worktree for session resumption",
			zap.String("task_id", req.TaskID),
			zap.String("worktree_id", wt.ID),
			zap.String("worktree_path", wt.Path),
			zap.String("branch", wt.Branch))
	} else {
		m.logger.Info("created new worktree for task",
			zap.String("task_id", req.TaskID),
			zap.String("worktree_id", wt.ID),
			zap.String("worktree_path", wt.Path),
			zap.String("branch", wt.Branch))
	}

	return wt, nil
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
	// Use the timeout context for event publishing instead of a fresh Background context
	m.eventPublisher.PublishAgentctlEvent(ctx, events.AgentctlReady, execution, "")
}
