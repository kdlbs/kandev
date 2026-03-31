// Package orchestrator provides the main orchestrator service that ties all components together.
package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/client"
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

var ErrAgentPromptInProgress = errors.New("agent is currently processing a prompt")
var ErrSessionResetInProgress = errors.New("session reset in progress")

func isAgentPromptInProgressError(err error) bool {
	return err != nil && (errors.Is(err, ErrAgentPromptInProgress) || strings.Contains(err.Error(), ErrAgentPromptInProgress.Error()))
}

func isSessionResetInProgressError(err error) bool {
	return err != nil && (errors.Is(err, ErrSessionResetInProgress) || strings.Contains(err.Error(), ErrSessionResetInProgress.Error()))
}

func isTransientPromptError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "agent stream disconnected") ||
		strings.Contains(msg, "use of closed network connection")
}

func isAgentAlreadyRunningError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already has an agent running")
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
// This allows the WS handler to return the session ID immediately while workspace setup
// continues in the background. Use StartTaskWithSession to continue with agent launch.
// When launchWorkspace is true, workspace infrastructure (agentctl) is launched asynchronously;
// the frontend receives preparation progress via executor.prepare.progress WS events.
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

	// Resolve agent/executor profile from task metadata if not explicitly provided
	if agentProfileID == "" {
		if v, ok := task.Metadata["agent_profile_id"].(string); ok && v != "" {
			agentProfileID = v
		}
	}
	if executorProfileID == "" {
		if v, ok := task.Metadata["executor_profile_id"].(string); ok && v != "" {
			executorProfileID = v
		}
	}

	// Fall back to the task's current workflow step when the caller didn't provide one.
	// This ensures sessions created via the kanban card (which doesn't send workflow_step_id)
	// inherit the task's step and participate in workflow events.
	if workflowStepID == "" {
		dbTask, err := s.repo.GetTask(ctx, taskID)
		if err != nil {
			s.logger.Warn("failed to fetch task for workflow step fallback",
				zap.String("task_id", taskID),
				zap.Error(err))
		} else if dbTask.WorkflowStepID != "" {
			workflowStepID = dbTask.WorkflowStepID
		}
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
		// Launch workspace infrastructure (agentctl) in the background so the WS response
		// returns the session ID immediately. The frontend navigates to the session page
		// and shows preparation progress via executor.prepare.progress WS events.
		go func() {
			bgCtx := context.Background()
			prepExec, launchErr := s.executor.LaunchPreparedSession(bgCtx, task, sessionID, executor.LaunchOptions{AgentProfileID: agentProfileID, ExecutorID: executorID, WorkflowStepID: workflowStepID})
			if launchErr != nil {
				s.logger.Warn("failed to launch workspace for prepared session (file browsing may be unavailable)",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID),
					zap.Error(launchErr))
				return
			}
			if prepExec != nil {
				s.ensureSessionPRWatch(bgCtx, taskID, prepExec.SessionID, prepExec.WorktreeBranch)
			}
		}()
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

	// Process on_turn_start before launching the agent.
	if session, err := s.repo.GetTaskSession(ctx, sessionID); err == nil {
		s.processOnTurnStartViaEngine(ctx, taskID, session)
	}

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

	effectivePrompt, planModeActive := s.applyWorkflowAndPlanMode(ctx, effectivePrompt, task.ID, sessionID, workflowStepID, planMode, task.IsEphemeral)

	execution, err := s.executor.LaunchPreparedSession(ctx, task, sessionID, executor.LaunchOptions{AgentProfileID: agentProfileID, ExecutorID: executorID, Prompt: effectivePrompt, WorkflowStepID: workflowStepID, StartAgent: true})
	if err != nil {
		return nil, err
	}

	if execution.SessionID != "" {
		s.recordInitialMessage(ctx, taskID, execution.SessionID, effectivePrompt, planModeActive, nil)

		if planModeActive {
			sess, sessErr := s.repo.GetTaskSession(ctx, execution.SessionID)
			if sessErr == nil {
				s.setSessionPlanMode(ctx, sess, true)
			}
		}
	}
	go s.ensureSessionPRWatch(context.Background(), taskID, execution.SessionID, execution.WorktreeBranch)

	return execution, nil
}

