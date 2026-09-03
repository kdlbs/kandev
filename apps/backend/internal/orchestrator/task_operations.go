// Package orchestrator provides the main orchestrator service that ties all components together.
package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/editors/capabilities"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/dto"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
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
	TurnID       string // The exact turn accepted for this prompt, when known.
}

// CoordinatorTaskStopStatus is the idempotent product result returned to a
// parent task that asks to halt a direct child.
type CoordinatorTaskStopStatus string

const (
	CoordinatorTaskStopStatusStopped    CoordinatorTaskStopStatus = "stopped"
	CoordinatorTaskStopStatusNotRunning CoordinatorTaskStopStatus = "not_running"
	coordinatorMCPStopReason                                      = "stopped by parent task via MCP"
)

// CoordinatorTaskStopResult reports whether at least one live child session
// accepted logical cancellation. Runtime teardown continues asynchronously.
type CoordinatorTaskStopResult struct {
	Status CoordinatorTaskStopStatus `json:"status"`
}

// resumeReasonErrorRecovery is the resume reason returned when a session is in
// error-recovery state (WAITING_FOR_INPUT with a non-empty ErrorMessage).
const resumeReasonErrorRecovery = "error_recovery"

// resumeReasonFailedSessionResumable is the resume reason returned when a
// FAILED session is auto-resumed because its runtime is Resumable. Distinct
// from "agent_not_running" so log filtering can isolate FAILED auto-resumes.
const resumeReasonFailedSessionResumable = "failed_session_resumable"

// resumeReasonArchiveCancelledResumable is the resume reason returned when a
// session cancelled by archiving (see models.IsArchiveCancelReason) is
// resumable once its task is unarchived — the cancellation was a system side
// effect of Service.ArchiveTask / HandoffService's cascade archive, not an
// explicit user/coordinator stop, so it must recover like a FAILED session
// instead of staying stuck on read-only workspace restore.
const resumeReasonArchiveCancelledResumable = "archive_cancelled_resumable"

var ErrAgentPromptInProgress = errors.New("agent is currently processing a prompt")
var ErrAgentNotReadyForPrompt = errors.New("agent not ready for prompt")
var ErrSessionResetInProgress = errors.New("session reset in progress")

type primarySessionTaskStateUpdater interface {
	UpdateTaskStateIfPrimarySessionState(
		ctx context.Context,
		taskID, sessionID string,
		expectedSessionState models.TaskSessionState,
		state v1.TaskState,
	) (bool, error)
}

// dynamicRouteStateLoader exposes the durable route row without coupling the
// orchestrator to the concrete task repository. The route row is authoritative
// when a generation claim committed before task-session attribution could be
// written; launch reconciliation repairs the session from that row.
type dynamicRouteStateLoader interface {
	LoadRouteState(context.Context, string) (*dynamicruntime.RouteState, error)
}

// ErrSessionNotPromptable is returned when a session cannot accept a prompt
// because of its lifecycle state (STARTING, CREATED, FAILED, CANCELLED).
// Distinct from ErrAgentPromptInProgress, which is RUNNING-only — confusing
// the two misleads the UI and any caller doing errors.Is checks.
var ErrSessionNotPromptable = errors.New("session not promptable")

// errQueuedDispatchSuperseded is returned by promptTask when a queued
// message's dispatch loses its claim (see isCurrentQueuedDispatch) to a
// newer dispatch for the same session — e.g. a second parent interrupt
// cancelling and re-taking while the first dispatch's own goroutine was
// still settling. Not surfaced to callers of the public PromptTask; only
// executeQueuedMessage's internal call passes a claim token that can
// trigger this.
var errQueuedDispatchSuperseded = errors.New("queued dispatch superseded by a newer one for this session")

var (
	// Backend restart recovery can restore the session state before the ACP
	// stream is promptable again. Keep this above CI's slow-start tail so a
	// valid resume waits instead of surfacing "Failed to send message to agent".
	agentPromptReadyTimeout  = 30 * time.Second
	agentPromptReadyInterval = 100 * time.Millisecond
)

const promptFailureCleanupTimeout = 5 * time.Second

const promptReadinessRecoveryStopReason = "prompt readiness recovery"

type agentPromptStreamRecoverer interface {
	RecoverAgentPromptStream(ctx context.Context, sessionID string) error
}

func isAgentPromptInProgressError(err error) bool {
	return err != nil && errors.Is(err, ErrAgentPromptInProgress)
}

// isSessionBusyError reports whether the session is in a state where a queued
// or auto-started prompt should be retried later rather than dropped. Covers
// both "the agent is mid-turn" (ErrAgentPromptInProgress, RUNNING) and "the
// session isn't yet ready to accept input" (ErrSessionNotPromptable —
// STARTING, CREATED, FAILED, CANCELLED). The pre-PR code path collapsed both
// into ErrAgentPromptInProgress; this helper preserves the requeue behaviour
// after the error split so queued messages targeting a session that is
// briefly STARTING/CREATED don't get silently dropped (see TODO about the
// missing dead-letter queue in executeQueuedMessage).
func isSessionBusyError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrAgentPromptInProgress) || errors.Is(err, ErrSessionNotPromptable)
}

func isSessionResetInProgressError(err error) bool {
	return err != nil && errors.Is(err, ErrSessionResetInProgress)
}

// isTransientPromptError reports whether a prompt error is worth retrying via
// the queue. ErrExecutionNotFound is intentionally NOT included here:
// callers that can recover (autoStartStepPrompt → fallbackFreshLaunchOnMissingExecution)
// detect it explicitly via errors.Is and route differently; callers that
// can't (executeQueuedMessage) should not infinite-requeue on it. Treating
// "execution not found" as transient blanket-applies a retry that loops
// forever when the execution is genuinely gone.
func isTransientPromptError(err error) bool {
	if err == nil {
		return false
	}
	var pendingCompletionTimeout *lifecycle.PendingDispatchedPromptTimeoutError
	if errors.As(err, &pendingCompletionTimeout) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "agent stream disconnected") ||
		strings.Contains(msg, "use of closed network connection")
}

// isManualRecoveryPromptError identifies a provider error that should retain
// the queued prompt until the user explicitly resumes the session. It is
// intentionally separate from isTransientPromptError: exhausted usage does
// not become available on a short automatic retry, and repeatedly attempting
// it would consume the queue's retry budget without making progress.
func isManualRecoveryPromptError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "usagelimitexceeded") ||
		strings.Contains(message, "you've hit your usage limit")
}

func isAgentAlreadyRunningError(err error) bool {
	return err != nil && errors.Is(err, lifecycle.ErrAgentAlreadyRunning)
}

// missingSessionWorktreePaths returns the session's recorded worktree
// directories that are no longer on disk.
func missingSessionWorktreePaths(session *models.TaskSession) []string {
	var missing []string
	for _, wt := range session.Worktrees {
		if wt == nil || wt.WorktreePath == "" {
			continue
		}
		if _, err := os.Stat(wt.WorktreePath); err != nil {
			missing = append(missing, wt.WorktreePath)
		}
	}
	return missing
}

