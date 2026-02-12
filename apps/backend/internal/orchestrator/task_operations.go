// Package orchestrator provides the main orchestrator service that ties all components together.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/dto"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/sysprompt"
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

// PrepareTaskSession creates a session entry without launching the agent.
// This allows the HTTP handler to return the session ID immediately while the agent setup
// continues in the background. Use StartTaskWithSession to continue with agent launch.
func (s *Service) PrepareTaskSession(ctx context.Context, taskID string, agentProfileID string, executorID string, workflowStepID string) (string, error) {
	s.logger.Debug("preparing task session",
		zap.String("task_id", taskID),
		zap.String("agent_profile_id", agentProfileID),
		zap.String("executor_id", executorID),
		zap.String("workflow_step_id", workflowStepID))

	// Fetch the task to get workspace info
	task, err := s.scheduler.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to fetch task for session preparation",
			zap.String("task_id", taskID),
			zap.Error(err))
		return "", err
	}

	// Create session entry in database
	sessionID, err := s.executor.PrepareSession(ctx, task, agentProfileID, executorID, workflowStepID)
	if err != nil {
		s.logger.Error("failed to prepare session",
			zap.String("task_id", taskID),
			zap.Error(err))
		return "", err
	}

	s.logger.Info("task session prepared",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))

	return sessionID, nil
}

// StartTaskWithSession starts agent execution for a task using a pre-created session.
// This is used after PrepareTaskSession to continue with the agent launch.
// If planMode is true and the workflow step doesn't already apply plan mode,
// default plan mode instructions are injected into the prompt.
func (s *Service) StartTaskWithSession(ctx context.Context, taskID string, sessionID string, agentProfileID string, executorID string, priority int, prompt string, workflowStepID string, planMode bool) (*executor.TaskExecution, error) {
	s.logger.Debug("starting task with existing session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("agent_profile_id", agentProfileID),
		zap.Bool("plan_mode", planMode))

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateScheduling); err != nil {
		s.logger.Warn("failed to update task state to SCHEDULING",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	task, err := s.scheduler.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	if priority > 0 {
		task.Priority = priority
	}

	effectivePrompt := prompt
	if effectivePrompt == "" {
		effectivePrompt = task.Description
	}

	effectivePrompt, planModeActive := s.applyWorkflowAndPlanMode(ctx, effectivePrompt, task.ID, workflowStepID, planMode)

	execution, err := s.executor.LaunchPreparedSession(ctx, task, sessionID, agentProfileID, executorID, effectivePrompt, workflowStepID)
	if err != nil {
		return nil, err
	}

	if execution.SessionID != "" {
		s.recordInitialMessage(ctx, taskID, execution.SessionID, effectivePrompt, planModeActive)
	}

	return execution, nil
}

// StartTask manually starts agent execution for a task.
// If workflowStepID is provided and workflowStepGetter is set, the prompt will be built
// using the step's prompt_prefix + base prompt + prompt_suffix, and plan mode will be
// applied if the step has plan_mode enabled.
// If planMode is true and the workflow step doesn't already apply plan mode,
// default plan mode instructions are injected into the prompt.
func (s *Service) StartTask(ctx context.Context, taskID string, agentProfileID string, executorID string, priority int, prompt string, workflowStepID string, planMode bool) (*executor.TaskExecution, error) {
	s.logger.Debug("manually starting task",
		zap.String("task_id", taskID),
		zap.String("agent_profile_id", agentProfileID),
		zap.String("executor_id", executorID),
		zap.Int("priority", priority),
		zap.Int("prompt_length", len(prompt)),
		zap.String("workflow_step_id", workflowStepID),
		zap.Bool("plan_mode", planMode))

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

	effectivePrompt, planModeActive := s.applyWorkflowAndPlanMode(ctx, effectivePrompt, task.ID, workflowStepID, planMode)

	execution, err := s.executor.ExecuteWithProfile(ctx, task, agentProfileID, executorID, effectivePrompt, workflowStepID)
	if err != nil {
		return nil, err
	}

	if execution.SessionID != "" {
		s.recordInitialMessage(ctx, taskID, execution.SessionID, effectivePrompt, planModeActive)
	}

	// Note: Task stays in SCHEDULING state until the agent is fully initialized.
	// The executor will transition to IN_PROGRESS after StartAgentProcess() succeeds.

	return execution, nil
}

// applyWorkflowAndPlanMode applies workflow step configuration and plan mode injection to a prompt.
// Returns the effective prompt and whether plan mode is active (from either the step or the caller).
func (s *Service) applyWorkflowAndPlanMode(ctx context.Context, prompt string, taskID string, workflowStepID string, planMode bool) (string, bool) {
	effectivePrompt := prompt

	stepHasPlanMode := false
	if workflowStepID != "" && s.workflowStepGetter != nil {
		step, err := s.workflowStepGetter.GetStep(ctx, workflowStepID)
		if err != nil {
			s.logger.Warn("failed to get workflow step for prompt building",
				zap.String("workflow_step_id", workflowStepID),
				zap.Error(err))
		} else {
			stepHasPlanMode = step.PlanMode
			effectivePrompt = s.buildWorkflowPrompt(effectivePrompt, step, taskID)
		}
	}

	if planMode && !stepHasPlanMode {
		var parts []string
		parts = append(parts, sysprompt.Wrap(sysprompt.PlanMode))
		parts = append(parts, sysprompt.Wrap(sysprompt.InterpolatePlaceholders(sysprompt.DefaultPlanPrefix, taskID)))
		parts = append(parts, effectivePrompt)
		effectivePrompt = strings.Join(parts, "\n\n")
	}

	return effectivePrompt, planMode || stepHasPlanMode
}

// recordInitialMessage creates the initial user message and updates session state after launch.
func (s *Service) recordInitialMessage(ctx context.Context, taskID, sessionID, prompt string, planModeActive bool) {
	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateRunning, "", true)
	if s.messageCreator != nil && prompt != "" {
		meta := NewUserMessageMeta().WithPlanMode(planModeActive)
		if err := s.messageCreator.CreateUserMessage(ctx, taskID, prompt, sessionID, s.getActiveTurnID(sessionID), meta.ToMap()); err != nil {
			s.logger.Error("failed to create initial user message",
				zap.String("task_id", taskID),
				zap.Error(err))
		}
	}
}

