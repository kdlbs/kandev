// Package orchestrator provides the main orchestrator service that ties all components together.
package orchestrator

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/dto"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// PromptResult contains the result of a prompt operation
type PromptResult struct {
	StopReason   string // The reason the agent stopped (e.g., "end_turn")
	AgentMessage string // The agent's accumulated response message
}

func validateSessionWorktrees(session *models.TaskSession) error {
	for _, wt := range session.Worktrees {
		if wt.WorktreePath == "" {
			continue
		}
		if _, err := os.Stat(wt.WorktreePath); err != nil {
			return fmt.Errorf("worktree path not found: %w", err)
		}
	}
	return nil
}

// EnqueueTask manually adds a task to the queue
func (s *Service) EnqueueTask(ctx context.Context, task *v1.Task) error {
	s.logger.Debug("manually enqueueing task",
		zap.String("task_id", task.ID),
		zap.String("title", task.Title))
	return s.scheduler.EnqueueTask(task)
}

// StartTask manually starts agent execution for a task
func (s *Service) StartTask(ctx context.Context, taskID string, agentProfileID string, priority int, prompt string) (*executor.TaskExecution, error) {
	s.logger.Debug("manually starting task",
		zap.String("task_id", taskID),
		zap.String("agent_profile_id", agentProfileID),
		zap.Int("priority", priority),
		zap.Int("prompt_length", len(prompt)))

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateScheduling); err != nil {
		s.logger.Warn("failed to update task state to SCHEDULING",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	// Fetch the task from the repository to get complete task info
	task, err := s.scheduler.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to fetch task for manual start",
			zap.String("task_id", taskID),
			zap.Error(err))
		return nil, err
	}

	// Override priority if provided in the request
	if priority > 0 {
		task.Priority = priority
	}

	// Use provided prompt, fall back to task description
	effectivePrompt := prompt
	if effectivePrompt == "" {
		effectivePrompt = task.Description
	}

	execution, err := s.executor.ExecuteWithProfile(ctx, task, agentProfileID, effectivePrompt)
	if err != nil {
		return nil, err
	}

	if execution.SessionID != "" {
		s.updateTaskSessionState(ctx, taskID, execution.SessionID, models.TaskSessionStateRunning, "", true)
		// Create user message for the initial prompt
		if s.messageCreator != nil && effectivePrompt != "" {
			if err := s.messageCreator.CreateUserMessage(ctx, taskID, effectivePrompt, execution.SessionID, s.getActiveTurnID(execution.SessionID)); err != nil {
				s.logger.Error("failed to create initial user message",
					zap.String("task_id", taskID),
					zap.Error(err))
			}
		}
	}

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateInProgress); err != nil {
		s.logger.Warn("failed to update task state to IN_PROGRESS",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	return execution, nil
}

// ResumeTaskSession restarts a specific task session using its stored worktree.
func (s *Service) ResumeTaskSession(ctx context.Context, taskID, sessionID string) (*executor.TaskExecution, error) {
	s.logger.Debug("resuming task session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.TaskID != taskID {
		return nil, fmt.Errorf("task session does not belong to task")
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || running == nil || running.ResumeToken == "" || !running.Resumable {
		return nil, fmt.Errorf("session is not resumable")
	}
	if err := validateSessionWorktrees(session); err != nil {
		return nil, err
	}

	execution, err := s.executor.ResumeSession(ctx, session, true)
	if err != nil {
		_ = s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateFailed, err.Error())
		return nil, err
	}
	// Preserve persisted task/session state; resume should not mutate state/columns.
	execution.SessionState = v1.TaskSessionState(session.State)

	s.logger.Debug("task session resumed and ready for input",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))

	return execution, nil
}