// noteMissingWorktreesBeforeResume records that a resume is about to go
// through the worktree manager's recreate path.
//
// A missing directory is NOT a resume blocker. Archive cleanup deletes the
// worktree directory while keeping the environment repository row and the git
// branch (DestroyWorktree passes removeBranch=false), so *every* resume after
// an unarchive observes exactly this shape. worktree.Manager.Create reuses the
// stored record, restores the branch — locally, or by fetching it back from
// origin — and rebuilds the directory at the same path, which is what the
// recreate path was written for. Rejecting the resume here instead made
// unarchive a dead end: the session could never be resumed and the workspace
// was never restored, even though the work itself was still intact.
func (s *Service) noteMissingWorktreesBeforeResume(sessionID string, session *models.TaskSession) {
	missing := missingSessionWorktreePaths(session)
	if len(missing) == 0 {
		return
	}
	s.logger.Info("worktree directory missing on resume; it will be recreated from the stored branch",
		zap.String("session_id", sessionID),
		zap.Strings("paths", missing))
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
// continues in the background. Use StartCreatedSession to continue with agent launch.
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
		if v, ok := task.Metadata[models.MetaKeyAgentProfileID].(string); ok && v != "" {
			agentProfileID = v
		}
	}
	if executorID == "" {
		if v, ok := task.Metadata[models.MetaKeyExecutorID].(string); ok && v != "" {
			executorID = v
		}
	}
	if executorProfileID == "" {
		if v, ok := task.Metadata[models.MetaKeyExecutorProfileID].(string); ok && v != "" {
			executorProfileID = v
		}
	}

	// Inherit agent/executor profile from parent task's primary session when not
	// explicitly provided. This covers subtasks created with start_agent=false that
	// are later opened manually from the UI. We check each field independently so
	// that a caller providing only some fields still gets the rest filled in.
	if (agentProfileID == "" || executorProfileID == "" || executorID == "") && task.ParentID != "" {
		agentProfileID, executorProfileID, executorID = s.inheritFromParentSession(
			ctx, task.ParentID, agentProfileID, executorProfileID, executorID,
		)

		// Fall back to workspace defaults for agent profile (subtasks only —
		// regular tasks resolve defaults downstream in the executor layer).
		if agentProfileID == "" {
			workspace, err := s.repo.GetWorkspace(ctx, task.WorkspaceID)
			if err == nil && workspace != nil && workspace.DefaultAgentProfileID != nil && *workspace.DefaultAgentProfileID != "" {
				agentProfileID = *workspace.DefaultAgentProfileID
			}
		}
		// An inherit_parent child must keep the parent's effective executor. An
		// empty executor here is meaningful: it selects the local executor when
		// the parent also used the workspace default. For other subtasks, keep
		// the historical worktree default so they receive an isolated checkout.
		if executorID == "" && executorProfileID == "" && !isInheritParentWorkspace(task) {
			executorID = models.ExecutorIDWorktree
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
	if s.profileExecutionResolver != nil {
		if err := s.profileExecutionResolver.ValidateProfile(ctx, agentProfileID); err != nil {
			return "", err
		}
	}

	// Create session entry in database. Office tasks route through
	// EnsureSessionForAgent so runs + advanced-mode reuse one row.
	// prepareSessionForStart also propagates any inherited workspace
	// environment (inherit_parent / shared_group) onto the new session.
	// A manual prepare call has no scheduler-owned Office run identity. Keep the
	// participant slot empty so createStartSession uses the task assignee, while
	// agentProfileID remains the concrete execution profile.
	sessionID, sessionCreated, err := s.prepareSessionForStart(ctx, task, agentProfileID, "", executorID, executorProfileID, workflowStepID)
	if err != nil {
		s.logger.Error("failed to prepare session",
			zap.String("task_id", taskID),
			zap.Error(err))
		return "", err
	}

	// Notify the frontend that a new CREATED session exists. The start path
	// transitions through updateTaskSessionState which broadcasts; the prepare
	// path writes the row directly, so without this the per-task session list
	// stays empty until a manual reload.
	if sessionCreated {
		s.publishSessionCreatedEvent(ctx, taskID, sessionID, workflowStepID)
	}

	if launchWorkspace {
		// Launch workspace infrastructure (agentctl) in the background so the WS response
		// returns the session ID immediately. The frontend navigates to the session page
		// and shows preparation progress via executor.prepare.progress WS events.
		go func() {
			bgCtx := context.Background()
			launchProfileID := agentProfileID
			if s.profileExecutionResolver != nil {
				launchSession, loadErr := s.repo.GetTaskSession(bgCtx, sessionID)
				if loadErr != nil {
					s.logger.Warn("failed to reload prepared session for dynamic route resolution",
						zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(loadErr))
					_ = s.handleSessionLaunchFailure(bgCtx, taskID, sessionID, loadErr)
					return
				}
				resolved, routeClaimed, resolveErr := s.resolveExecutionForLaunchSession(bgCtx, launchSession)
				if resolveErr != nil {
					s.recordDynamicRouteResolutionFailure(bgCtx, launchSession, resolveErr)
					s.logger.Warn("failed to resolve dynamic route for prepared session",
						zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(resolveErr))
					_ = s.handleSessionLaunchFailure(bgCtx, taskID, sessionID, resolveErr, launchSession)
					return
				}
				if resolved.ExecutionProfileID != "" {
					launchProfileID = resolved.ExecutionProfileID
					if routeClaimed {
						applyResolvedExecution(launchSession, resolved)
						if updateErr := s.repo.UpdateTaskSession(bgCtx, launchSession); updateErr != nil {
							s.logger.Warn("failed to persist dynamic route for prepared session",
								zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(updateErr))
							_ = s.handleSessionLaunchFailure(bgCtx, taskID, sessionID, updateErr, launchSession)
							return
						}
					}
				}
			}
			prepExec, launchErr := s.executor.LaunchPreparedSession(bgCtx, task, sessionID, executor.LaunchOptions{AgentProfileID: launchProfileID, ExecutorID: executorID, WorkflowStepID: workflowStepID})
			if launchErr != nil {
				// LaunchAgent failures persist FAILED in the executor. Earlier
				// workspace failures return here and are recorded through the same
				// session-level recovery claim.
				launchErr = s.handleSessionLaunchFailure(bgCtx, taskID, sessionID, launchErr)
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

func isInheritParentWorkspace(task *v1.Task) bool {
	if task == nil {
		return false
	}
	mode, _ := workspacePolicyMode(task.Metadata)
	return mode == "inherit_parent"
}

// StartCreatedSession starts agent execution for a task using a session that is in CREATED state.
// This is used when a session was prepared (via PrepareSession) but the agent was not launched,
// and the user now wants to start the agent with a prompt (e.g., from the plan panel or chat).
// When skipMessageRecord is true, only the session state is updated (the caller already stored the user message).
// When planMode is true, plan mode instructions are injected into the prompt and session metadata is set.
// autoStart marks the launch as having been triggered by an automated path
// (only consumed when skipMessageRecord is false — callers that store their
// own message control its metadata directly).
// references contains validated entity references whose exact server-generated
// context block may survive first-turn canonicalization.
//
//nolint:cyclop,funlen,gocognit // Existing complexity inherited from session-lifecycle handling.
func (s *Service) StartCreatedSession(
	ctx context.Context,
	taskID, sessionID, agentProfileID, prompt string,
	skipMessageRecord, planMode, autoStart bool,
	attachments []v1.MessageAttachment,
	references []v1.EntityReference,
) (*executor.TaskExecution, error) {
	return s.startCreatedSession(
		ctx, taskID, sessionID, agentProfileID, prompt,
		skipMessageRecord, planMode, autoStart, attachments, references, "",
	)
}

// StartCreatedSessionWithPromptContext starts a prepared session while
// preserving the exact server-generated saved-prompt context prepared by the
// message handler. Direct first turns need this narrow seam because the
// session-start path canonicalizes the prompt again before dispatch.
func (s *Service) StartCreatedSessionWithPromptContext(
	ctx context.Context,
	taskID, sessionID, agentProfileID, prompt string,
	skipMessageRecord, planMode, autoStart bool,
	attachments []v1.MessageAttachment,
	references []v1.EntityReference,
	promptReferenceContext string,
) (*executor.TaskExecution, error) {
	return s.startCreatedSession(
		ctx, taskID, sessionID, agentProfileID, prompt,
		skipMessageRecord, planMode, autoStart, attachments, references, promptReferenceContext,
	)
}

//nolint:cyclop,funlen,gocognit // Existing complexity inherited from session-lifecycle handling.
func (s *Service) startCreatedSession(
	ctx context.Context,
	taskID, sessionID, agentProfileID, prompt string,
	skipMessageRecord, planMode, autoStart bool,
	attachments []v1.MessageAttachment,
	references []v1.EntityReference,
	promptReferenceContext string,
) (*executor.TaskExecution, error) {
	releaseLifecycleLock := s.acquireSessionLifecycleLock(sessionID)
	defer releaseLifecycleLock()

	// One GetWorkflowMeta read shared by profile resolution and prompt build.
	ctx = withWorkflowMetaCache(ctx)

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

	// Office-owned sessions must be started by the scheduler. Reject before
	// resolving profiles or persisting any caller-derived session metadata.
	isOfficeTask, err := s.lookupOfficeTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to determine office task status: %w", err)
	}
	if isOfficeTask {
		return nil, errOfficeTaskStartRequiresScheduler
	}

	// Reserve any pending "start it later" intent for the rest of this start.
	// Taken here rather than just before the launch: everything below persists
	// session state this start owns, and a gate that claims the intent during
	// that work launches its own session alongside it. Deferred release covers
	// every failure path; see deferredLaunchClaim.
	launchClaim := s.claimDeferredLaunchForStart(ctx, taskID)
	defer launchClaim.releaseIfHeld(ctx)

	// Use agent profile from request, fall back to session's stored value.
	effectiveProfileID := agentProfileID
	if effectiveProfileID == "" {
		effectiveProfileID = session.AgentProfileID
	}

	// Resolve the workflow step override / workflow default before the
	// required-profile guard, so a session without its own agent_profile_id
	// inherits the workflow's default agent. resolveEffectiveAgentProfile keeps
	// the caller profile only when neither a step override nor a workflow
	// default applies; either of those overrides a non-empty caller.
	effectiveProfileID = s.resolveEffectiveAgentProfile(ctx, taskID, "", effectiveProfileID)

	if effectiveProfileID == "" {
		return nil, fmt.Errorf("agent_profile_id is required")
	}
	if s.profileExecutionResolver != nil {
		if err := s.profileExecutionResolver.ValidateProfile(ctx, effectiveProfileID); err != nil {
			return nil, err
		}
	}

	// If the workflow step overrode the profile, update the session record in DB
	// so the frontend tab displays the correct agent (it reads session.agent_profile_id).
	if effectiveProfileID != session.AgentProfileID {
		observedState := session.State
		s.logger.Info("updating session agent profile for workflow step override",
			zap.String("session_id", sessionID),
			zap.String("old_profile", session.AgentProfileID),
			zap.String("new_profile", effectiveProfileID))
		session.AgentProfileID = effectiveProfileID
		// Re-resolve the agent profile snapshot so the tab shows the correct agent logo/name.
		// Set a minimal snapshot first so stale data is never persisted if resolution fails.
		session.AgentProfileSnapshot = map[string]interface{}{"id": effectiveProfileID}
		if profileInfo, err := s.agentManager.ResolveAgentProfile(ctx, effectiveProfileID); err != nil {
			s.logger.Warn("failed to resolve agent profile snapshot for workflow step override",
				zap.String("session_id", sessionID),
				zap.String("profile_id", effectiveProfileID),
				zap.Error(err))
		} else if profileInfo != nil {
			session.AgentProfileSnapshot = map[string]interface{}{
				"id":             profileInfo.ProfileID,
				"name":           profileInfo.ProfileName,
				"agent_id":       profileInfo.AgentID,
				"agent_name":     profileInfo.AgentName,
				"model":          profileInfo.Model,
				"mode":           profileInfo.Mode,
				"config_options": maps.Clone(profileInfo.ConfigOptions),
			}
		}
		if err := s.persistFullTaskSessionIfCurrent(ctx, session, observedState); err != nil {
			return nil, fmt.Errorf("persist workflow session profile: %w", err)
		}
		// Tag as workflow-spawned provenance only after the guarded profile
		// write succeeds; a concurrent stop owns a rejected session.
		s.tagSessionAsWorkflowSwitched(ctx, sessionID)
		s.promoteSessionIfTaskHasNoPrimary(ctx, taskID, session)
	}

	// Transition task state: CREATED → SCHEDULING → (IN_PROGRESS via executor).
	if err := s.scheduleTaskForSession(ctx, taskID, sessionID); err != nil {
		s.logger.Warn("failed to update task state to SCHEDULING",
			zap.String("task_id", taskID),
			zap.Error(err))
		return nil, err
	}

	task, err := s.scheduler.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	effectivePrompt := prompt
	if effectivePrompt == "" {
		effectivePrompt = task.Description
	}

	// NOTE: on_turn_start is intentionally NOT processed here.
	//   - User-initiated path: dispatchPromptAsync (message_handlers.go) already
	//     calls ProcessOnTurnStart before invoking StartCreatedSession via
	//     forwardMessageAsPrompt, so on_turn_start has already fired.
	//   - Workflow auto-start path: autoStartStepPrompt calls us because the
	//     workflow just transitioned us into this step (via on_turn_complete or
	//     on_enter). Firing on_turn_start again here cascades the workflow back
	//     out before the step's auto-start prompt can be delivered to its agent.
	//
	// However, for the user-initiated path, on_turn_start may have switched the
	// session profile already, in which case the session ID we were called with
	// is now COMPLETED and we need to redirect to the new active session.
	session, err = s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload session: %w", err)
	}
	if session.State == models.TaskSessionStateCompleted {
		activeSession, activeErr := s.repo.GetActiveTaskSessionByTaskID(ctx, taskID)
		if activeErr != nil || activeSession == nil {
			return nil, fmt.Errorf("session was switched but no active session found: %w", activeErr)
		}
		session = activeSession
		sessionID = activeSession.ID
		effectiveProfileID = activeSession.AgentProfileID
	}

	if effectiveProfileID, err = s.resolveDynamicLaunchExecution(ctx, session, effectiveProfileID, true); err != nil {
		return nil, err
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
	configMode := false
	if cm, ok := session.Metadata["config_mode"].(bool); ok && cm {
		configMode = true
	}
	titleOwner := false
	if !configMode {
		titleOwner, err = s.ClaimTaskTitleSession(ctx, taskID, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to claim first-turn task title: %w", err)
		}
	}
	effectivePrompt, planModeActive, generatedPromptReferenceContext := s.applyWorkflowAndPlanModeWithPromptContext(
		ctx, effectivePrompt, taskID, sessionID, dbTask.WorkflowStepID,
		planMode, task.IsEphemeral, session.IsPassthrough, promptReferenceContext,
	)
	if generatedPromptReferenceContext != "" {
		// The workflow helper preserves a direct message's acceptance-time
		// context. Auto-started workflow prompts have no prior context, so their
		// launch-time expansion becomes the trusted value here.
		promptReferenceContext = generatedPromptReferenceContext
	}

	// Inject config context for config-mode sessions (dedicated settings chat)
	if configMode {
		effectivePrompt = sysprompt.InjectConfigContext(sessionID, effectivePrompt)
	}

	// Wrap the first prompt with the Kandev MCP system block. See the
	// matching block in startTask for the rationale (DB stores wrapped form;
	// Message.ToAPI strips for display). The injectors canonicalize any upstream
	// wrap from current server state before launch.
	// Passthrough profiles skip the wrap: the prompt is typed straight into the
	// agent CLI's TTY and the user sees it verbatim — they don't want a wall of
	// MCP-tool boilerplate prepended to "hello".
	if effectivePrompt != "" || len(attachments) > 0 {
		effectivePrompt = s.wrapCreatedSessionPrompt(
			ctx, effectivePrompt, taskID, sessionID, session, dbTask,
			isOfficeTask, configMode, titleOwner, references, promptReferenceContext,
		)
	}

	executorID := session.ExecutorID

	// Cache the raw prompt so a transient-provider-error (529) retry can
	// re-drive this first turn — initial launches bypass PromptTask.
	s.rememberTurnPrompt(sessionID, prompt, "", planMode, attachments)

	mcpMode := ""
	if isOfficeTask {
		mcpMode = executor.McpModeOffice
	}
	initialTurnID, initialTurnCreated := s.startTurnForSessionWithOwnership(ctx, sessionID)
	execution, err := s.launchPreparedSessionWithDynamicFallback(ctx, task, sessionID, executor.LaunchOptions{AgentProfileID: effectiveProfileID, ExecutorID: executorID, Prompt: effectivePrompt, StartAgent: true, McpMode: mcpMode, Attachments: attachments, TurnID: initialTurnID})
	if err != nil {
		// The executor persists LaunchAgent failures. Cover earlier prepared-session
		// failures here; the session-level claim makes either completion order safe.
		if initialTurnCreated {
			s.completeTurnIfCurrent(ctx, sessionID, initialTurnID)
		}
		return nil, s.handleSessionLaunchFailure(ctx, taskID, sessionID, err)
	}

	// Record the initial user message and set plan mode metadata after launch.
	// Note: we do NOT set session state here — the executor sets it to STARTING,
	// and event handlers (handleAgentReady) transition it to WAITING_FOR_INPUT.
	s.postLaunchCreated(ctx, taskID, sessionID, effectivePrompt, skipMessageRecord, planModeActive, autoStart, attachments)

	// The agent is running, so the reservation becomes a consumption.
	launchClaim.consume(ctx)

	// Ensure a PR watch exists so the poller can detect PRs created by the agent.
	// PrepareTaskSession may have already created one, but if that goroutine failed
	// or hadn't completed, this guarantees coverage.
	go s.ensureSessionPRWatch(context.Background(), taskID, execution.SessionID, execution.WorktreeBranch)

	return execution, nil
}

// wrapCreatedSessionPrompt adds the first-turn context for a prepared session.
// Passthrough sessions intentionally receive only the short pending-title
// instruction; structured sessions receive the normal task or Office block.
func (s *Service) wrapCreatedSessionPrompt(
	ctx context.Context,
	prompt, taskID, sessionID string,
	session *models.TaskSession,
	dbTask *models.Task,
	isOfficeTask, configMode, titleOwner bool,
	references []v1.EntityReference,
	promptReferenceContext string,
) string {
	prompt, pullRequestTargetContext := s.addTaskPullRequestTargetContext(
		ctx, taskID, prompt, session.IsPassthrough,
	)
	referenceContext := EntityReferenceContext(references)
	switch {
	case session.IsPassthrough:
		if !configMode && titleOwner {
			return sysprompt.PendingTaskTitlePassthroughInstruction() + "\n\n" + prompt
		}
		return prompt
	case isOfficeTask:
		return sysprompt.InjectOfficeContextWithOptions(
			taskID, sessionID, prompt,
			s.WorkflowStepRequiresCompletionSignal(ctx, dbTask.WorkflowStepID),
			referenceContext, promptReferenceContext, pullRequestTargetContext,
		)
	default:
		return sysprompt.InjectKandevContextWithOptions(taskID, sessionID, prompt, sysprompt.KandevContextOptions{
			RequiresCompletionSignal:       s.WorkflowStepRequiresCompletionSignal(ctx, dbTask.WorkflowStepID),
			IncludeCoordinatorTaskControls: !configMode,
			IncludeTaskTitleTool:           !configMode && titleOwner,
			Autopilot:                      dbTask.Autopilot,
			IncludeUserQuestionTool:        !dbTask.Autopilot && !session.IsPassthrough,
			IncludeParentQuestionTool:      dbTask.Autopilot && dbTask.ParentID != "",
		}, referenceContext, promptReferenceContext, pullRequestTargetContext)
	}
}

// handleSessionLaunchFailure covers launch and resume failures that have not
// already won terminal bookkeeping. The state CAS makes it safe as an
// idempotent fallback when executor bookkeeping did run.
func (s *Service) handleSessionLaunchFailure(
	ctx context.Context,
	taskID, sessionID string,
	launchErr error,
	preloadedSession ...*models.TaskSession,
) error {
	failureCtx := context.WithoutCancel(ctx)
	safeErr := routingerr.SanitizeError(launchErr)
	s.recordSessionLaunchFailure(
		failureCtx, taskID, sessionID, safeErr, preloadedSession...,
	)
	return safeErr
}

// recordSessionLaunchFailure transitions a still-active session to FAILED and
// updates the task only while the same failed session still owns it. Typed
// launch-error persistence belongs to the executor's session transition.
func (s *Service) recordSessionLaunchFailure(ctx context.Context, taskID, sessionID string, launchErr error, preloadedSession ...*models.TaskSession) {
	_, changed := s.updateTaskSessionStateWithHook(
		ctx, taskID, sessionID, models.TaskSessionStateFailed, launchErr.Error(), false,
		nil, preloadedSession...,
	)
	if !changed {
		return
	}
	updated, err := s.updateTaskStateForEarlyLaunchFailure(ctx, taskID, sessionID)
	if err != nil {
		s.logger.Warn("failed to update task state to FAILED after early launch error",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	if !updated {
		return
	}
	s.processParentChildrenCompletedForTaskState(ctx, taskID, v1.TaskStateFailed)
}

func (s *Service) updateTaskStateForEarlyLaunchFailure(
	ctx context.Context,
	taskID, sessionID string,
) (bool, error) {
	if updater, ok := s.taskRepo.(primarySessionTaskStateUpdater); ok {
		return updater.UpdateTaskStateIfPrimarySessionState(
			ctx, taskID, sessionID, models.TaskSessionStateFailed, v1.TaskStateFailed,
		)
	}
	// Lightweight test repositories predate the primary-aware extension.
	return s.taskRepo.UpdateTaskStateIfSessionState(
		ctx, taskID, sessionID, models.TaskSessionStateFailed, v1.TaskStateFailed,
	)
}

func (s *Service) reconcileTaskStateForEarlyLaunchFailure(
	ctx context.Context,
	taskID, sessionID string,
	state v1.TaskState,
) error {
	if state != v1.TaskStateFailed {
		return fmt.Errorf("unsupported early launch task state %q", state)
	}
	updated, err := s.updateTaskStateForEarlyLaunchFailure(ctx, taskID, sessionID)
	if err != nil || !updated {
		return err
	}
	s.processParentChildrenCompletedForTaskState(ctx, taskID, state)
	return nil
}

func (s *Service) scheduleTaskForSession(ctx context.Context, taskID, sessionID string) error {
	s.taskRuntimeStateMu.Lock()
	defer s.taskRuntimeStateMu.Unlock()

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return fmt.Errorf("%w: agent session not found: %s", models.ErrTaskSessionNotFound, sessionID)
	}
	if session.State != models.TaskSessionStateCreated &&
		session.State != models.TaskSessionStateWaitingForInput {
		if isTerminalSessionState(session.State) {
			return &executor.SessionStateSupersededError{SessionID: session.ID, State: session.State}
		}
		return fmt.Errorf("session %s is %s; cannot schedule task", session.ID, session.State)
	}
	return s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateScheduling)
}

func (s *Service) promoteSessionIfTaskHasNoPrimary(ctx context.Context, taskID string, session *models.TaskSession) {
	if session == nil || session.IsPrimary {
		return
	}
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to inspect task sessions for missing primary",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		return
	}
	for _, existing := range sessions {
		if existing.IsPrimary {
			return
		}
	}
	if err := s.SetPrimarySession(ctx, session.ID); err != nil {
		s.logger.Warn("failed to promote workflow session as primary",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		return
	}
	session.IsPrimary = true
}

// postLaunchCreated handles post-launch bookkeeping for a created session:
// records the initial user message (unless skipped) and sets plan mode metadata.
// It does NOT modify session state — the executor sets STARTING, and event handlers
// (handleAgentReady) handle the transition to WAITING_FOR_INPUT.
// autoStart marks the message as having been created by an automated trigger
// (preserved through to recordInitialMessage's auto_start metadata tag); the
// flag is only consumed when skipMessage is false (callers that store their
// own message control its metadata directly).
func (s *Service) postLaunchCreated(ctx context.Context, taskID, sessionID, prompt string, skipMessage, planModeActive, autoStart bool, attachments []v1.MessageAttachment) {
	if !skipMessage {
		s.recordInitialMessage(ctx, taskID, sessionID, prompt, planModeActive, autoStart, attachments)
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
// autoStart marks the launch as having been triggered by an automated path
// (PR/issue/Jira/Linear watch, workflow auto-start) rather than direct user
// input — the seed prompt is tagged so the github cleanup loop can tell
// "agent ran on its own" from "user actually engaged".
func (s *Service) StartTask(ctx context.Context, taskID string, agentProfileID string, executorID string, executorProfileID string, priority string, prompt string, workflowStepID string, planMode, autoStart bool, attachments []v1.MessageAttachment) (*executor.TaskExecution, error) {
	return s.startTask(ctx, taskID, agentProfileID, executorID, executorProfileID, priority, prompt, workflowStepID, planMode, autoStart, attachments, startTaskOptions{})
}

// StartTaskWithEnv starts a task and carries launch-scoped environment variables
// through to the agent runtime. Existing StartTask callers keep the old behavior.
func (s *Service) StartTaskWithEnv(ctx context.Context, taskID string, agentProfileID string, executorID string, executorProfileID string, priority string, prompt string, workflowStepID string, planMode, autoStart bool, attachments []v1.MessageAttachment, env map[string]string) (*executor.TaskExecution, error) {
	return s.startTask(ctx, taskID, agentProfileID, executorID, executorProfileID, priority, prompt, workflowStepID, planMode, autoStart, attachments, startTaskOptions{Env: env})
}

// startTaskOptions carries the optional, server-only launch inputs that only
// some callers supply. Keeping them in one struct avoids growing startTask's
// already long positional parameter list for every new orthogonal concern.
type startTaskOptions struct {
	// ProfileExplicit marks a non-empty profile selected through an explicit
	// selector-backed choice. It bypasses workflow-step profile resolution for
	// this new session.
	ProfileExplicit bool
	// Env holds launch-scoped environment variables for the agent runtime.
	Env map[string]string
	// Route pins a concrete execution profile chosen by Office provider routing.
	Route *executor.RouteOverride
	// SpawnOrigin is set when another agent session spawned this launch; it
	// produces the spawner-attribution system block on the first turn.
	SpawnOrigin *SpawnOrigin
}

// StartTaskWithRoute launches a stable Office identity through a complete
// concrete execution profile selected by the routing dispatcher.
//
// launch carries the Office-built launch context (prompt, env, workflow
// step, attachments, plan-mode flag) so routed launches behave
// identically to the legacy launch path for everything except provider
// selection. Without launch, routed runs would fall back to
// task.Description and drop role framing / AGENTS.md / wake context.
//
// Office-routed launches use autoStart=false: the user kicked off the
// task; the office layer only chose the provider.
func (s *Service) StartTaskWithRoute(
	ctx context.Context, taskID, agentProfileID string,
	launch executor.LaunchContext, route executor.RouteOverride,
) error {
	_, err := s.startTask(ctx, taskID, agentProfileID,
		launch.ExecutorID, launch.ExecutorProfileID, launch.Priority,
		launch.Prompt, launch.WorkflowStepID, launch.PlanMode, false,
		launch.Attachments, startTaskOptions{Env: launch.Env, Route: &route})
	return err
}

//nolint:cyclop,funlen,gocognit // launch path threads many orthogonal concerns (workflow-step / agent-profile / office-task / config-mode / route / system-prompt wrapping); splitting it would require shared mutable state across helpers
func (s *Service) startTask(ctx context.Context, taskID string, agentProfileID string, executorID string, executorProfileID string, priority string, prompt string, workflowStepID string, planMode, autoStart bool, attachments []v1.MessageAttachment, opts startTaskOptions) (*executor.TaskExecution, error) {
	// One GetWorkflowMeta read shared by profile resolution and prompt build.
	ctx = withWorkflowMetaCache(ctx)
	// Fail before task-state or session mutations when the selected logical
	// profile belongs to a disabled dynamic family. The workflow step may later
	// override the caller profile, so repeat the check after that resolution.
	if s.profileExecutionResolver != nil {
		preflightProfileID := s.resolveEffectiveAgentProfile(ctx, taskID, workflowStepID, agentProfileID)
		if err := s.profileExecutionResolver.ValidateProfile(ctx, preflightProfileID); err != nil {
			return nil, err
		}
	}

	env, route := opts.Env, opts.Route
	s.logger.Debug("manually starting task",
		zap.String("task_id", taskID),
		zap.String("agent_profile_id", agentProfileID),
		zap.String("executor_id", executorID),
		zap.String("priority", priority),
		zap.Int("prompt_length", len(prompt)),
		zap.String("workflow_step_id", workflowStepID),
		zap.Bool("plan_mode", planMode),
		zap.Bool("auto_start", autoStart),
		zap.Int("attachments", len(attachments)))

	// Reserve any pending "start it later" intent for the whole of this start,
	// taken before the session is prepared rather than just before the launch:
	// the gate only needs the intent to still be there, so any window in which
	// this start has already created a session is a window that ends in two.
	// The deferred release covers every failure path, including the early
	// returns just below.
	launchClaim := s.claimDeferredLaunchForStart(ctx, taskID)
	defer launchClaim.releaseIfHeld(ctx)

	isOfficeTask, err := s.lookupOfficeTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to determine office task status: %w", err)
	}
	if isOfficeTask {
		if err := validateOfficeLaunchEnv(taskID, env); err != nil {
			return nil, err
		}
	}
	// Automated launch callers perform this check before entering startTask,
	// but several event paths can race after that check and before session
	// preparation. Keep the terminal-PR guard at the session boundary too so
	// no automated path can create a session for a closed or merged PR.
	if autoStart {
		if task, taskErr := s.repo.GetTask(ctx, taskID); taskErr != nil {
			return nil, fmt.Errorf("failed to fetch task for automatic launch gate: %w", taskErr)
		} else if s.shouldSkipTerminalPRAutoStart(ctx, task) {
			return nil, nil
		}
	}
	workflowSessionConfigStepID := workflowStepID
	// Manual starts often omit workflow_step_id because the task is already
	// bound to its current step. Resolve that canonical step before profile
	// selection and launch-layer session configuration so a start-step rule is
	// applied before the first prompt as well.
	if workflowSessionConfigStepID == "" {
		if dbTask, taskErr := s.repo.GetTask(ctx, taskID); taskErr != nil {
			s.logger.Warn("failed to fetch task for workflow step fallback",
				zap.String("task_id", taskID), zap.Error(taskErr))
		} else if dbTask != nil {
			workflowSessionConfigStepID = dbTask.WorkflowStepID
		}
	}

	// Office tasks do NOT transition through SCHEDULING / IN_PROGRESS on
	// every run. Their lifecycle status (todo / in_review / done /
	// blocked / cancelled) reflects the *user-meaningful workflow*, not
	// the orchestrator's runtime cycle. Runs schedule the *agent*,
	// not the task — the agent's runtime state is shown via the topbar
	// Working spinner + inline session timeline entry. Suppressing the
	// transition here avoids gratuitous flicker (REVIEW → SCHEDULING →
	// IN_PROGRESS → REVIEW for a single comment-reply cycle) and matches
	// the user's mental model.
	if isOfficeTask {
		s.logger.Debug("skipping SCHEDULING transition for office task",
			zap.String("task_id", taskID))
	} else if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateScheduling); err != nil {
		s.logger.Warn("failed to update task state to SCHEDULING",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	s.moveTaskToWorkflowStep(ctx, taskID, workflowStepID)

	officeAgentProfileID := ""
	if isOfficeTask {
		officeAgentProfileID = agentProfileID
	}

	// Resolve the workflow step's agent profile override.
	// The frontend may pass the workspace default profile, but the step may
	// require a different agent (e.g., Codex on "In Progress", Auggie on "Review").
	callerProfileID := agentProfileID
	if opts.ProfileExplicit && agentProfileID != "" {
		s.logger.Info("explicit selector-backed agent profile selection takes precedence over workflow step",
			zap.String("task_id", taskID),
			zap.String("agent_profile_id", agentProfileID))
	} else {
		agentProfileID = s.resolveEffectiveAgentProfile(ctx, taskID, workflowStepID, agentProfileID)
	}
	if s.profileExecutionResolver != nil {
		if err := s.profileExecutionResolver.ValidateProfile(ctx, agentProfileID); err != nil {
			return nil, err
		}
	}
	overrideApplied := agentProfileID != callerProfileID
	if route != nil && route.ExecutionProfileID != "" {
		agentProfileID = route.ExecutionProfileID
	}

	// Fetch the task from the repository to get complete task info
	task, err := s.scheduler.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to fetch task for manual start",
			zap.String("task_id", taskID),
			zap.Error(err))
		return nil, err
	}
	launchErrorStamp := ""
	if launchError, found := models.LoadTaskLaunchError(task.Metadata); found {
		launchErrorStamp = launchError.Stamp()
	}
	if executorID == "" {
		if v, ok := task.Metadata[models.MetaKeyExecutorID].(string); ok && v != "" {
			executorID = v
		}
	}
	if executorProfileID == "" {
		if v, ok := task.Metadata[models.MetaKeyExecutorProfileID].(string); ok && v != "" {
			executorProfileID = v
		}
	}

	// Override priority if provided in the request
	if priority != "" {
		task.Priority = priority
	}

	// Use provided prompt, fall back to task description
	effectivePrompt := prompt
	if effectivePrompt == "" {
		effectivePrompt = task.Description
	}

	// Prepare session first so we have the sessionID for config context injection.
	// For office tasks, replace the per-launch PrepareSession with the per-(task,
	// agent) EnsureSessionForAgent so runs reuse one row across turns.
	sessionID, sessionCreated, err := s.prepareSessionForStart(ctx, task, agentProfileID, officeAgentProfileID, executorID, executorProfileID, workflowStepID)
	if err != nil {
		return nil, err
	}
	// Seed a matching conditional session configuration before lifecycle
	// startup. The ACP manager applies this durable runtime layer after the
	// selected profile and before the first prompt, preserving the original
	// session tab.
	s.applyWorkflowSessionConfigBeforeLaunchForStep(ctx, taskID, sessionID, workflowSessionConfigStepID)
	// Surface the newly created session before LaunchPreparedSession performs
	// potentially slow environment setup (for example, a Docker health check).
	// The frontend adopts this CREATED session and can render
	// executor.prepare.progress events while the launch request is still pending.
	if sessionCreated {
		s.publishSessionCreatedEvent(ctx, taskID, sessionID, workflowStepID)
	}

	// When the workflow step overrode the caller's profile, tag the session
	// for provenance: the profile came from workflow routing rather than
	// direct user selection.
	if overrideApplied {
		s.tagSessionAsWorkflowSwitched(ctx, sessionID)
	}

	// Passthrough sessions skip both the workflow-prompt "@name" expansion below
	// and the Kandev MCP wrap further down: the prompt is typed straight into
	// the agent CLI's TTY and the user sees it verbatim — they don't want a
	// wall of MCP-tool boilerplate or a hidden expansion block prepended to
	// "hello". Use the session snapshot, not a live profile lookup, so a
	// mid-run profile edit cannot change wrap behavior. Looked up once here and
	// reused below so we don't hit GetTaskSession twice for the same fact.
	// If the lookup fails, resolveIsPassthroughForLaunch fails safe by
	// treating the session as passthrough: skipping the wrap/expansion is
	// strictly less harmful than leaking hidden <kandev-system> content into
	// a real passthrough session's PTY.
	isPassthrough := s.resolveIsPassthroughForLaunch(ctx, sessionID)
	launchSession, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload launch session: %w", err)
	}

	if route == nil {
		if agentProfileID, err = s.resolveDynamicLaunchExecution(ctx, launchSession, agentProfileID, false); err != nil {
			return nil, err
		}
	}
	// Dynamic resolution updates the session snapshot after the initial
	// passthrough check. Use the concrete candidate's snapshot for prompt
	// wrapping and launch behavior.
	isPassthrough = launchSession.IsPassthrough

	effectivePrompt, planModeActive, promptReferenceContext := s.applyWorkflowAndPlanMode(
		ctx, effectivePrompt, task.ID, sessionID, workflowStepID,
		planMode, task.IsEphemeral, isPassthrough,
	)

	// Inject config context for config-mode sessions (dedicated settings chat)
	configMode := false
	if cm, ok := launchSession.Metadata["config_mode"].(bool); ok && cm {
		configMode = true
		effectivePrompt = sysprompt.InjectConfigContext(sessionID, effectivePrompt)
	}
	titleOwner := false
	if !configMode && !isOfficeTask {
		titleOwner, err = s.ClaimTaskTitleSession(ctx, task.ID, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to claim first-turn task title: %w", err)
		}
	}

	// Wrap the first prompt with the Kandev MCP system block (task/session IDs +
	// tool list). Done at the orchestrator layer so recordInitialMessage persists
	// the wrapped form to task_session_messages; Message.ToAPI strips the
	// <kandev-system> block for the UI bubble and exposes it via raw_content.
	// Only the first launch carries this wrap — follow-up prompts and resumes
	// rely on the agent CLI's conversation history retaining it.
	// Idempotent: upstream call sites (wsAddMessage on CREATED sessions,
	// recordAutoStartMessage) wrap before recording the user message so the DB
	// row carries the block; the mode-aware injector canonicalizes it instead of
	// double-wrapping.
	skipKandevMCPWrap := isPassthrough
	// `task` here is *v1.Task from the scheduler, which does NOT carry the
	// orchestrator's WorkflowStepID — go through the task-ID variant so the
	// repo lookup pulls the canonical step. Using the workflowStepID parameter
	// directly is wrong because it can be empty on manual user-initiated starts
	// while the task is already bound to a signal-gated step in the DB.
	if effectivePrompt != "" || len(attachments) > 0 {
		effectivePrompt = s.applyLaunchPromptContext(ctx, launchPromptContext{
			prompt:                    effectivePrompt,
			taskID:                    task.ID,
			sessionID:                 sessionID,
			isOfficeTask:              isOfficeTask,
			isPassthrough:             skipKandevMCPWrap,
			configMode:                configMode,
			referenceContext:          promptReferenceContext,
			includeTaskTitleTool:      !configMode && titleOwner,
			autopilot:                 task.Autopilot,
			includeParentQuestionTool: task.Autopilot && task.ParentID != "",
			spawnOrigin:               opts.SpawnOrigin,
		})
	}

	// Office tasks restrict the MCP toolset: kanban tools (move/update/list
	// task, etc.) are excluded because office agents call those via the
	// kandev CLI ($KANDEV_CLI). See docs/specs/office/system-design/agents-03.md.
	mcpMode := ""
	if isOfficeTask {
		mcpMode = executor.McpModeOffice
	}
	initialTurnID, initialTurnCreated := s.startTurnForSessionWithOwnership(ctx, sessionID)

	// Cache the raw prompt so a transient-provider-error (529) retry can
	// re-drive this first turn — initial launches bypass PromptTask.
	s.rememberTurnPrompt(sessionID, prompt, "", planMode, attachments)

	execution, err := s.launchPreparedSessionWithDynamicFallback(ctx, task, sessionID, executor.LaunchOptions{
		AgentProfileID:       agentProfileID,
		OfficeAgentProfileID: officeAgentProfileID,
		ExecutorID:           executorID,
		TurnID:               initialTurnID,
		Prompt:               effectivePrompt,
		WorkflowStepID:       workflowStepID,
		StartAgent:           true,
		McpMode:              mcpMode,
		Attachments:          attachments,
		Env:                  env,
		RouteOverride:        route,
	})
	if err != nil {
		if initialTurnCreated {
			s.completeTurnIfCurrent(ctx, sessionID, initialTurnID)
		}
		return nil, s.handleSessionLaunchFailure(ctx, taskID, sessionID, err)
	}

	s.postLaunchStart(ctx, taskID, execution, effectivePrompt, planModeActive || configMode, planModeActive, autoStart, attachments)
	execution.TurnID = initialTurnID
	s.clearTaskLaunchErrorIfStamp(ctx, taskID, launchErrorStamp)

	// The agent is running, so the reservation becomes a consumption.
	launchClaim.consume(ctx)

	// Note: Task stays in SCHEDULING state until the agent is fully initialized.
	// The executor will transition to IN_PROGRESS after StartAgentProcess() succeeds.

	return execution, nil
}

func (s *Service) applyWorkflowSessionConfigBeforeLaunchForStep(
	ctx context.Context,
	taskID string,
	sessionID string,
	workflowStepID string,
) {
	if s.workflowStepGetter == nil || workflowStepID == "" {
		return
	}
	workflowStep, err := s.workflowStepGetter.GetStep(ctx, workflowStepID)
	if err != nil {
		s.logger.Warn("failed to load workflow step for session configuration",
			zap.String("task_id", taskID), zap.String("step_id", workflowStepID), zap.Error(err))
		return
	}
	if workflowStep == nil {
		return
	}
	preparedSession, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to reload session for session configuration",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	s.applyWorkflowSessionConfigBeforeLaunch(ctx, taskID, preparedSession, workflowStep)
}

// launchPromptContext carries what the first turn of a launch needs in order to
// compose its system context.
type launchPromptContext struct {
	prompt                    string
	taskID                    string
	sessionID                 string
	isOfficeTask              bool
	isPassthrough             bool
	configMode                bool
	includeTaskTitleTool      bool
	autopilot                 bool
	includeParentQuestionTool bool
	referenceContext          string
	spawnOrigin               *SpawnOrigin
}

// applyLaunchPromptContext prepends the first-turn system context to a launch
// prompt: the Kandev (or Office) MCP block, plus spawner attribution when the
// launch came from spawn_session_kandev.
//
// Passthrough profiles get attribution only, as plain text — see
// applySpawnOriginText for why they skip the MCP block entirely.
func (s *Service) applyLaunchPromptContext(ctx context.Context, p launchPromptContext) string {
	var pullRequestTargetContext string
	p.prompt, pullRequestTargetContext = s.addTaskPullRequestTargetContext(
		ctx, p.taskID, p.prompt, p.isPassthrough,
	)
	if p.isPassthrough {
		prompt := applySpawnOriginText(p.prompt, p.spawnOrigin)
		if p.includeTaskTitleTool {
			return sysprompt.PendingTaskTitlePassthroughInstruction() + "\n\n" + prompt
		}
		return prompt
	}
	// Spawner attribution is built here, not by the MCP handler that filled
	// SpawnOrigin: the injectors below strip every <kandev-system> block they do
	// not recognize, so the block has to be generated from the same server state
	// that whitelists it as trusted content.
	prompt, spawnContext := applySpawnOriginContext(p.prompt, p.spawnOrigin)
	if p.isOfficeTask {
		return sysprompt.InjectOfficeContextWithOptions(
			p.taskID, p.sessionID, prompt,
			s.StepRequiresCompletionSignal(ctx, p.taskID),
			p.referenceContext, spawnContext, pullRequestTargetContext,
		)
	}
	return sysprompt.InjectKandevContextWithOptions(p.taskID, p.sessionID, prompt, sysprompt.KandevContextOptions{
		RequiresCompletionSignal:       s.StepRequiresCompletionSignal(ctx, p.taskID),
		IncludeCoordinatorTaskControls: !p.configMode,
		IncludeTaskTitleTool:           p.includeTaskTitleTool,
		Autopilot:                      p.autopilot,
		IncludeUserQuestionTool:        !p.autopilot && !p.isPassthrough,
		IncludeParentQuestionTool:      p.autopilot && p.includeParentQuestionTool,
	}, p.referenceContext, spawnContext, pullRequestTargetContext)
}

// spawnOriginContent renders the spawner-attribution text for a launch requested
// through spawn_session_kandev, or "" for ordinary launches.
func spawnOriginContent(origin *SpawnOrigin) string {
	if origin == nil {
		return ""
	}
	return sysprompt.SpawnedSessionContext(origin.TaskID, origin.SessionID, origin.SessionName)
}

// applySpawnOriginContext prepends the spawner-attribution system block for a
// launch requested through spawn_session_kandev and returns the content that
// the first-turn injector must whitelist so the block is not stripped again as
// untrusted. Ordinary launches pass through unchanged with empty content.
func applySpawnOriginContext(prompt string, origin *SpawnOrigin) (string, string) {
	content := spawnOriginContent(origin)
	if content == "" {
		return prompt, ""
	}
	return sysprompt.Wrap(content) + "\n\n" + prompt, content
}

// applySpawnOriginText prepends the same attribution as plain prompt text, for
// passthrough profiles whose prompt reaches the agent through a TTY the user is
// watching. There is no injector on that path to whitelist a system block, and
// wrapping it would only put raw <kandev-system> markup on the terminal.
func applySpawnOriginText(prompt string, origin *SpawnOrigin) string {
	content := spawnOriginContent(origin)
	if content == "" {
		return prompt
	}
	return content + "\n\n" + prompt
}

// isOfficeTask returns the repository's canonical Office ownership projection.
func (s *Service) isOfficeTask(ctx context.Context, taskID string) bool {
	isOfficeTask, err := s.lookupOfficeTask(ctx, taskID)
	return err == nil && isOfficeTask
}

func (s *Service) lookupOfficeTask(ctx context.Context, taskID string) (bool, error) {
	dbTask, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return false, err
	}
	return dbTask != nil && dbTask.IsFromOffice, nil
}

func validateOfficeRuntimeEnv(env map[string]string) error {
	required := []string{
		"KANDEV_CLI",
		"KANDEV_API_URL",
		"KANDEV_API_KEY",
		"KANDEV_AGENT_ID",
		"KANDEV_WORKSPACE_ID",
		"KANDEV_RUN_ID",
		"KANDEV_TASK_ID",
	}
	missing := make([]string, 0, len(required))
	for _, key := range required {
		if strings.TrimSpace(env[key]) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"office runtime context is incomplete (missing %s); start or wake the task through Office",
		strings.Join(missing, ", "),
	)
}

var (
	errOfficeTaskStartRequiresScheduler = errors.New(
		"office tasks must be started through Office; use StartTaskWithEnv with scheduler-injected credentials",
	)
	errOfficeTaskResumeRequiresScheduler = errors.New(
		"office tasks must be resumed through Office; use the Office scheduler to start a fresh run",
	)
	errOfficePreparedResumeRequiresScheduler = errors.New(
		"office tasks must be restarted through Office; prepared-workspace resume is not supported for Office-owned tasks",
	)
)

// validateOfficeLaunchEnv requires the scheduler context to be bound to the
// task being launched. StartTaskWithEnv is only wired to the internal Office
// scheduler adapter; the task binding still prevents a complete context map
// from being reused for a different task.
func validateOfficeLaunchEnv(taskID string, env map[string]string) error {
	if err := validateOfficeRuntimeEnv(env); err != nil {
		return err
	}
	if env["KANDEV_TASK_ID"] != taskID {
		return fmt.Errorf(
			"office runtime context is bound to task %q, not %q; start or wake the task through Office",
			env["KANDEV_TASK_ID"], taskID,
		)
	}
	return nil
}

// prepareSessionForStart creates the session for a launch and propagates any
// inherited workspace environment onto it.
//
// The propagation (inherit_parent / shared_group) lives here rather than only
// in PrepareTaskSession so every launch entry point inherits consistently —
// including the direct start path (startTask), which MCP-created subtasks reach
// via auto-start. Without this, an inherit_parent subtask launched through
// startTask would provision a fresh worktree instead of reusing the parent's.
// propagateInheritedEnvironment is a no-op for tasks without a workspace policy.
func (s *Service) prepareSessionForStart(
	ctx context.Context, task *v1.Task,
	agentProfileID, officeAgentProfileID, executorID, executorProfileID, workflowStepID string,
) (string, bool, error) {
	sessionID, created, err := s.createStartSession(ctx, task, agentProfileID, officeAgentProfileID, executorID, executorProfileID, workflowStepID)
	if err != nil {
		return "", false, err
	}
	if err := s.propagateInheritedEnvironment(ctx, task, sessionID); err != nil {
		// This legacy creation path cannot bind an inherited ID until it has a
		// session row. Compensate before returning so callers never observe a
		// partial sibling session when the required parent/group workspace is
		// unavailable.
		if deleteErr := s.repo.DeleteTaskSession(ctx, sessionID); deleteErr != nil {
			s.logger.Warn("failed to compensate inherited workspace session",
				zap.String("session_id", sessionID), zap.Error(deleteErr))
		}
		return "", false, err
	}
	return sessionID, created, nil
}

// resolveIsPassthroughForLaunch reloads the session snapshot to decide
// whether this launch is a passthrough session (skip the Kandev MCP wrap and
// the "@name" prompt-reference expansion). If the reload fails, this fails
// safe by treating the session as passthrough: skipping the wrap/expansion is
// strictly less harmful than leaking hidden <kandev-system> content into a
// real passthrough session's PTY.
func (s *Service) resolveIsPassthroughForLaunch(ctx context.Context, sessionID string) bool {
	launchSession, sessErr := s.repo.GetTaskSession(ctx, sessionID)
	if sessErr != nil {
		s.logger.Warn("failed to reload session for passthrough check; defaulting to passthrough (skip hidden-content injection) as the safer failure mode",
			zap.String("session_id", sessionID),
			zap.Error(sessErr))
		return true
	}
	return launchSession.IsPassthrough
}

// resolveExecutionForLaunchSession keeps the logical profile on the session
// while selecting a concrete profile only once for a new dynamic route. A
// persisted route is authoritative during resume and must not advance merely
// because a caller is launching the same logical session again.
func (s *Service) resolveExecutionForLaunchSession(
	ctx context.Context,
	session *models.TaskSession,
) (agentruntime.ProfileExecution, bool, error) {
	if session == nil {
		return agentruntime.ProfileExecution{}, false, errors.New("launch session is required for profile resolution")
	}
	if resolved, routeClaimed, handled, err := s.resolveDurableExecutionForLaunch(ctx, session); handled {
		return resolved, routeClaimed, err
	}
	if session.RouteGeneration > 0 && session.ExecutionProfileID != "" {
		resolved, err := s.profileExecutionResolver.ResolveExisting(
			ctx, session.ID, session.AgentProfileID, session.ExecutionProfileID,
			session.RouteGeneration, 0, session.RouteReason,
		)
		return resolved, false, err
	}
	resolved, err := s.profileExecutionResolver.Resolve(
		ctx, session.ID, session.AgentProfileID, session.RouteGeneration, "",
	)
	if err != nil {
		return agentruntime.ProfileExecution{}, false, err
	}
	return resolved, resolved.Generation > 0, nil
}

func (s *Service) resolveDurableExecutionForLaunch(
	ctx context.Context,
	session *models.TaskSession,
) (agentruntime.ProfileExecution, bool, bool, error) {
	loader, ok := s.repo.(dynamicRouteStateLoader)
	if !ok {
		return agentruntime.ProfileExecution{}, false, false, nil
	}
	state, err := loader.LoadRouteState(ctx, session.ID)
	if err != nil {
		return agentruntime.ProfileExecution{}, false, true, fmt.Errorf("load durable dynamic route state: %w", err)
	}
	if state == nil || state.LogicalProfileID != session.AgentProfileID || state.Generation <= 0 {
		return agentruntime.ProfileExecution{}, false, false, nil
	}
	if state.ExecutionProfileID == "" {
		if state.Status != dynamicRouteStatusWaiting {
			return agentruntime.ProfileExecution{}, false, true, &dynamicruntime.NoEligibleCandidateError{
				SessionID: session.ID, LogicalProfile: session.AgentProfileID,
				Generation: state.Generation,
			}
		}
		resolved, resolveErr := s.profileExecutionResolver.Resolve(
			ctx, session.ID, session.AgentProfileID, state.Generation, "",
		)
		return resolved, resolved.Generation > 0, true, resolveErr
	}
	resolved, err := s.profileExecutionResolver.ResolveExisting(
		ctx, session.ID, session.AgentProfileID, state.ExecutionProfileID,
		state.Generation, state.ProfileVersion, "durable_route_state",
	)
	return resolved, true, true, err
}

func (s *Service) resolveDynamicLaunchExecution(
	ctx context.Context,
	session *models.TaskSession,
	profileID string,
	validate bool,
) (string, error) {
	if s.profileExecutionResolver == nil {
		return profileID, nil
	}
	if validate {
		if err := s.profileExecutionResolver.ValidateProfile(ctx, session.AgentProfileID); err != nil {
			return "", err
		}
	}
	resolved, routeClaimed, err := s.resolveExecutionForLaunchSession(ctx, session)
	if err != nil {
		s.recordDynamicRouteResolutionFailure(ctx, session, err)
		return "", err
	}
	if !routeClaimed {
		if resolved.ExecutionProfileID == "" {
			return profileID, nil
		}
		return resolved.ExecutionProfileID, nil
	}
	applyResolvedExecution(session, resolved)
	if err := s.repo.UpdateTaskSession(ctx, session); err != nil {
		return "", fmt.Errorf("persist dynamic route attribution: %w", err)
	}
	return resolved.ExecutionProfileID, nil
}

func applyResolvedExecution(session *models.TaskSession, resolved agentruntime.ProfileExecution) {
	if session == nil {
		return
	}
	previousExecutionProfileID := session.ExecutionProfileID
	session.ExecutionProfileID = resolved.ExecutionProfileID
	session.RouteGeneration = resolved.Generation
	session.RouteState = "starting"
	session.RouteReason = resolved.Decision.Reason
	if session.RouteReason == "" {
		session.RouteReason = "candidate_order"
	}
	applyDynamicRouteDecisionProjection(session, resolved.Decision)
	// A new concrete candidate never inherits a provider-native ACP identity.
	// Reapplying the persisted route during restart must keep the identity so
	// native conversation resume remains possible.
	if previousExecutionProfileID != resolved.ExecutionProfileID {
		session.DownstreamACPSessionID = ""
	}
	if resolved.Profile != nil {
		session.AgentProfileSnapshot = map[string]interface{}{
			"id":                           resolved.Profile.ID,
			"name":                         resolved.Profile.Name,
			"agent_id":                     resolved.Profile.AgentID,
			"model":                        resolved.Profile.Model,
			"mode":                         resolved.Profile.Mode,
			"config_options":               resolved.Profile.ConfigOptions,
			"auto_approve":                 resolved.Profile.AutoApprove,
			"dangerously_skip_permissions": resolved.Profile.DangerouslySkipPermissions,
			"cli_passthrough":              resolved.Profile.CLIPassthrough,
		}
		session.IsPassthrough = resolved.Profile.CLIPassthrough
	}
}

func (s *Service) recordDynamicRouteResolutionFailure(
	ctx context.Context,
	session *models.TaskSession,
	err error,
) {
	if session == nil || err == nil {
		return
	}
	var noCandidate *dynamicruntime.NoEligibleCandidateError
	if !errors.As(err, &noCandidate) {
		return
	}
	session.RouteGeneration = noCandidate.Generation
	session.RouteState = dynamicRouteStatusWaiting
	session.RouteReason = "no_eligible_candidate"
	session.UpdatedAt = time.Now().UTC()
	if updateErr := s.repo.UpdateTaskSession(ctx, session); updateErr != nil {
		s.logger.Warn("failed to persist dynamic route waiting state",
			zap.String("session_id", session.ID), zap.Error(updateErr))
	}
}

// createStartSession picks the right session-creation path for the task:
// Office tasks use the per-(task, agent) EnsureSessionForAgent path (so runs
// reuse one row across turns); kanban / quick-chat fall through to the
// per-launch PrepareSession used since day one. An Office task can be created
// before its runner seat is projected, so the run's captured identity or its
// resolved execution profile supplies the owner until the seat is assigned.
//
// The session-owner identity passed to EnsureSessionForAgentWithCreation is,
// by default, the task's runner seat (dbTask.AssigneeAgentProfileID) — even
// for a reviewer/approver run whose agent differs from the runner, which
// wrongly binds that run's session (and later its decisions) to the runner's
// identity. When features.officeSessionIdentity is on, officeAgentProfileID
// (the run's own agent, captured by the caller before step/routing overrides
// mutate agentProfileID) is used instead so each participant agent gets its
// own session per task.
func (s *Service) createStartSession(
	ctx context.Context, task *v1.Task,
	agentProfileID, officeAgentProfileID, executorID, executorProfileID, workflowStepID string,
) (string, bool, error) {
	dbTask, err := s.repo.GetTask(ctx, task.ID)
	if err != nil || dbTask == nil || !dbTask.IsFromOffice {
		sessionID, prepareErr := s.executor.PrepareSession(ctx, task, agentProfileID, executorID, executorProfileID, workflowStepID)
		return sessionID, prepareErr == nil, prepareErr
	}

	sessionOwnerID := s.officeSessionOwnerID(dbTask, agentProfileID, officeAgentProfileID)
	if sessionOwnerID == "" {
		sessionID, prepareErr := s.executor.PrepareSession(ctx, task, agentProfileID, executorID, executorProfileID, workflowStepID)
		return sessionID, prepareErr == nil, prepareErr
	}
	session, created, ensureErr := s.executor.EnsureSessionForAgentWithCreation(
		ctx, task, sessionOwnerID, agentProfileID, executorID, executorProfileID,
	)
	if ensureErr != nil {
		return "", false, ensureErr
	}
	return session.ID, created, nil
}

// officeSessionOwnerID selects the stable identity for an Office session. A
// projected runner wins by default. When identity separation is enabled, a
// scheduler-captured participant identity wins for assigned runs. Before a
// runner seat is projected, the captured identity or execution profile keeps
// the session on the Office path instead of creating a generic workspace row.
func (s *Service) officeSessionOwnerID(task *models.Task, agentProfileID, officeAgentProfileID string) string {
	if task.AssigneeAgentProfileID == "" {
		if officeAgentProfileID != "" {
			return officeAgentProfileID
		}
		return agentProfileID
	}
	if s.config.OfficeSessionIdentity && officeAgentProfileID != "" {
		return officeAgentProfileID
	}
	return task.AssigneeAgentProfileID
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
		return
	}
	s.publishTaskUpdated(ctx, dbTask)
}

// resolveEffectiveAgentProfile checks whether the task's workflow step overrides
// the agent profile. If the step (or workflow default) specifies a different
// profile, that profile is returned instead of the caller-provided one.
// This ensures the initial task start uses the step's agent — not just the
// workspace default the frontend sends.
func (s *Service) resolveEffectiveAgentProfile(ctx context.Context, taskID, workflowStepID, callerProfileID string) string {
	if s.workflowStepGetter == nil {
		s.logger.Debug("resolveEffectiveAgentProfile: no workflowStepGetter, using caller profile",
			zap.String("task_id", taskID),
			zap.String("caller_profile", callerProfileID))
		return callerProfileID
	}

	// Determine the effective step ID: explicit param > task's current step.
	effectiveStepID := workflowStepID
	if effectiveStepID == "" {
		dbTask, err := s.repo.GetTask(ctx, taskID)
		if err != nil {
			s.logger.Debug("resolveEffectiveAgentProfile: failed to load task from DB",
				zap.String("task_id", taskID),
				zap.Error(err))
			return callerProfileID
		}
		s.logger.Debug("resolveEffectiveAgentProfile: loaded task from DB",
			zap.String("task_id", taskID),
			zap.String("db_workflow_step_id", dbTask.WorkflowStepID))
		if dbTask.WorkflowStepID == "" {
			s.logger.Debug("resolveEffectiveAgentProfile: task has no workflow step, using caller profile",
				zap.String("task_id", taskID))
			return callerProfileID
		}
		effectiveStepID = dbTask.WorkflowStepID
	}

	step, err := s.workflowStepGetter.GetStep(ctx, effectiveStepID)
	if err != nil || step == nil {
		s.logger.Debug("resolveEffectiveAgentProfile: failed to load step",
			zap.String("task_id", taskID),
			zap.String("step_id", effectiveStepID),
			zap.Error(err))
		return callerProfileID
	}

	s.logger.Debug("resolveEffectiveAgentProfile: loaded step",
		zap.String("task_id", taskID),
		zap.String("step_id", effectiveStepID),
		zap.String("step_name", step.Name),
		zap.String("step_agent_profile_id", step.AgentProfileID),
		zap.String("step_workflow_id", step.WorkflowID))

	stepProfile := s.resolveStepAgentProfile(ctx, step)
	s.logger.Debug("resolveEffectiveAgentProfile: resolved step profile",
		zap.String("task_id", taskID),
		zap.String("step_profile", stepProfile),
		zap.String("caller_profile", callerProfileID))

	if stepProfile == "" || stepProfile == callerProfileID {
		return callerProfileID
	}

	s.logger.Info("overriding agent profile with workflow step profile",
		zap.String("task_id", taskID),
		zap.String("step_id", effectiveStepID),
		zap.String("step_name", step.Name),
		zap.String("caller_profile", callerProfileID),
		zap.String("step_profile", stepProfile))
	return stepProfile
}

// postLaunchStart records the initial message and sets plan mode after a successful launch.
func (s *Service) postLaunchStart(ctx context.Context, taskID string, execution *executor.TaskExecution, prompt string, recordPlanMode, setPlanMode, autoStart bool, attachments []v1.MessageAttachment) {
	if execution.SessionID != "" {
		s.recordInitialMessage(ctx, taskID, execution.SessionID, prompt, recordPlanMode, autoStart, attachments)

		if setPlanMode {
			session, err := s.repo.GetTaskSession(ctx, execution.SessionID)
			if err == nil {
				s.setSessionPlanMode(ctx, session, true)
			}
		}

		// Persist prepare_result using SetSessionMetadataKey (json_set) which
		// atomically sets ONE key without touching others. This avoids the
		// read-modify-write race where UpdateSessionMetadata clobbers plan_mode.
		if execution.PrepareResult != nil && execution.PrepareResult.Success {
			pr := lifecycle.SerializePrepareResult(execution.PrepareResult)
			if err := s.repo.SetSessionMetadataKey(ctx, execution.SessionID, "prepare_result", pr); err != nil {
				s.logger.Warn("failed to persist prepare_result",
					zap.String("session_id", execution.SessionID), zap.Error(err))
			}
		}
	}
	go s.ensureSessionPRWatch(context.Background(), taskID, execution.SessionID, execution.WorktreeBranch)
}

// applyWorkflowAndPlanMode applies workflow step configuration and plan mode injection to a prompt.
// Returns the effective prompt and whether plan mode is active (from either the step or the caller).
// For ephemeral tasks (quick chat), workflow step processing is skipped since they have no workflow.
func (s *Service) applyWorkflowAndPlanMode(
	ctx context.Context,
	prompt string,
	taskID string,
	sessionID string,
	workflowStepID string,
	planMode bool,
	isEphemeral bool,
	isPassthrough bool,
) (string, bool, string) {
	return s.applyWorkflowAndPlanModeWithPromptContext(
		ctx, prompt, taskID, sessionID, workflowStepID, planMode,
		isEphemeral, isPassthrough, "",
	)
}

// applyWorkflowAndPlanModeWithPromptContext composes a workflow prompt while
// preserving a direct message's acceptance-time saved-prompt expansion. A
// direct message already has canonical reference content, so resolving it
// again would make persistence and ACP dispatch depend on mutable prompt
// records. Workflow-only prompts still use the normal expansion path.
func (s *Service) applyWorkflowAndPlanModeWithPromptContext(
	ctx context.Context,
	prompt string,
	taskID string,
	sessionID string,
	workflowStepID string,
	planMode bool,
	isEphemeral bool,
	isPassthrough bool,
	trustedPromptContext string,
) (string, bool, string) {
	effectivePrompt := prompt
	promptReferenceContext := ""

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
			if trustedPromptContext != "" {
				effectivePrompt, promptReferenceContext = s.buildWorkflowPromptWithTrustedContext(
					ctx, effectivePrompt, step, taskID, sessionID, isPassthrough, trustedPromptContext,
				)
			} else {
				effectivePrompt, promptReferenceContext = s.buildWorkflowPromptWithContext(
					ctx, effectivePrompt, step, taskID, sessionID, isPassthrough,
				)
			}
		}
	}

	if planMode && !stepHasPlanMode {
		var parts []string
		parts = append(parts, sysprompt.Wrap(sysprompt.DefaultPlanPrefix()))
		parts = append(parts, effectivePrompt)
		effectivePrompt = strings.Join(parts, "\n\n")
	}

	return effectivePrompt, planMode || stepHasPlanMode, promptReferenceContext
}

