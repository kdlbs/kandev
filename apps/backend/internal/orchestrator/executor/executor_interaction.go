package executor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// Stop stops an active execution by session ID
func (e *Executor) Stop(ctx context.Context, sessionID string, reason string, force bool) error {
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return ErrExecutionNotFound
	}
	return e.stopWithSession(ctx, session, reason, force)
}

// stopWithSession stops an active execution using an already-loaded session.
// It marks the session CANCELLED in the database immediately so the WebSocket
// caller can return quickly, then terminates the agent process asynchronously
// to avoid blocking the caller on agentctl's blocking stop HTTP call.
func (e *Executor) stopWithSession(ctx context.Context, session *models.TaskSession, reason string, force bool) error {
	// Look up the live execution for this session via the lifecycle manager —
	// the in-memory store is the single source of truth post-refactor.
	executionID, err := e.agentManager.GetExecutionIDForSession(ctx, session.ID)
	if err != nil || executionID == "" {
		return ErrExecutionNotFound
	}

	e.logger.Info("stopping execution",
		zap.String("task_id", session.TaskID),
		zap.String("session_id", session.ID),
		zap.String("agent_execution_id", executionID),
		zap.String("reason", reason),
		zap.Bool("force", force))

	// Mark the session CANCELLED and publish a WS event immediately so the
	// frontend updates without waiting for the agent process to exit.
	if dbErr := e.updateSessionState(ctx, session.TaskID, session.ID, models.TaskSessionStateCancelled, reason); dbErr != nil {
		e.logger.Error("failed to update agent session status",
			zap.String("session_id", session.ID),
			zap.Error(dbErr))
	}

	// Terminate the agent process in the background — agentctl's stop endpoint
	// blocks until the process exits, which can exceed the WS request timeout.
	stopCtx := context.WithoutCancel(ctx)
	go func() {
		if err := e.agentManager.StopAgentWithReason(stopCtx, executionID, reason, force); err != nil {
			// Log the error; the agent instance may already be gone
			e.logger.Warn("failed to stop agent (may already be stopped)",
				zap.String("session_id", session.ID),
				zap.Error(err))
		}
	}()

	return nil
}

// StopExecution stops a running execution by execution ID.
func (e *Executor) StopExecution(ctx context.Context, executionID string, reason string, force bool) error {
	if executionID == "" {
		return ErrExecutionNotFound
	}
	e.logger.Info("stopping execution by execution id",
		zap.String("agent_execution_id", executionID),
		zap.String("reason", reason),
		zap.Bool("force", force))
	if err := e.agentManager.StopAgentWithReason(ctx, executionID, reason, force); err != nil {
		e.logger.Warn("failed to stop agent by execution id",
			zap.String("agent_execution_id", executionID),
			zap.Error(err))
		return ErrExecutionNotFound
	}
	return nil
}

// StopByTaskID stops all active executions for a task
func (e *Executor) StopByTaskID(ctx context.Context, taskID string, reason string, force bool) error {
	// Get all active sessions for this task from database
	sessions, err := e.repo.ListActiveTaskSessionsByTaskID(ctx, taskID)
	if err != nil {
		e.logger.Warn("failed to list active sessions for task",
			zap.String("task_id", taskID),
			zap.Error(err))
		return ErrExecutionNotFound
	}

	if len(sessions) == 0 {
		return ErrExecutionNotFound
	}

	var lastErr error
	stoppedCount := 0
	for _, session := range sessions {
		if err := e.stopWithSession(ctx, session, reason, force); err != nil {
			e.logger.Warn("failed to stop session",
				zap.String("task_id", taskID),
				zap.String("session_id", session.ID),
				zap.Error(err))
			lastErr = err
		} else {
			stoppedCount++
		}
	}

	if stoppedCount == 0 && lastErr != nil {
		return lastErr
	}

	return nil
}

// Prompt sends a follow-up prompt to a running agent for a task
// Returns PromptResult indicating if the agent needs input
// Attachments (images) are passed to the agent if provided
func (e *Executor) Prompt(ctx context.Context, taskID, sessionID string, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, preloadedSession ...*models.TaskSession) (*PromptResult, error) {
	var session *models.TaskSession
	if len(preloadedSession) > 0 && preloadedSession[0] != nil {
		session = preloadedSession[0]
	} else {
		var err error
		session, err = e.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return nil, ErrExecutionNotFound
		}
	}
	if session.TaskID != taskID {
		return nil, ErrExecutionNotFound
	}
	executionID, err := e.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if err != nil || executionID == "" {
		return nil, ErrExecutionNotFound
	}

	e.logger.Debug("sending prompt to agent",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("agent_execution_id", executionID),
		zap.Int("prompt_length", len(prompt)),
		zap.Int("attachments_count", len(attachments)),
		zap.Bool("dispatch_only", dispatchOnly))

	result, err := e.agentManager.PromptAgent(ctx, executionID, prompt, attachments, dispatchOnly)
	if err != nil {
		if errors.Is(err, lifecycle.ErrExecutionNotFound) {
			return nil, ErrExecutionNotFound
		}
		return nil, err
	}
	return result, nil
}