// GetTaskSessionStatus returns the status of a task session including whether it's resumable
func (s *Service) GetTaskSessionStatus(ctx context.Context, taskID, sessionID string) (dto.TaskSessionStatusResponse, error) {
	s.logger.Debug("checking task session status",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))

	resp := dto.TaskSessionStatusResponse{
		SessionID: sessionID,
		TaskID:    taskID,
	}

	// 1. Load session from database
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		resp.Error = "session not found"
		return resp, nil
	}

	if session.TaskID != taskID {
		resp.Error = "session does not belong to task"
		return resp, nil
	}

	resp.State = string(session.State)
	resp.AgentProfileID = session.AgentProfileID

	// Extract resume token from executor runtime state.
	var resumeToken string
	if running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID); err == nil && running != nil {
		resumeToken = running.ResumeToken
		resp.ACPSessionID = resumeToken
		if running.Resumable {
			resp.IsResumable = true
		}
	}

	// Extract worktree info
	if len(session.Worktrees) > 0 {
		wt := session.Worktrees[0]
		if wt.WorktreePath != "" {
			resp.WorktreePath = &wt.WorktreePath
		}
		if wt.WorktreeBranch != "" {
			resp.WorktreeBranch = &wt.WorktreeBranch
		}
	}

	// 2. Check if this session's agent is running
	if exec, ok := s.executor.GetExecutionBySession(sessionID); ok && exec != nil {
		resp.IsAgentRunning = true
		resp.NeedsResume = false
		return resp, nil
	}

	// 3. Session can be resumed if it has a resume token
	if resumeToken == "" {
		resp.IsAgentRunning = false
		resp.IsResumable = false
		resp.NeedsResume = false
		return resp, nil
	}

	// 4. Additional validations for resumption
	if session.AgentProfileID == "" {
		resp.Error = "session missing agent profile"
		resp.IsResumable = false
		return resp, nil
	}

	// Check if worktree exists (if one was used)
	if len(session.Worktrees) > 0 && session.Worktrees[0].WorktreePath != "" {
		if _, err := os.Stat(session.Worktrees[0].WorktreePath); err != nil {
			resp.Error = "worktree not found"
			resp.IsResumable = false
			return resp, nil
		}
	}

	resp.IsAgentRunning = false
	resp.NeedsResume = true
	resp.ResumeReason = "agent_not_running"

	return resp, nil
}

// StopTask stops agent execution for a task (stops all active sessions for the task)
func (s *Service) StopTask(ctx context.Context, taskID string, reason string, force bool) error {
	s.logger.Info("stopping task execution",
		zap.String("task_id", taskID),
		zap.String("reason", reason),
		zap.Bool("force", force))
	return s.executor.StopByTaskID(ctx, taskID, reason, force)
}

// StopSession stops agent execution for a specific session
func (s *Service) StopSession(ctx context.Context, sessionID string, reason string, force bool) error {
	s.logger.Info("stopping session execution",
		zap.String("session_id", sessionID),
		zap.String("reason", reason),
		zap.Bool("force", force))
	return s.executor.Stop(ctx, sessionID, reason, force)
}

// PromptTask sends a follow-up prompt to a running agent for a task session.
func (s *Service) PromptTask(ctx context.Context, taskID, sessionID string, prompt string, model string) (*PromptResult, error) {
	s.logger.Debug("sending prompt to task agent",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.Int("prompt_length", len(prompt)))
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Check if model switching is requested
	if model != "" {
		// Get the current session to check current model
		session, err := s.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to get session for model switch: %w", err)
		}

		// Get current model from agent profile snapshot
		if session.AgentProfileSnapshot != nil {
			if currentModel, ok := session.AgentProfileSnapshot["model"].(string); ok && currentModel != model {
				// Model switch needed - delegate to executor
				s.logger.Debug("model switch requested",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID),
					zap.String("current_model", currentModel),
					zap.String("new_model", model))

				// Start a new turn for this prompt (model switch case)
				s.startTurnForSession(ctx, sessionID)

				// SwitchModel will stop agent, rebuild with new model, restart, and send prompt
				switchResult, err := s.executor.SwitchModel(ctx, taskID, sessionID, model, prompt)
				if err != nil {
					return nil, err
				}
				return &PromptResult{
					StopReason:   switchResult.StopReason,
					AgentMessage: switchResult.AgentMessage,
				}, nil
			}
		}
	}

	s.setSessionRunning(ctx, taskID, sessionID)
	// Start a new turn for this prompt
	s.startTurnForSession(ctx, sessionID)
	result, err := s.executor.Prompt(ctx, taskID, sessionID, prompt)
	if err != nil {
		return nil, err
	}
	return &PromptResult{
		StopReason:   result.StopReason,
		AgentMessage: result.AgentMessage,
	}, nil
}