// backfillInitialUserMessageIfMissing records the task's description as the
// first user message when the session has no messages at all. This covers the
// edge case where the initial launch failed before recordInitialMessage was
// called (postLaunchStart only runs after a successful LaunchAgent), leaving
// the chat empty even though the task carries the prompt the user originally
// typed.
//
// The check requires *zero* messages, not just zero user messages: if any
// agent output already exists from a partial prior run, the backfilled
// message would land at the bottom of the chat (CreateMessage stamps
// CreatedAt=now), which is worse than leaving the chat alone.
func (s *Service) backfillInitialUserMessageIfMissing(ctx context.Context, taskID, sessionID, prompt string) {
	if prompt == "" || s.messageCreator == nil {
		return
	}
	msgs, err := s.repo.ListMessages(ctx, sessionID)
	if err != nil {
		s.logger.Warn("backfill initial message: list messages failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	if len(msgs) > 0 {
		return
	}
	s.recordInitialMessage(ctx, taskID, sessionID, prompt, false, false, nil)
}

// recordInitialMessage creates the initial user message and updates session state after launch.
// autoStart marks the message as having been created by an automated trigger
// (workflow auto-start, PR/issue watch, Jira/Linear integration) so cleanup
// logic can distinguish "agent ran on its own" from "user actually engaged".
func (s *Service) recordInitialMessage(ctx context.Context, taskID, sessionID, prompt string, planModeActive, autoStart bool, attachments []v1.MessageAttachment) {
	if s.messageCreator != nil && (prompt != "" || len(attachments) > 0) {
		meta := NewUserMessageMeta().WithPlanMode(planModeActive).WithAutoStart(autoStart).WithAttachments(attachments)
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
func (s *Service) buildWorkflowPrompt(ctx context.Context, basePrompt string, step *wfmodels.WorkflowStep, taskID string, sessionID string, isPassthrough bool) string {
	prompt, _ := s.buildWorkflowPromptWithContext(ctx, basePrompt, step, taskID, sessionID, isPassthrough)
	return prompt
}

// workflowInstructionsHeading/End are stable, agent-facing markers for the
// optional workflow-level prompt block. Chat collapses everything between
// them by default. Do not i18n (sent to the model, same as step prompt English).
// The end marker is required so multi-paragraph workflow prompts do not break
// the frontend split (a first-blank-line heuristic would cut mid-body).
const (
	workflowInstructionsHeading = "## Workflow instructions"
	workflowInstructionsEnd     = "<!-- /workflow-instructions -->"
)

func (s *Service) buildWorkflowPromptWithContext(
	ctx context.Context,
	basePrompt string,
	step *wfmodels.WorkflowStep,
	taskID string,
	sessionID string,
	isPassthrough bool,
) (string, string) {
	return s.buildWorkflowPromptWithTrustedContext(
		ctx, basePrompt, step, taskID, sessionID, isPassthrough, "",
	)
}

// buildWorkflowPromptWithTrustedContext composes a workflow prompt without
// re-expanding an already canonical direct message. If the workflow step
// replaces the base prompt, it restores the exact trusted block so the
// accepted saved-prompt context remains available to the agent.
func (s *Service) buildWorkflowPromptWithTrustedContext(
	ctx context.Context,
	basePrompt string,
	step *wfmodels.WorkflowStep,
	taskID string,
	sessionID string,
	isPassthrough bool,
	trustedPromptContext string,
) (string, string) {
	_ = sessionID
	var parts []string

	if block := s.workflowInstructionsBlock(ctx, step, taskID); block != "" {
		parts = append(parts, block)
	}

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

	joined := strings.Join(parts, "\n\n")
	if trustedPromptContext != "" {
		trustedBlock := sysprompt.Wrap(trustedPromptContext)
		if !strings.Contains(joined, trustedBlock) {
			joined += "\n\n" + trustedBlock
		}
		return joined, trustedPromptContext
	}
	return s.expandPromptReferencesWithContext(ctx, joined, isPassthrough)
}

// workflowInstructionsBlock returns the visible "## Workflow instructions"
// section when the step's workflow has a non-empty prompt. Empty/whitespace
// prompts and missing getters/workflows omit the section entirely.
func (s *Service) workflowInstructionsBlock(ctx context.Context, step *wfmodels.WorkflowStep, taskID string) string {
	if s.workflowStepGetter == nil || step == nil || step.WorkflowID == "" {
		return ""
	}
	meta, err := s.getWorkflowMeta(ctx, step.WorkflowID)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to get workflow prompt for prompt building",
				zap.String("workflow_id", step.WorkflowID),
				zap.Error(err))
		}
		return ""
	}
	prompt := strings.TrimSpace(meta.Prompt)
	if prompt == "" {
		return ""
	}
	interpolated := strings.TrimSpace(sysprompt.InterpolatePlaceholders(prompt, taskID))
	if interpolated == "" {
		return ""
	}
	// Drop any accidental end-marker text from user content so chat split
	// cannot cut the block early (frontend also prefers the final marker).
	interpolated = strings.ReplaceAll(interpolated, workflowInstructionsEnd, "")
	interpolated = strings.TrimSpace(interpolated)
	if interpolated == "" {
		return ""
	}
	return workflowInstructionsHeading + "\n\n" + interpolated + "\n\n" + workflowInstructionsEnd
}

// expandPromptReferences resolves "@name" saved-prompt references in prompt
// via the configured PromptReferenceExpander. When no expander is set, prompt
// is returned unchanged. Passthrough sessions skip expansion entirely: the
// prompt is written raw to a PTY with no <kandev-system> stripping step, so
// the expansion's hidden wrapper block would be typed into the terminal
// verbatim instead of staying hidden.
func (s *Service) expandPromptReferences(ctx context.Context, prompt string, isPassthrough bool) string {
	expanded, _ := s.expandPromptReferencesWithContext(ctx, prompt, isPassthrough)
	return expanded
}

func (s *Service) expandPromptReferencesWithContext(
	ctx context.Context,
	prompt string,
	isPassthrough bool,
) (string, string) {
	if s.promptExpander == nil || isPassthrough {
		return prompt, ""
	}
	var zapLogger *zap.Logger
	if s.logger != nil {
		zapLogger = s.logger.Zap()
	}
	return s.promptExpander.AppendReferenceExpansionsWithContext(ctx, prompt, zapLogger)
}

// PrepareDirectPrompt applies the same backend-owned saved-prompt expansion
// used by workflow prompts to a direct user message. Message handlers call it
// before persistence so the stored content and the first dispatched prompt
// share one canonical representation.
func (s *Service) PrepareDirectPrompt(
	ctx context.Context,
	prompt string,
	isPassthrough bool,
) (string, string) {
	return s.expandPromptReferencesWithContext(ctx, prompt, isPassthrough)
}

// ResumeTaskSession restarts a specific task session using its stored worktree.
func (s *Service) ResumeTaskSession(ctx context.Context, taskID, sessionID string) (*executor.TaskExecution, error) {
	return s.ResumeTaskSessionWithOptions(ctx, taskID, sessionID, executor.ResumeOptions{})
}