// StartCreatedSession starts agent execution for a task using a session that is in CREATED state.
// This is used when a session was prepared (via PrepareSession) but the agent was not launched,
// and the user now wants to start the agent with a prompt (e.g., from the plan panel or chat).
// When skipMessageRecord is true, only the session state is updated (the caller already stored the user message).
// When planMode is true, plan mode instructions are injected into the prompt and session metadata is set.
func (s *Service) StartCreatedSession(ctx context.Context, taskID, sessionID, agentProfileID, prompt string, skipMessageRecord, planMode bool, attachments []v1.MessageAttachment) (*executor.TaskExecution, error) {
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
	// Accept CREATED (normal) or WAITING_FOR_INPUT (after on_turn_start step transition).
	// When the user sends the first message to a prepared session, on_turn_start may fire
	// and move the step, which sets the session to WAITING_FOR_INPUT before we get here.
	if session.State != models.TaskSessionStateCreated && session.State != models.TaskSessionStateWaitingForInput {
		return nil, fmt.Errorf("session is not in CREATED or WAITING_FOR_INPUT state (current: %s)", session.State)
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

	// Process on_turn_start before launching the agent, just like user-initiated messages.
	// This allows workflow transitions (e.g. move_to_next) to fire on the initial prompt.
	s.processOnTurnStartViaEngine(ctx, taskID, session)

	// Re-read the session after on_turn_start may have changed the workflow step.
	session, err = s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload session after on_turn_start: %w", err)
	}

	// Apply workflow step prompt wrapping and plan mode injection.
	// Called unconditionally so workflow-step prompt composition (prefix/suffix)
	// applies even when plan mode is not requested.
	// Re-read the task after on_turn_start may have changed the workflow step.
	// Ephemeral tasks skip workflow step processing since they have no workflow.
	dbTask, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload task after on_turn_start: %w", err)
	}
	effectivePrompt, planModeActive := s.applyWorkflowAndPlanMode(ctx, effectivePrompt, taskID, sessionID, dbTask.WorkflowStepID, planMode, task.IsEphemeral)

	// Inject config context for config-mode sessions (dedicated settings chat)
	if cm, ok := session.Metadata["config_mode"].(bool); ok && cm {
		effectivePrompt = sysprompt.InjectConfigContext(sessionID, effectivePrompt)
	}

	executorID := session.ExecutorID

	execution, err := s.executor.LaunchPreparedSession(ctx, task, sessionID, executor.LaunchOptions{AgentProfileID: effectiveProfileID, ExecutorID: executorID, Prompt: effectivePrompt, StartAgent: true, Attachments: attachments})
	if err != nil {
		return nil, err
	}

	// Record the initial user message and set plan mode metadata after launch.
	// Note: we do NOT set session state here — the executor sets it to STARTING,
	// and event handlers (handleAgentReady) transition it to WAITING_FOR_INPUT.
	s.postLaunchCreated(ctx, taskID, sessionID, effectivePrompt, skipMessageRecord, planModeActive, attachments)

	// Ensure a PR watch exists so the poller can detect PRs created by the agent.
	// PrepareTaskSession may have already created one, but if that goroutine failed
	// or hadn't completed, this guarantees coverage.
	go s.ensureSessionPRWatch(context.Background(), taskID, execution.SessionID, execution.WorktreeBranch)

	return execution, nil
}

// postLaunchCreated handles post-launch bookkeeping for a created session:
// records the initial user message (unless skipped) and sets plan mode metadata.
// It does NOT modify session state — the executor sets STARTING, and event handlers
// (handleAgentReady) handle the transition to WAITING_FOR_INPUT.
func (s *Service) postLaunchCreated(ctx context.Context, taskID, sessionID, prompt string, skipMessage, planModeActive bool, attachments []v1.MessageAttachment) {
	if !skipMessage {
		s.recordInitialMessage(ctx, taskID, sessionID, prompt, planModeActive, attachments)
	}

	if planModeActive {
		sess, err := s.repo.GetTaskSession(ctx, sessionID)
		if err == nil {
			s.setSessionPlanMode(ctx, sess, true)
		}
	}
}

// StartTask manually starts agent execution for a task.
// If workflowStepID is provided and workflowStepGetter is set, the prompt will be built
// using the step's prompt_prefix + base prompt + prompt_suffix, and plan mode will be
// applied if the step has plan_mode enabled.
// If planMode is true and the workflow step doesn't already apply plan mode,
// default plan mode instructions are injected into the prompt.
func (s *Service) StartTask(ctx context.Context, taskID string, agentProfileID string, executorID string, executorProfileID string, priority int, prompt string, workflowStepID string, planMode bool, attachments []v1.MessageAttachment) (*executor.TaskExecution, error) {
	s.logger.Debug("manually starting task",
		zap.String("task_id", taskID),
		zap.String("agent_profile_id", agentProfileID),
		zap.String("executor_id", executorID),
		zap.Int("priority", priority),
		zap.Int("prompt_length", len(prompt)),
		zap.String("workflow_step_id", workflowStepID),
		zap.Bool("plan_mode", planMode),
		zap.Int("attachments", len(attachments)))

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateScheduling); err != nil {
		s.logger.Warn("failed to update task state to SCHEDULING",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	s.moveTaskToWorkflowStep(ctx, taskID, workflowStepID)

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

	// Prepare session first so we have the sessionID for config context injection
	sessionID, err := s.executor.PrepareSession(ctx, task, agentProfileID, executorID, executorProfileID, workflowStepID)
	if err != nil {
		return nil, err
	}

	effectivePrompt, planModeActive := s.applyWorkflowAndPlanMode(ctx, effectivePrompt, task.ID, sessionID, workflowStepID, planMode, task.IsEphemeral)

	// Inject config context for config-mode sessions (dedicated settings chat)
	configMode := false
	if cm, ok := task.Metadata["config_mode"].(bool); ok && cm {
		configMode = true
		effectivePrompt = sysprompt.InjectConfigContext(sessionID, effectivePrompt)
	}

	execution, err := s.executor.LaunchPreparedSession(ctx, task, sessionID, executor.LaunchOptions{
		AgentProfileID: agentProfileID,
		ExecutorID:     executorID,
		Prompt:         effectivePrompt,
		WorkflowStepID: workflowStepID,
		StartAgent:     true,
		Attachments:    attachments,
	})
	if err != nil {
		return nil, err
	}

	s.postLaunchStart(ctx, taskID, execution, effectivePrompt, planModeActive || configMode, planModeActive, attachments)

	// Note: Task stays in SCHEDULING state until the agent is fully initialized.
	// The executor will transition to IN_PROGRESS after StartAgentProcess() succeeds.

	return execution, nil
}

// moveTaskToWorkflowStep moves a task to the target workflow step if provided and different from current.
func (s *Service) moveTaskToWorkflowStep(ctx context.Context, taskID, workflowStepID string) {
	if workflowStepID == "" {
		return
	}
	dbTask, err := s.repo.GetTask(ctx, taskID)
	if err != nil || dbTask.WorkflowStepID == workflowStepID {
		return
	}
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
			buildTaskEventPayload(dbTask),
		))
	}
}