// RespondToPermission sends a response to a permission request for a session
func (s *Service) RespondToPermission(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error {
	s.logger.Debug("responding to permission request",
		zap.String("session_id", sessionID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))

	// Respond to the permission via agentctl
	if err := s.executor.RespondToPermission(ctx, sessionID, pendingID, optionID, cancelled); err != nil {
		return err
	}

	// Determine status based on response
	status := "approved"
	if cancelled {
		status = "rejected"
	}

	// Update the permission message with the new status
	if s.messageCreator != nil {
		if err := s.messageCreator.UpdatePermissionMessage(ctx, sessionID, pendingID, status); err != nil {
			s.logger.Warn("failed to update permission message status",
				zap.String("session_id", sessionID),
				zap.String("pending_id", pendingID),
				zap.String("status", status),
				zap.Error(err))
			// Don't fail the whole operation if message update fails
		}
	}

	if !cancelled {
		session, err := s.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			s.logger.Warn("failed to load task session after permission response",
				zap.String("session_id", sessionID),
				zap.Error(err))
			return nil
		}
		s.setSessionRunning(ctx, session.TaskID, sessionID)
	}

	return nil
}

// CancelAgent interrupts the current agent turn without terminating the process,
// allowing the user to send a new prompt.
func (s *Service) CancelAgent(ctx context.Context, sessionID string) error {
	s.logger.Debug("cancelling agent turn", zap.String("session_id", sessionID))

	// Fetch session for state updates and message creation
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to get session for cancel",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	if err := s.agentManager.CancelAgent(ctx, sessionID); err != nil {
		return fmt.Errorf("cancel agent: %w", err)
	}

	// Transition to WAITING_FOR_INPUT so the user can send a new prompt
	if session != nil {
		s.updateTaskSessionState(ctx, session.TaskID, sessionID, models.TaskSessionStateWaitingForInput, "", true)
	}

	// Record cancellation in the message history
	if s.messageCreator != nil && session != nil {
		metadata := map[string]interface{}{
			"cancelled": true,
			"variant":   "warning",
		}
		if err := s.messageCreator.CreateSessionMessage(
			ctx,
			session.TaskID,
			"Turn cancelled by user",
			sessionID,
			string(v1.MessageTypeStatus),
			s.getActiveTurnID(sessionID),
			metadata,
			false,
		); err != nil {
			s.logger.Warn("failed to create cancel message",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}

	// Complete the turn since the agent was cancelled
	s.completeTurnForSession(ctx, sessionID)

	s.logger.Debug("agent turn cancelled", zap.String("session_id", sessionID))
	return nil
}

// CompleteTask explicitly completes a task and stops all its agents
func (s *Service) CompleteTask(ctx context.Context, taskID string) error {
	s.logger.Info("completing task",
		zap.String("task_id", taskID))

	// Stop all agents for this task (which will trigger AgentCompleted events and update session states)
	if err := s.executor.StopByTaskID(ctx, taskID, "task completed by user", false); err != nil {
		// If agents are already stopped, just update the task state directly
		s.logger.Warn("failed to stop agents, updating task state directly",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	// Update task state to COMPLETED
	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateCompleted); err != nil {
		return fmt.Errorf("failed to update task state: %w", err)
	}

	s.logger.Info("task marked as COMPLETED",
		zap.String("task_id", taskID))
	return nil
}

// GetQueuedTasks returns tasks in the queue
func (s *Service) GetQueuedTasks() []*queue.QueuedTask {
	return s.queue.List()
}