// ResumeTaskSessionWithOptions restarts a task session with an explicit
// recovery permission. The default ResumeTaskSession path remains unchanged.
//
//nolint:cyclop,gocognit,funlen // Resume coordinates state validation, launch recovery, and ready-state persistence.
func (s *Service) ResumeTaskSessionWithOptions(
	ctx context.Context,
	taskID, sessionID string,
	options executor.ResumeOptions,
) (*executor.TaskExecution, error) {
	s.logger.Debug("resuming task session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))
	releaseLifecycleLock := s.acquireSessionLifecycleLock(sessionID)
	defer releaseLifecycleLock()

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
	s.noteMissingWorktreesBeforeResume(sessionID, session)
	branchRecoveryBefore := s.captureBranchRecoveryBeforeResume(ctx, taskID, sessionID, options)

	// Completed sessions cannot be restarted — they require a new session.
	// Failed and cancelled sessions keep the resume token so the relaunched
	// agent continues the previous conversation (via ACP session/load for
	// native-resume agents, or --resume on CLI). Users who want a fresh start
	// after a failure can invoke RecoverSession with action="fresh_start".
	if session.State == models.TaskSessionStateCompleted {
		return nil, fmt.Errorf("session is completed and cannot be resumed; create a new session instead")
	}

	isOfficeTask, err := s.lookupOfficeTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to determine office task status: %w", err)
	}
	if isOfficeTask {
		return nil, errOfficeTaskResumeRequiresScheduler
	}
	if _, err := s.resolveDynamicLaunchExecution(ctx, session, session.AgentProfileID, true); err != nil {
		return nil, err
	}

	// Bury any open turns from the previous run before relaunching. Without
	// this, startTurnForSession adopts the orphan on the next prompt and the
	// UI's running timer counts from the orphan's started_at — which can be
	// hours or days ago. Zero-duration completion keeps analytics honest about
	// the dead window. A failure here shouldn't block the resume; the next
	// completeTurnForSession sweep will mop up.
	//
	// Drop the activeTurns cache entry first, mirroring completeTurnForSession.
	// Otherwise a stale entry would let getActiveTurnID return the now-abandoned
	// turn ID without re-reading the DB, tagging new messages to a closed turn.
	if s.turnService != nil {
		s.activeTurns.Delete(sessionID)
		if err := s.turnService.AbandonOpenTurns(ctx, sessionID); err != nil {
			s.logger.Warn("failed to abandon orphan turns on resume; continuing",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}

	// Use context.WithoutCancel to prevent WebSocket request timeout from canceling the resume.
	// Session resume can take time and shouldn't be tied to the WS request lifecycle.
	resumeCtx := context.WithoutCancel(ctx)
	persistBranchRecovery := func() {
		if options.AllowBranchReplacement {
			s.persistBranchRecoveryWarnings(resumeCtx, taskID, sessionID, branchRecoveryBefore)
		}
	}
	execution, err := s.executor.ResumeSessionWithOptions(resumeCtx, session, true, options)
	var readySession *models.TaskSession
	if err != nil {
		// If the execution is already running (duplicate resume request), return it as success.
		if errors.Is(err, executor.ErrExecutionAlreadyRunning) {
			execution, readySession, err = s.recoverAlreadyRunningResume(resumeCtx, taskID, sessionID, options)
			if err != nil && errors.Is(err, ErrAgentNotReadyForPrompt) {
				persistBranchRecovery()
				return nil, err
			}
		}
		if err != nil {
			// Task was archived while the resume was in flight — return the error
			// without mutating task/session state (archive already handled cleanup).
			// Check both the sentinel (early rejection) and re-read the task to catch
			// the race where archive completed after the executor's archived check.
			if errors.Is(err, executor.ErrTaskArchived) {
				return nil, err
			}
			if task, taskErr := s.repo.GetTask(resumeCtx, taskID); taskErr == nil && task != nil && task.ArchivedAt != nil {
				return nil, executor.ErrTaskArchived
			}
			persistBranchRecovery()
			// Use resumeCtx (WithoutCancel) for the failure-recording writes too —
			// if the caller's ctx was already cancelled (e.g. WS client navigated
			// away), the SessionStateFailed and TaskStateFailed updates would
			// themselves fail with "context canceled" and leave the task stuck
			// looking "running" forever.
			// ResumeSession launches the workspace directly rather than through
			// LaunchPreparedSession. Record this failure with the same state CAS,
			// persisted recovery claim, and archive-safe task CAS as early launch.
			err = s.branchRecoveryError(resumeCtx, taskID, sessionID, err)
			return nil, s.handleSessionLaunchFailure(
				resumeCtx, taskID, sessionID, err, session,
			)
		}
	}
	if readySession == nil {
		readySession, err = s.waitForResumedSessionReady(resumeCtx, sessionID)
		if err != nil {
			persistBranchRecovery()
			return nil, err
		}
	}
	execution.SessionState = v1.TaskSessionState(readySession.State)
	persistBranchRecovery()

	// Backfill the initial user message when a prior failed launch never got
	// to recordInitialMessage. Without this, the resume can succeed and the
	// agent starts replying, but the chat shows agent output with no user
	// prompt above it.
	//
	// We use task.Description (the raw user input) rather than the
	// workflow-effective prompt produced by applyWorkflowAndPlanMode. The
	// effective prompt may carry a plan-mode prefix or be templated through a
	// workflow step, but reconstructing the exact prompt the original launch
	// sent to the agent is brittle (workflow state may have advanced since).
	// Surfacing the raw description is intentionally conservative: it shows
	// what the user actually typed, which is what they expect to see in chat.
	if task, taskErr := s.repo.GetTask(resumeCtx, taskID); taskErr != nil {
		s.logger.Warn("resume: failed to load task for initial message backfill",
			zap.String("task_id", taskID),
			zap.Error(taskErr))
	} else if task != nil {
		s.backfillInitialUserMessageIfMissing(resumeCtx, taskID, sessionID, task.Description)
	}

	s.logger.Debug("task session resumed and ready for input",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))

	go s.ensureSessionPRWatch(context.Background(), taskID, execution.SessionID, execution.WorktreeBranch)

	return execution, nil
}

func (s *Service) captureBranchRecoveryBeforeResume(
	ctx context.Context,
	taskID, sessionID string,
	options executor.ResumeOptions,
) *branchRecoverySnapshot {
	if !options.AllowBranchReplacement {
		return nil
	}
	snapshot, err := s.captureBranchRecoverySnapshot(ctx, taskID, sessionID)
	if err != nil {
		s.logger.Warn("failed to capture branch recovery state before resume",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil
	}
	return snapshot
}

func (s *Service) waitForResumedSessionReady(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	return s.waitForSessionAndAgentReady(ctx, sessionID, "after resume")
}

func (s *Service) recoverAlreadyRunningResume(
	resumeCtx context.Context,
	taskID string,
	sessionID string,
	options executor.ResumeOptions,
) (*executor.TaskExecution, *models.TaskSession, error) {
	existing, ok := s.executor.GetExecutionBySession(sessionID)
	if !ok || existing == nil {
		return nil, nil, executor.ErrExecutionAlreadyRunning
	}

	readySession, waitErr := s.waitForResumedSessionReady(resumeCtx, sessionID)
	if waitErr == nil {
		existing.SessionState = v1.TaskSessionState(readySession.State)
		return existing, readySession, nil
	}
	if !errors.Is(waitErr, ErrAgentNotReadyForPrompt) {
		return nil, nil, waitErr
	}

	if err := s.reapPromptUnreadyExecution(resumeCtx, sessionID, waitErr); err != nil {
		return nil, nil, fmt.Errorf("%w: failed to stop prompt-unready execution: %w", ErrAgentNotReadyForPrompt, err)
	}

	session, err := s.repo.GetTaskSession(resumeCtx, sessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to reload session after prompt-readiness recovery: %w", err)
	}
	if session.TaskID != taskID {
		return nil, nil, fmt.Errorf("task session does not belong to task")
	}

	execution, err := s.executor.ResumeSessionWithOptions(resumeCtx, session, true, options)
	if err != nil {
		return nil, nil, err
	}
	readySession, err = s.waitForResumedSessionReady(resumeCtx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	execution.SessionState = v1.TaskSessionState(readySession.State)
	return execution, readySession, nil
}

func (s *Service) waitForSessionAndAgentReady(ctx context.Context, sessionID, waitContext string) (*models.TaskSession, error) {
	if err := s.waitForSessionReady(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("session not ready %s: %w", waitContext, err)
	}
	if err := s.waitForAgentPromptReady(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("agent not ready %s: %w", waitContext, err)
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload session %s: %w", waitContext, err)
	}
	return session, nil
}

func (s *Service) waitForStartingSessionPromptable(ctx context.Context, taskID, sessionID string) (*models.TaskSession, error) {
	s.logger.Debug("waiting for starting session to become promptable",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))
	session, err := s.waitForSessionAndAgentReady(ctx, sessionID, "for prompt")
	if err != nil {
		return nil, err
	}
	if err := s.checkSessionPromptable(taskID, sessionID, session.State); err != nil {
		return nil, err
	}
	return session, nil
}

// StartSessionForWorkflowStep starts an existing session with a workflow step's prompt configuration.
// If the session is not running, it will be resumed first. Then a prompt is sent using the
// step's prompt_prefix, prompt_suffix, and plan_mode settings combined with the task description.
func (s *Service) StartSessionForWorkflowStep(ctx context.Context, taskID, sessionID, workflowStepID string) error {
	ctx = withWorkflowMetaCache(ctx)
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
	effectiveProfile := s.resolveStepAgentProfile(ctx, step)
	if effectiveProfile != "" && effectiveProfile != session.AgentProfileID {
		return fmt.Errorf(
			"workflow step profile mismatch: step %q resolves to profile %q but session %q uses profile %q; route the session before prompting",
			workflowStepID,
			effectiveProfile,
			session.ID,
			session.AgentProfileID,
		)
	}

	dbTask, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	if session.ReviewStatus == models.ReviewStatusPending {
		return fmt.Errorf("session is pending approval - use Approve button to proceed or send a message to request changes")
	}

	s.advanceTaskWorkflowStep(ctx, dbTask, workflowStepID, session)

	effectivePrompt := s.buildWorkflowPrompt(ctx, dbTask.Description, step, taskID, sessionID, session.IsPassthrough)

	if err := s.ensureSessionRunning(ctx, sessionID, session); err != nil {
		return err
	}

	// Apply conditional session settings after a manual resume and before the
	// step prompt. The helper reloads the session so a resume-created runtime
	// state cannot be overwritten by the stale request snapshot.
	s.applyWorkflowSessionConfigOnEnter(ctx, taskID, session, step)

	stepPlanMode := step.HasOnEnterAction(wfmodels.OnEnterEnablePlanMode)
	_, err = s.PromptTask(ctx, taskID, sessionID, effectivePrompt, "", stepPlanMode, nil, false)
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
	if session.ReviewStatus != models.ReviewStatusNone {
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
	releaseLifecycleLock := s.acquireSessionLifecycleLock(sessionID)
	defer releaseLifecycleLock()

	isOfficeTask, err := s.lookupOfficeTask(ctx, session.TaskID)
	if err != nil {
		return fmt.Errorf("failed to determine office task status: %w", err)
	}

	// Check if agent is genuinely running (in-memory execution store, not just DB state)
	if exec, ok := s.executor.GetExecutionBySession(sessionID); ok && exec != nil {
		s.recoverAgentPromptStreamIfNeeded(ctx, sessionID)
		if err := s.waitForAgentPromptReady(ctx, sessionID); err != nil {
			if !errors.Is(err, ErrAgentNotReadyForPrompt) {
				return err
			}
			if isOfficeTask {
				return errOfficeTaskResumeRequiresScheduler
			}
			refreshed, reapErr := s.reapAndReloadSession(ctx, sessionID, err)
			if reapErr != nil {
				return reapErr
			}
			session = refreshed
		} else {
			return nil
		}
	}

	s.logger.Debug("agent not running for session, attempting resume",
		zap.String("session_id", sessionID),
		zap.String("session_state", string(session.State)))

	// Bounded to two attempts: a fresh cold resume, and — if the launched
	// agent never reports prompt-ready — one reap-and-retry, mirroring the
	// already-tracked-execution branch above. Without this, a wedged launch
	// right after a backend restart (the common "kandev restart" cold-resume
	// shape: in-memory execution store empty, executors_running row intact)
	// surfaced a bare "agent not ready after resume: ... context deadline
	// exceeded" with no self-heal, unlike every other resume path in this
	// file.
	for attempt := 1; ; attempt++ {
		retryable, err := s.attemptColdResume(ctx, sessionID, session, isOfficeTask)
		if err == nil {
			return nil
		}
		if attempt >= 2 || !retryable {
			return err
		}
		refreshed, reapErr := s.reapAndReloadSession(ctx, sessionID, err)
		if reapErr != nil {
			return reapErr
		}
		session = refreshed
	}
}

// reapAndReloadSession stops a prompt-unready execution and reloads the
// session row afterward. Shared by ensureSessionRunning's already-tracked-
// execution branch and its cold-resume retry loop so both recovery paths
// stay in lockstep.
func (s *Service) reapAndReloadSession(ctx context.Context, sessionID string, cause error) (*models.TaskSession, error) {
	recoveryCtx := context.WithoutCancel(ctx)
	if stopErr := s.reapPromptUnreadyExecution(recoveryCtx, sessionID, cause); stopErr != nil {
		return nil, fmt.Errorf("failed to stop prompt-unready execution: %w", stopErr)
	}
	refreshed, refreshErr := s.repo.GetTaskSession(recoveryCtx, sessionID)
	if refreshErr != nil {
		return nil, fmt.Errorf("failed to reload session after prompt-readiness recovery: %w", refreshErr)
	}
	return refreshed, nil
}

// attemptColdResume performs one resume attempt for ensureSessionRunning's
// cold-resume loop (no execution currently tracked in memory). retryable is
// true only when the caller should reap the stuck execution and retry —
// specifically the main-path prompt-readiness timeout below, which is the
// exact "kandev restart" wedged-launch shape this loop exists to self-heal.
// The concurrent-resume-race branch (ErrExecutionAlreadyRunning) reports its
// own timeout as non-retryable: that failure is against an execution this
// attempt never launched, not a wedged cold launch, so retrying it here would
// blur two distinct failure modes the caller's tests pin down separately.
func (s *Service) attemptColdResume(
	ctx context.Context,
	sessionID string,
	session *models.TaskSession,
	isOfficeTask bool,
) (retryable bool, err error) {
	// If the session is in CREATED state with an existing workspace (executors_running
	// row exists), the workspace was prepared but the agent was never started. Use
	// LaunchPreparedSession which routes to startAgentOnExistingWorkspace to reuse
	// the workspace rather than ResumeSession which tries a full LaunchAgent and
	// conflicts with the existing execution.
	if session.State == models.TaskSessionStateCreated {
		hasRunning, _ := s.repo.HasExecutorRunningRow(ctx, sessionID)
		if hasRunning {
			return false, s.startAgentOnPreparedWorkspace(ctx, sessionID, session)
		}
	}
	if isOfficeTask {
		return false, errOfficeTaskResumeRequiresScheduler
	}

	running, lookupErr := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if lookupErr != nil || running == nil {
		return false, fmt.Errorf("session is not resumable: no executor record (state: %s)", session.State)
	}

	s.noteMissingWorktreesBeforeResume(sessionID, session)

	// Use context.WithoutCancel to prevent WebSocket request timeout from canceling the resume.
	// The lifecycle layer publishes events.AgentBootReady (handled by handleAgentBootReady)
	// when the agent's ACP session initializes — that's what unblocks waitForSessionReady,
	// no flag-tracking needed.
	resumeCtx := context.WithoutCancel(ctx)
	if _, launchErr := s.executor.ResumeSession(resumeCtx, session, true); launchErr != nil {
		if errors.Is(launchErr, executor.ErrExecutionAlreadyRunning) {
			s.recoverAgentPromptStreamIfNeeded(resumeCtx, sessionID)
			if readyErr := s.waitForAgentPromptReady(resumeCtx, sessionID); readyErr != nil {
				return false, fmt.Errorf("agent not ready after resume race: %w", readyErr)
			}
			return false, nil
		}
		return false, s.handleSessionLaunchFailure(
			resumeCtx,
			session.TaskID,
			sessionID,
			fmt.Errorf("failed to resume session: %w", launchErr),
			session,
		)
	}

	// ResumeSession launches the agent asynchronously. Wait for it to finish
	// initializing before returning, so the caller can send a prompt immediately.
	//
	// Use resumeCtx (context.WithoutCancel) here too, not the original ctx: the
	// resume itself is already shielded from the caller's request deadline (see
	// comment above), but a short-lived caller context (WebSocket request,
	// MCP tool-call timeout, etc.) would otherwise still abort these polling
	// waits early with a misleading "context deadline exceeded" even though the
	// resume is progressing fine in the background and would succeed within its
	// own bounded timeouts (waitForSessionReady's AgentLaunchTimeout launch
	// budget, and waitForAgentPromptReady's 30s below).
	if err := s.waitForSessionReady(resumeCtx, sessionID); err != nil {
		return false, fmt.Errorf("session not ready after resume: %w", err)
	}
	readyErr := s.waitForAgentPromptReady(resumeCtx, sessionID)
	if readyErr == nil {
		s.logger.Debug("session resumed and ready for prompt")
		return false, nil
	}
	return errors.Is(readyErr, ErrAgentNotReadyForPrompt), fmt.Errorf("agent not ready after resume: %w", readyErr)
}

func (s *Service) recoverAgentPromptStreamIfNeeded(ctx context.Context, sessionID string) {
	if s.agentManager == nil || s.agentManager.IsAgentReadyForPrompt(ctx, sessionID) {
		return
	}
	recoverer, ok := s.agentManager.(agentPromptStreamRecoverer)
	if !ok {
		return
	}
	if err := recoverer.RecoverAgentPromptStream(ctx, sessionID); err != nil {
		s.logger.Debug("agent prompt stream recovery did not make session ready",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

func (s *Service) reapPromptUnreadyExecution(ctx context.Context, sessionID string, cause error) error {
	if s.agentManager == nil {
		return fmt.Errorf("agent manager is not configured")
	}
	executionID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if err != nil || executionID == "" {
		if running, runErr := s.repo.GetExecutorRunningBySessionID(ctx, sessionID); runErr == nil && running != nil {
			executionID = running.AgentExecutionID
		}
	}
	if executionID == "" {
		if err != nil {
			return fmt.Errorf("execution id for session %s: %w", sessionID, err)
		}
		return fmt.Errorf("execution id for session %s not found", sessionID)
	}
	if !s.claimForcedExecutionCleanup(sessionID, executionID) {
		return fmt.Errorf(
			"execution %s teardown is already owned for session %s",
			executionID,
			sessionID,
		)
	}

	s.logger.Warn("stopping prompt-unready agent execution before resume",
		zap.String("session_id", sessionID),
		zap.String("agent_execution_id", executionID),
		zap.Error(cause))
	if err := s.executor.StopExecution(ctx, executionID, promptReadinessRecoveryStopReason, true); err != nil {
		return err
	}
	s.markExecutionFailed(sessionID, executionID)
	refreshed, err := s.repo.GetTaskSession(ctx, sessionID)
	taskID := ""
	if refreshed != nil {
		taskID = refreshed.TaskID
	}
	s.retireExecutionActivityAndPublish(
		context.WithoutCancel(ctx),
		taskID,
		sessionID,
		executionID,
	)
	if err != nil {
		return fmt.Errorf("reload session after prompt-readiness teardown: %w", err)
	}
	if refreshed == nil {
		return fmt.Errorf("reload session after prompt-readiness teardown: session %s not found", sessionID)
	}
	if isTerminalSessionState(refreshed.State) {
		return &executor.SessionStateSupersededError{
			SessionID: refreshed.ID,
			State:     refreshed.State,
		}
	}
	return nil
}

func (s *Service) waitForAgentPromptReady(ctx context.Context, sessionID string) error {
	if s.agentManager == nil {
		return nil
	}

	readyCtx, cancel := context.WithTimeout(ctx, agentPromptReadyTimeout)
	defer cancel()

	ticker := time.NewTicker(agentPromptReadyInterval)
	defer ticker.Stop()

	for {
		if s.agentManager.IsAgentReadyForPrompt(readyCtx, sessionID) {
			return nil
		}

		select {
		case <-readyCtx.Done():
			if err := ctx.Err(); err != nil {
				return err
			}
			return fmt.Errorf("%w: %w", ErrAgentNotReadyForPrompt, readyCtx.Err())
		case <-ticker.C:
		}
	}
}

// startAgentOnPreparedWorkspace starts the agent subprocess on a session whose workspace
// was prepared (agentctl running) but whose agent process was never started. This avoids
// the "session already has an agent running" error from ResumeSession which tries a full
// LaunchAgent and conflicts with the existing execution in the lifecycle manager's store.
func (s *Service) startAgentOnPreparedWorkspace(ctx context.Context, sessionID string, session *models.TaskSession) error {
	s.logger.Debug("session has prepared workspace but no agent, starting agent on existing workspace",
		zap.String("session_id", sessionID))

	// Boot ready is published as events.AgentBootReady by the lifecycle layer
	// and routed to handleAgentBootReady, which flips the session to
	// WAITING_FOR_INPUT — that's what waitForSessionReady polls for. No flag
	// tracking required here.
	launchCtx := context.WithoutCancel(ctx)
	isOfficeTask, err := s.lookupOfficeTask(launchCtx, session.TaskID)
	if err != nil {
		return fmt.Errorf("failed to determine office task status: %w", err)
	}
	if isOfficeTask {
		return errOfficePreparedResumeRequiresScheduler
	}
	task, err := s.scheduler.GetTask(launchCtx, session.TaskID)
	if err != nil {
		return fmt.Errorf("failed to get task for prepared session: %w", err)
	}
	if _, err := s.ClaimTaskTitleSession(launchCtx, session.TaskID, sessionID); err != nil {
		return fmt.Errorf("failed to claim first-turn task title: %w", err)
	}
	if _, err = s.launchPreparedSessionWithDynamicFallback(launchCtx, task, sessionID, executor.LaunchOptions{
		AgentProfileID: session.AgentProfileID,
		ExecutorID:     session.ExecutorID,
		StartAgent:     true,
	}); err != nil {
		launchErr := fmt.Errorf("failed to start agent on prepared workspace: %w", err)
		return s.handleSessionLaunchFailure(launchCtx, session.TaskID, sessionID, launchErr)
	}

	// Same reasoning as ensureSessionRunning's resume path: use launchCtx
	// (already WithoutCancel'd for the launch call above) so a short-lived
	// caller context doesn't abort these polling waits early.
	if err := s.waitForSessionReady(launchCtx, sessionID); err != nil {
		return fmt.Errorf("session not ready after starting agent: %w", err)
	}
	if err := s.waitForAgentPromptReady(launchCtx, sessionID); err != nil {
		return fmt.Errorf("agent not ready after starting agent: %w", err)
	}
	s.logger.Debug("agent started on prepared workspace and ready for prompt")
	return nil
}

// waitForSessionReady polls the session state until the agent is ready for prompts.
func (s *Service) waitForSessionReady(ctx context.Context, sessionID string) error {
	const pollInterval = 500 * time.Millisecond
	maxWait := constants.AgentLaunchTimeout
	// Derive a bounded context so the overall wait AND each GetTaskSession query
	// inside the loop honor maxWait. Callers pass context.WithoutCancel(ctx) so
	// the wait survives the caller's request timeout during resume/launch — but
	// that context carries no deadline of its own, so without this a single
	// blocking query could hang well past maxWait (the loop only checks the
	// wall clock between iterations). Mirrors waitForAgentPromptReady.
	readyCtx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()
	// Ticker rather than time.After(pollInterval) in the loop: time.After
	// allocates a fresh timer each iteration that lives until it fires, so a
	// long wait would pile up ~maxWait/pollInterval live timers. Mirrors
	// waitForAgentPromptReady.
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-readyCtx.Done():
			return fmt.Errorf("timeout waiting for agent to become ready: %w", readyCtx.Err())
		case <-ticker.C:
		}
		sess, err := s.repo.GetTaskSession(readyCtx, sessionID)
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

// sessionNotFoundStatus is the status-response error used for both a missing
// session and one the caller does not own, so the response cannot be used to
// tell the two apart.
const sessionNotFoundStatus = "session not found"

// GetTaskSessionStatus returns the status of a task session including whether it's resumable
func (s *Service) GetTaskSessionStatus(ctx context.Context, taskID, sessionID string) (dto.TaskSessionStatusResponse, error) {
	s.logger.Debug("checking task session status",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))

	resp := dto.TaskSessionStatusResponse{
		SessionID: sessionID,
		TaskID:    taskID,
	}

	// Per-user scoping: a foreign session is indistinguishable from a missing
	// one, so reuse the same response shape rather than a distinct error.
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		resp.Error = sessionNotFoundStatus
		return resp, nil
	}

	// 1. Load session from database
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		resp.Error = sessionNotFoundStatus
		return resp, nil
	}

	if session.TaskID != taskID {
		resp.Error = "session does not belong to task"
		return resp, nil
	}

	resp.State = string(session.State)
	resp.UpdatedAt = session.UpdatedAt.UTC().Format(time.RFC3339Nano)
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
			zap.String("agent_execution_id", running.AgentExecutionID))
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		refreshedSession, refreshErr := s.repo.GetTaskSession(ctx, sessionID)
		if refreshErr == nil && refreshedSession != nil {
			session = refreshedSession
			resp.State = string(session.State)
			if !session.UpdatedAt.IsZero() {
				resp.UpdatedAt = session.UpdatedAt.UTC().Format(time.RFC3339Nano)
			}
		}
	}

	// Extract worktree info
	populateWorktreeInfo(session, &resp)
	s.populateEnvironmentWorkspaceInfo(ctx, session, &resp)

	// 2. Check if this session's agent is running
	if exec, ok := s.executor.GetExecutionBySession(sessionID); ok && exec != nil {
		resp.IsAgentRunning = true
		resp.NeedsResume = false
		return resp, nil
	}

	// 3. Session can be resumed if it has a resume token
	if resumeToken != "" {
		// Auto-resume FAILED sessions when the runtime is resumable: PR #670 made
		// terminal-state resume safe (cleanup + retry), so recover transparently
		// before surfacing the error. Frontend falls back to restore_workspace if
		// the resume itself fails.
		if session.State == models.TaskSessionStateFailed && running != nil && running.Resumable {
			out := s.validateResumeEligibility(session, resp)
			if out.NeedsResume {
				out.ResumeReason = resumeReasonFailedSessionResumable
			}
			return out, nil
		}
		// A session cancelled by archiving is a system side effect (Service.
		// ArchiveTask / HandoffService cascade), not an explicit user stop —
		// it must recover like a FAILED session once the task is unarchived
		// instead of falling into the terminal-session block below. See
		// models.IsArchiveCancelReason.
		if isArchiveCancelledSession(session) {
			if running != nil && running.Resumable {
				out := s.validateResumeEligibility(session, resp)
				if out.NeedsResume {
					out.ResumeReason = resumeReasonArchiveCancelledResumable
				}
				return out, nil
			}
			// The persisted token's runtime reports Resumable=false — it isn't
			// safe to resume with. Route through the same fresh-start result
			// used when there's no token/running row at all instead of
			// validateResumeEligibility, which never sets IsResumable and
			// would return NeedsResume=true with IsResumable=false, a
			// combination the frontend's auto-resume gate
			// (needs_resume && is_resumable) can never satisfy.
			return evaluateFreshStartResume(session, running, runErr, resp), nil
		}
		// Don't auto-resume other terminal sessions (CANCELLED stays stopped, COMPLETED is done).
		if !isActiveSessionState(session.State) {
			resp.IsAgentRunning = false
			resp.IsResumable = false
			resp.NeedsResume = false
			resp.NeedsWorkspaceRestore = canRestoreWorkspace(&resp)
			return resp, nil
		}
		return s.validateResumeEligibility(session, resp), nil
	}

	// 4. No resume token — check if session can be started fresh.
	return evaluateFreshStartResume(session, running, runErr, resp), nil
}

// evaluateFreshStartResume checks whether a session without a resume token can be
// started fresh. Sessions in error-recovery state (non-empty ErrorMessage) are marked
// resumable but not auto-resumed, so the user sees the error and can choose via action buttons.
func evaluateFreshStartResume(session *models.TaskSession, running *models.ExecutorRunning, runErr error, resp dto.TaskSessionStatusResponse) dto.TaskSessionStatusResponse {
	if runErr == nil && running != nil && isActiveSessionState(session.State) {
		if isErrorRecoveryState(session) {
			resp.IsAgentRunning = false
			resp.IsResumable = true
			resp.NeedsResume = false
			resp.ResumeReason = resumeReasonErrorRecovery
			return resp
		}
		resp.IsAgentRunning = false
		resp.IsResumable = true
		resp.NeedsResume = true
		resp.ResumeReason = "agent_not_running_fresh_start"
		return resp
	}
	// The archive cleanup (Service.ArchiveTask / HandoffService cascade) tears
	// down the ExecutorRunning row entirely, so an archive-cancelled session
	// reaches this function with running == nil — exactly the shape an
	// unarchive-then-open observes. Treat it as fresh-start resumable rather
	// than falling through to read-only workspace restore.
	if isArchiveCancelledSession(session) {
		resp.IsAgentRunning = false
		resp.IsResumable = true
		resp.NeedsResume = true
		resp.ResumeReason = resumeReasonArchiveCancelledResumable
		return resp
	}
	resp.IsAgentRunning = false
	resp.IsResumable = false
	resp.NeedsResume = false
	resp.NeedsWorkspaceRestore = canRestoreWorkspace(&resp)
	return resp
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
	resp.Capabilities.EmbeddedVscode = capabilities.SupportsEmbeddedVscode(execModel.Type, runtime.GOOS)
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
	resp.RemoteStatusErr = publicRemoteStatusError(status.ErrorMessage)
	if status.CreatedAt != nil && !status.CreatedAt.IsZero() {
		resp.RemoteCreatedAt = status.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !status.LastCheckedAt.IsZero() {
		resp.RemoteCheckedAt = status.LastCheckedAt.UTC().Format(time.RFC3339)
	}
}

const remoteStatusUnavailable = "remote executor status is unavailable"

func publicRemoteStatusError(errorMessage string) string {
	if errorMessage == "" {
		return ""
	}
	return remoteStatusUnavailable
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

func (s *Service) populateEnvironmentWorkspaceInfo(ctx context.Context, session *models.TaskSession, resp *dto.TaskSessionStatusResponse) {
	if hasWorktreeStatus(resp) {
		return
	}
	env, err := s.repo.GetTaskEnvironmentByTaskID(ctx, session.TaskID)
	if err != nil || env == nil {
		return
	}
	if session.TaskEnvironmentID != "" && env.ID != session.TaskEnvironmentID {
		return
	}
	if len(env.Repos) == 0 {
		return
	}
	if resp.WorktreePath == nil && env.Repos[0].WorktreePath != "" {
		resp.WorktreePath = &env.Repos[0].WorktreePath
	}
	if resp.WorktreeBranch == nil && env.Repos[0].WorktreeBranch != "" {
		resp.WorktreeBranch = &env.Repos[0].WorktreeBranch
	}
}

func hasWorktreeStatus(resp *dto.TaskSessionStatusResponse) bool {
	return resp != nil &&
		resp.WorktreePath != nil && *resp.WorktreePath != "" &&
		resp.WorktreeBranch != nil && *resp.WorktreeBranch != ""
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

// isErrorRecoveryState returns true when a session is in WAITING_FOR_INPUT with
// a non-empty ErrorMessage, indicating it was set by handleRecoverableFailure.
func isErrorRecoveryState(session *models.TaskSession) bool {
	return session != nil &&
		session.State == models.TaskSessionStateWaitingForInput &&
		session.ErrorMessage != ""
}

// isArchiveCancelledSession reports whether session was cancelled by an
// archive (Service.ArchiveTask's single-task path or HandoffService's
// cascade archive) rather than an explicit user/coordinator stop. Such
// sessions must resume like a FAILED session once the owning task is
// unarchived — see models.IsArchiveCancelReason and its two constants.
func isArchiveCancelledSession(session *models.TaskSession) bool {
	return session != nil &&
		session.State == models.TaskSessionStateCancelled &&
		models.IsArchiveCancelReason(session.ErrorMessage)
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
	// Pre-refactor this also checked session.AgentExecutionID vs running.AgentExecutionID
	// to skip healing on a divergent row. With the executors_running table now the
	// single source of truth, that comparison is structurally always equal — drop it.
	return true
}

// validateResumeEligibility performs final checks before marking a session as resumable.
func (s *Service) validateResumeEligibility(session *models.TaskSession, resp dto.TaskSessionStatusResponse) dto.TaskSessionStatusResponse {
	if session.AgentProfileID == "" {
		resp.Error = "session missing agent profile"
		resp.IsResumable = false
		return resp
	}

	// A missing worktree directory does not make the session unresumable.
	// Archive cleanup deletes the directory and keeps the branch, so reporting
	// IsResumable=false here would hide the resume affordance for every
	// unarchived task — see noteMissingWorktreesBeforeResume. The resume
	// itself recreates the directory through the worktree manager.

	// Don't auto-resume sessions in error-recovery state.
	if isErrorRecoveryState(session) {
		resp.IsAgentRunning = false
		resp.IsResumable = true
		resp.NeedsResume = false
		resp.ResumeReason = resumeReasonErrorRecovery
		return resp
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

// StopTaskForCoordinator gracefully halts every currently-observed live
// execution for taskID without accepting caller-controlled lifecycle options.
// The operation is idempotent: no accepted cancellation is not_running.
func (s *Service) StopTaskForCoordinator(ctx context.Context, taskID string) (CoordinatorTaskStopResult, error) {
	if s.executor == nil {
		return CoordinatorTaskStopResult{}, errors.New("coordinator stop: executor is not configured")
	}
	sessions, err := s.repo.ListActiveTaskSessionsByTaskID(ctx, taskID)
	if err != nil {
		return CoordinatorTaskStopResult{}, fmt.Errorf("coordinator stop: list active sessions for task %q: %w", taskID, err)
	}

	// Repository ordering is launch-oriented. Coordinator control uses stable
	// identity ordering so partial failures and retries are deterministic.
	sort.SliceStable(sessions, func(i, j int) bool {
		return coordinatorStopSessionID(sessions[i]) < coordinatorStopSessionID(sessions[j])
	})

	accepted := 0
	failures := make([]error, 0)
	for _, candidate := range sessions {
		if candidate == nil || candidate.ID == "" {
			failures = append(failures, errors.New("coordinator stop: active session candidate is nil or has an empty ID"))
			continue
		}
		changed, stopErr := s.stopTaskSessionForCoordinator(ctx, taskID, candidate.ID)
		if stopErr != nil {
			failures = append(failures, stopErr)
			continue
		}
		if changed {
			accepted++
		}
	}
	if accepted > 0 {
		// This helper owns Office/archive/active-state guards and publishes
		// through the task-service adapter. Remaining working sessions still
		// block REVIEW on a partial stop.
		s.writeTaskReviewState(ctx, taskID, "")
	}
	if len(failures) > 0 {
		s.logger.Warn("coordinator stop partially failed",
			zap.String("task_id", taskID),
			zap.Int("accepted", accepted),
			zap.Int("failed", len(failures)))
		return CoordinatorTaskStopResult{}, errors.Join(failures...)
	}
	if accepted == 0 {
		return CoordinatorTaskStopResult{Status: CoordinatorTaskStopStatusNotRunning}, nil
	}
	return CoordinatorTaskStopResult{Status: CoordinatorTaskStopStatusStopped}, nil
}

func coordinatorStopSessionID(session *models.TaskSession) string {
	if session == nil {
		return ""
	}
	return session.ID
}

func (s *Service) stopTaskSessionForCoordinator(ctx context.Context, taskID, sessionID string) (bool, error) {
	endCancel := s.beginCancelInFlight(sessionID)
	defer endCancel()

	lock, release := s.acquireCancelInFlightGuard(sessionID)
	lock.Lock()

	result, teardownClaimed, err := s.stopTaskSessionForCoordinatorLocked(ctx, taskID, sessionID)
	lock.Unlock()
	release()
	// The detached teardown callback is allowed to observe coordinator state;
	// clear the cancellation ownership marker before handing it off. The defer
	// above remains as an idempotent safety net for error returns.
	endCancel()
	if err != nil {
		return false, err
	}
	// Detached teardown must not observe this operation as an in-flight
	// cancellation. ScheduleTeardown starts a goroutine, so relying on the
	// deferred release above makes the ordering scheduler-dependent.
	endCancel()
	if result.Changed && teardownClaimed {
		result.ScheduleTeardown()
	}
	return result.Changed, nil
}

// stopTaskSessionForCoordinatorLocked commits cancellation and claims teardown
// ownership while sessionID's cancelInFlight guard is held. The caller must
// release that guard before scheduling the returned teardown.
func (s *Service) stopTaskSessionForCoordinatorLocked(
	ctx context.Context,
	taskID, sessionID string,
) (executor.SessionStopResult, bool, error) {

	// Re-read after acquiring the shared cancel/ready/queue guard. A candidate
	// may have become terminal while this call waited for another decision.
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return executor.SessionStopResult{}, false, fmt.Errorf("coordinator stop: reload session %q: %w", sessionID, err)
	}
	if session == nil {
		return executor.SessionStopResult{}, false, fmt.Errorf("coordinator stop: reload session %q returned nil", sessionID)
	}
	if session.TaskID != taskID {
		return executor.SessionStopResult{}, false, fmt.Errorf("coordinator stop: session %q belongs to task %q, not %q", sessionID, session.TaskID, taskID)
	}
	if !isCoordinatorStoppableSessionState(session.State) {
		return executor.SessionStopResult{FinalState: session.State}, false, nil
	}

	s.taskRuntimeStateMu.Lock()
	// Halt-only intent also disarms any provider-backoff retry. This must run
	// even when the failed execution has already disappeared and the result is
	// therefore not_running; otherwise its timer can launch replacement work.
	s.clearTransientRetryState(sessionID)
	result, stopErr := s.executor.StopSessionDetailed(ctx, session, coordinatorMCPStopReason, false)
	if stopErr == nil && result.Changed {
		// Cancellation takes effect before detached runtime teardown. Tombstone
		// the execution immediately so buffered agent frames cannot recreate
		// session output after the coordinator has acknowledged the stop.
		s.markExecutionFailed(sessionID, result.ExecutionID)
	}
	teardownClaimed := stopErr == nil && result.Changed && s.claimExecutionTeardown(
		sessionID,
		result.ExecutionID,
		executionTeardownIntentGraceful,
	)
	s.taskRuntimeStateMu.Unlock()
	s.resolveTransientRetryMessages(context.WithoutCancel(ctx), sessionID)
	if stopErr != nil {
		return result, false, fmt.Errorf("coordinator stop: session %q: %w", sessionID, stopErr)
	}
	return result, teardownClaimed, nil
}

func isCoordinatorStoppableSessionState(state models.TaskSessionState) bool {
	switch state {
	case models.TaskSessionStateCreated,
		models.TaskSessionStateStarting,
		models.TaskSessionStateRunning,
		models.TaskSessionStateWaitingForInput:
		return true
	default:
		return false
	}
}

// CancelTaskExecution stops active sessions for a task without mutating task state.
// It is used by office tree controls where pause/cancel state transitions are
// handled by the office service itself.
func (s *Service) CancelTaskExecution(ctx context.Context, taskID string, reason string, force bool) error {
	s.logger.Info("cancelling task execution",
		zap.String("task_id", taskID),
		zap.String("reason", reason),
		zap.Bool("force", force))
	return s.executor.StopByTaskID(ctx, taskID, reason, force)
}

// StopSession stops agent execution for a specific session
func (s *Service) StopSession(ctx context.Context, sessionID string, reason string, force bool) error {
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		return err
	}

	// A direct session stop is a true retry-ending transition. Retire the
	// in-memory loop and its durable notice before stopping the execution.
	s.resetTransientRetryWithContext(ctx, sessionID, true)

	s.logger.Info("stopping session execution",
		zap.String("session_id", sessionID),
		zap.String("reason", reason),
		zap.Bool("force", force))
	return s.executor.Stop(ctx, sessionID, reason, force)
}

// StopSessionSynchronously is the internal cleanup variant of StopSession. It
// skips user authorization because task cleanup may run after the task/session
// rows have been removed, and it waits for the executor process to exit before
// returning to the destructive worktree cleanup path.
func (s *Service) StopSessionSynchronously(ctx context.Context, sessionID string, reason string, force bool) error {
	return s.executor.StopSessionSynchronously(ctx, sessionID, reason, force)
}

// DeleteSession deletes a session that is not currently running.
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		return err
	}
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()

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

	// A settled DB state can still own a workspace execution or detached
	// background work. Quiesce that runtime boundary before removing the row.
	if err := s.quiesceSessionExecutionBeforeDeletion(ctx, taskID, sessionID); err != nil {
		return err
	}

	s.logger.Info("deleting session",
		zap.String("session_id", sessionID),
		zap.String("task_id", taskID),
		zap.String("state", string(session.State)),
		zap.Bool("was_primary", wasPrimary))

	if err := s.deleteSessionAndPublishError(ctx, taskID, sessionID); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Drop the in-memory git snapshot throttle entries for an environment only
	// after its session has been removed. The cache is environment-scoped, so a
	// session ID cannot identify the entries and must not be used as the key.
	if s.gitSnapshotCache != nil && session.TaskEnvironmentID != "" {
		s.gitSnapshotCache.forget(session.TaskEnvironmentID)
	}
	// Same reasoning for the push-detection tracker. Multi-repo sessions
	// accumulate one entry per repo; pushTrackerForget walks them all.
	s.pushTrackerForget(sessionID)
	// And the foreground/background turn-activity signal. Execution teardown
	// normally retires it after the final owner exits; deletion forcibly
	// invalidates any trailing token so the removed session cannot be recreated
	// through a stale activity pointer.
	s.clearTurnActivity(sessionID)

	// Pending queue rows are keyed by session_id but counted for the sidebar
	// badge by task_id. Cancel them after the session row is gone and publish
	// queue-status so the status-summary projector drops the orphaned count.
	// Best-effort: the session delete already committed.
	s.cancelDeletedSessionQueue(ctx, taskID, sessionID)

	// Auto-promote another session if we deleted the primary
	if wasPrimary {
		s.promoteNextPrimaryAfterRemoval(ctx, taskID, sessionID)
	}

	return nil
}

func (s *Service) deleteSessionAndPublishError(ctx context.Context, taskID, sessionID string) error {
	lock, release := s.acquireTaskSessionErrorGuard(taskID)
	defer release()
	lock.Lock()
	defer lock.Unlock()

	if err := s.repo.DeleteTaskSession(ctx, sessionID); err != nil {
		return err
	}
	s.publishDeletedSessionError(ctx, taskID, sessionID)
	return nil
}

func (s *Service) publishDeletedSessionError(ctx context.Context, taskID, sessionID string) {
	if s.eventBus == nil {
		return
	}
	eventCtx := context.WithoutCancel(ctx)
	if err := s.publishTaskSessionErrorEvent(eventCtx, taskID, sessionID, false, nil); err != nil {
		s.logger.Warn("failed to publish deleted task session error event",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	retainedSessionID, lastError, err := s.newestRetainedSessionError(eventCtx, taskID)
	if err != nil {
		s.logger.Warn("failed to reload retained task session error after delete",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	if lastError == nil {
		return
	}
	if err := s.publishTaskSessionErrorEvent(eventCtx, taskID, retainedSessionID, true, lastError); err != nil {
		s.logger.Warn("failed to republish retained task session error after delete",
			zap.String("task_id", taskID),
			zap.String("session_id", retainedSessionID),
			zap.Error(err))
	}
}

func (s *Service) newestRetainedSessionError(
	ctx context.Context,
	taskID string,
) (string, *models.LastAgentError, error) {
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return "", nil, err
	}
	var newestSessionID string
	var newest models.LastAgentError
	found := false
	for _, session := range sessions {
		if session == nil {
			continue
		}
		lastError, ok := models.LoadLastAgentError(session.Metadata)
		if !ok || lastError.IsDismissed() {
			continue
		}
		if found && !lastError.OccurredAt.After(newest.OccurredAt) {
			continue
		}
		newestSessionID = session.ID
		newest = lastError
		found = true
	}
	if !found {
		return "", nil, nil
	}
	return newestSessionID, &newest, nil
}

func (s *Service) publishTaskSessionErrorEvent(
	ctx context.Context,
	taskID, sessionID string,
	active bool,
	lastError *models.LastAgentError,
) error {
	eventData := map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"active":     active,
	}
	if active && lastError != nil {
		eventData["message"] = lastError.Message
		eventData["occurred_at"] = lastError.OccurredAt.Format(time.RFC3339Nano)
		eventData["stamp"] = lastError.Stamp()
		eventData["agent_execution_id"] = lastError.AgentExecutionID
		if lastError.TaskRepositoryID != "" {
			eventData["task_repository_id"] = lastError.TaskRepositoryID
		}
		if lastError.Code != "" {
			eventData["category"] = lastError.Code
		}
		if len(lastError.RecoveryActions) > 0 {
			eventData["recovery_actions"] = append([]string(nil), lastError.RecoveryActions...)
		}
		if lastError.RemediationURL != "" {
			eventData["remediation_url"] = lastError.RemediationURL
		}
	}
	return s.eventBus.Publish(ctx, events.TaskSessionErrorChanged, bus.NewEvent(
		events.TaskSessionErrorChanged,
		"orchestrator",
		eventData,
	))
}

// cancelDeletedSessionQueue removes pending prompts left on a deleted session
// and publishes a task-scoped queue status event for the live badge path.
func (s *Service) cancelDeletedSessionQueue(ctx context.Context, taskID, sessionID string) {
	if s.messageQueue == nil {
		return
	}
	if _, err := s.messageQueue.CancelAll(ctx, sessionID); err != nil {
		s.logger.Warn("failed to cancel queued prompts after session delete",
			zap.String("session_id", sessionID),
			zap.String("task_id", taskID),
			zap.Error(err))
	}
	s.publishTaskQueueStatusEvent(ctx, taskID, sessionID)
}

// quiesceSessionExecutionBeforeDeletion stops the in-memory lifecycle
// execution, if one exists, before a session row is removed. A runtime that
// explicitly reports the exact execution as absent is already quiesced; other
// stop failures preserve the row because detaching a possibly-live execution
// would let trailing frames recreate activity after deletion.
func (s *Service) quiesceSessionExecutionBeforeDeletion(
	ctx context.Context,
	taskID, sessionID string,
) error {
	if s.agentManager == nil {
		return nil
	}
	executionID, executionErr := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if executionErr != nil || executionID == "" {
		return nil
	}
	stopErr := s.executor.StopExecution(ctx, executionID, "session deleted", true)
	if stopErr != nil && !errors.Is(stopErr, agentruntime.ErrNotFound) {
		return fmt.Errorf("failed to stop session execution before deletion: %w", stopErr)
	}
	if stopErr != nil {
		s.logger.Debug("session execution already absent during deletion",
			zap.String("session_id", sessionID),
			zap.String("agent_execution_id", executionID),
			zap.Error(stopErr))
	}
	s.markExecutionFailed(sessionID, executionID)
	s.retireExecutionActivityAndPublish(
		context.WithoutCancel(ctx), taskID, sessionID, executionID,
	)
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
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		return err
	}

	if err := s.repo.SetSessionPrimary(ctx, sessionID); err != nil {
		return fmt.Errorf("failed to set session as primary: %w", err)
	}

	// Broadcast task.updated so frontend updates the primary star indicator.
	// The task service's publisher loads primary-session info from the DB,
	// which already reflects the SetSessionPrimary write above.
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
	s.publishTaskUpdated(ctx, task)
	return nil
}

// maxSessionNameLength caps user-supplied session names to keep tab labels sane.
const maxSessionNameLength = 120

// RenameSession sets the user-supplied name for a session and broadcasts a
// session.state_changed event (same state) so all clients update the tab label.
// An empty name clears the custom label, falling back to the derived title.
func (s *Service) RenameSession(ctx context.Context, sessionID, name string) error {
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		return err
	}

	name = strings.TrimSpace(name)
	// Truncate by runes, not bytes — a byte slice could split a multi-byte
	// UTF-8 sequence and persist an invalid string.
	if runes := []rune(name); len(runes) > maxSessionNameLength {
		name = string(runes[:maxSessionNameLength])
	}
	// Fetch BEFORE the write so a post-write lookup failure can never leave a
	// durably renamed row without its broadcast (clients would render stale
	// labels until the next full hydration).
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to load session for rename: %w", err)
	}
	if err := s.repo.RenameTaskSession(ctx, sessionID, name); err != nil {
		return fmt.Errorf("failed to rename session: %w", err)
	}
	session.Name = name
	updatedAt := time.Now().UTC()
	s.publishTaskSessionStateChanged(ctx, session.TaskID, sessionID,
		session.State, session.State, session.ErrorMessage, &updatedAt, session)
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

	baseCommit, baseBranch, err := s.resolveArchiveBaseCommitAndBranch(ctx, sessionID)
	if err != nil {
		return err
	}

	// Skip only if we have neither baseCommit nor baseBranch for merge-base calculation
	if baseCommit == "" && baseBranch == "" {
		s.logger.Debug("no base_commit or base_branch available, skipping archive snapshot capture",
			zap.String("session_id", sessionID))
		return nil
	}

	// Capture commits - baseBranch can be used for merge-base even if baseCommit is empty
	if !s.captureCommitsForTrigger(ctx, sessionID, baseCommit, baseBranch, commitCaptureTriggerArchive) {
		// Agent not running, skip diff capture as well
		return nil
	}

	// Diff capture requires baseCommit
	if baseCommit == "" {
		s.logger.Debug("no base_commit available, skipping archive diff capture",
			zap.String("session_id", sessionID))
		return nil
	}

	s.captureArchiveDiff(ctx, sessionID, baseCommit)
	return nil
}

// resolveArchiveBaseCommitAndBranch retrieves the base commit and branch for archive snapshot capture.
// It first checks the session's stored values, falling back to git status if empty.
func (s *Service) resolveArchiveBaseCommitAndBranch(ctx context.Context, sessionID string) (string, string, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get session: %w", err)
	}

	baseCommit := session.BaseCommitSHA
	baseBranch := session.BaseBranch

	if baseCommit != "" {
		return baseCommit, baseBranch, nil
	}

	// Fallback: try to get base commit from git status for legacy sessions
	status, err := s.agentManager.GetGitStatus(ctx, sessionID)
	if err != nil {
		s.logger.Debug("failed to get git status for base commit fallback",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return "", baseBranch, nil
	}
	if status != nil && status.BaseCommit != "" {
		s.logger.Debug("using git status base commit as fallback for archive",
			zap.String("session_id", sessionID),
			zap.String("base_commit", status.BaseCommit))
		return status.BaseCommit, baseBranch, nil
	}
	return "", baseBranch, nil
}

// captureCommitsForTrigger fetches commits from baseCommit to HEAD and
// persists them tagged with trigger (see persistSessionCommit). If
// targetBranch is provided, uses dynamic merge-base for accurate filtering.
// Returns false if the agent is not running (caller should skip remaining
// capture that also needs it, e.g. archive's diff capture).
func (s *Service) captureCommitsForTrigger(ctx context.Context, sessionID, baseCommit, targetBranch string, trigger commitCaptureTrigger) bool {
	logResult, err := s.agentManager.GetGitLog(ctx, sessionID, baseCommit, 0, targetBranch) // 0 = no limit
	if err != nil {
		s.logger.Warn("failed to capture git log",
			zap.String("session_id", sessionID),
			zap.String("trigger", string(trigger)),
			zap.Error(err))
		return true // Continue with diff capture even if log fails
	}
	if logResult == nil {
		s.logger.Debug("agent not running, skipping commit capture",
			zap.String("session_id", sessionID),
			zap.String("trigger", string(trigger)))
		return false
	}
	s.recordCommitCaptureFetchFailures(sessionID, trigger, logResult)
	if logResult.Success && len(logResult.Commits) > 0 {
		s.persistCommits(ctx, sessionID, trigger, logResult.Commits)
	}
	return true
}

// recordCommitCaptureFetchFailures makes a GetGitLog-level failure
// observable instead of silently dropping it. logResult can fail two ways
// that are otherwise indistinguishable from "nothing to capture this time":
// a total result-level failure (logResult.Success == false, single-repo git
// command failure or every repo failed in a multi-repo fan-out), or a
// partial multi-repo failure (Success == true because at least one repo
// succeeded, but PerRepoErrors names the ones that didn't). A multi-repo
// total failure sets BOTH Success == false AND populates PerRepoErrors for
// every repo (see agentctl/server/api/git.go's mergeGitLogResults), so this
// records one failure per PerRepoErrors entry when present, and only falls
// back to counting the top-level Success == false once when PerRepoErrors is
// empty (the single-repo case) - never both, to avoid double-counting.
func (s *Service) recordCommitCaptureFetchFailures(sessionID string, trigger commitCaptureTrigger, logResult *client.GitLogResult) {
	if len(logResult.PerRepoErrors) > 0 {
		for _, repoErr := range logResult.PerRepoErrors {
			recordCommitCaptureFetchFailed(trigger)
			s.logger.Warn("git log capture failed for repository",
				zap.String("session_id", sessionID),
				zap.String("trigger", string(trigger)),
				zap.String("repository_name", repoErr.RepositoryName),
				zap.String("error", repoErr.Error))
		}
		return
	}
	if !logResult.Success {
		recordCommitCaptureFetchFailed(trigger)
		s.logger.Warn("git log capture reported failure",
			zap.String("session_id", sessionID),
			zap.String("trigger", string(trigger)),
			zap.String("error", logResult.Error))
	}
}

// captureSessionCommitsSweep reconciles task_session_commits against
// agentctl while the agent process is still alive. The live event-driven
// path (handleGitCommitCreated) can miss a commit that was pushed just
// before agentctl's next poll tick (see filterLocalCommits' upstream-commit
// filter), and this sweep is the only trigger guaranteed to run with the
// agent still running - unlike archive capture, whose GetGitLog call
// usually finds the agent process already gone. Cost is one
// `git log --shortstat base..HEAD`, the same command agentctl's own poller
// already runs on every tick. Best-effort: errors are logged and swallowed,
// matching captureGitStatusSnapshot alongside which this is called.
//
// Called from handleAgentCompletedLocked while it holds the per-session
// cancel-in-flight mutex (acquireCancelInFlightGuard) - the same mutex
// stopTaskSessionForCoordinator and DeleteSession need to serve a user's
// Stop/Cancel/Delete for this session. GetGitLog is a real git shellout
// through agentctl with no bound of its own beyond the agentctl HTTP
// client's 60s default timeout, so an undecorated ctx here could hold that
// mutex - and therefore Stop/Cancel/Delete - for up to a minute on every
// single turn completion. Bound it the same way the pre-existing archive
// capture call site already does (service_tasks.go's CaptureArchiveSnapshot:
// "Use a bounded timeout to prevent blocking the archive operation if
// agentctl is stuck"). A timeout here falls through the existing
// warn-and-continue path in captureCommitsForTrigger; archive capture
// remains the reconciliation pass, so nothing is permanently lost.
func (s *Service) captureSessionCommitsSweep(ctx context.Context, sessionID string) {
	sweepCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	baseCommit, baseBranch, err := s.resolveArchiveBaseCommitAndBranch(sweepCtx, sessionID)
	if err != nil {
		s.logger.Debug("failed to resolve base commit for commit sweep",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	if baseCommit == "" && baseBranch == "" {
		return
	}
	s.captureCommitsForTrigger(sweepCtx, sessionID, baseCommit, baseBranch, commitCaptureTriggerSweep)
}

// captureGitStatusSnapshot fetches the current (cached) git status from agentctl
// and saves it as a DB snapshot. Called when a session's execution completes so
// the status is available when clients subscribe later.
func (s *Service) captureGitStatusSnapshot(ctx context.Context, sessionID string) {
	_, _ = s.saveGitStatusSnapshot(ctx, sessionID, false)
}

// captureGitStatusSnapshotWithRetry attempts a fresh capture with up to 3
// retries at 1-second intervals if the first attempt returns stale 0/0 data
// (caused by git lock contention between concurrent worktrees). Returns
// immediately without retrying when the environment or execution is missing,
// or when the agent genuinely has no file changes.
func (s *Service) captureGitStatusSnapshotWithRetry(ctx context.Context, sessionID string) {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Second)
		}
		wrote, noExec := s.saveGitStatusSnapshot(ctx, sessionID, true)
		if noExec {
			return // No execution exists — retrying won't help
		}
		if wrote {
			return // Successfully wrote a snapshot (may be 0/0 for no-change turns, that's fine)
		}
		// saveGitStatusSnapshot returned false without noExec — means fresh
		// returned 0/0 and was skipped due to possible lock contention. Retry.
	}
}