// postLaunchStart records the initial message and sets plan mode after a successful launch.
func (s *Service) postLaunchStart(ctx context.Context, taskID string, execution *executor.TaskExecution, prompt string, recordPlanMode, setPlanMode bool, attachments []v1.MessageAttachment) {
	if execution.SessionID != "" {
		s.recordInitialMessage(ctx, taskID, execution.SessionID, prompt, recordPlanMode, attachments)

		if setPlanMode {
			session, err := s.repo.GetTaskSession(ctx, execution.SessionID)
			if err == nil {
				s.setSessionPlanMode(ctx, session, true)
			}
		}
	}
	go s.ensureSessionPRWatch(context.Background(), taskID, execution.SessionID, execution.WorktreeBranch)
}

// applyWorkflowAndPlanMode applies workflow step configuration and plan mode injection to a prompt.
// Returns the effective prompt and whether plan mode is active (from either the step or the caller).
// For ephemeral tasks (quick chat), workflow step processing is skipped since they have no workflow.
func (s *Service) applyWorkflowAndPlanMode(ctx context.Context, prompt string, taskID string, sessionID string, workflowStepID string, planMode bool, isEphemeral bool) (string, bool) {
	effectivePrompt := prompt

	stepHasPlanMode := false
	// Skip workflow step prompt injection for ephemeral tasks - they don't have workflows
	if !isEphemeral && workflowStepID != "" && s.workflowStepGetter != nil {
		step, err := s.workflowStepGetter.GetStep(ctx, workflowStepID)
		if err != nil {
			s.logger.Warn("failed to get workflow step for prompt building",
				zap.String("workflow_step_id", workflowStepID),
				zap.Error(err))
		} else {
			stepHasPlanMode = step.HasOnEnterAction(wfmodels.OnEnterEnablePlanMode)
			effectivePrompt = s.buildWorkflowPrompt(effectivePrompt, step, taskID, sessionID)
		}
	}

	if planMode && !stepHasPlanMode {
		var parts []string
		parts = append(parts, sysprompt.Wrap(sysprompt.InterpolatePlaceholders(sysprompt.DefaultPlanPrefix, taskID)))
		parts = append(parts, effectivePrompt)
		effectivePrompt = strings.Join(parts, "\n\n")
	}

	return effectivePrompt, planMode || stepHasPlanMode
}

// recordInitialMessage creates the initial user message and updates session state after launch.
func (s *Service) recordInitialMessage(ctx context.Context, taskID, sessionID, prompt string, planModeActive bool, attachments []v1.MessageAttachment) {
	if s.messageCreator != nil && (prompt != "" || len(attachments) > 0) {
		meta := NewUserMessageMeta().WithPlanMode(planModeActive).WithAttachments(attachments)
		if err := s.messageCreator.CreateUserMessage(ctx, taskID, prompt, sessionID, s.getActiveTurnID(sessionID), meta.ToMap()); err != nil {
			s.logger.Error("failed to create initial user message",
				zap.String("task_id", taskID),
				zap.Error(err))
		}
	}
}

