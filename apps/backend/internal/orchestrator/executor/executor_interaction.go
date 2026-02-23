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
	if session.AgentExecutionID == "" {
		return ErrExecutionNotFound
	}

	e.logger.Info("stopping execution",
		zap.String("task_id", session.TaskID),
		zap.String("session_id", sessionID),
		zap.String("agent_execution_id", session.AgentExecutionID),
		zap.String("reason", reason),
		zap.Bool("force", force))

	err = e.agentManager.StopAgentWithReason(ctx, session.AgentExecutionID, reason, force)
	if err != nil {
		// Log the error but continue to clean up execution state
		// The agent instance may already be gone (container stopped externally)
		e.logger.Warn("failed to stop agent (may already be stopped)",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	// Update database
	if dbErr := e.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateCancelled, reason); dbErr != nil {
		e.logger.Error("failed to update agent session status in database",
			zap.String("session_id", sessionID),
			zap.Error(dbErr))
	}

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
		if err := e.Stop(ctx, session.ID, reason, force); err != nil {
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
func (e *Executor) Prompt(ctx context.Context, taskID, sessionID string, prompt string, attachments []v1.MessageAttachment) (*PromptResult, error) {
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, ErrExecutionNotFound
	}
	if session.TaskID != taskID {
		return nil, ErrExecutionNotFound
	}
	if session.AgentExecutionID == "" {
		return nil, ErrExecutionNotFound
	}

	e.logger.Debug("sending prompt to agent",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("agent_execution_id", session.AgentExecutionID),
		zap.Int("prompt_length", len(prompt)),
		zap.Int("attachments_count", len(attachments)))

	result, err := e.agentManager.PromptAgent(ctx, session.AgentExecutionID, prompt, attachments)
	if err != nil {
		if errors.Is(err, lifecycle.ErrExecutionNotFound) {
			return nil, ErrExecutionNotFound
		}
		return nil, err
	}
	return result, nil
}

// SwitchModel stops the current agent, restarts it with a new model, and sends the prompt.
// For agents that support session resume (can_recover: true), it attempts to resume context.
// For agents that don't support resume (can_recover: false), a fresh session is started.
func (e *Executor) SwitchModel(ctx context.Context, taskID, sessionID, newModel, prompt string) (*PromptResult, error) {
	e.logger.Info("switching model for session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("new_model", newModel))

	session, task, acpSessionID, err := e.prepareModelSwitch(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}

	execConfig := e.resolveExecutorConfig(ctx, session.ExecutorID, task.WorkspaceID, nil)

	req, err := e.buildSwitchModelRequest(ctx, task, session, sessionID, newModel, prompt, acpSessionID, execConfig)
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

	if err := e.launchModelSwitchAgent(ctx, task.ID, sessionID, newModel, session, req); err != nil {
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
// Returns the session, task, ACP session ID, and any error.
func (e *Executor) prepareModelSwitch(ctx context.Context, taskID, sessionID string) (*models.TaskSession, *models.Task, string, error) {
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to get session: %w", err)
	}
	if session.TaskID != taskID {
		return nil, nil, "", fmt.Errorf("session %s does not belong to task %s", sessionID, taskID)
	}
	if session.AgentExecutionID == "" {
		return nil, nil, "", ErrExecutionNotFound
	}

	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to get task: %w", err)
	}

	var acpSessionID string
	if running, runErr := e.repo.GetExecutorRunningBySessionID(ctx, sessionID); runErr == nil && running != nil {
		acpSessionID = running.ResumeToken
	}

	e.logger.Info("stopping current agent for model switch",
		zap.String("agent_execution_id", session.AgentExecutionID))
	if err := e.agentManager.StopAgent(ctx, session.AgentExecutionID, false); err != nil {
		e.logger.Warn("failed to stop agent for model switch, continuing anyway",
			zap.Error(err),
			zap.String("agent_execution_id", session.AgentExecutionID))
	}

	return session, task, acpSessionID, nil
}

// launchModelSwitchAgent launches the new agent, persists state, and starts the process.
func (e *Executor) launchModelSwitchAgent(ctx context.Context, taskID, sessionID, newModel string, session *models.TaskSession, req *LaunchAgentRequest) error {
	resp, err := e.agentManager.LaunchAgent(ctx, req)
	if err != nil {
		e.logger.Error("failed to launch agent with new model",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return fmt.Errorf("failed to launch agent with new model: %w", err)
	}

	e.persistModelSwitchState(ctx, taskID, sessionID, session, resp, newModel)

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
func (e *Executor) buildSwitchModelRequest(ctx context.Context, task *models.Task, session *models.TaskSession, sessionID, newModel, prompt, acpSessionID string, execConfig executorConfig) (*LaunchAgentRequest, error) {
	req := &LaunchAgentRequest{
		TaskID:          task.ID,
		SessionID:       sessionID,
		TaskTitle:       task.Title,
		AgentProfileID:  session.AgentProfileID,
		TaskDescription: prompt,
		ModelOverride:   newModel,
		ACPSessionID:    acpSessionID,
		ExecutorType:    execConfig.ExecutorType,
		Metadata:        execConfig.Metadata,
	}

	repositoryPath, err := e.applyRepositoryToSwitchRequest(ctx, req, session, execConfig)
	if err != nil {
		return nil, err
	}
	e.applyWorktreeToSwitchRequest(req, session, execConfig, repositoryPath)

	// Override repository URL with the running worktree path if available
	if running, err := e.repo.GetExecutorRunningBySessionID(ctx, sessionID); err == nil && running != nil {
		if running.WorktreePath != "" {
			req.RepositoryURL = running.WorktreePath
		}
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
	if models.ExecutorType(execConfig.ExecutorType) == models.ExecutorTypeRemoteDocker {
		cloneURL := repositoryCloneURL(repository)
		if cloneURL == "" {
			return "", ErrRemoteDockerNoRepoURL
		}
		req.RepositoryURL = cloneURL
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

// persistModelSwitchState updates session and executor running records after a model switch launch.
func (e *Executor) persistModelSwitchState(ctx context.Context, taskID, sessionID string, session *models.TaskSession, resp *LaunchAgentResponse, newModel string) {
	session.AgentExecutionID = resp.AgentExecutionID
	session.ContainerID = resp.ContainerID
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

	if running, err := e.repo.GetExecutorRunningBySessionID(ctx, sessionID); err == nil && running != nil {
		running.AgentExecutionID = resp.AgentExecutionID
		running.ContainerID = resp.ContainerID
		running.Status = "starting"
		if err := e.repo.UpsertExecutorRunning(ctx, running); err != nil {
			e.logger.Warn("failed to update executor running after model switch",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
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