// SwitchModel switches the model for a running session. It first attempts an in-place switch
// via ACP session/set_model (instant, no process restart). If the agent doesn't support
// in-place switching, it falls back to stopping and restarting the agent with the new model.
func (e *Executor) SwitchModel(ctx context.Context, taskID, sessionID, newModel, prompt string) (*PromptResult, error) {
	e.logger.Info("switching model for session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("new_model", newModel))

	// Try in-place model switch first (ACP agents support session/set_model)
	if err := e.agentManager.SetSessionModelBySessionID(ctx, sessionID, newModel); err == nil {
		e.logger.Info("model switched in-place via ACP session/set_model",
			zap.String("session_id", sessionID),
			zap.String("new_model", newModel))
		e.persistInPlaceModelSwitch(ctx, sessionID, newModel)
		return &PromptResult{StopReason: "model_switched_in_place"}, nil
	}

	e.logger.Debug("in-place model switch not available, falling back to agent restart",
		zap.String("session_id", sessionID))

	session, task, acpSessionID, existingRunning, err := e.prepareModelSwitch(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}

	execConfig := e.resolveExecutorConfig(ctx, session.ExecutorID, task.WorkspaceID, nil)

	req, err := e.buildSwitchModelRequest(ctx, task, session, sessionID, newModel, prompt, acpSessionID, execConfig, existingRunning)
	if err != nil {
		return nil, err
	}

	req.Env = e.applyPreferredShellEnv(ctx, req.ExecutorType, req.Env)

	e.logger.Info("launching new agent with model override",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
		zap.String("model", newModel),
		zap.String("executor_type", req.ExecutorType),
		zap.String("acp_session_id", acpSessionID),
		zap.Bool("use_worktree", req.UseWorktree),
		zap.String("repository_path", req.RepositoryPath))

	if err := e.launchModelSwitchAgent(ctx, task.ID, sessionID, newModel, session, req, existingRunning); err != nil {
		return nil, err
	}

	// The agent initialization and prompt are handled as part of StartAgentProcess
	// Return success - the actual prompt response will come via ACP events
	return &PromptResult{
		StopReason:   "model_switched",
		AgentMessage: "",
	}, nil
}

// prepareModelSwitch validates the session/task and stops the current agent.
// Returns the session, task, ACP session ID, existing ExecutorRunning record, and any error.
func (e *Executor) prepareModelSwitch(ctx context.Context, taskID, sessionID string) (*models.TaskSession, *models.Task, string, *models.ExecutorRunning, error) {
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("failed to get session: %w", err)
	}
	if session.TaskID != taskID {
		return nil, nil, "", nil, fmt.Errorf("session %s does not belong to task %s", sessionID, taskID)
	}
	executionID, err := e.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if err != nil || executionID == "" {
		return nil, nil, "", nil, ErrExecutionNotFound
	}

	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("failed to get task: %w", err)
	}

	var acpSessionID string
	var existingRunning *models.ExecutorRunning
	if running, runErr := e.repo.GetExecutorRunningBySessionID(ctx, sessionID); runErr == nil && running != nil {
		existingRunning = running
		acpSessionID = running.ResumeToken
	}

	e.logger.Info("stopping current agent for model switch",
		zap.String("agent_execution_id", executionID))
	if err := e.agentManager.StopAgent(ctx, executionID, false); err != nil {
		e.logger.Warn("failed to stop agent for model switch, continuing anyway",
			zap.Error(err),
			zap.String("agent_execution_id", executionID))
	}

	return session, task, acpSessionID, existingRunning, nil
}