// saveGitStatusSnapshot is the shared implementation for snapshot capture.
// When fresh is true, it bypasses the workspace tracker's poll cache.
// Returns (wrote, noExecution): wrote=true if a snapshot was persisted,
// noExecution=true if the environment or active execution is unavailable.
func (s *Service) saveGitStatusSnapshot(ctx context.Context, sessionID string, fresh bool) (wrote, noExecution bool) {
	taskEnvironmentID, ok := s.resolveGitSnapshotEnvironmentID(ctx, sessionID)
	if !ok {
		// A snapshot cannot be captured without an environment. Stop the
		// retry loop because another attempt cannot resolve this identity.
		return false, true
	}
	var status *client.GitStatusResult
	var err error
	if fresh {
		status, err = s.agentManager.GetGitStatusFresh(ctx, sessionID)
	} else {
		status, err = s.agentManager.GetGitStatus(ctx, sessionID)
	}
	if err != nil {
		s.logger.Debug("failed to capture git status snapshot",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false, false
	}
	if status == nil {
		return false, true // No execution — caller should not retry
	}
	if !status.Success {
		return false, false
	}

	// When a fresh query returns zero additions AND zero deletions, the result
	// may be stale due to git lock contention between concurrent worktrees
	// (merge-base computation fails transiently). Don't overwrite a potentially
	// better live_monitor snapshot that already has the correct non-zero values.
	if fresh && status.BranchAdditions == 0 && status.BranchDeletions == 0 {
		return false, false
	}

	metadata := map[string]interface{}{
		"repository_name":       status.RepositoryName,
		"timestamp":             status.Timestamp,
		"modified":              status.Modified,
		"added":                 status.Added,
		"deleted":               status.Deleted,
		"untracked":             status.Untracked,
		"renamed":               status.Renamed,
		"branch_additions":      status.BranchAdditions,
		"branch_deletions":      status.BranchDeletions,
		"comparison_target":     status.ComparisonTarget,
		"comparison_status":     status.ComparisonStatus,
		"comparison_error_code": status.ComparisonErrorCode,
	}

	if err := s.repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		TaskEnvironmentID: taskEnvironmentID,
		SessionID:         sessionID,
		SnapshotType:      models.SnapshotTypeStatusUpdate,
		Branch:            status.Branch,
		RemoteBranch:      status.RemoteBranch,
		HeadCommit:        status.HeadCommit,
		BaseCommit:        status.BaseCommit,
		Ahead:             status.Ahead,
		Behind:            status.Behind,
		Files:             status.Files,
		TriggeredBy:       gitSnapshotTriggeredByAgentCompleted,
		Metadata:          metadata,
	}); err != nil {
		s.logger.Warn("failed to save git status snapshot",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false, false
	}

	s.logger.Debug("saved git status snapshot",
		zap.String("task_environment_id", taskEnvironmentID),
		zap.String("session_id", sessionID),
		zap.String("branch", status.Branch),
		zap.Bool("fresh", fresh))
	return true, false
}

// captureArchiveDiff fetches and saves the cumulative diff from baseCommit to the working tree
// (including uncommitted/unstaged changes).
func (s *Service) captureArchiveDiff(ctx context.Context, sessionID, baseCommit string) {
	taskEnvironmentID, ok := s.resolveGitSnapshotEnvironmentID(ctx, sessionID)
	if !ok {
		return
	}
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

	snapshot := &models.GitSnapshot{
		TaskEnvironmentID: taskEnvironmentID,
		SessionID:         sessionID,
		SnapshotType:      models.SnapshotTypeArchive,
		HeadCommit:        diffResult.HeadCommit,
		BaseCommit:        diffResult.BaseCommit,
		Files:             diffResult.Files,
	}
	status, statusErr := s.agentManager.GetGitStatusFresh(ctx, sessionID)
	if statusErr != nil {
		s.logger.Warn("failed to capture git status metadata for archive",
			zap.String("session_id", sessionID),
			zap.Error(statusErr))
	} else if status != nil && status.Success {
		snapshot.Branch = status.Branch
		snapshot.RemoteBranch = status.RemoteBranch
		snapshot.Ahead = status.Ahead
		snapshot.Behind = status.Behind
		snapshot.Metadata = archiveGitStatusMetadata(status, diffResult.Files)
	}

	if err := s.repo.CreateGitSnapshot(ctx, snapshot); err != nil {
		s.logger.Warn("failed to save archive snapshot",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	s.logger.Debug("saved archive snapshot",
		zap.String("task_environment_id", taskEnvironmentID),
		zap.String("session_id", sessionID),
		zap.String("head_commit", diffResult.HeadCommit),
		zap.Int("total_commits", diffResult.TotalCommits))
}

func archiveGitStatusMetadata(status *client.GitStatusResult, files map[string]interface{}) map[string]interface{} {
	if status == nil {
		return nil
	}
	return map[string]interface{}{
		"timestamp": status.Timestamp,
		// The archive's files map is a cumulative diff, while these status
		// lists describe the working tree at capture time. Keep an explicit
		// count for summary consumers so they do not add two different views.
		"changed_files":      len(files),
		"modified":           status.Modified,
		"added":              status.Added,
		"deleted":            status.Deleted,
		"untracked":          status.Untracked,
		"renamed":            status.Renamed,
		"remote_ahead":       status.RemoteAhead,
		"remote_behind":      status.RemoteBehind,
		"remote_head_commit": status.RemoteHeadCommit,
		"branch_additions":   status.BranchAdditions,
		"branch_deletions":   status.BranchDeletions,
	}
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

// persistCommits persists a batch of commits fetched via GetGitLog, tagged
// with trigger. Shared by archive capture and the per-turn reconcile sweep
// (captureSessionCommitsSweep) - see persistSessionCommit for the
// idempotent single-commit write and writer-health counters.
func (s *Service) persistCommits(ctx context.Context, sessionID string, trigger commitCaptureTrigger, commits []*client.GitCommitInfo) {
	for _, commit := range commits {
		s.persistSessionCommit(ctx, sessionID, trigger, &models.SessionCommit{
			CommitSHA:     commit.CommitSHA,
			ParentSHA:     commit.ParentSHA,
			AuthorName:    commit.AuthorName,
			AuthorEmail:   commit.AuthorEmail,
			CommitMessage: commit.CommitMessage,
			CommittedAt:   parseCommitTime(commit.CommittedAt),
			FilesChanged:  commit.FilesChanged,
			Insertions:    commit.Insertions,
			Deletions:     commit.Deletions,
		})
	}
	s.logger.Debug("persisted commits",
		zap.String("session_id", sessionID),
		zap.String("trigger", string(trigger)),
		zap.Int("count", len(commits)))
}

// persistSessionCommit is the single write path shared by the live commit
// event (handleGitCommitCreated), the per-turn reconcile sweep
// (captureSessionCommitsSweep), and archive capture (saveArchiveCommits). It
// records writer-health counters (commit_capture_*, see
// commit_capture_metrics.go) so a trigger that silently stops firing is
// observable, and treats CreateSessionCommit reporting a duplicate
// (session_id, commit_sha) as a normal outcome, not a failure - the same
// underlying git commit is expected to be observed by more than one trigger.
func (s *Service) persistSessionCommit(ctx context.Context, sessionID string, trigger commitCaptureTrigger, commit *models.SessionCommit) {
	if s.repo == nil {
		return
	}
	recordCommitCaptureObserved(trigger)
	commit.SessionID = sessionID
	inserted, err := s.repo.CreateSessionCommit(ctx, commit)
	if err != nil {
		recordCommitCaptureFailed(trigger)
		s.logger.Warn("failed to persist session commit",
			zap.String("session_id", sessionID),
			zap.String("commit_sha", commit.CommitSHA),
			zap.String("trigger", string(trigger)),
			zap.Error(err))
		return
	}
	if inserted {
		recordCommitCaptureInserted(trigger)
	} else {
		recordCommitCaptureDuplicate(trigger)
	}
}

// PromptTask sends a follow-up prompt to a running agent for a task session.
// If planMode is true, a plan mode prefix is prepended to the prompt.
// Attachments (images) are passed through to the agent if provided.
func (s *Service) PromptTask(ctx context.Context, taskID, sessionID string, prompt string, model string, planMode bool, attachments []v1.MessageAttachment, dispatchOnly bool) (*PromptResult, error) {
	return s.promptTask(ctx, taskID, sessionID, prompt, model, planMode, attachments, dispatchOnly, promptTaskOptions{})
}

type promptTaskOptions struct {
	claimEntryID          string
	lifecyclePrompt       bool
	afterClaim            func() error
	preservePromptContext bool
	// reserveTurnUntilDispatch persists detached-resume ownership before agentctl
	// dispatch, while delaying the visible turn.started event until acceptance.
	reserveTurnUntilDispatch  bool
	promptDispatchRecovery    *models.PromptDispatchRecovery
	expectedCurrentTurnID     string
	requireNonterminalSession bool
}

type promptDispatchOutcome struct {
	mu             sync.Mutex
	accepted       bool
	publicationErr error
}

func (o *promptDispatchOutcome) recordAccepted(publicationErr error) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.accepted = true
	o.publicationErr = publicationErr
	o.mu.Unlock()
}

func (o *promptDispatchOutcome) snapshot() (bool, error) {
	if o == nil {
		return false, nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.accepted, o.publicationErr
}

// acceptedPromptDispatchError means agentctl accepted the prompt, but durable
// publication or later transport handling failed. Detached clarification
// callers must not reopen the already-delivered answer on this error.
type acceptedPromptDispatchError struct {
	err error
}

func (e *acceptedPromptDispatchError) Error() string {
	return fmt.Sprintf("prompt accepted but post-dispatch handling failed: %v", e.err)
}

func (e *acceptedPromptDispatchError) Unwrap() error { return e.err }

func (*acceptedPromptDispatchError) DetachedResumeAccepted() bool { return true }

func wrapAcceptedPromptDispatchFailure(
	dispatchAccepted bool,
	failureErr, publicationErr error,
) error {
	if !dispatchAccepted {
		return failureErr
	}
	combined := errors.Join(failureErr, publicationErr)
	if combined == nil {
		return nil
	}
	return &acceptedPromptDispatchError{err: combined}
}

func (options promptTaskOptions) executorContext(ctx context.Context) context.Context {
	if options.preservePromptContext {
		return ctx
	}
	return context.WithoutCancel(ctx)
}

func (promptTaskOptions) failureContext(ctx context.Context) (context.Context, context.CancelFunc) {
	// Preserve a small bounded window for state rollback after either the
	// dispatch deadline or an ordinary transport request expires; otherwise a
	// cancelled caller context can leave RUNNING state behind.
	return context.WithTimeout(context.WithoutCancel(ctx), promptFailureCleanupTimeout)
}

// promptTask is PromptTask's implementation. Its options carry queued-dispatch
// ownership and the bounded-context exception. When options.claimEntryID is
// non-empty, this acquires sessionID's cancelInFlight guard around a second
// ownership check and the "mark the session RUNNING" step immediately before
// startTurnForSession and the (potentially long-blocking) executor.Prompt call
// below it. The worker claims the handoff before visible side effects; this
// check keeps the final session transition tied to that same ownership record.
//
// A bare (unguarded) "check the token, then separately call
// setSessionRunning" would still let two settling dispatches for the same
// session both pass their own check before either actually marks the
// session RUNNING — the exact double-dispatch this mechanism exists to
// prevent, just narrowed rather than closed. Serializing the check
// *and* the mark through the same per-session guard used for every other
// cancel/take-and-dispatch decision (see the Service.cancelInFlight field
// doc comment) makes "exactly one dispatch wins" a property of mutual
// exclusion instead of a racy read. Ordinary prompts release the guard after
// this fast, DB-only step. Lifecycle prompts hold the same guard until agentctl
// accepts the prompt, so a context reset cannot cancel the claimed turn in the
// gap before provider dispatch.
//
// Every return path *before* this step (invalid session, not promptable,
// ensureSessionRunning failure, a model switch) is covered by
// executeQueuedMessage's own deferred cleanup instead
// (clearQueuedDispatchInFlightIfCurrent), which is safe since none of them
// block on an agent turn.
func (s *Service) promptTask(ctx context.Context, taskID, sessionID string, prompt string, model string, planMode bool, attachments []v1.MessageAttachment, dispatchOnly bool, options promptTaskOptions) (*PromptResult, error) {
	s.logPromptTaskCall(taskID, sessionID, prompt, model, planMode, attachments, dispatchOnly)
	if err := s.validatePromptTaskStart(sessionID); err != nil {
		return nil, err
	}

	// Only allow prompts when the session is ready for input.
	session, err := s.loadPromptableSession(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	if options.requireNonterminalSession {
		session, err = s.loadNonterminalWorkflowAutoStartSession(ctx, sessionID)
		if err != nil {
			return nil, err
		}
	}
	foregroundClaim, err := s.claimForegroundForPrompt(taskID, sessionID, session)
	if err != nil {
		return nil, err
	}

	// Apply config-mode and plan-mode prompt transforms.
	effectivePrompt := s.effectivePromptForSession(sessionID, prompt, planMode, session)

	// Ensure the agent process is actually running. After a lazy backend restart,
	// the session may be in WAITING_FOR_INPUT but no agent process exists yet.
	_, hadExecutionBeforeEnsure := s.executor.GetExecutionBySession(sessionID)
	resumedForPrompt := !hadExecutionBeforeEnsure
	if err := s.ensureSessionRunning(ctx, sessionID, session); err != nil {
		s.releaseForegroundClaimOnFailure(ctx, taskID, sessionID, foregroundClaim)
		return nil, fmt.Errorf("failed to ensure session is running: %w", err)
	}

	// Reload session after ensureSessionRunning. If a resume happened, ResumeSession
	// updated the session's AgentExecutionID via persistResumeState — but only on the
	// pointer it received. If anything along the way swapped the pointer or the
	// caller's struct is otherwise stale (e.g. a concurrent write), executor.Prompt
	// would call PromptAgent with the OLD execution ID and get ErrExecutionNotFound.
	// Re-reading from the DB after ensureSessionRunning guarantees we use the
	// freshly-persisted AgentExecutionID.
	session = s.reloadPromptSession(ctx, sessionID, session)
	// Re-apply transforms in case metadata changed during ensureSessionRunning.
	effectivePrompt = s.effectivePromptForSession(sessionID, prompt, planMode, session)
	activityExecutionID, _ := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	foregroundDispatch := s.beginForegroundDispatch(sessionID, foregroundClaim, activityExecutionID)
	if foregroundDispatch == nil {
		s.releaseForegroundClaimOnFailure(ctx, taskID, sessionID, foregroundClaim)
		return nil, fmt.Errorf("%w, please wait for completion", ErrAgentPromptInProgress)
	}

	// Cache and reserve the replay identity before model switching. A restart
	// based switch dispatches its prompt from StartAgentProcess, before the
	// ordinary PromptTask admission path can initialize these records.
	s.rememberTurnPrompt(sessionID, prompt, model, planMode, attachments)
	if modelSwitchRequired(session, model) {
		s.beginInitialPromptAttempt(sessionID, s.isDynamicPromptSession(session))
	}

	if result, handled, switchErr := s.trySwitchModelForPrompt(
		ctx, taskID, sessionID, model, effectivePrompt, session, foregroundDispatch,
	); handled {
		if switchErr != nil {
			s.clearPromptAttemptEvidence(sessionID, "", 0)
		}
		return result, switchErr
	}

	session, rollback, err := s.claimPromptDispatch(
		ctx, taskID, sessionID, options.claimEntryID, options.lifecyclePrompt,
		options.reserveTurnUntilDispatch, options.promptDispatchRecovery,
		options.afterClaim, foregroundClaim, options.expectedCurrentTurnID,
		options.requireNonterminalSession,
	)
	if err != nil {
		s.rollbackForegroundDispatchOnFailure(ctx, taskID, sessionID, foregroundDispatch)
		return nil, err
	}
	// beginForegroundDispatch atomically established foreground-generating
	// ownership before any cleanup/provider event could interleave. Publish that
	// flip now that session admission succeeded; claimed RUNNING prompts also need
	// the broadcast because their earlier atomic claim made the same transition.
	if foregroundDispatch.yieldedBeforeBegin || foregroundClaim != nil {
		s.publishForegroundActivityChanged(ctx, taskID, sessionID)
	}
	var releaseDispatchGuard func()
	if rollback.dispatchGuardRelease != nil {
		releaseDispatchGuard = rollback.dispatchGuardRelease
		defer releaseDispatchGuard()
	}
	if options.requireNonterminalSession && releaseDispatchGuard == nil {
		var guard *sync.Mutex
		guard, releaseDispatchGuard = s.acquireCancelInFlightGuard(sessionID)
		guard.Lock()
		fresh, reloadErr := s.repo.GetTaskSession(ctx, sessionID)
		if reloadErr != nil {
			guard.Unlock()
			releaseDispatchGuard()
			failureCtx, cancel := options.failureContext(ctx)
			defer cancel()
			s.rollbackForegroundDispatchOnFailure(failureCtx, taskID, sessionID, foregroundDispatch)
			s.rollbackPromptClaim(failureCtx, taskID, sessionID, rollback)
			return nil, reloadErr
		}
		if fresh == nil || isTerminalSessionState(fresh.State) {
			guard.Unlock()
			releaseDispatchGuard()
			failureCtx, cancel := options.failureContext(ctx)
			defer cancel()
			s.rollbackForegroundDispatchOnFailure(failureCtx, taskID, sessionID, foregroundDispatch)
			s.rollbackPromptClaim(failureCtx, taskID, sessionID, rollback)
			return nil, newWorkflowAutoStartSessionTerminalizedError(fresh)
		}
		var releaseOnce sync.Once
		originalRelease := releaseDispatchGuard
		releaseDispatchGuard = func() {
			releaseOnce.Do(func() {
				guard.Unlock()
				originalRelease()
			})
		}
		// The agentctl dispatch callback is the acceptance boundary. Releasing
		// here keeps terminalization mutually exclusive with admission without
		// holding the session guard for the rest of the (potentially long) turn.
		defer releaseDispatchGuard()
	}

	// Only bounded-ack callers preserve request context; ordinary prompts can take minutes.
	promptCtx := options.executorContext(ctx)
	s.bindPromptTurnID(promptCtx, session, rollback.turnID)
	s.beginInteractivePromptAttempt(
		promptCtx,
		sessionID,
		session.AgentExecutionID,
		s.isDynamicPromptSession(session),
	)
	dispatchOutcome := &promptDispatchOutcome{}
	onDispatched := s.promptDispatchCallback(
		promptCtx, taskID, sessionID, rollback.reservedTurn, foregroundDispatch, dispatchOutcome,
	)
	if releaseDispatchGuard != nil {
		originalOnDispatched := onDispatched
		onDispatched = func() {
			releaseDispatchGuard()
			originalOnDispatched()
		}
	}
	result, err := s.executor.PromptWithDispatchCallback(
		promptCtx, taskID, sessionID, effectivePrompt, attachments, dispatchOnly,
		onDispatched, session,
	)
	dispatchAccepted, publicationErr := dispatchOutcome.snapshot()
	if err != nil {
		return s.finishPromptDispatchFailure(
			ctx, taskID, sessionID, prompt, planMode, resumedForPrompt, attachments,
			rollback, options, err, foregroundDispatch, dispatchAccepted, publicationErr,
		)
	}
	if publicationErr != nil {
		return nil, &acceptedPromptDispatchError{err: publicationErr}
	}
	return &PromptResult{StopReason: result.StopReason, AgentMessage: result.AgentMessage, TurnID: rollback.turnID}, nil
}

func (s *Service) finishPromptDispatchFailure(
	ctx context.Context,
	taskID, sessionID, prompt string,
	planMode, resumedForPrompt bool,
	attachments []v1.MessageAttachment,
	rollback promptClaimRollback,
	options promptTaskOptions,
	promptErr error,
	foregroundDispatch *foregroundDispatch,
	dispatchAccepted bool,
	publicationErr error,
) (*PromptResult, error) {
	failureCtx, cancel := options.failureContext(ctx)
	defer cancel()
	s.rollbackForegroundDispatchOnFailure(failureCtx, taskID, sessionID, foregroundDispatch)
	if dispatchAccepted && rollback.reservedTurn != nil {
		// Agentctl acceptance made the turn authoritative. A later completion
		// error may reconcile session state, but must not delete this turn.
		rollback.reservedTurnAccepted = true
	}
	failureResult, failureErr := s.handlePromptDispatchFailure(
		failureCtx, taskID, sessionID, prompt, planMode, resumedForPrompt,
		attachments, rollback, options.lifecyclePrompt, dispatchAccepted, promptErr,
	)
	return failureResult, wrapAcceptedPromptDispatchFailure(
		dispatchAccepted,
		failureErr,
		publicationErr,
	)
}

func (s *Service) reloadPromptSession(
	ctx context.Context,
	sessionID string,
	fallback *models.TaskSession,
) *models.TaskSession {
	reloaded, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || reloaded == nil {
		return fallback
	}
	return reloaded
}

func (s *Service) bindPromptTurnID(ctx context.Context, session *models.TaskSession, turnID string) {
	setter, ok := s.agentManager.(executor.PromptTurnIDSetter)
	if turnID == "" || session == nil || !ok {
		return
	}
	if err := setter.SetPromptTurnID(ctx, session.AgentExecutionID, turnID); err != nil {
		s.logger.Warn("failed to bind prompt turn before dispatch",
			zap.String("session_id", session.ID),
			zap.String("turn_id", turnID),
			zap.Error(err))
	}
}

func (s *Service) validatePromptTaskStart(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if s.isSessionResetInProgress(sessionID) {
		return ErrSessionResetInProgress
	}
	return nil
}

func (s *Service) logPromptTaskCall(
	taskID, sessionID, prompt, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
) {
	s.logger.Debug("PromptTask called",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.Int("prompt_length", len(prompt)),
		zap.String("requested_model", model),
		zap.Bool("plan_mode", planMode),
		zap.Int("attachments_count", len(attachments)),
		zap.Bool("dispatch_only", dispatchOnly))
}

func (s *Service) loadPromptableSession(ctx context.Context, taskID, sessionID string) (*models.TaskSession, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if promptErr := s.checkSessionPromptable(taskID, sessionID, session.State); promptErr != nil {
		if !errors.Is(promptErr, ErrSessionNotPromptable) || session.State != models.TaskSessionStateStarting {
			return nil, promptErr
		}
		return s.waitForStartingSessionPromptable(ctx, taskID, sessionID)
	}
	return session, nil
}

// loadNonterminalWorkflowAutoStartSession reloads a session under the shared
// cancellation guard before auto-start can trigger a lazy ACP resume. Manual
// prompts intentionally do not use this path and can still resume terminal
// history when explicitly requested.
func (s *Service) loadNonterminalWorkflowAutoStartSession(
	ctx context.Context,
	sessionID string,
) (*models.TaskSession, error) {
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("reload workflow auto-start session: %w", err)
	}
	if session == nil || isTerminalSessionState(session.State) {
		return nil, newWorkflowAutoStartSessionTerminalizedError(session)
	}
	return session, nil
}

// claimForegroundForPrompt serializes two callers racing to take the
// background-idle foreground turn. For non-experiment RUNNING sessions,
// checkSessionPromptable already rejects the prompt before this helper is reached.
func (s *Service) claimForegroundForPrompt(taskID, sessionID string, session *models.TaskSession) (*foregroundClaim, error) {
	if session.State != models.TaskSessionStateRunning {
		return nil, nil
	}
	claim := s.claimForegroundTurn(sessionID)
	if claim != nil {
		return claim, nil
	}
	s.logger.Warn("rejected prompt: another prompt claimed the background-idle foreground turn first",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))
	return nil, fmt.Errorf("%w, please wait for completion", ErrAgentPromptInProgress)
}

func (s *Service) releaseForegroundClaimOnFailure(ctx context.Context, taskID, sessionID string, claim *foregroundClaim) {
	if s.releaseForegroundClaim(claim) {
		s.publishForegroundActivityChanged(ctx, taskID, sessionID)
	}
}

func (s *Service) rollbackForegroundDispatchOnFailure(
	ctx context.Context,
	taskID, sessionID string,
	dispatch *foregroundDispatch,
) {
	if s.rollbackForegroundDispatch(dispatch) {
		s.publishForegroundActivityChanged(ctx, taskID, sessionID)
	}
}

func (s *Service) promptDispatchCallback(
	ctx context.Context,
	taskID, sessionID string,
	reservedTurn *models.Turn,
	dispatch *foregroundDispatch,
	outcome *promptDispatchOutcome,
) func() {
	return func() {
		executionID, _ := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
		s.bindPromptAttemptToExecution(ctx, sessionID, executionID)
		var publicationErr error
		if reservedTurn != nil && s.turnService != nil {
			// Agentctl has already accepted the prompt. Publication must not inherit
			// an expiring HTTP context or the caller could receive success while the
			// durable turn still looks unpublished at restart.
			publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), promptFailureCleanupTimeout)
			publicationErr = s.turnService.PublishReservedTurn(publishCtx, reservedTurn)
			cancel()
			if publicationErr != nil {
				s.logger.Error("failed to persist or publish accepted prompt turn",
					zap.String("session_id", sessionID),
					zap.String("turn_id", reservedTurn.ID),
					zap.Error(publicationErr))
			}
			// The pre-dispatch attempt marker makes restart recovery and rollback
			// fail closed even if this post-acceptance publication fails. Do not
			// restore an active-turn cache entry when cancellation already made the
			// reservation terminal or publication could not find it.
			if reservedTurn.CompletedAt == nil && !errors.Is(publicationErr, sql.ErrNoRows) {
				s.activeTurns.Store(sessionID, reservedTurn.ID)
				s.bindAcceptedDispatchTurn(sessionID, reservedTurn.ID)
			}
			s.resolveReservedPromptTurn(sessionID, reservedTurn.ID, true)
		}
		outcome.recordAccepted(publicationErr)
		if s.acceptForegroundDispatch(dispatch) {
			s.publishForegroundActivityChanged(ctx, taskID, sessionID)
		}
	}
}

