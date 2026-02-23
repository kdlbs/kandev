// Package orchestrator provides the main orchestrator service that ties all components together.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/dto"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
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
// When launchWorkspace is true, workspace infrastructure (agentctl) is launched synchronously
// so file browsing works immediately. When false, the workspace launch is deferred to
// StartTaskWithSession (useful for remote executors where provisioning takes 30-60s).
func (s *Service) PrepareTaskSession(ctx context.Context, taskID string, agentProfileID string, executorID string, executorProfileID string, workflowStepID string, launchWorkspace bool) (string, error) {
	s.logger.Debug("preparing task session",
		zap.String("task_id", taskID),
		zap.String("agent_profile_id", agentProfileID),
		zap.String("executor_id", executorID),
		zap.String("executor_profile_id", executorProfileID),
		zap.String("workflow_step_id", workflowStepID),
		zap.Bool("launch_workspace", launchWorkspace))

	// Fetch the task to get workspace info
	task, err := s.scheduler.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to fetch task for session preparation",
			zap.String("task_id", taskID),
			zap.Error(err))
		return "", err
	}

	// Create session entry in database
	sessionID, err := s.executor.PrepareSession(ctx, task, agentProfileID, executorID, executorProfileID, workflowStepID)
	if err != nil {
		s.logger.Error("failed to prepare session",
			zap.String("task_id", taskID),
			zap.Error(err))
		return "", err
	}

	if launchWorkspace {
		// Launch workspace infrastructure (agentctl) without starting the agent subprocess.
		// This enables file browsing, editing, etc. while the session is in CREATED state.
		if prepExec, launchErr := s.executor.LaunchPreparedSession(ctx, task, sessionID, executor.LaunchOptions{AgentProfileID: agentProfileID, ExecutorID: executorID, WorkflowStepID: workflowStepID}); launchErr != nil {
			s.logger.Warn("failed to launch workspace for prepared session (file browsing may be unavailable)",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(launchErr))
			// Non-fatal: session is still usable, workspace will be launched when agent starts
		} else if prepExec != nil && prepExec.WorktreeBranch != "" {
			go s.ensureSessionPRWatch(context.Background(), taskID, prepExec.SessionID, prepExec.WorktreeBranch)
		}
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
func (s *Service) StartTaskWithSession(ctx context.Context, taskID string, sessionID string, agentProfileID string, executorID string, executorProfileID string, priority int, prompt string, workflowStepID string, planMode bool) (*executor.TaskExecution, error) {
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

	execution, err := s.executor.LaunchPreparedSession(ctx, task, sessionID, executor.LaunchOptions{AgentProfileID: agentProfileID, ExecutorID: executorID, Prompt: effectivePrompt, WorkflowStepID: workflowStepID, StartAgent: true})
	if err != nil {
		return nil, err
	}

	if execution.SessionID != "" {
		s.recordInitialMessage(ctx, taskID, execution.SessionID, effectivePrompt, planModeActive)
	}
	if execution.WorktreeBranch != "" {
		go s.ensureSessionPRWatch(context.Background(), taskID, execution.SessionID, execution.WorktreeBranch)
	}

	return execution, nil
}

// StartCreatedSession starts agent execution for a task using a session that is in CREATED state.
// This is used when a session was prepared (via PrepareSession) but the agent was not launched,
// and the user now wants to start the agent with a prompt (e.g., from the plan panel or chat).
func (s *Service) StartCreatedSession(ctx context.Context, taskID, sessionID, agentProfileID, prompt string) (*executor.TaskExecution, error) {
	s.logger.Debug("starting created session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("agent_profile_id", agentProfileID))

	// Load and verify session
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if session.TaskID != taskID {
		return nil, fmt.Errorf("session does not belong to task")
	}
	if session.State != models.TaskSessionStateCreated {
		return nil, fmt.Errorf("session is not in CREATED state (current: %s)", session.State)
	}

	// Use agent profile from request, fall back to session's stored value
	effectiveProfileID := agentProfileID
	if effectiveProfileID == "" {
		effectiveProfileID = session.AgentProfileID
	}
	if effectiveProfileID == "" {
		return nil, fmt.Errorf("agent_profile_id is required")
	}

	// Transition task state: CREATED → SCHEDULING → (IN_PROGRESS via executor)
	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateScheduling); err != nil {
		s.logger.Warn("failed to update task state to SCHEDULING",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	task, err := s.scheduler.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	effectivePrompt := prompt
	if effectivePrompt == "" {
		effectivePrompt = task.Description
	}

	executorID := session.ExecutorID

	execution, err := s.executor.LaunchPreparedSession(ctx, task, sessionID, executor.LaunchOptions{AgentProfileID: effectiveProfileID, ExecutorID: executorID, Prompt: effectivePrompt, StartAgent: true})
	if err != nil {
		return nil, err
	}

	if execution.SessionID != "" {
		s.updateTaskSessionState(ctx, taskID, execution.SessionID, models.TaskSessionStateRunning, "", true)
	}

	return execution, nil
}

// StartTask manually starts agent execution for a task.
// If workflowStepID is provided and workflowStepGetter is set, the prompt will be built
// using the step's prompt_prefix + base prompt + prompt_suffix, and plan mode will be
// applied if the step has plan_mode enabled.
// If planMode is true and the workflow step doesn't already apply plan mode,
// default plan mode instructions are injected into the prompt.
func (s *Service) StartTask(ctx context.Context, taskID string, agentProfileID string, executorID string, executorProfileID string, priority int, prompt string, workflowStepID string, planMode bool) (*executor.TaskExecution, error) {
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

	// Move task to the target workflow step if provided and different from current
	if workflowStepID != "" {
		dbTask, err := s.repo.GetTask(ctx, taskID)
		if err == nil && dbTask.WorkflowStepID != workflowStepID {
			dbTask.WorkflowStepID = workflowStepID
			dbTask.UpdatedAt = time.Now().UTC()
			if err := s.repo.UpdateTask(ctx, dbTask); err != nil {
				s.logger.Warn("failed to move task to workflow step",
					zap.String("task_id", taskID),
					zap.String("workflow_step_id", workflowStepID),
					zap.Error(err))
			} else if s.eventBus != nil {
				_ = s.eventBus.Publish(ctx, events.TaskUpdated, bus.NewEvent(
					events.TaskUpdated,
					"orchestrator",
					map[string]interface{}{
						"task_id":          dbTask.ID,
						"workflow_id":      dbTask.WorkflowID,
						"workflow_step_id": dbTask.WorkflowStepID,
						"title":            dbTask.Title,
						"description":      dbTask.Description,
						"state":            string(dbTask.State),
						"priority":         dbTask.Priority,
						"position":         dbTask.Position,
					},
				))
			}
		}
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

	execution, err := s.executor.ExecuteWithFullProfile(ctx, task, agentProfileID, executorID, executorProfileID, effectivePrompt, workflowStepID)
	if err != nil {
		return nil, err
	}

	if execution.SessionID != "" {
		s.recordInitialMessage(ctx, taskID, execution.SessionID, effectivePrompt, planModeActive)
	}
	if execution.WorktreeBranch != "" {
		go s.ensureSessionPRWatch(context.Background(), taskID, execution.SessionID, execution.WorktreeBranch)
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
			stepHasPlanMode = step.HasOnEnterAction(wfmodels.OnEnterEnablePlanMode)
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
// If step.Prompt contains {{task_prompt}}, it is replaced with the base prompt.
// Otherwise, step.Prompt is prepended to the base prompt.
// If the step has enable_plan_mode in on_enter events, plan mode prefix is also prepended.
// System-injected content is wrapped in <kandev-system> tags so it can be stripped when displaying to users.
func (s *Service) buildWorkflowPrompt(basePrompt string, step *wfmodels.WorkflowStep, taskID string) string {
	var parts []string

	// Apply plan mode prefix if enabled (wrapped in system tags)
	if step.HasOnEnterAction(wfmodels.OnEnterEnablePlanMode) {
		parts = append(parts, sysprompt.Wrap(sysprompt.PlanMode))
	}

	// Build the prompt from step.Prompt template and base prompt
	if step.Prompt != "" {
		interpolatedPrompt := sysprompt.InterpolatePlaceholders(step.Prompt, taskID)
		if strings.Contains(interpolatedPrompt, "{{task_prompt}}") {
			// Replace placeholder with base prompt
			combined := strings.Replace(interpolatedPrompt, "{{task_prompt}}", basePrompt, 1)
			parts = append(parts, combined)
		} else {
			// Prepend step prompt, then base prompt
			parts = append(parts, sysprompt.Wrap(interpolatedPrompt))
			parts = append(parts, basePrompt)
		}
	} else {
		// No step prompt, just use base prompt
		parts = append(parts, basePrompt)
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
	if err != nil || !canResumeRunning(running) {
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
		if stateErr := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateFailed); stateErr != nil {
			s.logger.Warn("failed to update task state to FAILED after resume error",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(stateErr))
		}
		return nil, err
	}
	// Preserve persisted task/session state; resume should not mutate state/columns.
	execution.SessionState = v1.TaskSessionState(session.State)

	s.logger.Debug("task session resumed and ready for input",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))

	if execution.WorktreeBranch != "" {
		go s.ensureSessionPRWatch(context.Background(), taskID, execution.SessionID, execution.WorktreeBranch)
	}

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

	step, err := s.workflowStepGetter.GetStep(ctx, workflowStepID)
	if err != nil {
		return fmt.Errorf("failed to get workflow step: %w", err)
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}
	if session.TaskID != taskID {
		return fmt.Errorf("session does not belong to task")
	}

	task, err := s.scheduler.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	if session.ReviewStatus != nil && *session.ReviewStatus == "pending" {
		return fmt.Errorf("session is pending approval - use Approve button to proceed or send a message to request changes")
	}

	s.advanceSessionWorkflowStep(ctx, sessionID, workflowStepID, session)

	effectivePrompt := s.buildWorkflowPrompt(task.Description, step, taskID)

	if err := s.ensureSessionRunning(ctx, sessionID, session); err != nil {
		return err
	}

	stepPlanMode := step.HasOnEnterAction(wfmodels.OnEnterEnablePlanMode)
	_, err = s.PromptTask(ctx, taskID, sessionID, effectivePrompt, "", stepPlanMode, nil)
	if err != nil {
		return fmt.Errorf("failed to prompt session: %w", err)
	}

	s.logger.Info("session started for workflow step",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("workflow_step_id", workflowStepID),
		zap.String("step_name", step.Name),
		zap.Bool("plan_mode", stepPlanMode))

	return nil
}

// advanceSessionWorkflowStep updates the session's workflow step and clears review status if the step changed.
func (s *Service) advanceSessionWorkflowStep(ctx context.Context, sessionID, workflowStepID string, session *models.TaskSession) {
	if session.WorkflowStepID != nil && *session.WorkflowStepID == workflowStepID {
		return
	}
	if err := s.repo.UpdateSessionWorkflowStep(ctx, sessionID, workflowStepID); err != nil {
		s.logger.Warn("failed to update session workflow step",
			zap.String("session_id", sessionID),
			zap.String("workflow_step_id", workflowStepID),
			zap.Error(err))
	}
	if session.ReviewStatus != nil && *session.ReviewStatus != "" {
		if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, ""); err != nil {
			s.logger.Warn("failed to clear session review status",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}
}

// ensureSessionRunning resumes the session if it is not currently running or waiting for input.
func (s *Service) ensureSessionRunning(ctx context.Context, sessionID string, session *models.TaskSession) error {
	sessionState := session.State
	if sessionState == models.TaskSessionStateRunning || sessionState == models.TaskSessionStateWaitingForInput {
		return nil
	}

	s.logger.Debug("session not running, attempting resume",
		zap.String("session_id", sessionID),
		zap.String("session_state", string(session.State)))

	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || !canResumeRunning(running) {
		return fmt.Errorf("session is not resumable (state: %s)", session.State)
	}

	if err := validateSessionWorktrees(session); err != nil {
		return err
	}

	// Use context.WithoutCancel to prevent WebSocket request timeout from canceling the resume.
	resumeCtx := context.WithoutCancel(ctx)
	if _, err = s.executor.ResumeSession(resumeCtx, session, true); err != nil {
		_ = s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateFailed, err.Error())
		if stateErr := s.taskRepo.UpdateTaskState(ctx, session.TaskID, v1.TaskStateFailed); stateErr != nil {
			s.logger.Warn("failed to update task state to FAILED after session ensure resume error",
				zap.String("task_id", session.TaskID),
				zap.String("session_id", sessionID),
				zap.Error(stateErr))
		}
		return fmt.Errorf("failed to resume session: %w", err)
	}

	s.logger.Debug("session resumed, now sending prompt")
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
	s.populateExecutorStatusInfo(ctx, session, &resp)

	// Extract resume token from executor runtime state.
	resumeToken := s.populateResumeInfo(ctx, sessionID, &resp)

	// Extract worktree info
	populateWorktreeInfo(session, &resp)

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
	return s.validateResumeEligibility(session, resp), nil
}

// populateResumeInfo extracts resume token from executor runtime state and populates resp fields.
// Returns the resume token (may be empty).
func (s *Service) populateResumeInfo(ctx context.Context, sessionID string, resp *dto.TaskSessionStatusResponse) string {
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || running == nil {
		return ""
	}
	resp.ACPSessionID = running.ResumeToken
	resp.Runtime = running.Runtime
	if running.Resumable {
		resp.IsResumable = true
	}
	s.applyRemoteRuntimeStatus(ctx, sessionID, resp)
	return running.ResumeToken
}

func (s *Service) populateExecutorStatusInfo(ctx context.Context, session *models.TaskSession, resp *dto.TaskSessionStatusResponse) {
	if session == nil || resp == nil {
		return
	}
	resp.ExecutorID = session.ExecutorID
	if session.ExecutorID == "" {
		return
	}
	execModel, err := s.repo.GetExecutor(ctx, session.ExecutorID)
	if err != nil || execModel == nil {
		return
	}
	resp.ExecutorType = string(execModel.Type)
	resp.ExecutorName = execModel.Name
	resp.IsRemoteExecutor = isRemoteExecutorType(execModel.Type)
}

func isRemoteExecutorType(t models.ExecutorType) bool {
	return t == models.ExecutorTypeSprites || t == models.ExecutorTypeRemoteDocker
}

func (s *Service) applyRemoteRuntimeStatus(ctx context.Context, sessionID string, resp *dto.TaskSessionStatusResponse) {
	if s.agentManager == nil || resp == nil || !resp.IsRemoteExecutor {
		return
	}
	status, err := s.agentManager.GetRemoteRuntimeStatusBySession(ctx, sessionID)
	if err != nil || status == nil {
		return
	}
	if status.RuntimeName != "" {
		resp.Runtime = status.RuntimeName
	}
	resp.RemoteState = status.State
	resp.RemoteName = status.RemoteName
	if status.ErrorMessage != "" {
		resp.RemoteStatusErr = status.ErrorMessage
	}
	if status.CreatedAt != nil && !status.CreatedAt.IsZero() {
		resp.RemoteCreatedAt = status.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !status.LastCheckedAt.IsZero() {
		resp.RemoteCheckedAt = status.LastCheckedAt.UTC().Format(time.RFC3339)
	}
}

// populateWorktreeInfo copies worktree path and branch into the response if present.
func populateWorktreeInfo(session *models.TaskSession, resp *dto.TaskSessionStatusResponse) {
	if len(session.Worktrees) == 0 {
		return
	}
	wt := session.Worktrees[0]
	if wt.WorktreePath != "" {
		resp.WorktreePath = &wt.WorktreePath
	}
	if wt.WorktreeBranch != "" {
		resp.WorktreeBranch = &wt.WorktreeBranch
	}
}

// validateResumeEligibility performs final checks before marking a session as resumable.
func (s *Service) validateResumeEligibility(session *models.TaskSession, resp dto.TaskSessionStatusResponse) dto.TaskSessionStatusResponse {
	if session.AgentProfileID == "" {
		resp.Error = "session missing agent profile"
		resp.IsResumable = false
		return resp
	}

	// Check if worktree exists (if one was used)
	if len(session.Worktrees) > 0 && session.Worktrees[0].WorktreePath != "" {
		if _, err := os.Stat(session.Worktrees[0].WorktreePath); err != nil {
			resp.Error = "worktree not found"
			resp.IsResumable = false
			return resp
		}
	}

	resp.IsAgentRunning = false
	resp.NeedsResume = true
	resp.ResumeReason = "agent_not_running"
	return resp
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

// StopExecution stops agent execution for a specific execution ID.
func (s *Service) StopExecution(ctx context.Context, executionID string, reason string, force bool) error {
	s.logger.Info("stopping execution",
		zap.String("execution_id", executionID),
		zap.String("reason", reason),
		zap.Bool("force", force))
	return s.executor.StopExecution(ctx, executionID, reason, force)
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

	// Check if model switching is requested
	if result, switched, err := s.trySwitchModel(ctx, taskID, sessionID, model, effectivePrompt, session); switched || err != nil {
		return result, err
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

// trySwitchModel handles model switching for a prompt. Returns (result, true, nil) if a switch was
// performed, (nil, false, err) on error, or (nil, false, nil) if no switch was needed.
func (s *Service) trySwitchModel(ctx context.Context, taskID, sessionID, model, effectivePrompt string, session *models.TaskSession) (*PromptResult, bool, error) {
	if model == "" {
		return nil, false, nil
	}
	var currentModel string
	if session.AgentProfileSnapshot != nil {
		if m, ok := session.AgentProfileSnapshot["model"].(string); ok {
			currentModel = m
		}
	}
	if currentModel == model {
		return nil, false, nil
	}
	s.logger.Info("switching model",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from", currentModel),
		zap.String("to", model))
	s.startTurnForSession(ctx, sessionID)
	switchCtx := context.WithoutCancel(ctx)
	switchResult, err := s.executor.SwitchModel(switchCtx, taskID, sessionID, model, effectivePrompt)
	if err != nil {
		return nil, true, fmt.Errorf("model switch failed: %w", err)
	}
	s.setSessionRunning(ctx, taskID, sessionID)
	return &PromptResult{
		StopReason:   switchResult.StopReason,
		AgentMessage: switchResult.AgentMessage,
	}, true, nil
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