// launchModelSwitchAgent launches the new agent, persists state, and starts the process.
func (e *Executor) launchModelSwitchAgent(ctx context.Context, taskID, sessionID, newModel string, session *models.TaskSession, req *LaunchAgentRequest, existingRunning *models.ExecutorRunning) error {
	resp, err := e.agentManager.LaunchAgent(ctx, req)
	if err != nil {
		e.logger.Error("failed to launch agent with new model",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return fmt.Errorf("failed to launch agent with new model: %w", err)
	}

	e.persistModelSwitchState(ctx, taskID, sessionID, session, newModel)

	if err := e.agentManager.StartAgentProcess(ctx, resp.AgentExecutionID); err != nil {
		e.logger.Error("failed to start agent process after model switch",
			zap.String("task_id", taskID),
			zap.String("agent_execution_id", resp.AgentExecutionID),
			zap.Error(err))
		return fmt.Errorf("failed to start agent after model switch: %w", err)
	}

	e.logger.Info("model switch complete, agent started",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("new_model", newModel),
		zap.String("agent_execution_id", resp.AgentExecutionID))

	return nil
}

// buildSwitchModelRequest constructs a LaunchAgentRequest for a model switch, applying
// repository and worktree config from the existing session.
func (e *Executor) buildSwitchModelRequest(ctx context.Context, task *models.Task, session *models.TaskSession, sessionID, newModel, prompt, acpSessionID string, execConfig executorConfig, running *models.ExecutorRunning) (*LaunchAgentRequest, error) {
	req := &LaunchAgentRequest{
		TaskID:            task.ID,
		SessionID:         sessionID,
		TaskTitle:         task.Title,
		AgentProfileID:    session.AgentProfileID,
		TaskDescription:   prompt,
		ModelOverride:     newModel,
		ACPSessionID:      acpSessionID,
		ExecutorType:      execConfig.ExecutorType,
		Metadata:          execConfig.Metadata,
		IsEphemeral:       task.IsEphemeral,
		TaskEnvironmentID: session.TaskEnvironmentID,
	}

	// Activate config-mode MCP tools when config_mode is set in session metadata.
	if isConfigModeSession(session) {
		req.McpMode = McpModeConfig
	}

	repositoryPath, err := e.applyRepositoryToSwitchRequest(ctx, req, session, execConfig)
	if err != nil {
		return nil, err
	}
	e.applyWorktreeToSwitchRequest(req, session, execConfig, repositoryPath)

	// Override repository URL with the running worktree path if available
	if running != nil && running.WorktreePath != "" {
		req.RepositoryURL = running.WorktreePath
	}

	return req, nil
}

// applyRepositoryToSwitchRequest resolves the repository for a model switch and sets
// the URL and branch on the request. Returns the local repository path.
func (e *Executor) applyRepositoryToSwitchRequest(ctx context.Context, req *LaunchAgentRequest, session *models.TaskSession, execConfig executorConfig) (string, error) {
	if session.RepositoryID == "" {
		return "", nil
	}
	repository, repoErr := e.repo.GetRepository(ctx, session.RepositoryID)
	if repoErr != nil || repository == nil {
		return "", nil
	}
	req.RepositoryURL = repository.LocalPath
	req.Branch = session.BaseBranch
	if e.capabilities != nil && e.capabilities.RequiresCloneURL(execConfig.ExecutorType) {
		cloneURL := repositoryCloneURL(repository)
		if cloneURL == "" {
			return "", ErrNoCloneURL
		}
		req.RepositoryURL = cloneURL
		if req.Metadata == nil {
			req.Metadata = make(map[string]interface{})
		}
		req.Metadata["repository_clone_url"] = cloneURL
	}
	return repository.LocalPath, nil
}

// applyWorktreeToSwitchRequest configures worktree fields on the request when applicable.
func (e *Executor) applyWorktreeToSwitchRequest(req *LaunchAgentRequest, session *models.TaskSession, execConfig executorConfig, repositoryPath string) {
	if !shouldUseWorktree(execConfig.ExecutorType) || repositoryPath == "" {
		return
	}
	req.UseWorktree = true
	req.RepositoryPath = repositoryPath
	req.RepositoryID = session.RepositoryID
	if session.BaseBranch != "" {
		req.BaseBranch = session.BaseBranch
	} else {
		req.BaseBranch = defaultBaseBranch
	}
	if len(session.Worktrees) > 0 && session.Worktrees[0].WorktreeID != "" {
		if req.Metadata == nil {
			req.Metadata = make(map[string]interface{})
		}
		req.Metadata["worktree_id"] = session.Worktrees[0].WorktreeID
	}
}

// persistModelSwitchState updates the session row's model metadata and state
// after a model switch launch. The executors_running row's agent_execution_id /
// container_id / status are written by the lifecycle manager during the launch
// itself (lifecycle.persistExecutorRunning) and not touched here.
func (e *Executor) persistModelSwitchState(ctx context.Context, taskID, sessionID string, session *models.TaskSession, newModel string) {
	session.State = models.TaskSessionStateStarting
	session.UpdatedAt = time.Now().UTC()

	if session.AgentProfileSnapshot == nil {
		session.AgentProfileSnapshot = make(map[string]interface{})
	}
	session.AgentProfileSnapshot["model"] = newModel

	if err := e.repo.UpdateTaskSession(ctx, session); err != nil {
		e.logger.Error("failed to update session after model switch",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// persistInPlaceModelSwitch updates the session snapshot model after a successful
// in-place model switch (no agent restart). Only the model field changes.
func (e *Executor) persistInPlaceModelSwitch(ctx context.Context, sessionID, newModel string) {
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		e.logger.Warn("failed to get session for in-place model switch persistence",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	if session.AgentProfileSnapshot == nil {
		session.AgentProfileSnapshot = make(map[string]interface{})
	}
	session.AgentProfileSnapshot["model"] = newModel
	if err := e.repo.UpdateTaskSession(ctx, session); err != nil {
		e.logger.Warn("failed to persist in-place model switch",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// RespondToPermission sends a response to a permission request for a session
func (e *Executor) RespondToPermission(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error {
	e.logger.Debug("responding to permission request",
		zap.String("session_id", sessionID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))

	return e.agentManager.RespondToPermissionBySessionID(ctx, sessionID, pendingID, optionID, cancelled)
}