// trySwitchModelForPrompt keeps foreground admission consistent when a model
// switch either dispatches the prompt itself or fails before reaching the agent.
func (s *Service) trySwitchModelForPrompt(ctx context.Context, taskID, sessionID, model, prompt string, session *models.TaskSession, dispatch *foregroundDispatch) (*PromptResult, bool, error) {
	result, switched, err := s.trySwitchModel(ctx, taskID, sessionID, model, prompt, session)
	if !switched && err == nil {
		return nil, false, nil
	}
	if err != nil {
		s.rollbackForegroundDispatchOnFailure(ctx, taskID, sessionID, dispatch)
		return result, true, err
	}
	if executionID, executionErr := s.agentManager.GetExecutionIDForSession(ctx, sessionID); executionErr == nil && executionID != "" {
		s.bindPromptAttemptToExecution(ctx, sessionID, executionID)
	}
	revealedBackground := s.acceptForegroundDispatch(dispatch)
	if revealedBackground || dispatch.yieldedBeforeBegin || s.acceptedForegroundDispatchClaim(dispatch) {
		s.publishForegroundActivityChanged(ctx, taskID, sessionID)
	}
	return result, true, nil
}

type promptClaimRollback struct {
	previousSessionState models.TaskSessionState
	previousTaskState    v1.TaskState
	taskStateClaimed     bool
	turnID               string
	createdTurn          bool
	reservedTurn         *models.Turn
	reservedTurnAccepted bool
	dispatchGuardRelease func()
}

func (s *Service) claimPromptDispatch(
	ctx context.Context,
	taskID, sessionID, claimEntryID string,
	lifecyclePrompt bool,
	reserveTurnUntilDispatch bool,
	promptDispatchRecovery *models.PromptDispatchRecovery,
	afterClaim func() error,
	foregroundClaim *foregroundClaim,
	expectedCurrentTurnID string,
	requireNonterminalSession bool,
) (*models.TaskSession, promptClaimRollback, error) {
	if lifecyclePrompt {
		return s.claimLifecyclePromptDispatch(
			ctx, taskID, sessionID, claimEntryID, afterClaim,
		)
	}
	claimed, previousState, turnID, createdTurn, reservedTurn, err := s.claimSessionRunningForPrompt(
		ctx, taskID, sessionID, claimEntryID, reserveTurnUntilDispatch,
		promptDispatchRecovery, foregroundClaim, expectedCurrentTurnID, requireNonterminalSession,
	)
	if err != nil {
		return nil, promptClaimRollback{}, err
	}
	rollback := promptClaimRollback{
		previousSessionState: previousState,
		turnID:               turnID,
		createdTurn:          createdTurn,
		reservedTurn:         reservedTurn,
	}
	return claimed, rollback, nil
}

func (s *Service) claimLifecyclePromptDispatch(
	ctx context.Context,
	taskID, sessionID, claimEntryID string,
	afterClaim func() error,
) (*models.TaskSession, promptClaimRollback, error) {
	session, previousSessionState, turnID, createdTurn, err := s.claimLifecycleSessionRunning(
		ctx, taskID, sessionID, claimEntryID,
	)
	if err != nil {
		return nil, promptClaimRollback{}, err
	}
	rollback := promptClaimRollback{
		previousSessionState: previousSessionState,
		turnID:               turnID,
		createdTurn:          createdTurn,
	}
	rollback.previousTaskState, rollback.taskStateClaimed, err = s.reconcileLifecycleClaim(
		ctx, taskID, sessionID, previousSessionState, session,
	)
	if err != nil {
		s.rollbackPromptClaimAfterAdmissionFailure(ctx, taskID, sessionID, rollback)
		return nil, promptClaimRollback{}, err
	}
	dispatchGuard := s.lockCancelInFlightGuard(sessionID)
	var releaseGuardOnce sync.Once
	releaseDispatchGuard := func() {
		releaseGuardOnce.Do(dispatchGuard.release)
	}
	keepDispatchGuard := false
	defer func() {
		if !keepDispatchGuard {
			releaseDispatchGuard()
		}
	}()
	rollback.dispatchGuardRelease = releaseDispatchGuard
	if err := s.validateLifecyclePromptTurn(ctx, sessionID, rollback.turnID); err != nil {
		s.rollbackPromptClaimAfterAdmissionFailure(ctx, taskID, sessionID, rollback)
		return nil, promptClaimRollback{}, err
	}
	if err := s.acknowledgePromptClaim(ctx, taskID, sessionID, afterClaim, rollback); err != nil {
		return nil, promptClaimRollback{}, err
	}
	keepDispatchGuard = true
	return session, rollback, nil
}

func (s *Service) acknowledgePromptClaim(
	ctx context.Context,
	taskID, sessionID string,
	afterClaim func() error,
	rollback promptClaimRollback,
) error {
	if afterClaim == nil {
		return nil
	}
	if err := afterClaim(); err != nil {
		s.rollbackPromptClaim(ctx, taskID, sessionID, rollback)
		if errors.Is(err, errLifecyclePromptReservationSuperseded) {
			return err
		}
		return fmt.Errorf("%w: %v", errLifecyclePromptMessagePersistence, err)
	}
	return nil
}

func (s *Service) rollbackPromptClaimAfterAdmissionFailure(
	ctx context.Context,
	taskID, sessionID string,
	rollback promptClaimRollback,
) {
	failureCtx, cancel := (promptTaskOptions{}).failureContext(ctx)
	defer cancel()
	s.rollbackPromptClaim(failureCtx, taskID, sessionID, rollback)
}

func (s *Service) rollbackPromptClaim(
	ctx context.Context,
	taskID, sessionID string,
	rollback promptClaimRollback,
) {
	if rollback.dispatchGuardRelease != nil {
		rollback.dispatchGuardRelease()
	}
	s.restoreLifecycleClaim(ctx, taskID, sessionID, rollback.previousSessionState)
	if rollback.taskStateClaimed {
		s.restoreLifecycleTaskState(ctx, taskID, rollback.previousTaskState)
	}
	if rollback.createdTurn {
		if rollback.reservedTurn != nil {
			if !rollback.reservedTurnAccepted {
				s.rollbackReservedPromptTurn(ctx, sessionID, rollback.turnID)
			}
		} else {
			s.completeTurnIfCurrent(ctx, sessionID, rollback.turnID)
		}
	}
}

func (s *Service) rollbackReservedPromptTurn(ctx context.Context, sessionID, turnID string) {
	if s.turnService == nil || turnID == "" {
		return
	}
	// Publication removes the private reservation. A later transport/completion
	// error must not delete a turn that agentctl already accepted.
	if s.reservedPromptTurnID(sessionID) != turnID {
		return
	}
	rolledBack, err := s.turnService.RollbackReservedTurn(ctx, sessionID, turnID)
	if err != nil {
		// The database outcome can be ambiguous (for example, a commit error).
		// Keep the live reservation unresolved so ready handling waits and later
		// prompts remain blocked until restart recovery reconciles durable state.
		s.logger.Error("failed to roll back reserved prompt turn; session remains quarantined",
			zap.String("session_id", sessionID),
			zap.String("turn_id", turnID),
			zap.Error(err))
		return
	}
	s.resolveReservedPromptTurn(sessionID, turnID, false)
	if rolledBack {
		s.activeTurns.CompareAndDelete(sessionID, turnID)
		return
	}
	active, lookupErr := s.turnService.GetActiveTurn(ctx, sessionID)
	if lookupErr == nil && active != nil && active.ID == turnID {
		s.activeTurns.Store(sessionID, turnID)
	}
}

// reconcileLifecycleClaim makes the atomic SQLite RUNNING claim observable to
// the task runtime before a lifecycle prompt creates visible work or reaches
// the executor.
func (s *Service) reconcileLifecycleClaim(
	ctx context.Context,
	taskID, sessionID string,
	previousState models.TaskSessionState,
	session *models.TaskSession,
) (v1.TaskState, bool, error) {
	previousTaskState, taskStateClaimed, err := s.claimLifecycleTaskState(ctx, taskID, sessionID)
	if err != nil {
		s.restoreLifecycleClaim(ctx, taskID, sessionID, previousState)
		return "", false, fmt.Errorf("%w: reconcile lifecycle task state: %v", errLifecyclePromptClaim, err)
	}
	if session == nil || session.State != models.TaskSessionStateRunning {
		s.restoreLifecycleClaim(ctx, taskID, sessionID, previousState)
		if taskStateClaimed {
			s.restoreLifecycleTaskState(ctx, taskID, previousTaskState)
		}
		return "", false, fmt.Errorf("%w: lifecycle claim no longer owns running session", errLifecyclePromptClaim)
	}
	s.publishTaskSessionStateChanged(
		ctx, taskID, sessionID, previousState, models.TaskSessionStateRunning,
		"", &session.UpdatedAt, session,
	)
	return previousTaskState, taskStateClaimed, nil
}

func (s *Service) claimLifecycleTaskState(
	ctx context.Context,
	taskID, sessionID string,
) (v1.TaskState, bool, error) {
	s.taskRuntimeStateMu.Lock()
	defer s.taskRuntimeStateMu.Unlock()

	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return "", false, err
	}
	if task == nil || taskArchived(task) || task.AssigneeAgentProfileID != "" {
		return "", false, nil
	}
	taskState, err := s.taskRepo.GetTask(ctx, taskID)
	if err != nil {
		return "", false, err
	}
	previousTaskState := task.State
	if taskState != nil {
		previousTaskState = taskState.State
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return "", false, err
	}
	if !runtimeSessionOwnsTaskState(session, v1.TaskStateInProgress) {
		return previousTaskState, false, nil
	}
	updated, err := s.taskRepo.UpdateTaskStateIfSessionState(
		ctx, taskID, sessionID, session.State, v1.TaskStateInProgress,
	)
	if err != nil {
		return "", false, err
	}
	return previousTaskState, updated, nil
}

func (s *Service) restoreLifecycleTaskState(
	ctx context.Context,
	taskID string,
	previousState v1.TaskState,
) {
	if previousState == "" {
		return
	}
	updated, err := s.taskRepo.UpdateTaskStateIfCurrentIn(
		ctx, taskID, previousState, []v1.TaskState{v1.TaskStateInProgress},
	)
	if err != nil {
		s.logger.Error("failed to restore task state after lifecycle prompt rejection",
			zap.String("task_id", taskID),
			zap.String("previous_state", string(previousState)),
			zap.Error(err))
		return
	}
	if !updated {
		s.logger.Debug("skipped lifecycle task-state restore after concurrent transition",
			zap.String("task_id", taskID),
			zap.String("previous_state", string(previousState)))
	}
}

// handlePromptDispatchFailure handles a failed executor.Prompt call from
// promptTask. If the failure is the specific "missing execution" error a
// just-resumed session can hit (see promptTask's resumedForPrompt comment),
// it falls back to a fresh launch instead of surfacing the error to the
// caller. Otherwise — or if that fallback launch itself fails — it
// delegates to handlePromptError for the caller-facing result.
func (s *Service) handlePromptDispatchFailure(
	ctx context.Context,
	taskID, sessionID, prompt string,
	planMode, resumedForPrompt bool,
	attachments []v1.MessageAttachment,
	rollback promptClaimRollback,
	lifecyclePrompt bool,
	dispatchAccepted bool,
	promptErr error,
) (*PromptResult, error) {
	if resumedForPrompt && !dispatchAccepted && !rollback.reservedTurnAccepted && rollback.reservedTurn == nil &&
		errors.Is(promptErr, executor.ErrExecutionNotFound) {
		s.logger.Warn("prompt after lazy resume hit missing execution; falling back to fresh launch",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
		if freshErr := s.fallbackFreshLaunchOnMissingExecution(
			ctx, taskID, sessionID, prompt, planMode, nil, attachments, nil,
		); freshErr == nil {
			return &PromptResult{}, nil
		} else {
			promptErr = freshErr
		}
	}
	if lifecyclePrompt {
		s.rollbackPromptClaim(ctx, taskID, sessionID, rollback)
		return nil, promptErr
	}
	if rollback.createdTurn && rollback.reservedTurn != nil && !rollback.reservedTurnAccepted {
		s.rollbackReservedPromptTurn(ctx, sessionID, rollback.turnID)
	}
	return nil, s.handlePromptError(ctx, taskID, sessionID, rollback.previousSessionState, promptErr)
}

// effectivePromptForSession applies promptTask's config-mode and plan-mode
// prompt transforms to the raw prompt, in that order, using session's
// *current* Metadata. Each call always starts from the raw prompt argument
// (never a previously-transformed result), so it is safe for promptTask to
// call this twice — once on session load and again after a reload that
// follows ensureSessionRunning, in case metadata changed in between —
// without double-prepending either transform's system-prompt wrapper.
func (s *Service) effectivePromptForSession(sessionID, prompt string, planMode bool, session *models.TaskSession) string {
	effectivePrompt := prompt
	if cm, ok := session.Metadata["config_mode"].(bool); ok && cm {
		effectivePrompt = sysprompt.InjectConfigContext(sessionID, prompt)
	}
	if planMode {
		effectivePrompt = sysprompt.InjectPlanMode(effectivePrompt)
	}
	return effectivePrompt
}

var (
	errLifecyclePromptInactive              = errors.New("lifecycle prompt task or session is inactive")
	errLifecyclePromptClaim                 = errors.New("lifecycle prompt claim failed")
	errLifecyclePromptMessagePersistence    = errors.New("lifecycle prompt message persistence failed")
	errLifecyclePromptReservationSuperseded = errors.New("lifecycle prompt reservation was superseded")
)

type lifecyclePromptReselectedError struct {
	sessionID string
}

func (e *lifecyclePromptReselectedError) Error() string {
	return fmt.Sprintf("lifecycle prompt session reselected: %s", e.sessionID)
}

// claimSessionRunningForPrompt marks sessionID RUNNING immediately before
// promptTask dispatches to the agent, and returns the session to use for
// the rest of the call (possibly reloaded).
//
// Direct and queued prompts both claim RUNNING under the per-session
// cancelInFlight guard. This makes the final "still promptable, now owned by
// this turn" decision atomic with coordinator stop; the potentially blocking
// executor.Prompt call remains outside the guard.
//
// When claimEntryID is set, the final claim happens under the same per-session
// cancelInFlightGuard every other cancel/take-and-dispatch decision in this
// file serializes through. Re-verifying
// ownership *and* promptability inside this same critical section,
// immediately before writing RUNNING, closes a gap that checking the token
// alone would leave open: nothing would otherwise stop this call from
// blindly marking the session RUNNING over top of a state some other
// guard-synchronized decision-maker (a cancel, another claim) already
// changed since promptTask's much-earlier, unguarded checkSessionPromptable
// call near its top. Reloading the session and re-running
// checkSessionPromptable here makes "is it actually still safe to mark this
// session RUNNING right now" an atomic fact, not a stale read.
func (s *Service) claimSessionRunningForPrompt(
	ctx context.Context,
	taskID, sessionID, claimEntryID string,
	reserveTurnUntilDispatch bool,
	promptDispatchRecovery *models.PromptDispatchRecovery,
	foregroundClaim *foregroundClaim,
	expectedCurrentTurnID string,
	requireNonterminalSession bool,
) (*models.TaskSession, models.TaskSessionState, string, bool, *models.Turn, error) {
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	// A reset-owned cancellation must not make prompt admission wait for the
	// reset that already forbids it. Keep the second check below for a reset
	// that starts while this prompt is waiting on an unrelated cancellation.
	if s.isSessionResetInProgress(sessionID) {
		return nil, "", "", false, nil, ErrSessionResetInProgress
	}
	if err := s.waitForCancellationWithGuard(ctx, sessionID, lock.Unlock, lock.Lock); err != nil {
		return nil, "", "", false, nil, err
	}
	if s.isSessionResetInProgress(sessionID) {
		return nil, "", "", false, nil, ErrSessionResetInProgress
	}
	if expectedCurrentTurnID != "" {
		if s.turnService == nil {
			return nil, "", "", false, nil, errors.New("cannot verify expected prompt turn without turn service")
		}
		activeTurn, err := s.turnService.GetActiveTurn(ctx, sessionID)
		if err != nil {
			return nil, "", "", false, nil, fmt.Errorf("verify expected prompt turn: %w", err)
		}
		if activeTurn == nil || activeTurn.ID != expectedCurrentTurnID {
			return nil, "", "", false, nil, fmt.Errorf(
				"clarification turn %s is no longer current",
				expectedCurrentTurnID,
			)
		}
	}

	if claimEntryID != "" && !s.isCurrentQueuedDispatch(sessionID, claimEntryID) {
		return nil, "", "", false, nil, errQueuedDispatchSuperseded
	}
	freshSession, sessErr := s.repo.GetTaskSession(ctx, sessionID)
	if sessErr != nil {
		return nil, "", "", false, nil, fmt.Errorf("reload session before claiming queued dispatch: %w", sessErr)
	}
	if freshSession == nil {
		return nil, "", "", false, nil, errQueuedDispatchSuperseded
	}
	if requireNonterminalSession && isTerminalSessionState(freshSession.State) {
		return nil, "", "", false, nil, newWorkflowAutoStartSessionTerminalizedError(freshSession)
	}
	if promptErr := s.recheckPromptableWithForegroundClaim(
		taskID, sessionID, freshSession.State, foregroundClaim,
	); promptErr != nil {
		return nil, "", "", false, nil, promptErr
	}
	previousState := freshSession.State
	s.setSessionRunning(ctx, taskID, sessionID, freshSession)
	// setSessionRunning refreshes freshSession in place when its guarded write
	// loses, so a cancellation landing after the promptability check is visible
	// here as a terminal state.
	if requireNonterminalSession && isTerminalSessionState(freshSession.State) {
		return nil, "", "", false, nil, newWorkflowAutoStartSessionTerminalizedError(freshSession)
	}
	if isTerminalSessionState(freshSession.State) && freshSession.State != models.TaskSessionStateCompleted {
		return nil, "", "", false, nil, &executor.SessionStateSupersededError{
			SessionID: freshSession.ID,
			State:     freshSession.State,
		}
	}
	turnID, createdTurn, reservedTurn, err := s.startTurnForSessionWithOwnershipChecked(
		ctx, sessionID, reserveTurnUntilDispatch, promptDispatchRecovery,
	)
	if err != nil {
		s.rollbackPromptClaimAfterAdmissionFailure(ctx, taskID, sessionID, promptClaimRollback{
			previousSessionState: previousState,
		})
		return nil, "", "", false, nil, fmt.Errorf("persist prompt turn: %w", err)
	}
	if reservedTurn != nil && s.turnService != nil {
		if err := s.turnService.MarkReservedTurnDispatchAttempted(ctx, reservedTurn); err != nil {
			rollback := promptClaimRollback{
				previousSessionState: previousState,
				turnID:               turnID,
				createdTurn:          createdTurn,
				reservedTurn:         reservedTurn,
			}
			s.rollbackPromptClaimAfterAdmissionFailure(ctx, taskID, sessionID, rollback)
			return nil, "", "", false, nil, fmt.Errorf("persist prompt dispatch attempt: %w", err)
		}
	}
	reservation := s.queuedDispatchReservationForEntry(sessionID, claimEntryID)
	if reservation != nil && reservation.liveEligible.Load() {
		// A Send Now reservation becomes replaceable only after this guarded
		// session claim. The turn was started/adopted under the same guard above,
		// so the phase and turn ownership cannot be observed half-way through the
		// handoff.
		s.markAcceptedDispatchLiveLocked(sessionID, reservation)
	}
	// Remove only the pre-acceptance marker here. The accepted ownership record
	// remains until the turn settles, so Send Now cannot cancel or duplicate
	// this successor while the executor call is still in progress.
	if claimEntryID != "" {
		s.releaseQueuedDispatchPendingIfCurrent(
			sessionID,
			s.queuedDispatchReservationForEntry(sessionID, claimEntryID),
		)
	}
	return freshSession, previousState, turnID, createdTurn, reservedTurn, nil
}

func (s *Service) claimLifecycleSessionRunning(
	ctx context.Context,
	taskID, sessionID, claimEntryID string,
) (*models.TaskSession, models.TaskSessionState, string, bool, error) {
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	// Match ordinary prompt admission: a reset-owned cancellation must reject
	// lifecycle admission before its cancellation wait can observe ctx.Err().
	if s.isSessionResetInProgress(sessionID) {
		return nil, "", "", false, ErrSessionResetInProgress
	}
	if err := s.waitForCancellationWithGuard(ctx, sessionID, lock.Unlock, lock.Lock); err != nil {
		return nil, "", "", false, err
	}
	if s.isSessionResetInProgress(sessionID) {
		return nil, "", "", false, ErrSessionResetInProgress
	}

	if claimEntryID != "" && !s.isCurrentQueuedDispatch(sessionID, claimEntryID) {
		return nil, "", "", false, errQueuedDispatchSuperseded
	}
	if err := s.validateLifecyclePromptSelection(ctx, taskID, sessionID); err != nil {
		return nil, "", "", false, err
	}
	claim, err := s.repo.ClaimPromptableTaskSessionIfActive(ctx, sessionID)
	if err != nil {
		return nil, "", "", false, fmt.Errorf("%w: %v", errLifecyclePromptClaim, err)
	}
	switch claim.Status {
	case models.PromptableTaskSessionInactive:
		return nil, "", "", false, errLifecyclePromptInactive
	case models.PromptableTaskSessionBusy:
		return nil, "", "", false, fmt.Errorf("%w: lifecycle prompt session is busy", ErrAgentPromptInProgress)
	}
	if claimEntryID != "" && !s.isCurrentQueuedDispatch(sessionID, claimEntryID) {
		s.restoreLifecycleClaim(ctx, taskID, sessionID, claim.PreviousState)
		return nil, "", "", false, errQueuedDispatchSuperseded
	}
	if s.isSessionResetInProgress(sessionID) {
		s.restoreLifecycleClaim(ctx, taskID, sessionID, claim.PreviousState)
		return nil, "", "", false, ErrSessionResetInProgress
	}
	freshSession, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || freshSession == nil {
		s.restoreLifecycleClaim(ctx, taskID, sessionID, claim.PreviousState)
		return nil, "", "", false, errLifecyclePromptInactive
	}
	// Keep the lifecycle admission guard through durable turn creation. A reset
	// that follows this claim must observe this turn and quiesce it before it
	// replaces the provider context.
	turnID, createdTurn, _, err := s.startTurnForSessionWithOwnershipChecked(ctx, sessionID, false, nil)
	if err != nil {
		failureCtx, cancel := (promptTaskOptions{}).failureContext(ctx)
		defer cancel()
		s.restoreLifecycleClaim(failureCtx, taskID, sessionID, claim.PreviousState)
		return nil, "", "", false, fmt.Errorf("persist lifecycle prompt turn: %w", err)
	}
	s.releaseQueuedDispatchPendingIfCurrent(
		sessionID,
		s.queuedDispatchReservationForEntry(sessionID, claimEntryID),
	)
	return freshSession, claim.PreviousState, turnID, createdTurn, nil
}

func (s *Service) validateLifecyclePromptSelection(
	ctx context.Context,
	taskID, sessionID string,
) error {
	selectedSession, err := s.resolveTaskPRAgentSession(ctx, taskID)
	if err != nil || selectedSession == nil {
		task, taskErr := s.repo.GetTask(ctx, taskID)
		if taskErr != nil || task == nil || task.ArchivedAt != nil {
			return errLifecyclePromptInactive
		}
		return fmt.Errorf("%w: no promptable lifecycle session", ErrAgentPromptInProgress)
	}
	if selectedSession.ID != sessionID {
		return &lifecyclePromptReselectedError{sessionID: selectedSession.ID}
	}
	return nil
}

func (s *Service) validateLifecyclePromptTurn(
	ctx context.Context,
	sessionID, turnID string,
) error {
	if s.isSessionResetInProgress(sessionID) {
		return ErrSessionResetInProgress
	}
	if s.turnService == nil || turnID == "" {
		return nil
	}
	activeTurn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("verify lifecycle prompt turn: %w", err)
	}
	if activeTurn == nil || activeTurn.ID != turnID {
		return ErrSessionResetInProgress
	}
	return nil
}