// buildWorkflowPrompt constructs the effective prompt using workflow step configuration.
// If step.Prompt contains {{task_prompt}}, it is replaced with the base prompt.
// Otherwise, step.Prompt fully replaces the base prompt.
// If the step has enable_plan_mode in on_enter events, plan mode prefix is also prepended.
// Only true internal instructions are wrapped in <kandev-system> tags so they can be stripped from the visible chat.
func (s *Service) buildWorkflowPrompt(basePrompt string, step *wfmodels.WorkflowStep, taskID string, sessionID string) string {
	_ = sessionID
	var parts []string

	// Build the prompt from step.Prompt template and base prompt
	if step.Prompt != "" {
		interpolatedPrompt := sysprompt.InterpolatePlaceholders(step.Prompt, taskID)
		if strings.Contains(interpolatedPrompt, "{{task_prompt}}") {
			// Replace placeholder with base prompt
			combined := strings.Replace(interpolatedPrompt, "{{task_prompt}}", basePrompt, 1)
			parts = append(parts, combined)
		} else {
			// A step prompt without {{task_prompt}} is treated as the full visible prompt.
			parts = append(parts, interpolatedPrompt)
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
	if (err != nil || running == nil) &&
		session.State != models.TaskSessionStateCancelled &&
		session.State != models.TaskSessionStateFailed {
		// Executor record is required for non-terminal sessions. For cancelled/failed sessions
		// the record may already have been cleaned up before the user clicked Resume — allow it.
		return nil, fmt.Errorf("session is not resumable: no executor record")
	}
	if err := validateSessionWorktrees(session); err != nil {
		return nil, err
	}

	// Completed sessions cannot be restarted — they require a new session.
	// Failed sessions clear the resume token so the agent starts fresh (the old
	// process crashed). Cancelled sessions keep the token so --resume restores
	// the conversation when the user re-starts a manually stopped session.
	switch session.State {
	case models.TaskSessionStateCompleted:
		return nil, fmt.Errorf("session is completed and cannot be resumed; create a new session instead")
	case models.TaskSessionStateFailed:
		s.clearResumeToken(ctx, sessionID)
	}

	// Use context.WithoutCancel to prevent WebSocket request timeout from canceling the resume.
	// Session resume can take time and shouldn't be tied to the WS request lifecycle.
	resumeCtx := context.WithoutCancel(ctx)
	execution, err := s.executor.ResumeSession(resumeCtx, session, true)
	if err != nil {
		// If the execution is already running (duplicate resume request), return it as success.
		if errors.Is(err, executor.ErrExecutionAlreadyRunning) {
			if existing, ok := s.executor.GetExecutionBySession(sessionID); ok && existing != nil {
				existing.SessionState = v1.TaskSessionState(session.State)
				return existing, nil
			}
		}
		s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateFailed, err.Error(), false, session)
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

	go s.ensureSessionPRWatch(context.Background(), taskID, execution.SessionID, execution.WorktreeBranch)

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

	dbTask, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	if session.ReviewStatus != nil && *session.ReviewStatus == "pending" {
		return fmt.Errorf("session is pending approval - use Approve button to proceed or send a message to request changes")
	}

	s.advanceTaskWorkflowStep(ctx, dbTask, workflowStepID, session)

	effectivePrompt := s.buildWorkflowPrompt(dbTask.Description, step, taskID, sessionID)

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

// advanceTaskWorkflowStep updates the task's workflow step and clears session review status if the step changed.
func (s *Service) advanceTaskWorkflowStep(ctx context.Context, task *models.Task, workflowStepID string, session *models.TaskSession) {
	if task.WorkflowStepID == workflowStepID {
		return
	}
	task.WorkflowStepID = workflowStepID
	task.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		s.logger.Warn("failed to update task workflow step",
			zap.String("task_id", task.ID),
			zap.String("workflow_step_id", workflowStepID),
			zap.Error(err))
	}
	if session.ReviewStatus != nil && *session.ReviewStatus != "" {
		if err := s.repo.UpdateSessionReviewStatus(ctx, session.ID, ""); err != nil {
			s.logger.Warn("failed to clear session review status",
				zap.String("session_id", session.ID),
				zap.Error(err))
		}
	}
}

// ensureSessionRunning resumes the session if the agent is not actually running.
// After lazy recovery, a session may be in WAITING_FOR_INPUT with no agent process;
// this function detects that case and triggers a resume.
func (s *Service) ensureSessionRunning(ctx context.Context, sessionID string, session *models.TaskSession) error {
	// Check if agent is genuinely running (in-memory execution store, not just DB state)
	if exec, ok := s.executor.GetExecutionBySession(sessionID); ok && exec != nil {
		return nil
	}

	s.logger.Debug("agent not running for session, attempting resume",
		zap.String("session_id", sessionID),
		zap.String("session_state", string(session.State)))

	// If the session is in CREATED state with an existing workspace (AgentExecutionID set),
	// the workspace was prepared but the agent was never started. Use LaunchPreparedSession
	// which routes to startAgentOnExistingWorkspace to reuse the workspace rather than
	// ResumeSession which tries a full LaunchAgent and conflicts with the existing execution.
	if session.State == models.TaskSessionStateCreated && session.AgentExecutionID != "" {
		return s.startAgentOnPreparedWorkspace(ctx, sessionID, session)
	}

	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || running == nil {
		return fmt.Errorf("session is not resumable: no executor record (state: %s)", session.State)
	}

	if err := validateSessionWorktrees(session); err != nil {
		return err
	}

	// Use context.WithoutCancel to prevent WebSocket request timeout from canceling the resume.
	resumeCtx := context.WithoutCancel(ctx)
	if _, err = s.executor.ResumeSession(resumeCtx, session, true); err != nil {
		if errors.Is(err, executor.ErrExecutionAlreadyRunning) {
			return nil // Agent is already running, nothing to do
		}
		s.updateTaskSessionState(ctx, session.TaskID, sessionID, models.TaskSessionStateFailed, err.Error(), false, session)
		if stateErr := s.taskRepo.UpdateTaskState(ctx, session.TaskID, v1.TaskStateFailed); stateErr != nil {
			s.logger.Warn("failed to update task state to FAILED after session ensure resume error",
				zap.String("task_id", session.TaskID),
				zap.String("session_id", sessionID),
				zap.Error(stateErr))
		}
		return fmt.Errorf("failed to resume session: %w", err)
	}

	// ResumeSession launches the agent asynchronously. Wait for it to finish
	// initializing before returning, so the caller can send a prompt immediately.
	if err := s.waitForSessionReady(ctx, sessionID); err != nil {
		return fmt.Errorf("session not ready after resume: %w", err)
	}

	s.logger.Debug("session resumed and ready for prompt")
	return nil
}

// startAgentOnPreparedWorkspace starts the agent subprocess on a session whose workspace
// was prepared (agentctl running) but whose agent process was never started. This avoids
// the "session already has an agent running" error from ResumeSession which tries a full
// LaunchAgent and conflicts with the existing execution in the lifecycle manager's store.
func (s *Service) startAgentOnPreparedWorkspace(ctx context.Context, sessionID string, session *models.TaskSession) error {
	s.logger.Debug("session has prepared workspace but no agent, starting agent on existing workspace",
		zap.String("session_id", sessionID),
		zap.String("agent_execution_id", session.AgentExecutionID))

	launchCtx := context.WithoutCancel(ctx)
	task, err := s.scheduler.GetTask(launchCtx, session.TaskID)
	if err != nil {
		return fmt.Errorf("failed to get task for prepared session: %w", err)
	}
	if _, err = s.executor.LaunchPreparedSession(launchCtx, task, sessionID, executor.LaunchOptions{
		AgentProfileID: session.AgentProfileID,
		ExecutorID:     session.ExecutorID,
		StartAgent:     true,
	}); err != nil {
		return fmt.Errorf("failed to start agent on prepared workspace: %w", err)
	}

	if err := s.waitForSessionReady(ctx, sessionID); err != nil {
		return fmt.Errorf("session not ready after starting agent: %w", err)
	}
	s.logger.Debug("agent started on prepared workspace and ready for prompt")
	return nil
}

// waitForSessionReady polls the session state until the agent is ready for prompts.
func (s *Service) waitForSessionReady(ctx context.Context, sessionID string) error {
	const (
		pollInterval = 500 * time.Millisecond
		maxWait      = 90 * time.Second
	)
	deadline := time.Now().Add(maxWait)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for agent to become ready")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
		sess, err := s.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to check session state: %w", err)
		}
		switch sess.State {
		case models.TaskSessionStateWaitingForInput:
			return nil
		case models.TaskSessionStateFailed:
			errMsg := sess.ErrorMessage
			if errMsg == "" {
				errMsg = "session failed during startup"
			}
			return fmt.Errorf("session failed: %s", errMsg)
		case models.TaskSessionStateCancelled, models.TaskSessionStateCompleted:
			return fmt.Errorf("session in unexpected state: %s", sess.State)
		}
	}
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

	running, runErr := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	resumeToken := ""
	if runErr == nil && running != nil {
		resumeToken = running.ResumeToken
		resp.ACPSessionID = running.ResumeToken
		resp.Runtime = running.Runtime
		if running.Resumable {
			resp.IsResumable = true
		}
		s.applyRemoteRuntimeStatus(ctx, sessionID, &resp)
	}

	if shouldHealStuckStartingSession(session, running) {
		s.logger.Info("healing stale STARTING session state from ready runtime status",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("agent_execution_id", session.AgentExecutionID))
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		refreshedSession, refreshErr := s.repo.GetTaskSession(ctx, sessionID)
		if refreshErr == nil && refreshedSession != nil {
			session = refreshedSession
			resp.State = string(session.State)
		}
	}

	// Extract worktree info
	populateWorktreeInfo(session, &resp)

	// 2. Check if this session's agent is running
	if exec, ok := s.executor.GetExecutionBySession(sessionID); ok && exec != nil {
		resp.IsAgentRunning = true
		resp.NeedsResume = false
		return resp, nil
	}

	// 3. Session can be resumed if it has a resume token
	if resumeToken != "" {
		// Don't auto-resume terminal sessions — they failed/completed for a reason.
		if !isActiveSessionState(session.State) {
			resp.IsAgentRunning = false
			resp.IsResumable = false
			resp.NeedsResume = false
			resp.NeedsWorkspaceRestore = canRestoreWorkspace(&resp)
			return resp, nil
		}
		return s.validateResumeEligibility(session, resp), nil
	}

	// 4. No resume token, but session may still be startable as a fresh session
	// if there's an ExecutorRunning record (worktree info) and session is in an active state.
	// NeedsResume triggers the frontend to auto-resume, which launches agentctl and the agent
	// process (idle, no prompt). For agents with HistoryContextInjection, conversation history
	// is injected into the user's first message.
	if runErr == nil && running != nil && isActiveSessionState(session.State) {
		resp.IsAgentRunning = false
		resp.IsResumable = true
		resp.NeedsResume = true
		resp.ResumeReason = "agent_not_running_fresh_start"
		return resp, nil
	}

	resp.IsAgentRunning = false
	resp.IsResumable = false
	resp.NeedsResume = false
	resp.NeedsWorkspaceRestore = canRestoreWorkspace(&resp)
	return resp, nil
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
	resp.IsRemoteExecutor = models.IsRemoteExecutorType(execModel.Type)
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
func canRestoreWorkspace(resp *dto.TaskSessionStatusResponse) bool {
	return resp != nil && resp.WorktreePath != nil && *resp.WorktreePath != ""
}

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

// isActiveSessionState returns true for session states where lazy resume makes sense.
func isActiveSessionState(state models.TaskSessionState) bool {
	switch state {
	case models.TaskSessionStateWaitingForInput,
		models.TaskSessionStateStarting,
		models.TaskSessionStateRunning:
		return true
	}
	return false
}

func shouldHealStuckStartingSession(session *models.TaskSession, running *models.ExecutorRunning) bool {
	if session == nil || running == nil {
		return false
	}
	if session.State != models.TaskSessionStateStarting {
		return false
	}
	if running.Status != "ready" {
		return false
	}
	if session.AgentExecutionID != "" && running.AgentExecutionID != "" && session.AgentExecutionID != running.AgentExecutionID {
		return false
	}
	return true
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

// DeleteSession deletes a session that is not currently running.
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Prevent deleting active sessions
	switch session.State {
	case models.TaskSessionStateRunning, models.TaskSessionStateStarting:
		return fmt.Errorf("cannot delete session in %s state — stop it first", session.State)
	}

	taskID := session.TaskID
	wasPrimary := session.IsPrimary

	s.logger.Info("deleting session",
		zap.String("session_id", sessionID),
		zap.String("task_id", taskID),
		zap.String("state", string(session.State)),
		zap.Bool("was_primary", wasPrimary))

	if err := s.repo.DeleteTaskSession(ctx, sessionID); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Auto-promote another session if we deleted the primary
	if wasPrimary {
		s.promoteNextPrimaryAfterRemoval(ctx, taskID, sessionID)
	}

	return nil
}

// promoteNextPrimaryAfterRemoval picks the best remaining session as primary
// after a session is deleted. Prefers RUNNING > active > any remaining.
func (s *Service) promoteNextPrimaryAfterRemoval(ctx context.Context, taskID, deletedSessionID string) {
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil || len(sessions) == 0 {
		return
	}
	var candidate string
	for _, sess := range sessions {
		if sess.ID == deletedSessionID {
			continue
		}
		if sess.State == models.TaskSessionStateRunning {
			candidate = sess.ID
			break
		}
		if candidate == "" {
			candidate = sess.ID
		} else if isActiveSessionState(sess.State) {
			// Prefer active over terminal
			candidate = sess.ID
		}
	}
	if candidate != "" {
		if err := s.SetPrimarySession(ctx, candidate); err != nil {
			s.logger.Warn("failed to auto-promote primary after delete",
				zap.String("task_id", taskID),
				zap.String("candidate", candidate),
				zap.Error(err))
		}
	}
}

// SetPrimarySession marks a session as the primary session for its task
// and broadcasts a task.updated event so the frontend reflects the change.
func (s *Service) SetPrimarySession(ctx context.Context, sessionID string) error {
	if err := s.repo.SetSessionPrimary(ctx, sessionID); err != nil {
		return fmt.Errorf("failed to set session as primary: %w", err)
	}

	// Broadcast task.updated so frontend updates the primary star indicator.
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to fetch session after setting primary", zap.Error(err))
		return nil
	}
	task, err := s.repo.GetTask(ctx, session.TaskID)
	if err != nil {
		s.logger.Warn("failed to fetch task after setting primary", zap.Error(err))
		return nil
	}
	if s.eventBus != nil {
		payload := buildTaskEventPayload(task)
		payload["primary_session_id"] = sessionID
		payload["primary_session_state"] = string(session.State)
		_ = s.eventBus.Publish(ctx, events.TaskUpdated, bus.NewEvent(
			events.TaskUpdated, "orchestrator", payload,
		))
	}
	return nil
}