// buildWorkflowPrompt constructs the effective prompt using workflow step configuration.
// It combines: step.PromptPrefix + basePrompt + step.PromptSuffix
// If step.PlanMode is true, it also prepends the plan mode prefix.
// Placeholders like {task_id} are interpolated with actual values.
// System-injected content (prefix, suffix, plan mode) is wrapped in <kandev-system> tags
// so it can be stripped when displaying to users.
func (s *Service) buildWorkflowPrompt(basePrompt string, step *WorkflowStep, taskID string) string {
	var parts []string

	// Apply plan mode prefix if enabled (wrapped in system tags)
	if step.PlanMode {
		parts = append(parts, sysprompt.Wrap(sysprompt.PlanMode))
	}

	// Add prompt prefix if set (with interpolation, wrapped in system tags)
	if step.PromptPrefix != "" {
		parts = append(parts, sysprompt.Wrap(sysprompt.InterpolatePlaceholders(step.PromptPrefix, taskID)))
	}

	// Add base prompt (user's actual message - not wrapped)
	parts = append(parts, basePrompt)

	// Add prompt suffix if set (with interpolation, wrapped in system tags)
	if step.PromptSuffix != "" {
		parts = append(parts, sysprompt.Wrap(sysprompt.InterpolatePlaceholders(step.PromptSuffix, taskID)))
	}

	return strings.Join(parts, "\n\n")
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

	// Use context.WithoutCancel to prevent WebSocket request timeout from canceling the resume.
	// Session resume can take time and shouldn't be tied to the WS request lifecycle.
	resumeCtx := context.WithoutCancel(ctx)
	execution, err := s.executor.ResumeSession(resumeCtx, session, true)
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

// StartSessionForWorkflowStep starts an existing session with a workflow step's prompt configuration.
// If the session is not running, it will be resumed first. Then a prompt is sent using the
// step's prompt_prefix, prompt_suffix, and plan_mode settings combined with the task description.
func (s *Service) StartSessionForWorkflowStep(ctx context.Context, taskID, sessionID, workflowStepID string) error {
	s.logger.Debug("starting session for workflow step",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("workflow_step_id", workflowStepID))

	if workflowStepID == "" {
		return fmt.Errorf("workflow_step_id is required")
	}
	if s.workflowStepGetter == nil {
		return fmt.Errorf("workflow step getter not configured")
	}

	// Get the workflow step
	step, err := s.workflowStepGetter.GetStep(ctx, workflowStepID)
	if err != nil {
		return fmt.Errorf("failed to get workflow step: %w", err)
	}

	// Get the session
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}
	if session.TaskID != taskID {
		return fmt.Errorf("session does not belong to task")
	}

	// Get the task to use its description as the base prompt
	task, err := s.scheduler.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	// Check if session is in a review step with pending approval
	// If so, reject the request - user must use Approve button or send a chat message
	if session.ReviewStatus != nil && *session.ReviewStatus == "pending" {
		if session.WorkflowStepID != nil && *session.WorkflowStepID != "" {
			currentStep, err := s.workflowStepGetter.GetStep(ctx, *session.WorkflowStepID)
			if err == nil && currentStep.RequireApproval {
				return fmt.Errorf("session is pending approval - use Approve button to proceed or send a message to request changes")
			}
		}
	}

	// Update session's workflow step to the new step
	if session.WorkflowStepID == nil || *session.WorkflowStepID != workflowStepID {
		if err := s.repo.UpdateSessionWorkflowStep(ctx, sessionID, workflowStepID); err != nil {
			s.logger.Warn("failed to update session workflow step",
				zap.String("session_id", sessionID),
				zap.String("workflow_step_id", workflowStepID),
				zap.Error(err))
		}
		// Clear review status when moving to a new step
		if session.ReviewStatus != nil && *session.ReviewStatus != "" {
			if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, ""); err != nil {
				s.logger.Warn("failed to clear session review status",
					zap.String("session_id", sessionID),
					zap.Error(err))
			}
		}
	}

	// Build prompt from workflow step configuration
	effectivePrompt := s.buildWorkflowPrompt(task.Description, step, taskID)

	// Check if session needs to be resumed first
	// A session in "running" or "waiting_for_input" state can receive prompts directly
	// Other states (created, starting, completed, failed, cancelled) need resume
	sessionState := session.State
	if sessionState != models.TaskSessionStateRunning && sessionState != models.TaskSessionStateWaitingForInput {
		// Try to resume the session
		s.logger.Debug("session not running, attempting resume",
			zap.String("session_id", sessionID),
			zap.String("session_state", string(session.State)))

		running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
		if err != nil || running == nil || running.ResumeToken == "" || !running.Resumable {
			return fmt.Errorf("session is not resumable (state: %s)", session.State)
		}

		if err := validateSessionWorktrees(session); err != nil {
			return err
		}

		// Use context.WithoutCancel to prevent WebSocket request timeout from canceling the resume.
		// Session resume can take time and shouldn't be tied to the WS request lifecycle.
		resumeCtx := context.WithoutCancel(ctx)
		_, err = s.executor.ResumeSession(resumeCtx, session, true)
		if err != nil {
			_ = s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateFailed, err.Error())
			return fmt.Errorf("failed to resume session: %w", err)
		}

		s.logger.Debug("session resumed, now sending prompt")
	}

	// Send the prompt using PromptTask (with planMode from step)
	// Note: PromptTask internally uses context.WithoutCancel for the executor call
	// No attachments for workflow-initiated prompts
	_, err = s.PromptTask(ctx, taskID, sessionID, effectivePrompt, "", step.PlanMode, nil)
	if err != nil {
		return fmt.Errorf("failed to prompt session: %w", err)
	}

	s.logger.Info("session started for workflow step",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("workflow_step_id", workflowStepID),
		zap.String("step_name", step.Name),
		zap.Bool("plan_mode", step.PlanMode))

	return nil
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

	// Stop all agents for this task
	if err := s.executor.StopByTaskID(ctx, taskID, reason, force); err != nil {
		return err
	}

	// Move task to REVIEW state for user review
	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateReview); err != nil {
		s.logger.Error("failed to update task state to REVIEW after stop",
			zap.String("task_id", taskID),
			zap.Error(err))
		// Don't return error - the stop was successful
	} else {
		s.logger.Info("task moved to REVIEW state after stop",
			zap.String("task_id", taskID))
	}

	return nil
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
// If planMode is true, a plan mode prefix is prepended to the prompt.
// Attachments (images) are passed through to the agent if provided.
func (s *Service) PromptTask(ctx context.Context, taskID, sessionID string, prompt string, model string, planMode bool, attachments []v1.MessageAttachment) (*PromptResult, error) {
	s.logger.Debug("PromptTask called",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.Int("prompt_length", len(prompt)),
		zap.String("requested_model", model),
		zap.Bool("plan_mode", planMode),
		zap.Int("attachments_count", len(attachments)))
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Check if session is already processing a prompt (RUNNING state)
	// This prevents concurrent prompts that can cause race conditions
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if session.State == models.TaskSessionStateRunning {
		s.logger.Warn("rejected prompt while agent is already running",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("session_state", string(session.State)))
		return nil, fmt.Errorf("agent is currently processing a prompt, please wait for completion")
	}

	// Apply plan mode prefix if enabled
	effectivePrompt := prompt
	if planMode {
		effectivePrompt = sysprompt.InjectPlanMode(prompt)
	}

	// Check if session is in a review step - if so, move back to the previous step
	// This handles the case where the user sends a message to iterate on the work
	s.handleReviewStepRollback(ctx, taskID, sessionID)

	// Check if model switching is requested
	if model != "" {
		// Get current model from agent profile snapshot (session already fetched above)
		var currentModel string
		if session.AgentProfileSnapshot != nil {
			if m, ok := session.AgentProfileSnapshot["model"].(string); ok {
				currentModel = m
			}
		}

		// Trigger switch if models differ
		if currentModel != model {
			s.logger.Info("switching model",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("from", currentModel),
				zap.String("to", model))

			// Start a new turn for this prompt (model switch case)
			s.startTurnForSession(ctx, sessionID)

			// Use context.WithoutCancel to prevent WebSocket request timeout from canceling the operation.
			switchCtx := context.WithoutCancel(ctx)

			// SwitchModel will stop agent, rebuild with new model, restart, and send prompt
			switchResult, err := s.executor.SwitchModel(switchCtx, taskID, sessionID, model, effectivePrompt)
			if err != nil {
				return nil, fmt.Errorf("model switch failed: %w", err)
			}

			// Update session state to RUNNING and publish event so frontend knows agent is active
			s.setSessionRunning(ctx, taskID, sessionID)

			return &PromptResult{
				StopReason:   switchResult.StopReason,
				AgentMessage: switchResult.AgentMessage,
			}, nil
		}
		// Model is the same, fall through to regular prompt
	}

	previousSessionState := session.State

	s.setSessionRunning(ctx, taskID, sessionID)
	s.startTurnForSession(ctx, sessionID)

	// Use context.WithoutCancel to prevent WebSocket request timeout from canceling the prompt.
	// Prompts can take a long time (minutes) while the WS request may timeout in 15 seconds.
	// We still want to log and respond, but the prompt should continue regardless.
	promptCtx := context.WithoutCancel(ctx)
	result, err := s.executor.Prompt(promptCtx, taskID, sessionID, effectivePrompt, attachments)
	if err != nil {
		s.logger.Error("prompt failed",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		// Revert session state so it doesn't stay stuck in RUNNING.
		// Use repo directly to bypass state machine guards that block transitions from terminal states.
		_ = s.repo.UpdateTaskSessionState(ctx, sessionID, previousSessionState, "")
		_ = s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateReview)
		s.completeTurnForSession(ctx, sessionID)
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
		// Permission likely expired — update message so frontend reflects this
		if s.messageCreator != nil {
			if updateErr := s.messageCreator.UpdatePermissionMessage(ctx, sessionID, pendingID, "expired"); updateErr != nil {
				s.logger.Warn("failed to mark expired permission message",
					zap.String("session_id", sessionID),
					zap.String("pending_id", pendingID),
					zap.Error(updateErr))
			}
		}
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