func (s *Service) restoreLifecycleClaim(
	ctx context.Context,
	taskID, sessionID string,
	previousState models.TaskSessionState,
) {
	s.updateTaskSessionState(ctx, taskID, sessionID, previousState, "", false)
}

func (s *Service) recheckPromptableWithForegroundClaim(
	taskID, sessionID string,
	state models.TaskSessionState,
	claim *foregroundClaim,
) error {
	if claim == nil {
		return s.checkSessionPromptable(taskID, sessionID, state)
	}
	if state != models.TaskSessionStateRunning {
		// ensureSessionRunning may resume the agent after this prompt claimed a
		// background-idle RUNNING session. AgentBootReady advances the durable
		// state to WAITING_FOR_INPUT before dispatch; keep the valid claim across
		// that expected transition while still rejecting terminal or otherwise
		// non-promptable states.
		if err := s.checkSessionPromptable(taskID, sessionID, state); err != nil {
			return err
		}
	}
	if !s.isForegroundClaimCurrent(sessionID, claim) {
		return fmt.Errorf("%w, please wait for completion", ErrAgentPromptInProgress)
	}
	return nil
}

// checkSessionPromptable returns nil when the session's state accepts a new
// prompt. RUNNING is rejected with ErrAgentPromptInProgress; any other
// non-acceptable state (STARTING / CREATED / FAILED / CANCELLED) is rejected
// with ErrSessionNotPromptable so callers can distinguish "wait for the
// current turn" from "this session is not in a state where it can take a
// prompt". IDLE is acceptable: office sessions intentionally park in IDLE
// between turns (agent torn down, ACP session preserved) and the next prompt
// resumes them — see ensureSessionRunning.
func (s *Service) checkSessionPromptable(taskID, sessionID string, state models.TaskSessionState) error {
	switch state {
	case models.TaskSessionStateWaitingForInput,
		models.TaskSessionStateCompleted,
		models.TaskSessionStateIdle:
		return nil
	case models.TaskSessionStateRunning:
		// The safe default is coarse: every RUNNING session is busy. A
		// deployment may explicitly opt a persisted Claude Code session into
		// ADR-0049's adapter-attested background handoff experiment. Every
		// missing identity and non-Claude provider still fails closed.
		if s.ForegroundActivity(sessionID) == v1.ForegroundActivityBackground {
			s.logger.Debug("accepting prompt: enabled Claude foreground handoff is background-idle",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID))
			return nil
		}
		s.logger.Warn("rejected prompt while agent is already running",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("session_state", string(state)))
		return fmt.Errorf("%w, please wait for completion", ErrAgentPromptInProgress)
	default:
		s.logger.Warn("rejected prompt: session not ready for input",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("session_state", string(state)))
		return fmt.Errorf("%w: session is in %s state", ErrSessionNotPromptable, state)
	}
}

// handlePromptError reverts session state, logs the failure, transitions the
// task to REVIEW for non-transient errors, and completes the in-flight turn.
// Returns the (possibly remapped) error for the caller to surface.
func (s *Service) handlePromptError(ctx context.Context, taskID, sessionID string, previousSessionState models.TaskSessionState, err error) error {
	if isTransientPromptError(err) && s.isSessionResetInProgress(sessionID) {
		s.logger.Warn("prompt deferred while session reset is in progress; retry expected",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		err = ErrSessionResetInProgress
	} else {
		s.logger.Error("prompt failed",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
	// Revert session state so it doesn't stay stuck in RUNNING. Route through the
	// wrapper so WS subscribers are notified — the UI relies on the
	// session.state_changed broadcast to flip the composer/pause button out of
	// "Agent is running". The wrapper's terminal-state guard is also correct here:
	// if a concurrent agent-failure handler moved the session to FAILED we don't
	// want to overwrite that with previousSessionState. Do NOT pass the preloaded
	// `session` — the wrapper must re-read the row to see any such concurrent
	// terminal transition; the stale pre-RUNNING snapshot would defeat the guard.
	// allowWakeFromWaiting=false — this is a revert away from RUNNING, never a
	// wake transition; the flag only matters when going WAITING_FOR_INPUT → RUNNING.
	s.updateTaskSessionState(ctx, taskID, sessionID, previousSessionState, "", false)
	// ErrCancelEscalated means the user cancelled and the lifecycle manager had to
	// force-unblock a hung agent. Service.CancelAgent owns the cancel reconcile
	// (session → WAITING_FOR_INPUT, task → REVIEW, cancel message, complete
	// turn); skip the REVIEW write here so we don't race that path with a
	// duplicate update.
	// A short transient provider error is owned by the async
	// retry-with-backoff path (handleTransientFailure), which keeps the task
	// in progress while it retries — so don't flap it to REVIEW here.
	if !isTransientPromptError(err) && !errors.Is(err, lifecycle.ErrCancelEscalated) &&
		!routingerr.IsTransientProviderError(err.Error()) {
		s.writeTaskReviewState(ctx, taskID, sessionID)
	}
	s.completeTurnForSession(ctx, sessionID)
	return err
}

// trySwitchModel handles model switching for a prompt. Returns (result, true, nil) if a switch was
// performed, (nil, false, err) on error, or (nil, false, nil) if no switch was needed.
func modelSwitchRequired(session *models.TaskSession, model string) bool {
	if model == "" || session == nil {
		return false
	}
	var currentModel string
	if session.AgentProfileSnapshot != nil {
		if m, ok := session.AgentProfileSnapshot["model"].(string); ok {
			currentModel = m
		}
	}
	return currentModel != model
}

func (s *Service) trySwitchModel(ctx context.Context, taskID, sessionID, model, effectivePrompt string, session *models.TaskSession) (*PromptResult, bool, error) {
	if !modelSwitchRequired(session, model) {
		return nil, false, nil
	}
	var currentModel string
	if session.AgentProfileSnapshot != nil {
		if m, ok := session.AgentProfileSnapshot["model"].(string); ok {
			currentModel = m
		}
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
	s.runtimeModelBySession.Store(sessionID, model)
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

// RespondToPermission is the existing web/internal compatibility entry point.
// Option selections use the same strict audited service as external MCP;
// dismissal uses a separate generation-safe internal cancellation operation.
func (s *Service) RespondToPermission(ctx context.Context, taskID, sessionID, requestID, pendingID, optionID string, cancelled, rejected bool) error {
	_ = rejected // The immutable provider option kind determines rejection.
	request := ResolveAgentPermissionRequest{
		TaskID: taskID, SessionID: sessionID, RequestID: requestID,
		PendingID: pendingID, OptionID: optionID, Source: models.PermissionSourceWeb,
	}
	if cancelled {
		return s.cancelAgentPermission(ctx, request)
	}
	_, err := s.ResolveAgentPermission(ctx, request)
	return err
}

// DrainQueuedMessage dispatches one queued message for a session that is ready
// for input. It is intentionally one-at-a-time: each successful prompt will
// complete its own turn and then drain the next entry through handleAgentReady.
func (s *Service) DrainQueuedMessage(ctx context.Context, sessionID string) (bool, error) {
	if sessionID == "" {
		return false, fmt.Errorf("session_id is required")
	}
	// Acquire the guard *before* checking promptability, not after: check-
	// then-lock would leave a gap where a concurrent
	// QueueAndInterruptForPeerMessage (or another drain) changes the
	// session's state between this call's check and its take, letting a
	// manual drain request race a parent interrupt for the same entry —
	// see the Service.cancelInFlight field doc comment. A blocking Lock
	// (not a TryLock skip) is deliberate: unlike handleAgentReady /
	// handleAgentBootReady (where a losing side can rely on the winning
	// side's own take-and-dispatch, or a future turn's own agent.ready, to
	// retry), a manual drain request has no other future trigger —
	// skipping here could strand an already-queued message instead of
	// just reporting an accurate "not promptable right now".
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	guardLocked := true
	defer func() {
		if guardLocked {
			lock.Unlock()
		}
	}()
	for s.isCancelInFlight(sessionID) {
		lock.Unlock()
		guardLocked = false
		if err := s.waitForCancelInFlight(ctx, sessionID); err != nil {
			return false, err
		}
		lock.Lock()
		guardLocked = true
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("failed to get session: %w", err)
	}
	if err := s.checkSessionPromptable(session.TaskID, sessionID, session.State); err != nil {
		return false, err
	}
	if s.sessionHasPendingClarification(ctx, sessionID) {
		return false, ErrSessionNotPromptable
	}
	if s.messageQueue == nil {
		return false, errors.New("message queue is not configured")
	}
	if err := s.messageQueue.SetAutoRun(ctx, sessionID, true); err != nil {
		return false, fmt.Errorf("resume queue Auto-run: %w", err)
	}
	return s.drainQueuedMessageForPromptableSessionLocked(ctx, sessionID), nil
}

// SetQueueAutoRun persists the queue policy and, when enabling an eligible
// session, immediately attempts the FIFO head. It never cancels an active turn
// and treats busy, clarification, and lifecycle guards as a successful armed
// policy with no immediate dispatch.
func (s *Service) SetQueueAutoRun(ctx context.Context, sessionID string, enabled bool) (bool, bool, error) {
	if sessionID == "" {
		return false, false, errors.New("session_id is required")
	}
	if s.messageQueue == nil {
		return false, false, errors.New("message queue is not configured")
	}
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()

	if err := s.messageQueue.SetAutoRun(ctx, sessionID, enabled); err != nil {
		return false, false, fmt.Errorf("set queue Auto-run: %w", err)
	}
	if !enabled || s.isCancelInFlight(sessionID) || s.isQueuedDispatchInFlight(sessionID) || s.isSteerInFlight(sessionID) {
		return s.messageQueue.GetStatus(ctx, sessionID).AutoRun, false, nil
	}
	if s.sessionHasPendingClarification(ctx, sessionID) {
		return s.messageQueue.GetStatus(ctx, sessionID).AutoRun, false, nil
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return s.messageQueue.GetStatus(ctx, sessionID).AutoRun, false, fmt.Errorf("load session for queue Auto-run: %w", err)
	}
	if err := s.checkSessionPromptable(session.TaskID, sessionID, session.State); err != nil {
		if isSessionBusyError(err) {
			return s.messageQueue.GetStatus(ctx, sessionID).AutoRun, false, nil
		}
		return s.messageQueue.GetStatus(ctx, sessionID).AutoRun, false, err
	}
	dispatched := s.drainQueuedMessageForPromptableSessionLocked(ctx, sessionID)
	return s.messageQueue.GetStatus(ctx, sessionID).AutoRun, dispatched, nil
}

// cancelInFlightGuard is a per-session mutex serializing cancel/interrupt/
// queue-take decisions (see the Service.cancelInFlight field doc comment),
// paired with a reference count so the registry can safely reclaim the
// entry once every acquirer has released it — refs is guarded by
// Service.cancelInFlightMu, never by mu itself, since mu can be held for
// the caller's whole critical section while refs bookkeeping must stay
// brief and independent of that.
type cancelInFlightGuard struct {
	mu   sync.Mutex
	refs int
}

type taskSessionErrorGuard struct {
	mu   sync.Mutex
	refs int
}

func (s *Service) acquireTaskSessionErrorGuard(taskID string) (*sync.Mutex, func()) {
	s.taskSessionErrorLocksMu.Lock()
	if s.taskSessionErrorLocks == nil {
		s.taskSessionErrorLocks = make(map[string]*taskSessionErrorGuard)
	}
	guard, ok := s.taskSessionErrorLocks[taskID]
	if !ok {
		guard = &taskSessionErrorGuard{}
		s.taskSessionErrorLocks[taskID] = guard
	}
	guard.refs++
	s.taskSessionErrorLocksMu.Unlock()

	var released bool
	release := func() {
		if released {
			return
		}
		released = true
		s.taskSessionErrorLocksMu.Lock()
		guard.refs--
		if guard.refs == 0 {
			delete(s.taskSessionErrorLocks, taskID)
		}
		s.taskSessionErrorLocksMu.Unlock()
	}
	return &guard.mu, release
}

type cancellationKind string

const (
	cancellationKindExplicit     cancellationKind = "explicit"
	cancellationKindSilent       cancellationKind = "silent"
	cancellationKindPeer         cancellationKind = "peer"
	cancellationKindInternal     cancellationKind = "internal"
	cancellationKindQueueSendNow cancellationKind = "queue_send_now"
	cancellationOperationTTL                      = 30 * time.Second
)

type cancellationIdentity struct {
	executionID      string
	promptGeneration uint64
	activityEpoch    uint64
	turnID           string
}

type cancelOperation struct {
	done                       chan struct{}
	joined                     chan struct{}
	joinObserved               bool
	err                        error
	projectionRelease          func()
	kind                       cancellationKind
	identity                   cancellationIdentity
	identityReady              bool
	expectedTurnID             string
	expectedTurnReady          bool
	completionEligible         bool
	completionEligibilityReady bool
	actions                    []*cancellationAction
	nextAction                 int
	explicitReconcileOnce      sync.Once
	explicitReconcileErr       error
}

// cancellationAction is source-specific work that must run after the shared
// lifecycle cancellation/reconciliation, but before the coordinator releases
// the operation to other state-mutating paths. The action runs while holding
// the session guard, so a manual drain or ready event cannot slip between the
// shared cancel and the source's own queue/visible-message decision.
type cancellationAction struct {
	done       chan struct{}
	run        func(context.Context, *cancelOperation) (bool, error)
	dispatched bool
	err        error
}

// acquireCancelInFlightGuard returns the shared per-session mutex guarding
// cancel/interrupt/queue-take decisions for sessionID, creating one if it
// doesn't exist yet and registering the caller's reference to it. Callers
// MUST call the returned release func exactly once when done with the
// mutex — whether or not they actually acquired it (e.g. a TryLock that
// returned false still needs release to drop its reference). Pairing every
// acquire with a release is what keeps cancelInFlight bounded by
// concurrently-active sessions: once refs drops to zero the entry is
// deleted from the map instead of accumulating one permanent entry per
// session ever created.
func (s *Service) acquireCancelInFlightGuard(sessionID string) (*sync.Mutex, func()) {
	s.cancelInFlightMu.Lock()
	if s.cancelInFlight == nil {
		s.cancelInFlight = make(map[string]*cancelInFlightGuard)
	}
	guard, ok := s.cancelInFlight[sessionID]
	if !ok {
		guard = &cancelInFlightGuard{}
		s.cancelInFlight[sessionID] = guard
	}
	guard.refs++
	s.cancelInFlightMu.Unlock()

	var released bool
	release := func() {
		if released {
			return
		}
		released = true
		s.cancelInFlightMu.Lock()
		guard.refs--
		if guard.refs == 0 {
			delete(s.cancelInFlight, sessionID)
		}
		s.cancelInFlightMu.Unlock()
	}
	return &guard.mu, release
}

// lockedCancelInFlightGuard keeps the yield/reacquire protocol in one place
// for operations that must release the session guard around lifecycle I/O.
type lockedCancelInFlightGuard struct {
	mutex      *sync.Mutex
	releaseRef func()
	locked     bool
}

func (s *Service) lockCancelInFlightGuard(sessionID string) *lockedCancelInFlightGuard {
	mutex, release := s.acquireCancelInFlightGuard(sessionID)
	mutex.Lock()
	return &lockedCancelInFlightGuard{mutex: mutex, releaseRef: release, locked: true}
}

func (g *lockedCancelInFlightGuard) unlock() {
	if g == nil || !g.locked {
		return
	}
	g.mutex.Unlock()
	g.locked = false
}

func (g *lockedCancelInFlightGuard) relock() {
	if g == nil || g.locked {
		return
	}
	g.mutex.Lock()
	g.locked = true
}

func (g *lockedCancelInFlightGuard) release() {
	if g == nil || g.releaseRef == nil {
		return
	}
	g.unlock()
	g.releaseRef()
	g.releaseRef = nil
}

// claimCancellation establishes the single owner for every cancellation source
// targeting one session. The operation stays registered until its owner has
// finished lifecycle cancellation and source-specific reconciliation.
func (s *Service) claimCancellation(sessionID string, kind cancellationKind) (*cancelOperation, bool) {
	operation, owner, _ := s.claimCancellationWithAction(sessionID, kind, nil)
	return operation, owner
}

func (s *Service) claimCancellationWithAction(
	sessionID string,
	kind cancellationKind,
	action func(context.Context, *cancelOperation) (bool, error),
) (*cancelOperation, bool, *cancellationAction) {
	s.cancellationOperationsMu.Lock()
	defer s.cancellationOperationsMu.Unlock()
	if s.cancellationOperations == nil {
		s.cancellationOperations = make(map[string]*cancelOperation)
	}
	if operation, ok := s.cancellationOperations[sessionID]; ok {
		operation.markJoinedLocked()
		if action == nil {
			return operation, false, nil
		}
		registered := &cancellationAction{done: make(chan struct{}), run: action}
		operation.actions = append(operation.actions, registered)
		return operation, false, registered
	}
	operation := &cancelOperation{done: make(chan struct{}), joined: make(chan struct{}), kind: kind}
	var registered *cancellationAction
	if action != nil {
		registered = &cancellationAction{done: make(chan struct{}), run: action}
		operation.actions = append(operation.actions, registered)
	}
	s.cancellationOperations[sessionID] = operation
	return operation, true, registered
}

// claimCancellationWithActionExclusive establishes a cancellation owner only
// when no source is already active for the session. Send Now uses this stricter
// variant because joining an explicit cancel (or another Send Now click) would
// otherwise let the shared cancellation finish with the wrong source
// semantics. The accepted result is false when a caller must fail closed.
func (s *Service) claimCancellationWithActionExclusive(
	sessionID string,
	kind cancellationKind,
	action func(context.Context, *cancelOperation) (bool, error),
) (*cancelOperation, bool, *cancellationAction, bool) {
	s.cancellationOperationsMu.Lock()
	defer s.cancellationOperationsMu.Unlock()
	if s.cancellationOperations == nil {
		s.cancellationOperations = make(map[string]*cancelOperation)
	}
	if operation, ok := s.cancellationOperations[sessionID]; ok {
		return operation, false, nil, false
	}
	operation := &cancelOperation{done: make(chan struct{}), joined: make(chan struct{}), kind: kind}
	var registered *cancellationAction
	if action != nil {
		registered = &cancellationAction{done: make(chan struct{}), run: action}
		operation.actions = append(operation.actions, registered)
	}
	s.cancellationOperations[sessionID] = operation
	return operation, true, registered, true
}

func (s *Service) claimExplicitCancellation(
	sessionID string,
	action func(context.Context, *cancelOperation) (bool, error),
) (*cancelOperation, bool, *cancellationAction) {
	s.cancellationOperationsMu.Lock()
	defer s.cancellationOperationsMu.Unlock()
	if s.cancellationOperations == nil {
		s.cancellationOperations = make(map[string]*cancelOperation)
	}
	if operation, ok := s.cancellationOperations[sessionID]; ok {
		if operation.kind == cancellationKindQueueSendNow {
			return operation, false, nil
		}
		operation.markJoinedLocked()
		if operation.kind == cancellationKindExplicit || action == nil {
			return operation, false, nil
		}
		registered := &cancellationAction{done: make(chan struct{}), run: action}
		operation.actions = append(operation.actions, registered)
		return operation, false, registered
	}
	operation := &cancelOperation{
		done:   make(chan struct{}),
		joined: make(chan struct{}),
		kind:   cancellationKindExplicit,
	}
	s.cancellationOperations[sessionID] = operation
	return operation, true, nil
}

// markJoinedLocked records that another accepted cancellation source has
// attached to this operation. cancellationOperationsMu must be held.
func (operation *cancelOperation) markJoinedLocked() {
	if operation == nil || operation.joinObserved {
		return
	}
	operation.joinObserved = true
	close(operation.joined)
}

func (s *Service) currentCancellation(sessionID string) *cancelOperation {
	s.cancellationOperationsMu.Lock()
	defer s.cancellationOperationsMu.Unlock()
	return s.cancellationOperations[sessionID]
}

func (s *Service) setCancellationIdentity(sessionID string, operation *cancelOperation, identity cancellationIdentity) {
	s.cancellationOperationsMu.Lock()
	defer s.cancellationOperationsMu.Unlock()
	if current := s.cancellationOperations[sessionID]; current == operation {
		operation.identity = identity
		operation.identityReady = true
	}
}

func (s *Service) setCancellationExpectedTurn(sessionID string, operation *cancelOperation, turnID string) {
	s.cancellationOperationsMu.Lock()
	defer s.cancellationOperationsMu.Unlock()
	if current := s.cancellationOperations[sessionID]; current == operation {
		operation.expectedTurnID = turnID
		operation.expectedTurnReady = true
	}
}

func (s *Service) cancellationExpectedTurnSnapshot(operation *cancelOperation) (string, bool) {
	s.cancellationOperationsMu.Lock()
	defer s.cancellationOperationsMu.Unlock()
	if operation == nil {
		return "", false
	}
	return operation.expectedTurnID, operation.expectedTurnReady
}

func (s *Service) setCancellationCompletionEligible(sessionID string, operation *cancelOperation, eligible bool) {
	s.cancellationOperationsMu.Lock()
	defer s.cancellationOperationsMu.Unlock()
	if current := s.cancellationOperations[sessionID]; current == operation {
		operation.completionEligible = eligible
		operation.completionEligibilityReady = true
	}
}

func (s *Service) cancellationIdentitySnapshot(operation *cancelOperation) (cancellationIdentity, bool) {
	s.cancellationOperationsMu.Lock()
	defer s.cancellationOperationsMu.Unlock()
	if operation == nil {
		return cancellationIdentity{}, false
	}
	return operation.identity, operation.identityReady
}

func (s *Service) cancellationPreparationSnapshot(operation *cancelOperation) (cancellationIdentity, bool, bool) {
	s.cancellationOperationsMu.Lock()
	defer s.cancellationOperationsMu.Unlock()
	if operation == nil {
		return cancellationIdentity{}, false, false
	}
	return operation.identity, operation.completionEligible, operation.completionEligibilityReady
}

func (s *Service) finishCancellation(sessionID string, operation *cancelOperation, err error) {
	s.finishCancellationWithActions(context.Background(), sessionID, operation, err)
}

// finishCancellationWithActions runs all source-specific actions that joined
// the operation before removing it from the coordinator. The global
// coordinator mutex is reacquired between actions; a joiner that claims the
// operation before the final removal is therefore included, while a caller
// arriving after removal starts a fresh operation and re-evaluates state.
func (s *Service) finishCancellationWithActions(
	ctx context.Context,
	sessionID string,
	operation *cancelOperation,
	err error,
) {
	for {
		s.cancellationOperationsMu.Lock()
		if current := s.cancellationOperations[sessionID]; current != operation {
			s.cancellationOperationsMu.Unlock()
			return
		}
		if operation.nextAction >= len(operation.actions) {
			releaseProjection := operation.projectionRelease
			operation.projectionRelease = nil
			operation.err = err
			delete(s.cancellationOperations, sessionID)
			s.cancellationOperationsMu.Unlock()
			if releaseProjection != nil {
				releaseProjection()
			}
			close(operation.done)
			return
		}
		action := operation.actions[operation.nextAction]
		operation.nextAction++
		s.cancellationOperationsMu.Unlock()

		actionErr := err
		var dispatched bool
		if actionErr == nil && action.run != nil {
			lock, release := s.acquireCancelInFlightGuard(sessionID)
			lock.Lock()
			dispatched, actionErr = action.run(ctx, operation)
			lock.Unlock()
			release()
		}

		s.cancellationOperationsMu.Lock()
		action.dispatched = dispatched
		action.err = actionErr
		close(action.done)
		s.cancellationOperationsMu.Unlock()
	}
}

// beginCancelInFlight keeps the legacy marker seam used by coordinator-stop
// and focused tests, but its marker is now the same ownership record used by
// explicit, silent, and peer cancellation paths.
func (s *Service) beginCancelInFlight(sessionID string) func() {
	if sessionID == "" {
		return func() {}
	}
	endProjection := s.beginCancellationProjection(sessionID)
	operation, owner := s.claimCancellation(sessionID, cancellationKindInternal)
	if !owner {
		return endProjection
	}
	operation.projectionRelease = endProjection

	var once sync.Once
	return func() {
		once.Do(func() {
			s.finishCancellation(sessionID, operation, nil)
			endProjection()
		})
	}
}

// beginCancellationProjection records accepted cancellation work for the
// backend-owned progress projection. It is deliberately separate from the
// lifecycle ownership coordinator above: stream persistence can use the
// per-session guard without changing the user-visible cancellation state.
func (s *Service) beginCancellationProjection(sessionID string) func() {
	if sessionID == "" {
		return func() {}
	}
	s.cancelOperationsMu.Lock()
	if s.cancelOperations == nil {
		s.cancelOperations = make(map[string]*cancellationOperationState)
	}
	state := s.cancelOperations[sessionID]
	if state == nil {
		state = &cancellationOperationState{}
		s.cancelOperations[sessionID] = state
	}
	state.count++
	startDrain := false
	if state.count == 1 {
		state.revision++
		startDrain = s.enqueueCancellationPendingLocked(sessionID, state, true)
	}
	s.cancelOperationsMu.Unlock()
	if startDrain {
		s.drainCancellationPublicationQueue(sessionID, state)
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			startDrain := false
			s.cancelOperationsMu.Lock()
			state := s.cancelOperations[sessionID]
			if state != nil && state.count > 0 {
				state.count--
				if state.count == 0 {
					state.revision++
					startDrain = s.enqueueCancellationPendingLocked(sessionID, state, false)
				}
			}
			s.cancelOperationsMu.Unlock()
			if startDrain {
				s.drainCancellationPublicationQueue(sessionID, state)
			}
		})
	}
}

// CancellationPending reports whether at least one accepted cancellation
// operation is active for sessionID. The projection is intentionally runtime
// scoped: a backend restart clears it because no cancellation work survives
// the process restart.
func (s *Service) CancellationPending(sessionID string) bool {
	pending, _ := s.CancellationPendingSnapshot(sessionID)
	return pending
}

// CancellationPendingSnapshot returns the pending projection and its
// process-local transition revision from one critical section. Keeping these
// values together prevents REST/boot hydration from observing a boolean from
// one generation with the revision from another.
func (s *Service) CancellationPendingSnapshot(sessionID string) (bool, uint64) {
	if sessionID == "" {
		return false, 0
	}
	s.cancelOperationsMu.Lock()
	defer s.cancelOperationsMu.Unlock()
	state := s.cancelOperations[sessionID]
	if state == nil {
		return false, 0
	}
	return state.count > 0, state.revision
}

func (s *Service) publishCancellationPending(sessionID string, pending bool, revision uint64) {
	if s.eventBus == nil || sessionID == "" {
		return
	}
	payload := map[string]interface{}{
		"session_id":            sessionID,
		"cancellation_pending":  pending,
		"cancellation_revision": revision,
	}
	if err := s.eventBus.Publish(context.Background(), events.TaskSessionCancellationChanged,
		bus.NewEvent(events.TaskSessionCancellationChanged, "orchestrator", payload)); err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to publish cancellation state",
				zap.String("session_id", sessionID),
				zap.Bool("cancellation_pending", pending),
				zap.Uint64("cancellation_revision", revision),
				zap.Error(err))
		}
	}
}

type cancellationOperationState struct {
	count        int
	revision     uint64
	publications cancellationPublicationQueue
}

type cancellationPublication struct {
	pending  bool
	revision uint64
}

type cancellationPublicationQueue struct {
	pending    []cancellationPublication
	publishing bool
}

// enqueueCancellationPendingLocked appends a transition while the count and
// revision are still protected by cancelOperationsMu. The caller must drain
// the queue after releasing that mutex when this returns true.
func (s *Service) enqueueCancellationPendingLocked(
	sessionID string,
	state *cancellationOperationState,
	pending bool,
) bool {
	if s.eventBus == nil || sessionID == "" {
		return false
	}
	queue := &state.publications
	queue.pending = append(queue.pending, cancellationPublication{pending: pending, revision: state.revision})
	if queue.publishing {
		return false
	}
	queue.publishing = true
	return true
}

func (s *Service) drainCancellationPublicationQueue(
	sessionID string,
	state *cancellationOperationState,
) {
	for {
		s.cancelOperationsMu.Lock()
		queue := &state.publications
		if len(queue.pending) == 0 {
			queue.publishing = false
			s.cancelOperationsMu.Unlock()
			return
		}
		publication := queue.pending[0]
		queue.pending = queue.pending[1:]
		s.cancelOperationsMu.Unlock()

		s.publishCancellationPending(sessionID, publication.pending, publication.revision)
	}
}

// isCancelInFlight reports whether an actual cancellation operation for
// sessionID is active or waiting on the shared guard. It deliberately does
// not inspect the guard mutex itself because stream/lifecycle handlers also
// use that mutex for non-cancelling side effects.
func (s *Service) isCancelInFlight(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	return s.currentCancellation(sessionID) != nil
}