// StopExecution stops agent execution for a specific execution ID.
func (s *Service) StopExecution(ctx context.Context, executionID string, reason string, force bool) error {
	s.logger.Info("stopping execution",
		zap.String("execution_id", executionID),
		zap.String("reason", reason),
		zap.Bool("force", force))
	return s.executor.StopExecution(ctx, executionID, reason, force)
}

// CaptureArchiveSnapshot captures git state (commits, cumulative diff) for a session before archiving.
// This preserves the final git state for historical purposes.
func (s *Service) CaptureArchiveSnapshot(ctx context.Context, sessionID string) error {
	s.logger.Info("capturing archive snapshot", zap.String("session_id", sessionID))

	baseCommit, err := s.resolveArchiveBaseCommit(ctx, sessionID)
	if err != nil {
		return err
	}
	if baseCommit == "" {
		s.logger.Debug("no base_commit available, skipping archive snapshot capture",
			zap.String("session_id", sessionID))
		return nil
	}

	if !s.captureArchiveCommits(ctx, sessionID, baseCommit) {
		// Agent not running, skip diff capture as well
		return nil
	}

	s.captureArchiveDiff(ctx, sessionID, baseCommit)
	return nil
}

// resolveArchiveBaseCommit retrieves the base commit for archive snapshot capture.
// It first checks the session's stored base_commit_sha, falling back to git status if empty.
func (s *Service) resolveArchiveBaseCommit(ctx context.Context, sessionID string) (string, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	if session.BaseCommitSHA != "" {
		return session.BaseCommitSHA, nil
	}

	// Fallback: try to get base commit from git status for legacy sessions
	status, err := s.agentManager.GetGitStatus(ctx, sessionID)
	if err != nil {
		s.logger.Debug("failed to get git status for base commit fallback",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return "", nil
	}
	if status != nil && status.BaseCommit != "" {
		s.logger.Debug("using git status base commit as fallback for archive",
			zap.String("session_id", sessionID),
			zap.String("base_commit", status.BaseCommit))
		return status.BaseCommit, nil
	}
	return "", nil
}

// captureArchiveCommits fetches and saves commits from baseCommit to HEAD.
// Returns false if the agent is not running (caller should skip remaining capture).
func (s *Service) captureArchiveCommits(ctx context.Context, sessionID, baseCommit string) bool {
	logResult, err := s.agentManager.GetGitLog(ctx, sessionID, baseCommit, 0) // 0 = no limit
	if err != nil {
		s.logger.Warn("failed to capture git log for archive",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return true // Continue with diff capture even if log fails
	}
	if logResult == nil {
		s.logger.Debug("agent not running, skipping archive snapshot capture",
			zap.String("session_id", sessionID))
		return false
	}
	if logResult.Success && len(logResult.Commits) > 0 {
		s.saveArchiveCommits(ctx, sessionID, logResult.Commits)
	}
	return true
}

// captureArchiveDiff fetches and saves the cumulative diff from baseCommit to the working tree
// (including uncommitted/unstaged changes).
func (s *Service) captureArchiveDiff(ctx context.Context, sessionID, baseCommit string) {
	diffResult, err := s.agentManager.GetCumulativeDiff(ctx, sessionID, baseCommit)
	if err != nil {
		s.logger.Warn("failed to capture cumulative diff for archive",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	if diffResult == nil || !diffResult.Success {
		return
	}

	if err := s.repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		SessionID:    sessionID,
		SnapshotType: models.SnapshotTypeArchive,
		HeadCommit:   diffResult.HeadCommit,
		BaseCommit:   diffResult.BaseCommit,
		Files:        diffResult.Files,
	}); err != nil {
		s.logger.Warn("failed to save archive snapshot",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	s.logger.Debug("saved archive snapshot",
		zap.String("session_id", sessionID),
		zap.String("head_commit", diffResult.HeadCommit),
		zap.Int("total_commits", diffResult.TotalCommits))
}

// parseCommitTime parses a commit timestamp from git log output.
// Returns UTC time to ensure consistent timestamps across environments.
func parseCommitTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}

// saveArchiveCommits persists commits to the database for archive purposes.
func (s *Service) saveArchiveCommits(ctx context.Context, sessionID string, commits []*client.GitCommitInfo) {
	for _, commit := range commits {
		if err := s.repo.CreateSessionCommit(ctx, &models.SessionCommit{
			SessionID:     sessionID,
			CommitSHA:     commit.CommitSHA,
			ParentSHA:     commit.ParentSHA,
			AuthorName:    commit.AuthorName,
			AuthorEmail:   commit.AuthorEmail,
			CommitMessage: commit.CommitMessage,
			CommittedAt:   parseCommitTime(commit.CommittedAt),
			FilesChanged:  commit.FilesChanged,
			Insertions:    commit.Insertions,
			Deletions:     commit.Deletions,
		}); err != nil {
			s.logger.Warn("failed to save commit for archive",
				zap.String("session_id", sessionID),
				zap.String("commit_sha", commit.CommitSHA),
				zap.Error(err))
		}
	}
	s.logger.Debug("saved archive commits",
		zap.String("session_id", sessionID),
		zap.Int("count", len(commits)))
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
	if s.isSessionResetInProgress(sessionID) {
		return nil, ErrSessionResetInProgress
	}

	// Only allow prompts when the session is ready for input.
	// Reject when the agent is still starting, already processing, or in a terminal state.
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	switch session.State {
	case models.TaskSessionStateWaitingForInput, models.TaskSessionStateCompleted:
		// OK — session is ready for a new prompt
	case models.TaskSessionStateRunning:
		s.logger.Warn("rejected prompt while agent is already running",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("session_state", string(session.State)))
		return nil, fmt.Errorf("%w, please wait for completion", ErrAgentPromptInProgress)
	default:
		s.logger.Warn("rejected prompt: session not ready for input",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("session_state", string(session.State)))
		return nil, fmt.Errorf("%w, session is in %s state", ErrAgentPromptInProgress, session.State)
	}

	// Ensure the agent process is actually running. After a lazy backend restart,
	// the session may be in WAITING_FOR_INPUT but no agent process exists yet.
	if err := s.ensureSessionRunning(ctx, sessionID, session); err != nil {
		return nil, fmt.Errorf("failed to ensure session is running: %w", err)
	}

	// Inject config context for config-mode sessions (dedicated settings chat, not plan mode)
	effectivePrompt := prompt
	if cm, ok := session.Metadata["config_mode"].(bool); ok && cm {
		effectivePrompt = sysprompt.InjectConfigContext(sessionID, prompt)
	}

	// Inject plan mode prefix for follow-up messages in plan mode sessions.
	if planMode {
		effectivePrompt = sysprompt.InjectPlanMode(effectivePrompt)
	}

	// Check if model switching is requested
	if result, switched, err := s.trySwitchModel(ctx, taskID, sessionID, model, effectivePrompt, session); switched || err != nil {
		return result, err
	}

	previousSessionState := session.State

	s.setSessionRunning(ctx, taskID, sessionID, session)
	s.startTurnForSession(ctx, sessionID)

	// Use context.WithoutCancel to prevent WebSocket request timeout from canceling the prompt.
	// Prompts can take a long time (minutes) while the WS request may timeout in 15 seconds.
	// We still want to log and respond, but the prompt should continue regardless.
	promptCtx := context.WithoutCancel(ctx)
	result, err := s.executor.Prompt(promptCtx, taskID, sessionID, effectivePrompt, attachments, session)
	if err != nil {
		expectedResetInterrupt := false
		if isTransientPromptError(err) && s.isSessionResetInProgress(sessionID) {
			s.logger.Warn("prompt interrupted by in-progress session reset; retry expected",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
			err = ErrSessionResetInProgress
			expectedResetInterrupt = true
		}

		if expectedResetInterrupt {
			s.logger.Warn("prompt deferred while session reset is in progress",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else {
			s.logger.Error("prompt failed",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
		// Revert session state so it doesn't stay stuck in RUNNING.
		// Use repo directly to bypass state machine guards that block transitions from terminal states.
		if revertErr := s.repo.UpdateTaskSessionState(ctx, sessionID, previousSessionState, ""); revertErr != nil {
			s.logger.Error("failed to revert session state after prompt error",
				zap.String("session_id", sessionID),
				zap.String("target_state", string(previousSessionState)),
				zap.Error(revertErr))
		}
		if !isTransientPromptError(err) {
			_ = s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateReview)
		}
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
	switchCtx := context.WithoutCancel(ctx)
	switchResult, err := s.executor.SwitchModel(switchCtx, taskID, sessionID, model, effectivePrompt)
	if err != nil {
		return nil, true, fmt.Errorf("model switch failed: %w", err)
	}
	if switchResult.StopReason == "model_switched_in_place" {
		// Agent is still running with the new model — let PromptTask send the prompt normally.
		// Invalidate the message creator's model cache so the next message picks up the new model.
		if s.messageCreator != nil {
			s.messageCreator.InvalidateModelCache(sessionID)
		}
		return nil, false, nil
	}
	s.startTurnForSession(ctx, sessionID)
	s.setSessionRunning(ctx, taskID, sessionID, session)
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
		s.setSessionRunning(ctx, session.TaskID, sessionID, session)
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
		s.updateTaskSessionState(ctx, session.TaskID, sessionID, models.TaskSessionStateWaitingForInput, "", true, session)
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

// ResetAgentContext resets the agent's conversation context for a session,
// clearing conversation history while preserving the workspace environment.
func (s *Service) ResetAgentContext(ctx context.Context, sessionID string) error {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		return fmt.Errorf("agent must be idle to reset context, current state: %s", session.State)
	}
	if session.AgentExecutionID == "" {
		return fmt.Errorf("no active agent execution for session %s", sessionID)
	}

	// Set STARTING so frontend disables input and shows restarting state
	s.updateTaskSessionState(ctx, session.TaskID, sessionID, models.TaskSessionStateStarting, "", false, session)

	if ok := s.resetAgentContext(ctx, session.TaskID, session, "user_request"); !ok {
		// Restore WAITING_FOR_INPUT on failure
		s.setSessionWaitingForInput(ctx, session.TaskID, sessionID)
		return fmt.Errorf("failed to reset agent context for session %s", sessionID)
	}

	// Restore WAITING_FOR_INPUT — handleAgentReady ignores events during reset
	s.setSessionWaitingForInput(ctx, session.TaskID, sessionID)

	if s.messageCreator != nil {
		if err := s.messageCreator.CreateSessionMessage(
			ctx, session.TaskID,
			"Context reset — new conversation started",
			sessionID, string(v1.MessageTypeStatus),
			s.getActiveTurnID(sessionID),
			nil, false,
		); err != nil {
			s.logger.Warn("failed to create context reset message",
				zap.String("session_id", sessionID), zap.Error(err))
		}
	}
	return nil
}

// GetQueuedTasks returns tasks in the queue
func (s *Service) GetQueuedTasks() []*queue.QueuedTask {
	return s.queue.List()
}