// waitForCancelInFlight blocks until all cancellation intents for sessionID
// have finished. Callers that hold the per-session guard must release it
// before waiting; the cancellation owner may need that mutex for terminal
// stream persistence and final reconciliation.
func (s *Service) waitForCancelInFlight(ctx context.Context, sessionID string) error {
	operation := s.currentCancellation(sessionID)
	if operation == nil {
		return nil
	}
	return operation.wait(ctx)
}

// waitForCancellationWithGuard releases a caller-held per-session mutex while
// a cancellation owner completes, then reacquires it before returning. The
// loop closes the small handoff window where a fresh cancellation can claim
// the session immediately after the first operation finishes.
func (s *Service) waitForCancellationWithGuard(
	ctx context.Context,
	sessionID string,
	unlock func(),
	relock func(),
) error {
	for {
		operation := s.currentCancellation(sessionID)
		if operation == nil {
			return nil
		}
		unlock()
		err := operation.wait(ctx)
		relock()
		if err != nil {
			return err
		}
	}
}

func (s *Service) cancellationOwnsStreamEvent(sessionID, executionID string, promptGeneration uint64) bool {
	operation := s.currentCancellation(sessionID)
	if operation == nil {
		return true
	}
	identity, ready := s.cancellationIdentitySnapshot(operation)
	if !ready {
		// The owner has claimed the operation but has not yet reached the
		// guarded identity snapshot. The stream event is ordered before the
		// yielded lifecycle wait and may proceed to let the owner capture it.
		return true
	}
	if identity.executionID != "" && executionID != "" && identity.executionID != executionID {
		return false
	}
	if identity.promptGeneration != 0 && promptGeneration != 0 && identity.promptGeneration != promptGeneration {
		return false
	}
	return true
}

func (operation *cancelOperation) wait(ctx context.Context) error {
	select {
	case <-operation.done:
		return operation.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (action *cancellationAction) wait(ctx context.Context) (bool, error) {
	if action == nil {
		return false, nil
	}
	select {
	case <-action.done:
		return action.dispatched, action.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// reconcileCancelledTurn durably settles the cancelled session and its turn.
// A user cancellation may trigger workflow completion only after both pieces
// of bookkeeping have been confirmed from their authoritative stores. The
// best-effort lifecycle helpers intentionally remain unchanged for other event
// paths; this path fails closed so a persistence or turn-service failure cannot
// advance the task while the cancelled turn is still running.
func (s *Service) reconcileCancelledTurn(
	ctx context.Context,
	taskID string,
	sessionID string,
	session *models.TaskSession,
	requireWaiting bool,
) (*models.TaskSession, error) {
	return s.reconcileCancelledTurnOwned(ctx, taskID, sessionID, session, requireWaiting, "")
}

// reconcileCancelledTurnOwned settles cancellation bookkeeping for the turn
// captured by the caller. An expected turn ID makes every authoritative check
// fail closed when a successor turn has appeared, rather than allowing the
// generic cleanup sweep to close unrelated work.
func (s *Service) reconcileCancelledTurnOwned(
	ctx context.Context,
	taskID string,
	sessionID string,
	session *models.TaskSession,
	requireWaiting bool,
	expectedTurnID string,
) (*models.TaskSession, error) {
	if err := s.verifyCapturedCancelledTurn(ctx, sessionID, expectedTurnID); err != nil {
		return nil, err
	}
	authoritativeSession, err := s.reconcileCancelledSessionState(ctx, taskID, session, requireWaiting)
	if err != nil {
		return nil, err
	}
	if err := s.verifyCapturedCancelledTurn(ctx, sessionID, expectedTurnID); err != nil {
		return nil, err
	}
	if err := s.completeTurnForTaskSessionCheckedOwned(ctx, taskID, sessionID, expectedTurnID); err != nil {
		return nil, fmt.Errorf("settle cancelled turn: %w", err)
	}

	if session == nil {
		return nil, nil
	}
	if err := validateCancelledSessionState(session.ID, authoritativeSession, requireWaiting); err != nil {
		return nil, err
	}
	if err := s.verifyCancelledTurnClosedOwned(ctx, sessionID, expectedTurnID); err != nil {
		return nil, err
	}

	return authoritativeSession, nil
}

func (s *Service) verifyCapturedCancelledTurn(ctx context.Context, sessionID, expectedTurnID string) error {
	if s.turnService == nil || expectedTurnID == "" {
		return nil
	}
	activeTurn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		if isNoActiveTurnError(err) {
			return nil
		}
		return fmt.Errorf("inspect captured cancelled turn: %w", err)
	}
	if activeTurn != nil && activeTurn.ID != expectedTurnID {
		return fmt.Errorf("captured cancelled turn %s was superseded by active turn %s", expectedTurnID, activeTurn.ID)
	}
	return nil
}

func (s *Service) reconcileCancelledSessionState(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	requireWaiting bool,
) (*models.TaskSession, error) {
	if session == nil || !requireWaiting {
		return session, nil
	}
	updated := s.updateTaskSessionState(
		ctx,
		taskID,
		session.ID,
		models.TaskSessionStateWaitingForInput,
		"",
		true,
		session,
	)
	if updated == nil {
		return nil, fmt.Errorf("persist cancelled session %s as WAITING_FOR_INPUT", session.ID)
	}
	if updated.State != models.TaskSessionStateWaitingForInput {
		return nil, fmt.Errorf(
			"cancelled session %s persisted as %s, want WAITING_FOR_INPUT",
			session.ID,
			updated.State,
		)
	}
	authoritative, err := s.repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("verify cancelled session %s state: %w", session.ID, err)
	}
	if authoritative == nil {
		return nil, fmt.Errorf(
			"cancelled session %s is missing before turn settlement, want WAITING_FOR_INPUT",
			session.ID,
		)
	}
	if authoritative.State != models.TaskSessionStateWaitingForInput {
		return nil, fmt.Errorf(
			"cancelled session %s is %s before turn settlement, want WAITING_FOR_INPUT",
			session.ID,
			authoritative.State,
		)
	}
	return authoritative, nil
}

func validateCancelledSessionState(
	sessionID string,
	authoritativeSession *models.TaskSession,
	requireWaiting bool,
) error {
	if authoritativeSession == nil {
		return fmt.Errorf("cancelled session %s has no authoritative state", sessionID)
	}
	if requireWaiting && authoritativeSession.State != models.TaskSessionStateWaitingForInput {
		return fmt.Errorf(
			"cancelled session %s settled as %s, want WAITING_FOR_INPUT",
			sessionID,
			authoritativeSession.State,
		)
	}
	return nil
}

func (s *Service) verifyCancelledTurnClosed(ctx context.Context, sessionID string) error {
	return s.verifyCancelledTurnClosedOwned(ctx, sessionID, "")
}

func (s *Service) verifyCancelledTurnClosedOwned(ctx context.Context, sessionID, expectedTurnID string) error {
	if s.turnService == nil {
		return nil
	}
	activeTurn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		if isNoActiveTurnError(err) {
			return nil
		}
		return fmt.Errorf("verify cancelled turn closure: %w", err)
	}
	if activeTurn != nil {
		if expectedTurnID != "" && activeTurn.ID != expectedTurnID {
			return fmt.Errorf("captured cancelled turn %s was superseded by active turn %s", expectedTurnID, activeTurn.ID)
		}
		return fmt.Errorf("cancelled turn %s remains open", activeTurn.ID)
	}
	return nil
}

// CancelAgent interrupts the current agent turn without terminating the process,
// allowing the user to send a new prompt.
//
// Idempotent w.r.t. missing executions: if the agent manager reports no live execution
// for the session (ErrNoExecutionForSession), the method still reconciles the session's
// DB state (transitions to WAITING_FOR_INPUT, records the cancel message, completes the
// turn) so the user can unstick a session whose agent subprocess crashed. Other errors
// still fail the cancel.
type cancelAgentPreparation struct {
	session            *models.TaskSession
	completionEligible bool
	capturedTurnID     string
	cancelTurnID       string
	identity           cancellationIdentity
}

func (s *Service) CancelAgent(ctx context.Context, sessionID string) (err error) {
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		return err
	}
	if s.repo == nil {
		return errors.New("cancel agent: repository is not configured")
	}
	var operation *cancelOperation
	var owner bool
	var action *cancellationAction
	operation, owner, action = s.claimExplicitCancellation(sessionID, func(actionCtx context.Context, operation *cancelOperation) (bool, error) {
		return false, s.reconcileJoinedExplicitCancellationLocked(actionCtx, sessionID, operation)
	})
	if !owner {
		if err := operation.wait(ctx); err != nil {
			return err
		}
		if operation.kind == cancellationKindExplicit {
			return nil
		}
		if operation.kind == cancellationKindQueueSendNow {
			return ErrSendNowConflict
		}
		if action == nil {
			return s.reconcileJoinedExplicitCancellation(ctx, sessionID, operation)
		}
		_, err := action.wait(ctx)
		return err
	}
	go s.runExplicitCancellation(ctx, sessionID, operation)
	return operation.wait(ctx)
}

func (s *Service) runExplicitCancellation(requestCtx context.Context, sessionID string, operation *cancelOperation) {
	endProjection := s.beginCancellationProjection(sessionID)
	operation.projectionRelease = endProjection
	defer endProjection()
	operationCtx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), cancellationOperationTTL)
	defer cancel()
	err := s.runExplicitCancellationOwned(operationCtx, sessionID, operation)
	s.finishCancellationWithActions(operationCtx, sessionID, operation, err)
}

func (s *Service) runExplicitCancellationOwned(ctx context.Context, sessionID string, operation *cancelOperation) (err error) {
	s.logger.Debug("cancelling agent turn", zap.String("session_id", sessionID))

	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	guardLocked := true
	unlockGuard := func() {
		if guardLocked {
			lock.Unlock()
			guardLocked = false
		}
	}
	relockGuard := func() {
		if !guardLocked {
			lock.Lock()
			guardLocked = true
		}
	}
	defer unlockGuard()

	prepared, err := s.prepareCancelAgent(ctx, sessionID)
	if err != nil {
		return err
	}
	s.setCancellationIdentity(sessionID, operation, prepared.identity)
	s.setCancellationCompletionEligible(sessionID, operation, prepared.completionEligible)
	if err := s.cancelAgentWhileUnlocked(ctx, sessionID, unlockGuard, relockGuard); err != nil {
		return err
	}
	if err := s.finishCancelledAgentTurn(ctx, sessionID, prepared); err != nil {
		return err
	}

	s.logger.Debug("agent turn cancelled", zap.String("session_id", sessionID))
	return nil
}

// reconcileJoinedExplicitCancellation applies the user-facing part of an
// explicit cancel after joining a silent/internal operation. The joined caller
// never invokes lifecycle cancellation; it only re-evaluates the current
// session and performs the explicit source's reconciliation once.
func (s *Service) reconcileJoinedExplicitCancellation(
	requestCtx context.Context,
	sessionID string,
	operation *cancelOperation,
) error {
	operationCtx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), cancellationOperationTTL)
	defer cancel()
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	return s.reconcileJoinedExplicitCancellationLocked(operationCtx, sessionID, operation)
}

// reconcileJoinedExplicitCancellationLocked is the source-specific action for
// a user cancel that joined a silent/internal cancellation. The caller must
// already hold the session guard; the cancellation coordinator uses this form
// so the explicit reconciliation is completed before the shared operation is
// released to other state-mutating paths.
func (s *Service) reconcileJoinedExplicitCancellationLocked(
	operationCtx context.Context,
	sessionID string,
	operation *cancelOperation,
) error {
	if _, _, ready := s.cancellationPreparationSnapshot(operation); !ready {
		return errors.New("joined cancellation preparation is incomplete")
	}
	operation.explicitReconcileOnce.Do(func() {
		identity, completionEligible, ready := s.cancellationPreparationSnapshot(operation)
		if !ready {
			operation.explicitReconcileErr = errors.New("joined cancellation preparation became incomplete")
			return
		}
		session, err := s.repo.GetTaskSession(operationCtx, sessionID)
		if err != nil {
			operation.explicitReconcileErr = fmt.Errorf("load session after joined cancel: %w", err)
			return
		}
		if session == nil || isTerminalSessionState(session.State) {
			return
		}
		prepared := cancelAgentPreparation{
			session:            session,
			completionEligible: completionEligible,
			capturedTurnID:     identity.turnID,
			cancelTurnID:       identity.turnID,
			identity:           identity,
		}
		operation.explicitReconcileErr = s.finishCancelledAgentTurn(operationCtx, sessionID, prepared)
	})
	return operation.explicitReconcileErr
}

func (s *Service) prepareCancelAgent(ctx context.Context, sessionID string) (cancelAgentPreparation, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to get session for cancel",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
	completionEligible, err := s.cancelTurnCompletionEligible(ctx, session, sessionID)
	if err != nil {
		return cancelAgentPreparation{}, err
	}

	identity, err := s.captureCancellationIdentity(ctx, sessionID)
	if err != nil {
		return cancelAgentPreparation{}, err
	}
	capturedTurnID := identity.turnID
	cancelTurnID := capturedTurnID
	if cancelTurnID == "" {
		cancelTurnID = s.getActiveTurnID(sessionID)
	}
	return cancelAgentPreparation{
		session:            session,
		completionEligible: completionEligible,
		capturedTurnID:     capturedTurnID,
		cancelTurnID:       cancelTurnID,
		identity:           identity,
	}, nil
}

func (s *Service) captureCancellationIdentity(ctx context.Context, sessionID string) (cancellationIdentity, error) {
	executionID, promptGeneration := s.captureCancellationAgentIdentity(ctx, sessionID)
	identity := cancellationIdentity{executionID: executionID, promptGeneration: promptGeneration}
	if s.turnService != nil {
		turnID, err := s.peekActiveTurnID(ctx, sessionID)
		if err != nil {
			return cancellationIdentity{}, fmt.Errorf("inspect active turn before cancel: %w", err)
		}
		identity.turnID = turnID
	}
	return identity, nil
}

func (s *Service) captureCancellationAgentIdentity(ctx context.Context, sessionID string) (string, uint64) {
	if s.agentManager == nil {
		return "", 0
	}
	executionID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if err != nil && !errors.Is(err, lifecycle.ErrNoExecutionForSession) {
		s.logger.Debug("could not capture execution identity before cancellation",
			zap.String("session_id", sessionID), zap.Error(err))
	}

	generationReader, ok := s.agentManager.(interface {
		GetPromptGenerationForSession(context.Context, string) (uint64, error)
	})
	if !ok {
		return executionID, 0
	}
	generation, generationErr := generationReader.GetPromptGenerationForSession(ctx, sessionID)
	if generationErr != nil && !errors.Is(generationErr, lifecycle.ErrNoExecutionForSession) {
		s.logger.Debug("could not capture prompt generation before cancellation",
			zap.String("session_id", sessionID), zap.Error(generationErr))
	}
	return executionID, generation
}

func (s *Service) cancelTurnCompletionEligible(ctx context.Context, session *models.TaskSession, sessionID string) (bool, error) {
	if session == nil {
		return false, nil
	}
	switch session.State {
	case models.TaskSessionStateRunning, models.TaskSessionStateStarting:
		return true, nil
	case models.TaskSessionStateWaitingForInput:
		activeTurnID, err := s.peekActiveTurnID(ctx, sessionID)
		if err != nil {
			return false, fmt.Errorf("inspect active turn before cancel retry: %w", err)
		}
		return activeTurnID != "", nil
	default:
		return false, nil
	}
}

func (s *Service) cancelAgentWhileUnlocked(ctx context.Context, sessionID string, unlockGuard, relockGuard func()) error {
	if s.agentManager == nil {
		return nil
	}
	// The lifecycle manager waits for the in-flight prompt to consume terminal
	// stream frames delivered through the same per-session guard. Keep the
	// operation marker and guard registry reference active, but release only the
	// mutex around that blocking wait.
	unlockGuard()
	cancelErr := s.agentManager.CancelAgent(ctx, sessionID)
	relockGuard()
	if cancelErr == nil {
		return nil
	}
	switch {
	case errors.Is(cancelErr, lifecycle.ErrNoExecutionForSession):
		// A crashed or unregistered process still needs DB reconciliation so the
		// session does not remain stuck.
		s.logger.Error("agent process appears to have crashed: no live execution for session on cancel",
			zap.String("session_id", sessionID),
			zap.Error(cancelErr))
	case errors.Is(cancelErr, lifecycle.ErrCancelEscalated):
		// The lifecycle manager already unblocked the prompt and marked the
		// execution ready; reconcile the session state below.
		s.logger.Warn("agent did not acknowledge cancel; reconciling session state",
			zap.String("session_id", sessionID),
			zap.Error(cancelErr))
	default:
		return fmt.Errorf("cancel agent: %w", cancelErr)
	}
	return nil
}

func (s *Service) finishCancelledAgentTurn(ctx context.Context, sessionID string, prepared cancelAgentPreparation) error {
	session := prepared.session
	if session != nil {
		reconciled, err := s.reconcileCancelledTurnOwned(
			ctx,
			session.TaskID,
			sessionID,
			session,
			prepared.completionEligible,
			prepared.capturedTurnID,
		)
		if err != nil {
			return fmt.Errorf("reconcile cancelled turn: %w", err)
		}
		if reconciled != nil {
			session = reconciled
		}
	} else if _, err := s.reconcileCancelledTurnOwned(ctx, "", sessionID, nil, false, prepared.capturedTurnID); err != nil {
		return fmt.Errorf("reconcile cancelled turn: %w", err)
	}
	if s.messageQueue != nil {
		paused, err := s.messageQueue.PauseAutoRunIfPending(ctx, sessionID)
		if err != nil {
			s.logger.Warn("failed to pause queued messages after cancel",
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else if paused {
			s.publishQueueStatusEvent(ctx, sessionID)
		}
	}

	s.recordCancelledAgentMessage(ctx, session, sessionID, prepared.cancelTurnID)
	s.reconcileCancelledAgentWorkflow(ctx, session, prepared.completionEligible)
	return nil
}

func (s *Service) recordCancelledAgentMessage(ctx context.Context, session *models.TaskSession, sessionID, cancelTurnID string) {
	if s.messageCreator == nil || session == nil {
		return
	}
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
		cancelTurnID,
		metadata,
		false,
	); err != nil {
		s.logger.Warn("failed to create cancel message",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

func (s *Service) reconcileCancelledAgentWorkflow(ctx context.Context, session *models.TaskSession, completionEligible bool) {
	if session == nil {
		return
	}
	transitioned := false
	if completionEligible {
		transitioned = s.processOnTurnCompleteViaEngineWithCause(
			ctx, session.TaskID, session, turnCompletionCauseUserCancellation,
		)
	}
	s.reconcileCancelledTaskReview(ctx, session.TaskID, session.ID, transitioned)
}

func (s *Service) reconcileCancelledTaskReview(ctx context.Context, taskID, sessionID string, transitioned bool) {
	if transitioned {
		task, err := s.repo.GetTask(ctx, taskID)
		if err == nil && task != nil && task.WorkflowStepID != "" && s.workflowStepGetter != nil {
			step, stepErr := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
			if stepErr == nil && step != nil && s.workflowStepIsTerminal(ctx, step.ID) {
				return
			}
		}
	}
	s.writeTaskReviewState(ctx, taskID, sessionID)
}

// takeAndDispatchEntryLocked takes entryID from sessionID's queue and
// dispatches it, falling back to draining the FIFO head only if the
// targeted take doesn't find it (a benign not-found, e.g. already taken by
// a concurrent path). A genuine repository error on the targeted take is
// propagated instead of falling back — see cancelAndTakeForPeerMessage's
// doc comment for why. The caller must already hold sessionID's
// cancelInFlight lock; this neither acquires nor releases it.
//
// Deliberately does NOT check isQueuedDispatchInFlight itself: callers
// that reach here have either just performed a cancel
// (cancelAndTakeForPeerMessage) — which supersedes whatever another
// dispatch was in the middle of settling, so a stale in-flight marker from
// that dispatch must not block taking a *different* entry here — or have
// already confirmed the session is genuinely idle (takeIfPromptableLocked,
// which checks the marker itself before delegating here). See
// drainQueuedMessageForPromptableSessionLocked for the *non-cancelling*
// FIFO-head drain path, which does check the marker directly.
func (s *Service) takeAndDispatchEntryLocked(ctx context.Context, sessionID, entryID string) (bool, error) {
	if s.messageQueue != nil {
		queuedMsg, ok, err := s.messageQueue.TakeQueuedEntry(ctx, sessionID, entryID)
		if err != nil {
			return false, fmt.Errorf("take targeted queued message: %w", err)
		}
		if s.dispatchTakenQueuedMessage(ctx, sessionID, queuedMsg, ok) {
			return true, nil
		}
	}
	return s.drainQueuedMessageForPromptableSessionLocked(ctx, sessionID), nil
}

// takeIfPromptableLocked takes and dispatches entryID only if sessionID is
// currently promptable, without ever issuing a cancel — used by
// QueueAndInterruptForPeerMessage when cancelling would risk hitting a
// *different*, unrelated turn instead of the one the caller meant to
// interrupt (see its active-turn revalidation doc comment). Returns
// (false, nil), never an error, when the session turns out not to be
// promptable — or when a *different* dispatch is already settling for
// this session (isQueuedDispatchInFlight; see the Service.dispatchingQueued
// field doc comment) — mirroring cancelAndTakeForPeerMessage's own "cancel
// failed but already promptable" recovery except without ever attempting
// the cancel. Unlike cancelAndTakeForPeerMessage's path, this never
// cancels anything, so it has no way to supersede a settling dispatch the
// way a genuine cancel would — it must defer to it instead. The caller
// must already hold sessionID's cancelInFlight lock.
//
// Also defers to an admitted-but-not-yet-dispatched steer (isSteerInFlight;
// see steerInFlight's field doc comment) for the same reason: this path
// never cancels, so it has no way to supersede the steer's dispatch either.
func (s *Service) takeIfPromptableLocked(ctx context.Context, taskID, sessionID, entryID string) (bool, error) {
	if s.isQueuedDispatchInFlight(sessionID) || s.isSteerInFlight(sessionID) {
		return false, nil
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || s.checkSessionPromptable(taskID, sessionID, session.State) != nil {
		return false, nil
	}
	return s.takeAndDispatchEntryLocked(ctx, sessionID, entryID)
}

// cancelAndTakeForPeerMessage cancels sessionID's in-flight turn and takes
// and dispatches entryID — the specific message that triggered this
// interrupt — falling back to draining the FIFO head only if the targeted
// take doesn't find it (a benign not-found, e.g. already taken by a
// concurrent path). entryID must be non-empty: the only caller,
// QueueAndInterruptForPeerMessage, always passes the entry it just
// inserted. A genuine repository error on the targeted take is propagated
// instead of falling back: an error says nothing about which entry is
// actually at the FIFO head, and dispatching it anyway would risk
// delivering the wrong message while the caller still reports "sent" for
// the parent's.
//
// The caller must already hold sessionID's cancelInFlight lock; this
// neither acquires nor releases it — see QueueAndInterruptForPeerMessage.
//
// The returned bool reports whether this call actually dispatched a
// message; a false result means the caller's message is still only
// queued, to be delivered later by whichever drain gets to it.
//
// Deliberately mirrors cancelAgentSilent + dispatchTakenQueuedMessage rather
// than delegating to CancelAgent: the interrupt is an internal steering
// signal from the parent, not a user action, so unlike the cancel button it
// must not write a visible "Turn cancelled" message and must not move the
// task to REVIEW (writeTaskReviewStateOnCancel).
func (s *Service) cancelAndTakeForPeerMessage(
	ctx context.Context,
	taskID string,
	sessionID string,
	entryID string,
	unlockGuard func(),
	relockGuard func(),
) (bool, error) {
	return s.cancelAgentSilentWithGuardActionKind(
		ctx,
		taskID,
		sessionID,
		unlockGuard,
		relockGuard,
		func(actionCtx context.Context) (bool, error) {
			return s.takeAndDispatchEntryLocked(actionCtx, sessionID, entryID)
		},
		cancellationKindPeer,
	)
}

// QueueAndInterruptForPeerMessage atomically queues prompt for sessionID
// and then interrupts the child's in-flight turn to deliver it immediately,
// instead of waiting for the turn to end naturally. It is used only by
// handleMessageTask (mcp/handlers) when the sender is the target task's
// parent: a long-running child otherwise leaves its parent's control/steer
// messages parked on the FIFO queue until the child's current turn finishes
// on its own, and with several such children running in parallel the
// per-session queue can fill up to messagequeue.DefaultMaxPerSession before
// any of them are delivered.
//
// This must be one atomic operation rather than "queue, then separately
// interrupt" (the original two-step split across the handlers/orchestrator
// boundary): between an insert becoming visible and a later, separate
// interrupt call acquiring the session's cancelInFlight lock, the child's
// turn can complete naturally and handleAgentReady's normal FIFO drain can
// grab the just-queued entry and start dispatching it as an ordinary turn —
// only for the later interrupt's cancel to land on and kill that very turn,
// orphaning the parent's message mid-delivery. Taking the lock before the
// queue insert closes that: handleAgentReady and handleAgentBootReady's
// take decisions claim the exact same lock before their own take, so neither
// can ever observe — and steal — an
// entry this call is still in the process of queueing-and-claiming.
//
// The lock is a genuine, blocking claim (Lock, mirroring
// executor.Executor.getSessionLock's precedent for per-session mutual
// exclusion in this codebase) — not a try-once peek with a fallback
// unguarded insert. Working around a busy lock with an unguarded insert
// would leave that insert visible to nobody in particular: the current
// holder may have already finished its own take-or-skip decision before
// the insert lands, and nothing then guarantees a future drain (the
// session can go idle with no further agent.ready to retry on). Every
// existing holder already bounds its own hold time through an independent
// mechanism (cancelAgentSilent's underlying agent-cancel escalation
// timeout, or a single fast DB round-trip in the natural-drain paths), so
// blocking here adds no new deadlock risk.
//
// Active-turn revalidation: after acquiring the lock and queueing the
// message, this re-checks whether the turn active for sessionID is still
// the one that was active when the caller decided to interrupt (snapshotted
// via peekActiveTurnID *before* contending for the lock). If a *different*
// turn is now active — e.g. a workflow on_turn_complete transition
// auto-started a successor for this same session while this call waited
// for the guard — cancelling now would kill that unrelated successor turn
// instead of the one the parent meant to interrupt. In that case this
// falls back to takeIfPromptableLocked (dispatch directly only if the
// session happens to already be idle *and* no other dispatch is
// concurrently settling for it, per isQueuedDispatchInFlight — see the
// Service.dispatchingQueued field doc comment; otherwise leave the
// message queued for whichever turn is actually running to drain
// naturally) instead of ever calling cancelAndTakeForPeerMessage. The
// turn-identity comparison only trusts a positively-confirmed match — both
// snapshots read without error, both non-empty, and equal — before it
// will risk a cancel; any error or mismatch (including "no turn observed
// at all") takes the safe fallback. The one exception is turnService
// being unset entirely: with no turn identity obtainable at all, this
// preserves the exact unconditional-interrupt behavior every existing
// (turn-unaware) caller and test exercises.
//
// Note this deliberately does *not* also gate on isQueuedDispatchInFlight
// when the turn *is* confirmed unchanged: a cancel (via
// cancelAndTakeForPeerMessage below) supersedes whatever another dispatch
// for this same session was in the middle of settling — e.g. a second
// parent message arriving right behind a first one whose own dispatch
// hasn't yet reached PromptTask — so it must still be allowed to cancel
// and take its own entry rather than being blocked by a marker the cancel
// itself is about to invalidate.
func (s *Service) QueueAndInterruptForPeerMessage(ctx context.Context, taskID, sessionID, prompt string, metadata map[string]interface{}) (*messagequeue.QueuedMessage, bool, error) {
	if s.messageQueue == nil {
		return nil, false, errors.New("message queue not available")
	}

	turnBeforeWait, turnPeekErr := s.peekActiveTurnID(ctx, sessionID)
	if turnPeekErr != nil {
		s.logger.Warn("failed to snapshot active turn before peer-message interrupt; will fall back to a promptable-only dispatch instead of risking a cancel of unrelated work",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(turnPeekErr))
	}

	lock, release := s.acquireCancelInFlightGuard(sessionID)
	lock.Lock()
	guardLocked := true
	unlockGuard := func() {
		if guardLocked {
			lock.Unlock()
			guardLocked = false
		}
	}
	relockGuard := func() {
		if !guardLocked {
			lock.Lock()
			guardLocked = true
		}
	}
	defer func() {
		unlockGuard()
		release()
	}()

	queued, err := s.messageQueue.QueueMessageWithMetadata(ctx, sessionID, taskID, prompt, "", messagequeue.QueuedByAgent, false, nil, metadata)
	if err != nil {
		return nil, false, err
	}
	s.publishQueueStatusEvent(ctx, sessionID)
	if operation := s.currentCancellation(sessionID); operation != nil && operation.kind != cancellationKindPeer {
		s.logger.Debug("leaving peer message queued while a different cancellation source is in progress",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("queue_id", queued.ID))
		unlockGuard()
		if waitErr := s.waitForCancelInFlight(ctx, sessionID); waitErr != nil {
			return queued, false, waitErr
		}
		return queued, false, nil
	}

	if s.turnService != nil {
		turnNow, turnNowErr := s.peekActiveTurnID(ctx, sessionID)
		confirmedUnchanged := turnPeekErr == nil && turnNowErr == nil && turnBeforeWait != "" && turnNow == turnBeforeWait
		if !confirmedUnchanged {
			s.logger.Warn("active turn could not be confirmed unchanged while waiting to interrupt; dispatching only if already idle instead of risking a cancel of unrelated work",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("turn_before_wait", turnBeforeWait),
				zap.Bool("turn_peek_error", turnPeekErr != nil || turnNowErr != nil))
			dispatched, fallbackErr := s.takeIfPromptableLocked(ctx, taskID, sessionID, queued.ID)
			return queued, dispatched, fallbackErr
		}
	}

	dispatched, err := s.cancelAndTakeForPeerMessage(ctx, taskID, sessionID, queued.ID, unlockGuard, relockGuard)
	return queued, dispatched, err
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
	s.processParentChildrenCompletedForTaskState(ctx, taskID, v1.TaskStateCompleted)

	s.logger.Info("task marked as COMPLETED",
		zap.String("task_id", taskID))
	return nil
}

// ResetAgentContext resets the agent's conversation context for a session,
// clearing conversation history while preserving the workspace environment.
func (s *Service) ResetAgentContext(ctx context.Context, sessionID string) error {
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		return err
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		return fmt.Errorf("agent must be idle to reset context, current state: %s", session.State)
	}
	if hasRunning, hasErr := s.repo.HasExecutorRunningRow(ctx, sessionID); hasErr != nil || !hasRunning {
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
