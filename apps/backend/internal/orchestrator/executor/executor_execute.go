package executor

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/common/subproc"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator/sessionstate"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// isConfigModeSession returns true if the session has config_mode: true in its metadata.
// Config-mode sessions are dedicated settings-chat sessions that get config MCP tools.
func isConfigModeSession(session *models.TaskSession) bool {
	if session == nil || session.Metadata == nil {
		return false
	}
	cm, ok := session.Metadata["config_mode"].(bool)
	return ok && cm
}

// resolveTaskSessionMCPMode derives restricted MCP access from canonical task
// ownership and session purpose. Config mode wins because those sessions need
// config tools even if their backing task is Office-owned.
func (e *Executor) resolveTaskSessionMCPMode(ctx context.Context, taskID string, session *models.TaskSession, allowTitleTool bool) (string, error) {
	if isConfigModeSession(session) {
		return McpModeConfig, nil
	}
	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("load task for MCP mode: %w", err)
	}
	if task != nil && task.Origin == models.TaskOriginAutomationRun {
		return McpModeAutomation, nil
	}
	if task != nil && task.IsFromOffice {
		return McpModeOffice, nil
	}
	if allowTitleTool && task != nil && models.IsAgentTitleOwner(task.Metadata, session.ID) {
		return McpModeTaskTitlePending, nil
	}
	return "", nil
}

func (e *Executor) resolveTaskSessionMCPProfile(ctx context.Context, taskID string, session *models.TaskSession, allowTitleTool bool) (mcpprofile.Context, error) {
	if isConfigModeSession(session) {
		capabilities := []mcpprofile.Capability{mcpprofile.CapabilityUserQuestion}
		if session.IsPassthrough {
			capabilities = nil
		}
		return mcpprofile.New(mcpprofile.SurfaceConfiguration, capabilities, nil), nil
	}
	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return mcpprofile.Context{}, fmt.Errorf("load task for MCP profile: %w", err)
	}
	if task == nil {
		// A few lifecycle paths can prepare a request from a session snapshot
		// before the task row is visible (and older executor fakes model that
		// state). Keep the legacy kanban profile in that narrow case; production
		// task launches resolve the persisted task above and therefore still get
		// the exact office/autopilot capability set.
		return mcpprofile.Legacy("", session != nil && session.IsPassthrough, nil), nil
	}
	if task.Origin == models.TaskOriginAutomationRun {
		return mcpprofile.NewAutomation(), nil
	}
	surface := mcpprofile.SurfaceKanbanTask
	if task.IsFromOffice {
		surface = mcpprofile.SurfaceOfficeTask
	}
	capabilities := make([]mcpprofile.Capability, 0, 2)
	if task.Autopilot {
		if task.ParentID != "" && !session.IsPassthrough {
			capabilities = append(capabilities, mcpprofile.CapabilityParentQuestion)
		}
	} else if !session.IsPassthrough {
		capabilities = append(capabilities, mcpprofile.CapabilityUserQuestion)
	}
	if allowTitleTool && surface == mcpprofile.SurfaceKanbanTask && models.IsAgentTitleOwner(task.Metadata, session.ID) {
		capabilities = append(capabilities, mcpprofile.CapabilityTaskTitle)
	}
	return mcpprofile.New(surface, capabilities, nil), nil
}

// isContainerizedExecutor returns true for executor types that run agents in
// containers or remote sandboxes (Docker variants, Sprites, and Kubernetes).
// These are the same executors that need explicitly configured remote
// credentials and the kandev-managed feature branch propagated through env
// metadata.
func isContainerizedExecutor(executorType string) bool {
	return models.IsContainerizedExecutorType(models.ExecutorType(executorType))
}

// executorNeedsResolvedCredentials reports whether an executor runs the agent
// off the control-plane host and therefore needs explicitly selected profile
// credentials resolved into req.Env. This is every containerized executor plus
// SSH, whose remote agentctl only receives the keys we forward in req.Env.
func executorNeedsResolvedCredentials(executorType string) bool {
	return isContainerizedExecutor(executorType) ||
		models.ExecutorType(executorType) == models.ExecutorTypeSSH
}

// runAgentProcessAsync starts the agent subprocess in a background goroutine.
// On error it marks the session as FAILED. The task is also marked FAILED only
// when escalateTaskOnFailure is true; resume callers pass false so a transient
// background bootstrap error does not destructively overwrite the task's
// existing state (e.g. REVIEW). fromResume is forwarded to onAgentStartFailed
// so the orchestrator can suppress user-facing toasts on background recovery.
// On success it calls onSuccess with a non-cancellable context derived from ctx.
// ctx is used with WithoutCancel so trace spans are preserved without inheriting cancellation.
func (e *Executor) runAgentProcessAsync(ctx context.Context, taskID, sessionID, agentExecutionID string, onSuccess func(context.Context), escalateTaskOnFailure, fromResume bool) {
	go func() {
		startCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
		defer cancel()
		updateCtx := context.WithoutCancel(ctx)

		if err := e.agentManager.StartAgentProcess(startCtx, agentExecutionID); err != nil {
			e.handleAgentProcessStartFailure(
				updateCtx, taskID, sessionID, agentExecutionID, err,
				escalateTaskOnFailure, fromResume,
			)
			return
		}
		if _, terminal := e.stopStartedExecutionIfSessionTerminal(
			updateCtx,
			sessionID,
			agentExecutionID,
			"terminal post-start race",
		); terminal {
			return
		}

		onSuccess(updateCtx)
	}()
}

func (e *Executor) handleAgentProcessStartFailure(
	ctx context.Context,
	taskID, sessionID, agentExecutionID string,
	startErr error,
	escalateTaskOnFailure, fromResume bool,
) {
	// A cancelled context or a terminal-session error is a benign teardown race
	// (the session ended while StartAgentProcess was blocked), not a genuine
	// start fault, so it logs at WARN without a stacktrace. DeadlineExceeded
	// is NOT treated as teardown: runAgentProcessAsync owns a 5-minute startup
	// deadline, and a hung agent that hits it on a still-active session is a
	// real operational failure that must stay at ERROR.
	if errors.Is(startErr, context.Canceled) ||
		errors.Is(startErr, lifecycle.ErrSessionTerminal) {
		e.logger.Warn("agent process start aborted by session teardown",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("agent_execution_id", agentExecutionID),
			zap.Error(startErr))
	} else {
		e.logger.Error("failed to start agent process",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("agent_execution_id", agentExecutionID),
			zap.Error(startErr))
	}

	// A terminal transition may have landed while StartAgentProcess was
	// blocked. Drop all failure/recovery side effects in that case. CANCELLED
	// owns teardown only when another path has claimed this exact execution.
	if terminalState, terminal := e.currentTerminalSessionState(ctx, sessionID); terminal {
		if terminalState != models.TaskSessionStateCancelled ||
			e.claimForcedExecutionCleanup(sessionID, agentExecutionID) {
			e.stopFailedStartExecution(ctx, agentExecutionID, "terminal start race")
		}
		return
	}

	// Let the orchestrator handle auth errors as recoverable failures and
	// (for resume) suppress the toast before the session is marked FAILED.
	if e.onAgentStartFailed != nil && e.onAgentStartFailed(
		ctx, taskID, sessionID, agentExecutionID, startErr, fromResume,
	) {
		return
	}

	changed, finalState, updateErr := e.transitionSessionState(
		ctx, taskID, sessionID, models.TaskSessionStateFailed, startErr.Error(),
	)
	if updateErr != nil {
		e.logger.Warn("failed to mark session as failed after start error",
			zap.String("session_id", sessionID),
			zap.Error(updateErr))
	}
	if changed && finalState == models.TaskSessionStateFailed && escalateTaskOnFailure {
		if updateErr := e.writeTaskFailedForRuntime(ctx, taskID, sessionID); updateErr != nil {
			e.logger.Warn("failed to mark task as failed after start error",
				zap.String("task_id", taskID),
				zap.Error(updateErr))
		}
	} else if changed && finalState == models.TaskSessionStateFailed {
		e.writeTaskReviewStateIfNoWorkingSessions(ctx, taskID, sessionID)
	}

	// The agent process never fully started. Skip forced cleanup only when a
	// concurrent path owns teardown for this exact cancelled execution.
	if finalState != models.TaskSessionStateCancelled ||
		e.claimForcedExecutionCleanup(sessionID, agentExecutionID) {
		e.stopFailedStartExecution(ctx, agentExecutionID, "start failure")
	}
}

func (e *Executor) claimForcedExecutionCleanup(sessionID, agentExecutionID string) bool {
	if e.onExecutionCleanupClaim == nil {
		return true
	}
	return e.onExecutionCleanupClaim(sessionID, agentExecutionID)
}

func (e *Executor) stopFailedStartExecution(ctx context.Context, agentExecutionID, phase string) {
	if stopErr := e.agentManager.StopAgent(ctx, agentExecutionID, true); stopErr != nil {
		e.logger.Warn("failed to clean up agent after "+phase,
			zap.String("agent_execution_id", agentExecutionID),
			zap.Error(stopErr))
	}
}

func (e *Executor) currentTerminalSessionState(
	ctx context.Context,
	sessionID string,
) (models.TaskSessionState, bool) {
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || !isStopTerminalSessionState(session.State) {
		return "", false
	}
	return session.State, true
}

func (e *Executor) stopStartedExecutionIfSessionTerminal(
	ctx context.Context,
	sessionID, agentExecutionID, phase string,
) (models.TaskSessionState, bool) {
	terminalState, terminal := e.currentTerminalSessionState(ctx, sessionID)
	if !terminal {
		return "", false
	}
	if e.claimForcedExecutionCleanup(sessionID, agentExecutionID) {
		e.stopFailedStartExecution(ctx, agentExecutionID, phase)
	}
	return terminalState, true
}

// startAgentProcessAsync starts the agent subprocess and transitions its session
// to RUNNING before reconciling the owning task to IN_PROGRESS on success.
func (e *Executor) startAgentProcessAsync(ctx context.Context, taskID, sessionID, agentExecutionID string) {
	e.runAgentProcessAsync(ctx, taskID, sessionID, agentExecutionID, func(updCtx context.Context) {
		if !e.markSessionRunningAfterProcessStart(updCtx, taskID, sessionID) {
			return
		}
		if updateErr := e.writeTaskInProgressForRuntime(updCtx, taskID, sessionID); updateErr != nil {
			e.logger.Warn("failed to update task state to IN_PROGRESS after agent start",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(updateErr))
		}
	}, true, false)
}

// markSessionRunningAfterProcessStart records that a successfully started
// process is active. Agent stream events may settle a fast turn before this
// callback runs, so only a still-STARTING session may move to RUNNING.
func (e *Executor) markSessionRunningAfterProcessStart(ctx context.Context, taskID, sessionID string) bool {
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		e.logger.Warn("failed to load session after agent start",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false
	}
	if session == nil {
		e.logger.Warn("session missing after agent start",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
		return false
	}

	switch session.State {
	case models.TaskSessionStateRunning:
		return true
	case models.TaskSessionStateStarting:
		changed, finalState, transitionErr := e.transitionSessionState(
			ctx, taskID, sessionID, models.TaskSessionStateRunning, "",
		)
		if transitionErr != nil {
			e.logger.Warn("failed to mark session running after agent start",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(transitionErr))
			return false
		}
		return changed || finalState == models.TaskSessionStateRunning
	default:
		return false
	}
}

func (e *Executor) stopUnstartedExecution(ctx context.Context, sessionID, agentExecutionID string) {
	if agentExecutionID == "" {
		return
	}
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if stopErr := e.agentManager.StopAgent(stopCtx, agentExecutionID, true); stopErr != nil {
		e.logger.Warn("failed to stop unstarted agent execution",
			zap.String("session_id", sessionID),
			zap.String("agent_execution_id", agentExecutionID),
			zap.Error(stopErr))
	}
}

func (e *Executor) cleanupUnstartedExecutionAfterPersistError(
	ctx context.Context,
	sessionID, agentExecutionID string,
	persistErr error,
) {
	var superseded *SessionStateSupersededError
	if errors.As(persistErr, &superseded) &&
		superseded.State == models.TaskSessionStateCancelled &&
		!e.claimForcedExecutionCleanup(sessionID, agentExecutionID) {
		return
	}
	e.stopUnstartedExecution(ctx, sessionID, agentExecutionID)
}

func (e *Executor) markTaskEnvironmentMaterializationFailed(
	ctx context.Context,
	env *models.TaskEnvironment,
	sessionID string,
) {
	if env == nil || env.Status != models.TaskEnvironmentStatusCreating || env.MaterializationSessionID != sessionID {
		return
	}
	env.Status = models.TaskEnvironmentStatusFailed
	env.MaterializationSessionID = ""
	if err := e.repo.UpdateTaskEnvironment(ctx, env); err != nil {
		e.logger.Warn("failed to mark workspace materialization failed",
			zap.String("task_environment_id", env.ID),
			zap.Error(err))
	}
}

func (e *Executor) writeTaskReviewStateIfNoWorkingSessions(ctx context.Context, taskID, failedSessionID string) {
	if e.onTaskReviewStateReconcile != nil {
		e.onTaskReviewStateReconcile(ctx, taskID, failedSessionID)
		return
	}

	if e.shouldSkipFailedStartReviewForTask(ctx, taskID, failedSessionID) {
		return
	}
	if e.failedSessionStillWorkingOrUnknown(ctx, taskID, failedSessionID) {
		return
	}
	if e.hasOtherWorkingSessions(ctx, taskID, failedSessionID) {
		return
	}
	// When onTaskStateChange is configured, it owns event publishing for
	// this write (see its doc comment) — keep routing through it rather
	// than bypassing it. Only when NEITHER callback is set (no orchestrator
	// wiring at all — production always wires both, see service.go's
	// exec.SetOnTaskStateChange/SetOnTaskReviewStateReconcile) do we fall
	// back to the archive-aware UpdateTaskStateIfCurrentIn CAS directly on
	// the repository, so even that raw path can't race an archive that
	// commits between shouldSkipFailedStartReviewForTask's read and this write.
	if e.onTaskStateChange != nil {
		if updateErr := e.onTaskStateChange(ctx, taskID, v1.TaskStateReview); updateErr != nil {
			e.logger.Warn("failed to update task state to REVIEW after start error",
				zap.String("task_id", taskID),
				zap.Error(updateErr))
		}
		return
	}
	if _, _, updateErr := e.repo.UpdateTaskStateIfCurrentIn(ctx, taskID, v1.TaskStateReview, []v1.TaskState{v1.TaskStateInProgress, v1.TaskStateScheduling}); updateErr != nil {
		e.logger.Warn("failed to update task state to REVIEW after start error",
			zap.String("task_id", taskID),
			zap.Error(updateErr))
	}
}

func (e *Executor) shouldSkipFailedStartReviewForTask(ctx context.Context, taskID, failedSessionID string) bool {
	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		e.logger.Warn("failed to load task before failed-start REVIEW state reconcile",
			zap.String("task_id", taskID),
			zap.String("session_id", failedSessionID),
			zap.Error(err))
		return true
	}
	if task != nil && task.IsFromOffice {
		e.logger.Debug("skipping failed-start task REVIEW state for office task",
			zap.String("task_id", taskID),
			zap.String("session_id", failedSessionID))
		return true
	}
	if task != nil && task.ArchivedAt != nil {
		e.logger.Debug("skipping failed-start task REVIEW state for archived task",
			zap.String("task_id", taskID),
			zap.String("session_id", failedSessionID))
		return true
	}
	return false
}

func (e *Executor) failedSessionStillWorkingOrUnknown(ctx context.Context, taskID, failedSessionID string) bool {
	if failedSessionID == "" {
		return false
	}
	session, err := e.repo.GetTaskSession(ctx, failedSessionID)
	if err != nil {
		e.logger.Warn("failed to load failed session before failed-start REVIEW state reconcile",
			zap.String("task_id", taskID),
			zap.String("session_id", failedSessionID),
			zap.Error(err))
		return true
	}
	if session != nil && isRuntimeWorkingSessionState(session.State) {
		e.logger.Debug("skipping failed-start task REVIEW state because failed session is active again",
			zap.String("task_id", taskID),
			zap.String("session_id", failedSessionID),
			zap.String("session_state", string(session.State)))
		return true
	}
	return false
}

func (e *Executor) hasOtherWorkingSessions(ctx context.Context, taskID, failedSessionID string) bool {
	sessions, err := e.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		e.logger.Warn("failed to list task sessions before failed-start REVIEW state reconcile",
			zap.String("task_id", taskID),
			zap.String("session_id", failedSessionID),
			zap.Error(err))
		return true
	}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if failedSessionID != "" && session.ID == failedSessionID {
			continue
		}
		if isRuntimeWorkingSessionState(session.State) {
			e.logger.Debug("skipping failed-start task REVIEW state while another session is working",
				zap.String("task_id", taskID),
				zap.String("failed_session_id", failedSessionID),
				zap.String("blocking_session_id", session.ID))
			return true
		}
	}
	return false
}

func isRuntimeWorkingSessionState(state models.TaskSessionState) bool {
	return sessionstate.IsWorking(state)
}

// updateTaskState updates a task's state, using the callback if set for event publishing,
// or falling back to the archive-aware CAS (UpdateTaskStateIfNotArchived) directly. Every
// call site here is a runtime-driven write (IN_PROGRESS on start/resume, FAILED on launch
// error) that must never resurrect an archived task's state (PR #1706 review).
func (e *Executor) updateTaskState(ctx context.Context, taskID string, state v1.TaskState) error {
	if e.onTaskStateChange != nil {
		return e.onTaskStateChange(ctx, taskID, state)
	}
	_, _, err := e.repo.UpdateTaskStateIfNotArchived(ctx, taskID, state)
	return err
}

// updateSessionState updates a session's state, using the callback if set for event publishing,
// or falling back to the raw repository.
func (e *Executor) updateSessionState(ctx context.Context, taskID, sessionID string, state models.TaskSessionState, errorMessage string) error {
	if e.onSessionStateChange != nil {
		return e.onSessionStateChange(ctx, taskID, sessionID, state, errorMessage)
	}
	return e.repo.UpdateTaskSessionState(ctx, sessionID, state, errorMessage)
}

// transitionSessionState updates a session only when its freshly observed
// state is non-terminal and different from the requested state. It returns the
// authoritative final state so callers can tell an accepted write from an
// idempotent or naturally-terminal race.
func (e *Executor) transitionSessionState(
	ctx context.Context,
	taskID, sessionID string,
	state models.TaskSessionState,
	errorMessage string,
) (bool, models.TaskSessionState, error) {
	return e.transitionSessionStateWithHook(ctx, taskID, sessionID, state, errorMessage, nil)
}

func (e *Executor) transitionSessionStateWithHook(
	ctx context.Context,
	taskID, sessionID string,
	state models.TaskSessionState,
	errorMessage string,
	onChanged func(),
) (bool, models.TaskSessionState, error) {
	if e.onSessionStateTransition != nil {
		return e.onSessionStateTransition(ctx, taskID, sessionID, state, errorMessage, onChanged)
	}

	current, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return false, "", fmt.Errorf("get session before state transition: %w", err)
	}
	if current == nil {
		return false, "", fmt.Errorf("get session before state transition: session %q is nil", sessionID)
	}
	if isStopTerminalSessionState(current.State) || current.State == state {
		return false, current.State, nil
	}
	if err := e.updateSessionState(ctx, taskID, sessionID, state, errorMessage); err != nil {
		return false, current.State, err
	}
	refreshed, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return false, "", fmt.Errorf("get session after state transition: %w", err)
	}
	if refreshed == nil {
		return false, "", fmt.Errorf("get session after state transition: session %q is nil", sessionID)
	}
	if refreshed.State != state {
		return false, refreshed.State, nil
	}
	if onChanged != nil {
		onChanged()
	}
	return true, refreshed.State, nil
}

func isStopTerminalSessionState(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateCompleted ||
		state == models.TaskSessionStateFailed ||
		state == models.TaskSessionStateCancelled
}

func allowsSessionStartingRecovery(
	nextState, expectedState, currentState models.TaskSessionState,
	promoteTask bool,
) bool {
	return !promoteTask &&
		nextState == models.TaskSessionStateStarting &&
		currentState == expectedState &&
		(expectedState == models.TaskSessionStateFailed ||
			expectedState == models.TaskSessionStateCancelled)
}

// updateSessionStarting persists a full session-row STARTING transition, using
// the orchestrator callback when present so task/session runtime state stays
// serialized with guarded REVIEW reconciliation.
func (e *Executor) updateSessionStarting(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	expectedState models.TaskSessionState,
	promoteTask bool,
) error {
	if e.onSessionStarting != nil {
		return e.onSessionStarting(ctx, taskID, session, expectedState, promoteTask)
	}
	current, err := e.repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		return err
	}
	if current == nil {
		return fmt.Errorf("%w: agent session not found: %s", models.ErrTaskSessionNotFound, session.ID)
	}
	allowedTerminalRecovery := allowsSessionStartingRecovery(
		session.State, expectedState, current.State, promoteTask,
	)
	if isStopTerminalSessionState(current.State) && !allowedTerminalRecovery {
		return &SessionStateSupersededError{SessionID: session.ID, State: current.State}
	}
	return e.persistSessionFullRowIfCurrentState(ctx, session, expectedState)
}

func (e *Executor) persistSessionFullRowIfCurrentState(
	ctx context.Context,
	session *models.TaskSession,
	expected models.TaskSessionState,
) error {
	changed, err := e.repo.UpdateTaskSessionIfCurrentState(ctx, session, expected)
	if err != nil {
		return err
	}
	if changed {
		return nil
	}
	current, err := e.repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		return err
	}
	if current == nil {
		return fmt.Errorf("%w: agent session not found: %s", models.ErrTaskSessionNotFound, session.ID)
	}
	if isStopTerminalSessionState(current.State) {
		return &SessionStateSupersededError{SessionID: session.ID, State: current.State}
	}
	return fmt.Errorf(
		"session %s state changed from %s to %s before runtime persistence",
		session.ID,
		expected,
		current.State,
	)
}

// shouldUseWorktree returns true if the given executor type should use Git worktrees.
func shouldUseWorktree(executorType string) bool {
	return models.ExecutorType(executorType) == models.ExecutorTypeWorktree
}

// repositoryCloneURL builds a clone URL for the repository. It prefers the
// provider info when present (HTTPS GitHub/GitLab/Bitbucket URL); otherwise
// it inspects the local checkout's `origin` remote. The latter lets local-only
// repos with a real remote (or a file:// remote, used by Docker E2E tests)
// participate in remote executors that clone inside the container/sandbox.
func repositoryCloneURL(repo *models.Repository) string {
	if strings.TrimSpace(repo.RemoteURL) != "" {
		return strings.TrimSpace(repo.RemoteURL)
	}
	if repo.ProviderOwner != "" && repo.ProviderName != "" {
		return providerHTTPSCloneURL(repo)
	}
	if repo.LocalPath == "" {
		return ""
	}
	cmd := subproc.NewGitCommand(context.Background(), "-C", repo.LocalPath, "remote", "get-url", "origin")
	out, err := subproc.RunGitOutputClass(context.Background(), subproc.GitLifecycle, cmd)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// providerHTTPSCloneURL derives the HTTPS clone URL from the persisted provider
// identity. Its host is authoritative: it carries the provider's real HTTPS
// origin, including a non-default port, which a clone URL from another
// transport cannot supply.
func providerHTTPSCloneURL(repo *models.Repository) string {
	if repo.ProviderOwner == "" || repo.ProviderName == "" {
		return ""
	}
	if strings.EqualFold(repo.Provider, "gitlab") && strings.TrimSpace(repo.ProviderHost) == "" {
		return ""
	}
	cloneURL, err := repoclone.CloneURLWithHost(
		repo.Provider, repo.ProviderHost, repo.ProviderOwner, repo.ProviderName, repoclone.ProtocolHTTPS,
	)
	if err != nil {
		return ""
	}
	return cloneURL
}

// getSessionLock returns a per-session mutex, creating one if it doesn't exist.
// This serializes concurrent resume/launch operations on the same session to prevent
// duplicate agent processes after backend restart.
func (e *Executor) getSessionLock(sessionID string) *sync.Mutex {
	val, _ := e.sessionLocks.LoadOrStore(sessionID, &sync.Mutex{})
	return val.(*sync.Mutex)
}

func (e *Executor) applyPreferredShellEnv(ctx context.Context, executorType string, env map[string]string) map[string]string {
	result, _ := e.applyPreferredShellEnvWithStatus(ctx, executorType, env)
	return result
}

func (e *Executor) applyPreferredShellEnvWithStatus(ctx context.Context, executorType string, env map[string]string) (map[string]string, bool) {
	if e.capabilities == nil || !e.capabilities.ShouldApplyPreferredShell(executorType) {
		return env, false
	}
	if e.shellPrefs == nil {
		return env, false
	}
	preferred, err := e.shellPrefs.PreferredShell(ctx)
	if err != nil {
		return env, false
	}
	preferred = strings.TrimSpace(preferred)
	if preferred == "" {
		return env, false
	}
	if env == nil {
		env = make(map[string]string)
	}
	env["AGENTCTL_SHELL_COMMAND"] = preferred
	env["SHELL"] = preferred
	return env, true
}

// Execute starts agent execution for a task
func (e *Executor) Execute(ctx context.Context, task *v1.Task) (*TaskExecution, error) {
	return e.ExecuteWithFullProfile(ctx, task, "", "", "", task.Description, "")
}

// ExecuteWithProfile starts agent execution for a task using an explicit agent profile.
// The executorID parameter specifies which executor to use (determines runtime: local, worktree, local_docker, etc.).
// If executorID is empty, falls back to workspace's default executor.
// The prompt parameter is the initial prompt to send to the agent.
// The workflowStepID parameter associates the session with a workflow step for transitions.
func (e *Executor) ExecuteWithProfile(ctx context.Context, task *v1.Task, agentProfileID string, executorID string, prompt string, workflowStepID string) (*TaskExecution, error) {
	return e.ExecuteWithFullProfile(ctx, task, agentProfileID, executorID, "", prompt, workflowStepID)
}

// ExecuteWithFullProfile starts agent execution for a task using an explicit agent profile and executor profile.
func (e *Executor) ExecuteWithFullProfile(ctx context.Context, task *v1.Task, agentProfileID string, executorID string, executorProfileID string, prompt string, workflowStepID string) (*TaskExecution, error) {
	// Create session entry in database first
	sessionID, err := e.PrepareSession(ctx, task, agentProfileID, executorID, executorProfileID, workflowStepID)
	if err != nil {
		return nil, err
	}
	dbTask, err := e.repo.GetTask(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("load task before title claim: %w", err)
	}
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session before title claim: %w", err)
	}
	if (dbTask == nil || !dbTask.IsFromOffice) && !isConfigModeSession(session) {
		if claimer, ok := e.repo.(interface {
			ClaimTaskTitleSession(context.Context, string, string) (bool, bool, error)
		}); ok {
			if _, _, err := claimer.ClaimTaskTitleSession(ctx, task.ID, sessionID); err != nil {
				return nil, fmt.Errorf("claim first-turn task title: %w", err)
			}
		}
	}

	// Launch the agent for the prepared session.
	execution, err := e.LaunchPreparedSession(ctx, task, sessionID, LaunchOptions{
		AgentProfileID: agentProfileID,
		ExecutorID:     executorID,
		Prompt:         prompt,
		WorkflowStepID: workflowStepID,
		StartAgent:     true,
	})
	if err != nil {
		// LaunchAgent errors are already persisted by LaunchPreparedSession.
		// Calling the session-aware recorder again is a no-op for those errors
		// and covers failures from earlier environment preparation without
		// letting a superseded session fail the task.
		return nil, e.handleEarlyLaunchFailure(ctx, task.ID, sessionID, "", err)
	}
	return execution, nil
}

// PrepareSession creates a session entry in the database without launching the agent.
// This allows the caller to get the session ID immediately and launch the agent later.
// Returns the session ID.
func (e *Executor) PrepareSession(ctx context.Context, task *v1.Task, agentProfileID string, executorID string, executorProfileID string, workflowStepID string) (string, error) {
	return e.prepareSession(ctx, task, agentProfileID, executorID, executorProfileID, workflowStepID, true, "")
}

// PrepareSessionForExistingEnvironment creates a workflow replacement session
// that will be bound by its caller to an already selected canonical
// environment. It must not claim a temporary task-local materialization.
func (e *Executor) PrepareSessionForExistingEnvironment(ctx context.Context, task *v1.Task, agentProfileID string, executorID string, executorProfileID string, workflowStepID string, taskEnvironmentID string) (string, error) {
	if taskEnvironmentID == "" {
		return "", fmt.Errorf("%w: workflow replacement requires a canonical workspace", models.ErrWorkspaceReuseUnsafe)
	}
	if err := e.preflightWorkflowWorkspaceReuse(ctx, task, taskEnvironmentID); err != nil {
		return "", err
	}
	return e.prepareSession(ctx, task, agentProfileID, executorID, executorProfileID, workflowStepID, false, taskEnvironmentID)
}

// preflightWorkflowWorkspaceReuse rejects an invalid retained environment
// before a workflow replacement can promote its session and stop the current
// one. Lifecycle still validates the physical executor resource immediately
// before attach; this preflight covers durable readiness and inventory while
// the old session remains usable.
func (e *Executor) preflightWorkflowWorkspaceReuse(ctx context.Context, task *v1.Task, environmentID string) error {
	environment, err := e.repo.GetTaskEnvironment(ctx, environmentID)
	if err != nil || environment == nil {
		return fmt.Errorf("%w: workflow workspace is unavailable", models.ErrWorkspaceReuseUnsafe)
	}
	switch environment.Status {
	case models.TaskEnvironmentStatusCreating:
		return fmt.Errorf("%w: retry after the workspace launch completes", models.ErrWorkspacePreparing)
	case models.TaskEnvironmentStatusReady, models.TaskEnvironmentStatusStopped:
	default:
		return fmt.Errorf("%w: workflow workspace is not attachable", models.ErrWorkspaceReuseUnsafe)
	}
	if task == nil {
		return fmt.Errorf("%w: workflow task is unavailable", models.ErrWorkspaceReuseUnsafe)
	}
	taskRepos, err := e.repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("%w: load workflow repositories", models.ErrWorkspaceReuseUnsafe)
	}
	if len(taskRepos) == 0 {
		return nil
	}
	rows, err := e.repo.ListTaskEnvironmentRepos(ctx, environment.ID)
	if err != nil {
		return fmt.Errorf("%w: load workflow workspace inventory", models.ErrWorkspaceReuseUnsafe)
	}
	for _, taskRepo := range taskRepos {
		if !workflowEnvironmentHasRepository(rows, taskRepo.RepositoryID) {
			return fmt.Errorf("%w: workflow workspace repository inventory is incomplete", models.ErrWorkspaceReuseUnsafe)
		}
	}
	return nil
}

func workflowEnvironmentHasRepository(rows []*models.TaskEnvironmentRepo, repositoryID string) bool {
	for _, row := range rows {
		if row != nil && row.RepositoryID == repositoryID && row.DeletedAt == nil && row.Status != taskEnvironmentRepoStatusFailed && row.Status != taskEnvironmentRepoStatusDeleted {
			return true
		}
	}
	return false
}

//nolint:cyclop,funlen // Session construction keeps its existing validation sequence in one transaction boundary.
func (e *Executor) prepareSession(ctx context.Context, task *v1.Task, agentProfileID string, executorID string, executorProfileID string, workflowStepID string, bindWorkspace bool, taskEnvironmentID string) (string, error) {
	if agentProfileID == "" {
		e.logger.Error("task has no agent_profile_id configured", zap.String("task_id", task.ID))
		return "", ErrNoAgentProfileID
	}

	metadata := cloneMetadata(task.Metadata)
	initialRuntimeConfig, hasInitialRuntimeConfig := models.LoadInitialSessionRuntimeConfig(task.Metadata)
	delete(metadata, models.MetaKeyInitialSessionRuntimeConfig)
	delete(metadata, models.MetaKeyInitialSessionRuntimeConfigProfileID)
	initialRuntimeConfigProfileID := models.LoadInitialSessionRuntimeConfigProfileID(task.Metadata)
	_, hasAtomicInitialRuntimeSeed := e.repo.(initialRuntimeSeedTaskSessionCreator)
	var repositoryID string
	var baseBranch string

	// Get the primary repository for this task
	primaryTaskRepo, err := e.repo.GetPrimaryTaskRepository(ctx, task.ID)
	if err != nil {
		e.logger.Error("failed to get primary task repository",
			zap.String("task_id", task.ID),
			zap.Error(err))
		return "", err
	}

	if primaryTaskRepo != nil {
		repositoryID = primaryTaskRepo.RepositoryID
		baseBranch = primaryTaskRepo.BaseBranch
	}

	// Resolve agent profile to get model and other settings for snapshot
	agentProfileSnapshot, isPassthrough := e.resolveAgentProfileSnapshot(ctx, agentProfileID)

	// Determine whether this is the task's first session and whether it should
	// become primary. The immutable origin marker follows creation order, not
	// primary-session ownership, because a later user-selected primary must not
	// change which conversation workflow rules treat as the original.
	existingSessions, err := e.repo.ListTaskSessions(ctx, task.ID)
	if err != nil {
		e.logger.Error("failed to list task sessions before creating initial session",
			zap.String("task_id", task.ID), zap.Error(err))
		return "", fmt.Errorf("list task sessions: %w", err)
	}
	isTaskInitialSession := len(existingSessions) == 0
	hasPrimary := false
	for _, s := range existingSessions {
		if s.IsPrimary {
			hasPrimary = true
			break
		}
	}
	isPrimarySession := !hasPrimary
	if isTaskInitialSession {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata[models.SessionMetaKeyOrigin] = models.SessionOriginTaskInitial
		if hasInitialRuntimeConfig && initialRuntimeConfigProfileID == agentProfileID && !hasAtomicInitialRuntimeSeed {
			metadata[models.SessionMetaKeyRuntimeConfigOverrides] = initialRuntimeConfig
		}
	}

	// Create agent session in database. WorkspacePath is propagated from task
	// metadata for repo-less tasks where the user picked a starting folder.
	workspacePath, _ := task.Metadata[models.MetaKeyWorkspacePath].(string)
	sessionID := uuid.New().String()
	now := time.Now().UTC()
	session := &models.TaskSession{
		ID:                   sessionID,
		TaskID:               task.ID,
		AgentProfileID:       agentProfileID,
		RepositoryID:         repositoryID,
		BaseBranch:           baseBranch,
		WorkspacePath:        workspacePath,
		State:                models.TaskSessionStateCreated,
		StartedAt:            now,
		UpdatedAt:            now,
		AgentProfileSnapshot: agentProfileSnapshot,
		IsPrimary:            isPrimarySession,
		IsPassthrough:        isPassthrough,
		Metadata:             metadata,
	}
	if taskEnvironmentID != "" {
		session.TaskEnvironmentID = taskEnvironmentID
	}
	// workflow_step_id is a task-level field; no longer stored on sessions.

	// Store executor profile ID on session
	if executorProfileID != "" {
		session.ExecutorProfileID = executorProfileID
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["executor_profile_id"] = executorProfileID
	}

	// Resolve executor configuration
	execConfig := e.resolveExecutorConfig(ctx, executorID, task.WorkspaceID, metadata)
	if execConfig.ExecutorID != "" {
		session.ExecutorID = execConfig.ExecutorID
	}

	// Validate every managed-credential repository binding before persisting
	// the session row. Doing this after the row exists would leave a
	// zero-message session behind once launch fails at credential issuance.
	if err := e.preflightManagedGitCredentials(ctx, task.WorkspaceID, task.ID, execConfig); err != nil {
		e.logger.Error("managed Git credential preflight failed",
			zap.String("task_id", task.ID),
			zap.Error(err))
		return "", err
	}

	createErr := e.createPreparedSession(ctx, session, task.Metadata, bindWorkspace, execConfig)
	if createErr != nil {
		e.logger.Error("failed to persist agent session",
			zap.String("task_id", task.ID),
			zap.Error(createErr))
		return "", createErr
	}

	// Set primary flag only for the first session (no existing primary).
	// Subsequent sessions do not override the established primary.
	if isPrimarySession {
		if err := e.repo.SetSessionPrimary(ctx, sessionID); err != nil {
			e.logger.Warn("failed to update primary session flag",
				zap.String("task_id", task.ID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
		if e.onPrimarySessionSet != nil {
			e.onPrimarySessionSet(ctx, task.ID, sessionID)
		}
	}

	e.logger.Info("session entry created",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID))

	return sessionID, nil
}

func (e *Executor) createPreparedSession(
	ctx context.Context,
	session *models.TaskSession,
	metadata map[string]interface{},
	bindWorkspace bool,
	execConfig executorConfig,
) error {
	candidate := &models.TaskEnvironment{
		TaskID:            session.TaskID,
		ExecutorType:      execConfig.ExecutorType,
		ExecutorID:        execConfig.ExecutorID,
		ExecutorProfileID: session.ExecutorProfileID,
		Status:            models.TaskEnvironmentStatusCreating,
	}
	if groupID := sharedWorkspaceGroupID(metadata); bindWorkspace && groupID != "" {
		binder, ok := e.repo.(sharedGroupWorkspaceBindingTaskSessionCreator)
		if !ok {
			return fmt.Errorf("%w: shared workspace binding is unavailable", models.ErrWorkspaceReuseUnsafe)
		}
		return binder.CreateTaskSessionWithSharedGroupWorkspaceBinding(ctx, session, candidate, groupID)
	}
	if binder, ok := e.repo.(workspaceBindingTaskSessionCreator); ok && bindWorkspace && !taskUsesDeferredEnvironmentInheritance(metadata) {
		return binder.CreateTaskSessionWithWorkspaceBinding(ctx, session, candidate)
	}
	if atomicCreator, ok := e.repo.(initialRuntimeSeedTaskSessionCreator); ok {
		return atomicCreator.CreateTaskSessionWithInitialRuntimeSeed(ctx, session)
	}
	return e.repo.CreateTaskSession(ctx, session)
}

// taskUsesDeferredEnvironmentInheritance keeps the handoff resolver
// authoritative for inherited tasks. Shared groups use the transactional
// shared-group binder above, which elects the first materializer before any
// member can enter lifecycle preparation.
func taskUsesDeferredEnvironmentInheritance(metadata map[string]interface{}) bool {
	workspace, ok := metadata["workspace"].(map[string]interface{})
	if !ok {
		return false
	}
	mode, _ := workspace["mode"].(string)
	return mode == "inherit_parent"
}

func taskUsesInheritedWorkspace(task *v1.Task) bool {
	if task == nil {
		return false
	}
	workspace, ok := task.Metadata["workspace"].(map[string]interface{})
	if !ok {
		return false
	}
	mode, _ := workspace["mode"].(string)
	return mode == "inherit_parent" || mode == "shared_group"
}

func (e *Executor) resolveLaunchTaskEnvironment(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
) (*models.TaskEnvironment, error) {
	existingEnv, err := e.repo.GetTaskEnvironmentByTaskID(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("lookup existing task environment: %w", err)
	}
	if existingEnv != nil || session == nil || session.TaskEnvironmentID == "" {
		return existingEnv, nil
	}

	// Child tasks created by office task-handoffs may have had
	// session.TaskEnvironmentID rewritten to point at the parent's or shared
	// group's environment. The by-task-id lookup misses that row because it
	// indexes by the child task id, so validate the inherited row's owner before
	// allowing any launch path to reuse it.
	inherited, envErr := e.repo.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
	if envErr != nil || inherited == nil {
		return nil, fmt.Errorf("%w: %s", models.ErrWorkspaceReuseUnsafe, e.describeInheritedEnvironmentUnavailable(ctx, task))
	}
	if err := e.validateInheritedEnvironmentOwner(ctx, task, inherited); err != nil {
		return nil, err
	}
	return inherited, nil
}

func (e *Executor) validateInheritedEnvironmentOwner(
	ctx context.Context,
	task *v1.Task,
	env *models.TaskEnvironment,
) error {
	if env == nil || env.TaskID == "" || (task != nil && env.TaskID == task.ID) {
		return nil
	}
	owner, err := e.repo.GetTask(ctx, env.TaskID)
	if err != nil || owner == nil {
		return fmt.Errorf("%w: inherited task environment owner %s could not be verified", models.ErrWorkspaceReuseUnsafe, env.TaskID)
	}
	if owner.ArchivedAt != nil {
		return fmt.Errorf("%w: %s", models.ErrWorkspaceReuseUnsafe,
			models.DescribeInheritedEnvironmentUnavailable(env.TaskID, owner))
	}
	return nil
}

func sharedWorkspaceGroupID(metadata map[string]interface{}) string {
	workspace, ok := metadata["workspace"].(map[string]interface{})
	if !ok {
		return ""
	}
	mode, _ := workspace["mode"].(string)
	if mode != "shared_group" {
		return ""
	}
	groupID, _ := workspace["group_id"].(string)
	return groupID
}

// resolveAgentProfileSnapshot resolves an agent profile ID to a snapshot map and passthrough flag.
func (e *Executor) resolveAgentProfileSnapshot(ctx context.Context, agentProfileID string) (map[string]interface{}, bool) {
	profileInfo, err := e.agentManager.ResolveAgentProfile(ctx, agentProfileID)
	if err != nil || profileInfo == nil {
		return map[string]interface{}{
			"id":    agentProfileID,
			"model": "",
		}, false
	}
	return map[string]interface{}{
		"id":                           profileInfo.ProfileID,
		"name":                         profileInfo.ProfileName,
		"agent_id":                     profileInfo.AgentID,
		"agent_name":                   profileInfo.AgentName,
		"model":                        profileInfo.Model,
		"mode":                         profileInfo.Mode,
		"config_options":               maps.Clone(profileInfo.ConfigOptions),
		"auto_approve":                 profileInfo.AutoApprove,
		"dangerously_skip_permissions": profileInfo.DangerouslySkipPermissions,
		"cli_passthrough":              profileInfo.CLIPassthrough,
	}, profileInfo.CLIPassthrough
}

// LaunchPreparedSession launches the workspace (and optionally the agent) for a pre-created session.
// The session must have been created using PrepareSession.
// When opts.StartAgent is false, only the workspace infrastructure (agentctl) is launched; the agent
// subprocess is not started and the session state remains CREATED.
// When opts.StartAgent is true and the workspace was already launched (AgentExecutionID set), only the
// agent subprocess is started.
func (e *Executor) LaunchPreparedSession(ctx context.Context, task *v1.Task, sessionID string, opts LaunchOptions) (*TaskExecution, error) {
	agentProfileID := opts.AgentProfileID
	executorID := opts.ExecutorID
	prompt := opts.Prompt
	startAgent := opts.StartAgent
	// Serialise concurrent launches for the same session. Two callers reach
	// this path on every task: PrepareTaskSession spawns a background launch
	// (workspace only) the moment a session is created, and StartCreatedSession
	// is called when the agent is actually started (auto-start, user click).
	// Without this lock both run env-prep + executionStore.Add in parallel and
	// the second one fails at register with "already has an agent running
	// (race resolved during register)" — visible in the UI as
	// "Environment setup failed". Multi-repo amplifies this because the
	// per-repo prep runs sequentially, widening the race window.
	sessionLock := e.getSessionLock(sessionID)
	sessionLock.Lock()
	defer sessionLock.Unlock()

	// Re-fetch the session under the lock so the fast-path check below sees
	// any AgentExecutionID the previous holder just persisted. Without the
	// re-fetch we'd hold a stale snapshot and run a second full launch.
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		e.logger.Error("failed to get session for launch",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, err
	}

	if session.TaskID != task.ID {
		return nil, fmt.Errorf("session does not belong to task")
	}
	if strings.TrimSpace(executorID) == "" {
		// PrepareSession already persisted the selected executor. Launch calls
		// that omit the option must retain that selection so the authoritative
		// connection config reaches lifecycle instead of falling back to the
		// workspace default (or an empty config).
		executorID = strings.TrimSpace(session.ExecutorID)
	}
	if opts.McpMode == "" {
		opts.McpMode, err = e.resolveTaskSessionMCPMode(ctx, task.ID, session, opts.StartAgent)
		if err != nil {
			return nil, err
		}
	}
	if opts.McpProfile == nil {
		profileContext, profileErr := e.resolveTaskSessionMCPProfile(ctx, task.ID, session, opts.StartAgent)
		if profileErr != nil {
			return nil, profileErr
		}
		opts.McpProfile = &profileContext
	}

	running, runningErr := e.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if runningErr != nil && !errors.Is(runningErr, models.ErrExecutorRunningNotFound) {
		return nil, fmt.Errorf("load runtime inventory for session %q: %w", sessionID, runningErr)
	}
	if running != nil && running.ExecutionProfileID != "" &&
		running.ExecutionProfileID != agentProfileID {
		if running.AgentExecutionID != "" {
			if err := e.agentManager.StopAgentWithReason(
				ctx, running.AgentExecutionID, "execution profile changed", true,
			); err != nil && !errors.Is(err, lifecycle.ErrExecutionNotFound) {
				return nil, fmt.Errorf("stop previous execution profile: %w", err)
			}
		}
		if err := e.repo.DeleteExecutorRunningBySessionID(ctx, sessionID); err != nil {
			return nil, fmt.Errorf("clear previous execution profile: %w", err)
		}
		running = nil
	}

	// Inject session handover context if there are previous sessions for this task.
	prompt = e.injectHandoverIfNeeded(ctx, task.ID, sessionID, prompt)

	allRepos, err := e.resolveAllRepoInfoForSession(ctx, task.ID, sessionID)
	if err != nil {
		return nil, err
	}

	// Resolve and validate the environment before the existing-runtime fast
	// path. A persisted executor row can outlive the parent task that owns an
	// inherited environment, so returning through that path without this check
	// would restart an agent in a workspace that is no longer valid.
	existingEnv, err := e.resolveLaunchTaskEnvironment(ctx, task, session)
	if err != nil {
		return nil, err
	}

	// Fast path: workspace already launched (executors_running row exists).
	// Only start the agent subprocess if requested; otherwise return early.
	// If startAgentOnExistingWorkspace returns ErrStaleExecution, the in-memory
	// execution was lost (e.g. backend restart). The full LaunchAgent path below
	// will create a new execution and lifecycle.persistExecutorRunning will
	// overwrite the stale row.
	hasRunning, hasRunningErr := e.repo.HasExecutorRunningRow(ctx, sessionID)
	if hasRunningErr != nil {
		return nil, fmt.Errorf("check runtime inventory for session %q: %w", sessionID, hasRunningErr)
	}
	if hasRunning {
		result, err := e.startAgentOnExistingWorkspace(ctx, task, session, prompt, startAgent, opts.McpMode, opts.Env, opts.TurnID)
		if !errors.Is(err, ErrStaleExecution) && !errors.Is(err, ErrAgentCommandMissing) {
			return result, err
		}
		e.logger.Info("falling through to full LaunchAgent for existing workspace",
			zap.String("task_id", task.ID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	// Primary = first by Position. For repo-less tasks (e.g. quick chat), allRepos
	// is empty and primary is a zero-value placeholder; downstream code already
	// handles the missing-repo path.
	var primaryRepo *repoInfo
	if len(allRepos) > 0 {
		primaryRepo = allRepos[0]
	} else {
		primaryRepo = &repoInfo{}
	}

	assignLaunchTaskEnvironmentID(session, existingEnv)

	// A sibling can be prepared while the elected materializer is still
	// creating the environment (for example, an inherit_parent autopilot child
	// starts immediately after its parent). Wait for that durable owner to
	// publish READY instead of turning a recoverable race into a terminal
	// session failure.
	if existingEnv != nil && existingEnv.Status == models.TaskEnvironmentStatusCreating && existingEnv.MaterializationSessionID != session.ID {
		readyEnv, waitErr := e.waitForTaskEnvironmentReady(ctx, existingEnv.ID)
		if waitErr != nil {
			return nil, waitErr
		}
		existingEnv = readyEnv
	}
	workspaceReuseRequired := existingEnv != nil && existingEnv.MaterializationSessionID != session.ID
	if existingEnv != nil && existingEnv.Status == models.TaskEnvironmentStatusFailed {
		return nil, fmt.Errorf("%w: existing task environment is not attachable", models.ErrWorkspaceReuseUnsafe)
	}
	req, execCfg, err := e.buildLaunchAgentRequest(ctx, task, session, agentProfileID, executorID, prompt, primaryRepo, allRepos, workspaceReuseRequired, existingEnv)
	if err != nil {
		return nil, err
	}
	if execCfg.ExecutorID != "" {
		session.ExecutorID = execCfg.ExecutorID
	}
	req.OfficeAgentProfileID = opts.OfficeAgentProfileID
	req.TurnID = opts.TurnID
	if req.OfficeAgentProfileID == "" && session.AgentProfileID != "" {
		req.OfficeAgentProfileID = session.AgentProfileID
	}
	req.StartAgent = startAgent
	mergeEnv(req, opts.Env)
	if opts.RouteOverride != nil {
		req.RouteOverride = opts.RouteOverride
		if opts.RouteOverride.ExecutionProfileID == "" {
			mergeEnv(req, opts.RouteOverride.Env)
		}
	}

	// Apply McpMode from options (takes precedence over session metadata check in buildLaunchAgentRequest)
	if opts.McpMode != "" {
		req.McpMode = opts.McpMode
	}
	if opts.McpProfile != nil {
		profileContext := *opts.McpProfile
		profileContext.Providers = deriveMCPProviders(allRepos)
		req.McpProfile = &profileContext
	}

	// Carry the prior ACP session id forward so the agent CLI resumes the
	// existing conversation (session/load) instead of opening a fresh one.
	// Reading from executors_running covers both:
	//   - office wakeups where startAgentOnExistingWorkspace returned
	//     ErrStaleExecution (in-memory exec gone after IDLE), and
	//   - kanban / quick-chat re-launches that hit the full path.
	// Unlike ResumeSession we do NOT clear req.TaskDescription — wakeups
	// deliver the new comment / event as the prompt.
	if startAgent && opts.PriorACPSession != "" {
		req.ACPSessionID = opts.PriorACPSession
		e.logger.Info("resuming ACP session via dynamic route state",
			zap.String("task_id", task.ID),
			zap.String("session_id", sessionID),
			zap.String("acp_session_id", opts.PriorACPSession))
	} else if startAgent {
		if token := resumeTokenForExecutionProfile(running, agentProfileID); token != "" {
			req.ACPSessionID = token
			e.logger.Info("resuming ACP session via stored resume token",
				zap.String("task_id", task.ID),
				zap.String("session_id", sessionID),
				zap.String("acp_session_id", token))
		}
	}
	if err := e.validateReuseEnvironmentInventory(ctx, req, existingEnv); err != nil {
		return nil, err
	}

	// Pass attachments for the initial prompt
	if len(opts.Attachments) > 0 {
		req.Attachments = opts.Attachments
	}

	// Check for an existing task environment to reuse worktree, container, or sandbox
	e.reuseExistingEnvironment(ctx, req, existingEnv)

	e.logger.Info("launching agent for prepared session",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
		zap.String("agent_profile_id", agentProfileID),
		zap.String("executor_type", req.ExecutorType),
		zap.Bool("use_worktree", req.UseWorktree))

	if err := e.resolveLaunchEnvironment(ctx, req, execCfg.ProfileEnvVars, allRepos); err != nil {
		return nil, err
	}

	// Call the AgentManager to launch the container
	resp, err := e.agentManager.LaunchAgent(ctx, req)
	if err != nil || resp == nil {
		if err == nil {
			err = errors.New("agent launch returned no response")
		}
		if existingEnv != nil && existingEnv.Status == models.TaskEnvironmentStatusCreating && existingEnv.MaterializationSessionID == session.ID {
			existingEnv.Status = models.TaskEnvironmentStatusFailed
			existingEnv.MaterializationSessionID = ""
			if updateErr := e.repo.UpdateTaskEnvironment(ctx, existingEnv); updateErr != nil {
				e.logger.Warn("failed to mark workspace materialization failed",
					zap.String("task_id", task.ID), zap.String("task_environment_id", existingEnv.ID), zap.Error(updateErr))
			}
		}
		repositoryID, taskRepositoryID := failingLaunchRepositoryIdentity(req, err)
		return nil, e.handleLaunchFailure(ctx, task.ID, sessionID, repositoryID, taskRepositoryID, err)
	}

	// Create or update the task environment with launch results
	if err := e.persistTaskEnvironment(ctx, task.ID, session, existingEnv, req, resp, execCfg); err != nil {
		e.cleanupUnstartedExecutionAfterPersistError(ctx, sessionID, resp.AgentExecutionID, err)
		e.markTaskEnvironmentMaterializationFailed(ctx, existingEnv, session.ID)
		repositoryID, taskRepositoryID := failingLaunchRepositoryIdentity(req, err)
		return nil, e.handleLaunchFailure(ctx, task.ID, sessionID, repositoryID, taskRepositoryID, err)
	}

	// Capture the current HEAD commit as the base commit for this session asynchronously.
	// This allows us to filter git log to only show commits made during the session.
	// We do this async to avoid delaying session launch while waiting for agentctl to be ready.
	// Use a bounded timeout context to prevent blocking indefinitely if agentctl never becomes ready.
	go func(sid string) {
		captureCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		e.captureBaseCommit(captureCtx, sid)
	}(sessionID)

	return e.finalizeLaunch(ctx, task, session, agentProfileID, sessionID, primaryRepo, resp, startAgent, execCfg)
}

func (e *Executor) waitForTaskEnvironmentReady(ctx context.Context, environmentID string) (*models.TaskEnvironment, error) {
	if environmentID == "" || e.repo == nil {
		return nil, fmt.Errorf("%w: task environment is unavailable", models.ErrWorkspaceReuseUnsafe)
	}
	waitCtx, cancel := context.WithTimeout(ctx, constants.AgentLaunchTimeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		env, err := e.repo.GetTaskEnvironment(waitCtx, environmentID)
		if err != nil {
			return nil, fmt.Errorf("%w: load task environment while waiting: %v", models.ErrWorkspaceReuseUnsafe, err)
		}
		if env == nil {
			return nil, fmt.Errorf("%w: task environment disappeared while waiting", models.ErrWorkspaceReuseUnsafe)
		}
		switch env.Status {
		case models.TaskEnvironmentStatusReady, models.TaskEnvironmentStatusStopped:
			return env, nil
		case models.TaskEnvironmentStatusFailed:
			return nil, fmt.Errorf("%w: existing task environment is not attachable", models.ErrWorkspaceReuseUnsafe)
		}

		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("%w: timed out waiting for task environment %s", models.ErrWorkspacePreparing, environmentID)
		case <-ticker.C:
		}
	}
}

// failingLaunchRepositoryID identifies the repository that caused a
// multi-repository branch-fetch failure. A lifecycle launch error only carries
// the failed branch name, so correlate it with the unique per-repository
// checkout branch in the request. Ambiguous or unrecognized branches fail
// closed: no repository-scoped destructive guidance can be offered.
func failingLaunchRepositoryID(req *LaunchAgentRequest, launchErr error) string {
	repositoryID, _ := failingLaunchRepositoryIdentity(req, launchErr)
	return repositoryID
}

func failingLaunchRepositoryIdentity(
	req *LaunchAgentRequest,
	launchErr error,
) (repositoryID, taskRepositoryID string) {
	if req == nil {
		return "", ""
	}
	if len(req.Repositories) == 0 {
		return req.RepositoryID, req.TaskRepositoryID
	}
	var preparationErr *lifecycle.RepositoryPreparationError
	if errors.As(launchErr, &preparationErr) && preparationErr != nil {
		return preparationErr.RepositoryID, preparationErr.TaskRepositoryID
	}

	branch := extractLaunchFailureBranch(launchErr)
	if len(req.Repositories) == 1 && branch == "" {
		return req.Repositories[0].RepositoryID, req.Repositories[0].TaskRepositoryID
	}
	if branch == "" {
		return "", ""
	}
	var matched RepoSpec
	found := false
	for _, spec := range req.Repositories {
		if strings.TrimSpace(spec.CheckoutBranch) != branch &&
			strings.TrimSpace(spec.BaseBranch) != branch {
			continue
		}
		if found {
			return "", ""
		}
		matched = spec
		found = true
	}
	if !found {
		return "", ""
	}
	return matched.RepositoryID, matched.TaskRepositoryID
}

var (
	launchQuotedBranchPattern   = regexp.MustCompile(`branch "([^"]+)"`)
	launchRemoteRefPattern      = regexp.MustCompile(`remote ref ([^\s]+)`)
	launchPathspecBranchPattern = regexp.MustCompile(`pathspec '([^']+)'`)
)

func extractLaunchFailureBranch(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	for _, pattern := range []*regexp.Regexp{
		launchQuotedBranchPattern,
		launchRemoteRefPattern,
		launchPathspecBranchPattern,
	} {
		if match := pattern.FindStringSubmatch(message); len(match) == 2 {
			return strings.TrimSpace(match[1])
		}
	}
	return ""
}

func resumeTokenForExecutionProfile(running *models.ExecutorRunning, profileID string) string {
	if running == nil || profileID == "" ||
		(running.ExecutionProfileID != "" && running.ExecutionProfileID != profileID) {
		return ""
	}
	return running.ResumeToken
}

// handleLaunchFailure marks the session and task as FAILED and returns a
// sanitized error that preserves the original cause for errors.Is/errors.As.
func (e *Executor) handleLaunchFailure(
	ctx context.Context,
	taskID, sessionID, repositoryID, taskRepositoryID string,
	launchErr error,
) error {
	// Detach from caller context so failure bookkeeping completes even if the
	// original request context was cancelled.
	failCtx := context.WithoutCancel(ctx)
	safeErr, changed := e.transitionLaunchFailure(
		failCtx, taskID, sessionID, repositoryID, taskRepositoryID, launchErr,
	)
	if changed {
		if updateErr := e.updateTaskState(failCtx, taskID, v1.TaskStateFailed); updateErr != nil {
			e.logger.Warn("failed to mark task as failed after launch error",
				zap.String("task_id", taskID),
				zap.Error(updateErr))
		}
	}
	return safeErr
}

func (e *Executor) handleEarlyLaunchFailure(
	ctx context.Context,
	taskID, sessionID, repositoryID string,
	launchErr error,
) error {
	failCtx := context.WithoutCancel(ctx)
	safeErr, changed := e.transitionLaunchFailure(
		failCtx, taskID, sessionID, repositoryID, "", launchErr,
	)
	if !changed {
		return safeErr
	}
	if e.onEarlyLaunchTaskStateReconcile != nil {
		if err := e.onEarlyLaunchTaskStateReconcile(
			failCtx, taskID, sessionID, v1.TaskStateFailed,
		); err != nil {
			e.logger.Warn("failed to reconcile task after early launch error",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
		return safeErr
	}
	updater, ok := e.repo.(primarySessionTaskStateStore)
	if !ok {
		// Older repository adapters may not expose the primary-aware extension.
		// Preserve terminal failure behavior while those adapters migrate; the
		// production adapter takes the guarded path above.
		if _, _, err := e.repo.UpdateTaskStateIfNotArchived(
			failCtx, taskID, v1.TaskStateFailed,
		); err != nil {
			e.logger.Warn("failed to mark task after early launch error",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
		return safeErr
	}
	if _, _, err := updater.UpdateTaskStateIfPrimarySessionState(
		failCtx,
		taskID,
		sessionID,
		models.TaskSessionStateFailed,
		v1.TaskStateFailed,
	); err != nil {
		e.logger.Warn("failed to mark task as failed after early launch error",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
	return safeErr
}

func (e *Executor) transitionLaunchFailure(
	failCtx context.Context,
	taskID, sessionID, repositoryID, taskRepositoryID string,
	launchErr error,
) (error, bool) {
	safeErr := routingerr.SanitizeError(launchErr)
	e.logger.Error("failed to launch agent",
		zap.String("task_id", taskID),
		zap.Error(safeErr))
	var onChanged func()
	onChanged = func() {
		e.persistLastAgentError(
			failCtx,
			sessionID,
			e.buildLastAgentError(failCtx, taskID, taskRepositoryID, launchErr),
		)
		if e.onLaunchFailed != nil {
			e.onLaunchFailed(failCtx, taskID, sessionID, repositoryID, safeErr)
		}
	}
	changed, _, updateErr := e.transitionSessionStateWithHook(
		failCtx, taskID, sessionID, models.TaskSessionStateFailed, safeErr.Error(), onChanged,
	)
	if updateErr != nil {
		e.logger.Warn("failed to mark session as failed after launch error",
			zap.String("session_id", sessionID),
			zap.Error(updateErr))
	}
	return safeErr, changed
}

// finalizeLaunch persists launch state and returns the resulting TaskExecution.
func (e *Executor) finalizeLaunch(ctx context.Context, task *v1.Task, session *models.TaskSession, agentProfileID, sessionID string, repoInfo *repoInfo, resp *LaunchAgentResponse, startAgent bool, execCfg executorConfig) (*TaskExecution, error) {
	now := time.Now().UTC()
	if err := e.persistLaunchState(ctx, task.ID, sessionID, session, resp, startAgent, now); err != nil {
		e.cleanupUnstartedExecutionAfterPersistError(ctx, sessionID, resp.AgentExecutionID, err)
		return nil, err
	}

	sessionState := v1.TaskSessionStateCreated
	if startAgent {
		sessionState = v1.TaskSessionStateStarting
	}
	execution := &TaskExecution{
		TaskID:           task.ID,
		AgentExecutionID: resp.AgentExecutionID,
		AgentProfileID:   agentProfileID,
		StartedAt:        session.StartedAt,
		SessionState:     sessionState,
		LastUpdate:       now,
		SessionID:        sessionID,
		WorktreePath:     resp.WorktreePath,
		WorktreeBranch:   resp.WorktreeBranch,
		PrepareResult:    resp.PrepareResult,
	}

	if startAgent {
		e.startAgentProcessAsync(ctx, task.ID, sessionID, resp.AgentExecutionID)
	} else {
		// Prepare-only launch: the workspace + agentctl are up but the agent
		// process is intentionally not being started. The lifecycle manager
		// writes an active runtime status on row creation; flip it to
		// 'prepared' so the row doesn't look like an agent process is running.
		// When the user later starts the agent (StartCreatedSession), Launch
		// re-runs and rewrites the row with the active runtime status via the
		// usual path.
		//
		// Detach from the caller context so a client disconnect / WS timeout
		// right after launch returns can't drop this write — that would leave
		// the row stuck on "starting", which is the exact UX this fix closes.
		statusCtx := context.WithoutCancel(ctx)
		if err := e.repo.UpdateExecutorRunningStatus(statusCtx, sessionID, models.ExecutorRunningStatusPrepared); err != nil {
			e.logger.Warn("failed to mark executors_running as prepared",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}

	e.logger.Info("agent launched for prepared session",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
		zap.String("agent_execution_id", resp.AgentExecutionID))

	return execution, nil
}

func bindSessionToTaskEnvironment(session *models.TaskSession, env *models.TaskEnvironment) {
	if session == nil || env == nil {
		return
	}
	session.TaskEnvironmentID = env.ID
	session.WorkspacePath = env.WorkspacePath
}

// describeInheritedEnvironmentUnavailable builds a diagnostic reason for a
// missing inherited task_environments row (session.TaskEnvironmentID no
// longer resolves). For an inherit_parent task it names the parent and, when
// the parent was archived, calls that out explicitly — archive tears down
// the parent's runtime resources (worktree, container/sandbox) but
// preserves its own task_environments row, and leaves the parent's
// session.TaskEnvironmentID pointer in place either way, so a child that
// inherited it only finds out here. shared_group members have no single
// parent to name, so they get a generic message instead.
func (e *Executor) describeInheritedEnvironmentUnavailable(ctx context.Context, task *v1.Task) string {
	if task == nil || task.ParentID == "" {
		return "inherited task environment is unavailable: shared workspace group environment could not be resolved"
	}
	parent, _ := e.repo.GetTask(ctx, task.ParentID)
	return models.DescribeInheritedEnvironmentUnavailable(task.ParentID, parent)
}

func assignLaunchTaskEnvironmentID(session *models.TaskSession, existingEnv *models.TaskEnvironment) {
	if existingEnv != nil && existingEnv.ID != "" {
		session.TaskEnvironmentID = existingEnv.ID
		return
	}
	if session.TaskEnvironmentID == "" {
		session.TaskEnvironmentID = uuid.New().String()
	}
}

// buildLaunchAgentRequest constructs a LaunchAgentRequest for a new session launch,
// applying executor config, repository/worktree settings, and remote docker URL as needed.
// allRepos carries every repository for the task in Position order; for single-repo
// or repo-less tasks it has length <=1 and the legacy single-repo path runs unchanged.
func (e *Executor) buildLaunchAgentRequest(ctx context.Context, task *v1.Task, session *models.TaskSession, agentProfileID, executorID, prompt string, repoInfo *repoInfo, allRepos []*repoInfo, workspaceReuseRequired bool, existingEnv *models.TaskEnvironment) (*LaunchAgentRequest, executorConfig, error) {
	metadata := cloneMetadata(task.Metadata)
	if session.ExecutorProfileID != "" {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["executor_profile_id"] = session.ExecutorProfileID
	}
	sessionID := session.ID
	req := &LaunchAgentRequest{
		TaskID:                 task.ID,
		WorkspaceID:            task.WorkspaceID,
		TaskTitle:              task.Title,
		AgentProfileID:         agentProfileID,
		TaskDescription:        prompt,
		Priority:               task.Priority,
		SessionID:              sessionID,
		TaskEnvironmentID:      session.TaskEnvironmentID,
		IsEphemeral:            task.IsEphemeral,
		IsPassthrough:          session.IsPassthrough,
		WorkspacePath:          session.WorkspacePath,
		WorkspaceReuseRequired: workspaceReuseRequired,
		McpProviders:           deriveMCPProviders(allRepos),
	}

	execConfig := e.resolveExecutorConfig(ctx, executorID, task.WorkspaceID, metadata)
	if execConfig.ExecutorID != "" {
		metadata = execConfig.Metadata
		req.ExecutorType = execConfig.ExecutorType
		req.ExecutorConfig = execConfig.ExecutorCfg
		req.SetupScript = execConfig.SetupScript
	}
	// A task environment is reusable only by the executor type that owns it.
	// If the caller selected a different profile, this launch must provision
	// the new backend instead of attaching to the old environment. Resolve this
	// before repository configuration so clone URL requirements are applied to
	// the fresh launch as well.
	repoBacked := e.taskIsRepoBacked(ctx, task.ID)
	workspaceReuseRequired = workspaceReuseAllowed(existingEnv, req.ExecutorType, workspaceReuseRequired, repoBacked)
	req.WorkspaceReuseRequired = workspaceReuseRequired

	// For remote executors (containerized *and* SSH), resolve only explicitly
	// selected profile auth secrets. Workspace GitHub automation is configured
	// below through a scoped renewable broker lease.
	// SSH is included so env-authenticated agents (e.g. claude-acp reading
	// CLAUDE_CODE_OAUTH_TOKEN) and remote git get their credentials from the
	// configured profile/secret store rather than from a blanket forward of the
	// control-plane process env — the SSH executor only forwards req.Env keys.
	if executorNeedsResolvedCredentials(execConfig.ExecutorType) {
		e.applyContainerCredentials(ctx, req, metadata)
	}
	e.injectGitLabWorkspaceCredentials(ctx, req)
	req.WorktreeBranchTicket = worktree.TicketForBranchName(task.Identifier, metadata)

	metadata, err := e.applyRepositoryConfig(req, task, repoInfo, execConfig, metadata)
	if err != nil {
		return nil, execConfig, err
	}
	if err := e.configureGitCredentialBrokerForRepositories(ctx, req, allRepos); err != nil {
		return nil, execConfig, err
	}
	if err := e.applyGitCredentialSnapshot(ctx, req, session); err != nil {
		return nil, execConfig, err
	}

	// Multi-repo: when more than one repository is associated with the task,
	// populate req.Repositories so the lifecycle preparer creates one worktree
	// per repo. The legacy single-repo top-level fields above stay populated
	// (mirroring the primary) for downstream code that has not been migrated.
	if len(allRepos) > 1 {
		req.Repositories = buildRepoSpecs(allRepos)
		for i := range req.Repositories {
			req.Repositories[i].WorktreeBranchTicket = req.WorktreeBranchTicket
		}
	}
	if folders, folderErr := e.repo.ListTaskWorkspaceFolders(ctx, task.ID); folderErr != nil {
		return nil, execConfig, folderErr
	} else {
		for _, f := range folders {
			if f != nil {
				req.WorkspaceFolders = append(req.WorkspaceFolders, WorkspaceFolderSpec{Name: f.DisplayName, LocalPath: f.LocalPath})
			}
		}
	}

	// Activate config-mode MCP tools when config_mode is set in session metadata.
	if isConfigModeSession(session) {
		req.McpMode = McpModeConfig
	}

	if len(metadata) > 0 {
		req.Metadata = metadata
	}

	return req, execConfig, nil
}

func workspaceReuseAllowed(existingEnv *models.TaskEnvironment, requestedExecutorType string, required, repoBacked bool) bool {
	if !required || existingEnv == nil {
		return required
	}
	if existingEnv.ExecutorType != "" && existingEnv.ExecutorType != requestedExecutorType {
		return false
	}
	if repoBacked && len(existingEnv.Repos) == 0 {
		return false
	}
	if requestedExecutorType == string(models.ExecutorTypeWorktree) {
		return hasLiveWorktreeRepo(existingEnv)
	}
	return true
}

func hasLiveWorktreeRepo(env *models.TaskEnvironment) bool {
	for _, repo := range env.Repos {
		if repo == nil || repo.WorktreeID == "" || repo.DeletedAt != nil || repo.Status == taskEnvironmentRepoStatusFailed || repo.Status == taskEnvironmentRepoStatusDeleted {
			continue
		}
		return true
	}
	return false
}

func mergeEnv(req *LaunchAgentRequest, env map[string]string) {
	if len(env) == 0 {
		return
	}
	if req.Env == nil {
		req.Env = make(map[string]string, len(env))
	}
	for k, v := range env {
		req.Env[k] = v
	}
}

// applyContainerCredentials resolves explicit profile credentials for remote executors.
func (e *Executor) applyContainerCredentials(ctx context.Context, req *LaunchAgentRequest, metadata map[string]interface{}) {
	e.resolveRemoteCredentials(ctx, req, metadata)
}

// buildRepoSpecs converts resolved repoInfos into per-repo launch specs for
// the lifecycle layer. Used only when the task has more than one repository.
// When the same RepositoryID appears more than once, each row gets a stable
// BranchIdentitySlug for reuse while the lowest-position branch keeps the flat
// layout (<task>/<repo>/). Other branches use sibling directories like
// <task>/<repo>-<branch-slug>/.
// This preserves the legacy single-branch path when a task later gains another
// branch of the same repository.
func buildRepoSpecs(allRepos []*repoInfo) []RepoSpec {
	branchPlans := buildRepoBranchPlans(allRepos)
	out := make([]RepoSpec, 0, len(allRepos))
	for _, info := range allRepos {
		spec := RepoSpec{
			TaskRepositoryID:           info.TaskRepositoryID,
			RepositoryID:               info.RepositoryID,
			RepositoryPath:             info.RepositoryPath,
			BaseBranch:                 info.BaseBranch,
			CheckoutBranch:             info.CheckoutBranch,
			PRNumber:                   info.PRNumber,
			RemoteContribution:         info.RemoteContribution,
			ContributionDestination:    info.ContributionDestination,
			ComparisonTarget:           info.ComparisonTarget,
			WorktreeBranchPrefix:       info.WorktreeBranchPrefix,
			WorktreeBranchTemplate:     info.WorktreeBranchTemplate,
			PullBeforeWorktree:         info.PullBeforeWorktree,
			RemoteSyncHandled:          info.RemoteSyncHandled,
			RefreshRepository:          info.RefreshRepository,
			RefreshRepositoryWithState: info.RefreshRepositoryWithState,
			RemoteRefState:             info.RemoteRefState,
		}
		if info.Repository != nil {
			spec.RepoName = info.Repository.Name
			spec.RepoSetupScript = info.Repository.SetupScript
			spec.RepoCleanupScript = info.Repository.CleanupScript
			spec.DefaultBranch = info.Repository.DefaultBranch
			spec.CopyFiles = info.Repository.CopyFiles
		}
		// Containerized executors need a clone URL; reuse the same helper as
		// the single-repo path (best-effort — skipped if Repository is nil).
		if info.Repository != nil {
			if u := repositoryCloneURL(info.Repository); u != "" {
				spec.RepositoryURL = u
			}
		}
		if plan, ok := branchPlans[info]; ok {
			spec.BranchIdentitySlug = plan.identitySlug
			spec.BranchSlug = plan.pathSlug
		}
		out = append(out, spec)
	}
	return out
}

type repoBranchPlan struct {
	identitySlug string
	pathSlug     string
}

func buildRepoBranchPlans(allRepos []*repoInfo) map[*repoInfo]repoBranchPlan {
	plans := make(map[*repoInfo]repoBranchPlan, len(allRepos))
	inputs := make([]worktree.BranchIdentityInput, 0, len(allRepos))
	infos := make([]*repoInfo, 0, len(allRepos))
	for _, info := range allRepos {
		if info == nil || info.RepositoryID == "" {
			continue
		}
		defaultBranch := ""
		if info.Repository != nil {
			defaultBranch = info.Repository.DefaultBranch
		}
		inputs = append(inputs, worktree.BranchIdentityInput{
			RepositoryID: info.RepositoryID, BaseBranch: info.BaseBranch, CheckoutBranch: info.CheckoutBranch,
			DefaultBranch: defaultBranch, PRNumber: info.PRNumber, Position: info.Position,
		})
		infos = append(infos, info)
	}
	for index, plan := range worktree.BuildBranchIdentityPlans(inputs) {
		plans[infos[index]] = repoBranchPlan{identitySlug: plan.IdentitySlug, pathSlug: plan.PathSlug}
	}
	return plans
}

// applyRepositoryConfig sets repository-related fields on the request and resolves clone URLs.
func (e *Executor) applyRepositoryConfig(req *LaunchAgentRequest, task *v1.Task, repoInfo *repoInfo, execConfig executorConfig, metadata map[string]interface{}) (map[string]interface{}, error) {
	if repoInfo.RepositoryPath != "" {
		req.UseWorktree = shouldUseWorktree(execConfig.ExecutorType)
		req.RepositoryID = repoInfo.RepositoryID
		req.TaskRepositoryID = repoInfo.TaskRepositoryID
		req.RepositoryPath = repoInfo.RepositoryPath
		req.BaseBranch = repoInfo.BaseBranch
		req.CheckoutBranch = repoInfo.CheckoutBranch
		req.PRNumber = repoInfo.PRNumber
		req.RemoteContribution = repoInfo.RemoteContribution
		req.ContributionDestination = repoInfo.ContributionDestination
		req.ComparisonTarget = repoInfo.ComparisonTarget
		req.WorktreeBranchPrefix = repoInfo.WorktreeBranchPrefix
		req.WorktreeBranchTemplate = repoInfo.WorktreeBranchTemplate
		req.PullBeforeWorktree = repoInfo.PullBeforeWorktree
		req.RemoteSyncHandled = repoInfo.RemoteSyncHandled
		req.RefreshRepository = repoInfo.RefreshRepository
		req.RefreshRepositoryWithState = repoInfo.RefreshRepositoryWithState
		req.RemoteRefState = repoInfo.RemoteRefState
		if repoInfo.Repository != nil {
			req.DefaultBranch = repoInfo.Repository.DefaultBranch
			if req.UseWorktree {
				req.RepoName = repoInfo.Repository.Name
			} else {
				req.RepoName = worktree.SanitizeRepoDirName(repoInfo.Repository.Name)
				if req.RepoName == "" {
					req.RepoName = worktree.SanitizeRepoDirName(repoInfo.RepositoryID)
				}
			}
		}
		// Task directory mode: place worktree inside per-task directory
		if req.UseWorktree && repoInfo.Repository != nil && repoInfo.Repository.Name != "" {
			req.TaskDirName = worktree.SemanticWorktreeName(task.Title, worktree.TaskDirSuffix(task.ID))
		}
		if repoInfo.Repository != nil && repoInfo.Repository.SetupScript != "" {
			if metadata == nil {
				metadata = make(map[string]interface{})
			}
			metadata[lifecycle.MetadataKeyRepoSetupScript] = repoInfo.Repository.SetupScript
		}
		if repoInfo.Repository != nil {
			req.CopyFiles = repoInfo.Repository.CopyFiles
		}
	}

	// Remote executors need a clone URL since the remote host has no access to the local filesystem.
	if !req.WorkspaceReuseRequired && e.capabilities != nil && e.capabilities.RequiresCloneURL(execConfig.ExecutorType) && repoInfo.Repository != nil {
		cloneURL := repositoryCloneURL(repoInfo.Repository)
		// Local Docker can bind-mount a task's checked-out source directory and
		// clone it there. RepositoryPath is authoritative for that launch even
		// when the persisted generic repository has no provider identity or
		// origin URL (as in workspace-source and E2E fixtures).
		if cloneURL == "" && execConfig.ExecutorType == string(models.ExecutorTypeLocalDocker) {
			cloneURL = dockerLocalCloneSource(repoInfo.RepositoryPath)
		}
		if cloneURL == "" {
			return metadata, ErrNoCloneURL
		}
		req.RepositoryURL = cloneURL
		// Surface the clone URL to the script engine so {{repository.clone_url}}
		// resolves in prepare scripts even when no host repo path is mounted.
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["repository_clone_url"] = cloneURL
	}

	return metadata, nil
}

// dockerLocalCloneSource returns a host directory suitable for Docker's
// read-only bind mount. A repository path can also be a remote URL while a
// workspace-source attachment is in progress; treating a missing or non-local
// path as a local source delegates an opaque failure to Docker instead of
// returning the recoverable clone-source error.
func dockerLocalCloneSource(repositoryPath string) string {
	source := strings.TrimSpace(repositoryPath)
	if source == "" {
		return ""
	}
	info, err := os.Stat(source)
	if err != nil || !info.IsDir() {
		return ""
	}
	return source
}

// startAgentOnExistingWorkspace handles the case where LaunchPreparedSession is called on a session
// whose workspace (agentctl) was already launched. It optionally starts just the agent subprocess.
//
// The in-memory ExecutionStore is the single source of truth here: if no execution
// exists for this session in the store, the workspace is gone (or was never
// created in this process — e.g. after restart) and the caller must take the full
// re-launch path. Pre-refactor this also consulted session.AgentExecutionID and
// reconciled DB drift; that's now structurally impossible because executors_running
// is owned by the lifecycle manager and writes are atomic with executionStore.Add.
func (e *Executor) startAgentOnExistingWorkspace(ctx context.Context, task *v1.Task, session *models.TaskSession, prompt string, startAgent bool, mcpMode string, env map[string]string, turnIDs ...string) (*TaskExecution, error) {
	executionID, err := e.agentManager.GetExecutionIDForSession(ctx, session.ID)
	if err != nil || executionID == "" {
		// No execution exists in memory (e.g. backend restarted since workspace was prepared).
		// Return ErrStaleExecution so the caller falls through to the full LaunchAgent path,
		// which creates a complete execution with agent commands, worktree, and all required
		// configuration. The lifecycle manager will overwrite any pre-existing executors_running
		// row when it runs persistExecutorRunning, so we don't pre-clean here.
		e.logger.Info("no in-memory execution for session, falling through to full re-launch",
			zap.String("session_id", session.ID))
		return nil, ErrStaleExecution
	}

	if !startAgent {
		// Workspace already launched, nothing else to do
		now := time.Now().UTC()
		return &TaskExecution{
			TaskID:           task.ID,
			AgentExecutionID: executionID,
			AgentProfileID:   session.AgentProfileID,
			StartedAt:        session.StartedAt,
			SessionState:     v1.TaskSessionState(session.State),
			LastUpdate:       now,
			SessionID:        session.ID,
		}, nil
	}

	// Update the task description in the existing execution so StartAgentProcess picks it up
	if prompt != "" {
		if err := e.agentManager.SetExecutionDescription(ctx, executionID, prompt); err != nil {
			e.logger.Warn("failed to set execution description for existing workspace",
				zap.String("session_id", session.ID),
				zap.String("agent_execution_id", executionID),
				zap.Error(err))
			// Non-fatal: agent may start without description
		}
	}
	e.bindPromptTurnID(ctx, session.ID, executionID, turnIDs)
	if err := e.configureExistingWorkspace(ctx, task, session, executionID, mcpMode, env); err != nil {
		return nil, err
	}

	// Lazy workspace restoration creates an execution without an agent command.
	// Preserve the request's description, environment, and MCP mode above, then
	// route it through LaunchAgent so lifecycle.Launch can promote the execution
	// with the effective profile, model, route override, and CLI flags before the
	// subprocess is started.
	if !e.agentManager.IsAgentCommandConfigured(executionID) {
		return nil, ErrAgentCommandMissing
	}

	// Transition session to STARTING
	expectedState := session.State
	now := time.Now().UTC()
	session.State = models.TaskSessionStateStarting
	session.ErrorMessage = ""
	session.UpdatedAt = now
	if err := e.updateSessionStarting(ctx, task.ID, session, expectedState, true); err != nil {
		e.logger.Error("failed to update session state for agent start",
			zap.String("session_id", session.ID),
			zap.Error(err))
		return nil, err
	}

	execution := &TaskExecution{
		TaskID:           task.ID,
		AgentExecutionID: executionID,
		AgentProfileID:   session.AgentProfileID,
		StartedAt:        now,
		SessionState:     v1.TaskSessionStateStarting,
		LastUpdate:       now,
		SessionID:        session.ID,
	}

	// Start the agent process asynchronously
	e.startAgentProcessAsync(ctx, task.ID, session.ID, executionID)

	e.logger.Info("agent starting on existing workspace",
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.String("agent_execution_id", executionID))

	return execution, nil
}

func (e *Executor) configureExistingWorkspace(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	executionID, mcpMode string,
	env map[string]string,
) error {
	credentialReq := &LaunchAgentRequest{WorkspaceID: task.WorkspaceID, Env: cloneStringMap(env)}
	e.injectGitLabWorkspaceCredentials(ctx, credentialReq)
	if len(credentialReq.Env) > 0 {
		if err := e.agentManager.SetExecutionEnv(ctx, executionID, credentialReq.Env); err != nil {
			e.logger.Warn("failed to set execution env for existing workspace",
				zap.String("session_id", session.ID),
				zap.String("agent_execution_id", executionID),
				zap.Error(err))
		}
	}

	// If config MCP mode is needed, reconfigure the MCP server before starting the agent.
	// The workspace may have been prepared before config_mode was set on the session.
	effectiveMcpMode := mcpMode
	if effectiveMcpMode == "" && isConfigModeSession(session) {
		effectiveMcpMode = McpModeConfig
	}
	if effectiveMcpMode == "" {
		return nil
	}
	if err := e.agentManager.SetMcpMode(ctx, executionID, effectiveMcpMode); err != nil {
		e.logger.Error("failed to set MCP mode for existing workspace",
			zap.String("session_id", session.ID),
			zap.String("agent_execution_id", executionID),
			zap.String("mcp_mode", effectiveMcpMode),
			zap.Error(err))
		return fmt.Errorf("set MCP mode %q: %w", effectiveMcpMode, err)
	}
	return nil
}

func (e *Executor) bindPromptTurnID(ctx context.Context, sessionID, executionID string, turnIDs []string) {
	if len(turnIDs) == 0 || turnIDs[0] == "" {
		return
	}
	setter, ok := e.agentManager.(PromptTurnIDSetter)
	if !ok {
		return
	}
	turnID := turnIDs[0]
	if err := setter.SetPromptTurnID(ctx, executionID, turnID); err != nil {
		e.logger.Warn("failed to bind prompt turn for existing workspace",
			zap.String("session_id", sessionID),
			zap.String("agent_execution_id", executionID),
			zap.String("turn_id", turnID),
			zap.Error(err))
	}
}

// captureBaseCommit retrieves the merge-base commit from agentctl and stores it
// as the base commit for the session. This allows calculating cumulative diffs
// that show all changes on the branch relative to the target branch (e.g., main).
func (e *Executor) captureBaseCommit(ctx context.Context, sessionID string) {
	// Wait for agentctl to be ready before trying to get git status.
	// LaunchAgent returns before agentctl is fully ready (waits in goroutine),
	// so we need to explicitly wait here.
	if err := e.agentManager.WaitForAgentctlReady(ctx, sessionID); err != nil {
		e.logger.Warn("agentctl not ready for base commit capture",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	status, err := e.agentManager.GetGitStatus(ctx, sessionID)
	if err != nil {
		e.logger.Warn("failed to get git status for base commit capture",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	// GetGitStatus returns (nil, nil) when the execution or agentctl client
	// has been torn down (e.g. the task was deleted between LaunchAgent and
	// this async capture). Skip silently — there's nothing to record.
	if status == nil {
		return
	}

	// Prefer BaseCommit (merge-base with target branch) over HeadCommit.
	// BaseCommit gives us the common ancestor with main/origin, which is correct
	// for showing all changes on the feature branch. HeadCommit would only show
	// changes made after the session started, missing commits already on the branch.
	baseCommit := status.BaseCommit
	if baseCommit == "" {
		// Fallback to HeadCommit if no merge-base is available (e.g., detached HEAD)
		baseCommit = status.HeadCommit
	}
	if baseCommit == "" {
		e.logger.Debug("no base commit available for capture",
			zap.String("session_id", sessionID))
		return
	}

	// Update the session's base commit in the database
	if err := e.repo.UpdateTaskSessionBaseCommit(ctx, sessionID, baseCommit); err != nil {
		e.logger.Warn("failed to update session base commit",
			zap.String("session_id", sessionID),
			zap.String("base_commit", baseCommit),
			zap.Error(err))
		return
	}

	e.logger.Info("captured base commit for session",
		zap.String("session_id", sessionID),
		zap.String("base_commit", baseCommit),
		zap.String("head_commit", status.HeadCommit))
}

// injectHandoverIfNeeded prepends session handover context to the prompt when the task
// already has previous sessions. The context includes the session count and the task plan
// (if one exists) so the new agent avoids repeating already-completed work.
func (e *Executor) injectHandoverIfNeeded(ctx context.Context, taskID, currentSessionID, prompt string) string {
	sessions, err := e.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		e.logger.Warn("failed to list sessions for handover context",
			zap.String("task_id", taskID),
			zap.Error(err))
		return prompt
	}

	// Count previous sessions (exclude the current one being launched).
	var previousCount int
	for _, s := range sessions {
		if s.ID != currentSessionID {
			previousCount++
		}
	}
	if previousCount == 0 {
		return prompt
	}

	// Build the plan section if a plan exists.
	var planSection string
	plan, err := e.repo.GetTaskPlan(ctx, taskID)
	if err == nil && plan != nil && plan.Content != "" {
		planSection = fmt.Sprintf("\nThe task has an implementation plan:\n\n%s\n", plan.Content)
	}

	e.logger.Info("injecting session handover context",
		zap.String("task_id", taskID),
		zap.String("session_id", currentSessionID),
		zap.Int("previous_sessions", previousCount))

	return sysprompt.InjectSessionHandover(previousCount, planSection, prompt)
}

// computeWorkspacePath derives the env's workspace_path from the launch
// request and response. The value must mirror the agent process cwd
// (cfg.WorkDir set from execution.WorkspacePath in utility.go) so that ACP
// session/load on cold start finds the jsonl saved under the same
// sanitized-cwd folder on hot start. Collapsing single-repo worktree paths
// to the task root via filepath.Dir would diverge hot vs cold cwd and break
// resume with -32002 Resource not found.
//
// resp.WorkspacePath is the lifecycle execution's actual working directory and
// is therefore authoritative. WorktreePath remains a compatibility fallback
// for lifecycle responses that only expose the preparer's legacy metadata.
func computeWorkspacePath(req *LaunchAgentRequest, resp *LaunchAgentResponse) string {
	if resp.WorkspacePath != "" {
		return resp.WorkspacePath
	}
	if resp.WorktreePath != "" {
		return resp.WorktreePath
	}
	return req.RepositoryPath
}

// persistTaskEnvironment creates or updates the task environment record after a successful launch.
// It also links the session to the environment via TaskEnvironmentID. For
// multi-repo launches it additionally writes one TaskEnvironmentRepo row per repo.
//
// Serialised per-task: concurrent launches for the same task previously raced
// here (each saw existingEnv == nil, each created a new row, both succeeded
// before the unique index existed). Hold the per-task lock and re-fetch the
// existing env inside the critical section so siblings reuse the row the
// first one persisted.
func (e *Executor) persistTaskEnvironment(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	existingEnv *models.TaskEnvironment,
	req *LaunchAgentRequest,
	resp *LaunchAgentResponse,
	execCfg executorConfig,
) error {
	mu := e.taskEnvLock(taskID)
	mu.Lock()
	defer mu.Unlock()

	// Re-fetch under the lock — a sibling launch for the same task may have
	// just created the env and released the lock. Without this we'd still
	// see existingEnv == nil from the original call and try to create a
	// duplicate.
	if existingEnv == nil {
		fresh, err := e.repo.GetTaskEnvironmentByTaskID(ctx, taskID)
		if err != nil {
			return fmt.Errorf("load task environment before persistence: %w", err)
		}
		if fresh != nil {
			existingEnv = fresh
		}
	}

	workspacePath := computeWorkspacePath(req, resp)

	if existingEnv != nil {
		materializationSessionID := existingEnv.MaterializationSessionID
		isInitialMaterializer := existingEnv.Status == models.TaskEnvironmentStatusCreating && materializationSessionID == session.ID
		// A session attaching to an environment owned by a *different* task is a
		// guest: office inherit_parent / shared_group bind every member session
		// to one canonical environment (see handoff_inheritance.go), and the
		// members share the owner's worktree rather than owning any
		// task_environment_repos rows of their own. The owner already
		// materialized (or will materialize) that inventory, so a guest must not
		// rewrite the owner's repo rows or re-evaluate its ready status. Doing so
		// on resume tripped "ready status requires repository inventory": the
		// guest's request carries no repo specs (repos empty) and, when the
		// owner's canonical inventory is empty, the repo-backed guard below failed
		// a resume the guest has no authority to fix. The one exception is a guest
		// session elected to materialize a still-CREATING canonical environment
		// (shared_group), which must run the normal finalize path below.
		if existingEnv.TaskID != "" && existingEnv.TaskID != taskID && !isInitialMaterializer {
			bindSessionToTaskEnvironment(session, existingEnv)
			return nil
		}
		previousStatus := existingEnv.Status
		previousMaterializationSessionID := existingEnv.MaterializationSessionID
		repos := environmentReposForLaunch(req, resp)
		// agent_execution_id is no longer stored on task_environments — the column
		// is being dropped (executors_running is the single source of truth).
		// Status, workspace, and container fields are still env-row-owned; the
		// physical worktree lives on task_environment_repos. Status is decided
		// per branch below rather than forced here: the initial materializer
		// always transitions to ready (its inventory publishes atomically in
		// the same call), but a non-materializing sibling must not force ready
		// onto an environment whose canonical inventory turned out empty for a
		// repo-backed task — that permanently bricks reuse (see
		// validateReuseEnvironmentInventory).
		// Refresh workspace + container/sandbox fields. The original update
		// branch only touched AgentExecutionID/Status, so envs created with
		// empty paths (e.g. before the worktree resolved) stayed permanently
		// broken. Sandbox ID gets refreshed too in case a fallback created a
		// new sprite.
		if workspacePath != "" {
			existingEnv.WorkspacePath = workspacePath
		}
		if resp.ContainerID != "" {
			existingEnv.ContainerID = resp.ContainerID
		}
		if bootstrapSecretID := extractContainerBootstrapNonceSecretID(resp.Metadata); bootstrapSecretID != "" {
			existingEnv.ContainerBootstrapNonceSecretID = bootstrapSecretID
		}
		if controlSecretID := extractContainerControlAuthTokenSecretID(resp.Metadata); controlSecretID != "" {
			existingEnv.ContainerControlAuthTokenSecretID = controlSecretID
		}
		if sandboxID := extractSandboxID(resp.Metadata); sandboxID != "" {
			existingEnv.SandboxID = sandboxID
		}
		// Refresh TaskDirName when the request carries a new value — covers
		// resume-after-failure where the original env row was stamped with an
		// empty task_dir_name and the resume regenerates it.
		if req.TaskDirName != "" {
			existingEnv.TaskDirName = req.TaskDirName
		}
		if isInitialMaterializer {
			// The initial materializer must publish its full repository inventory
			// in the same transaction as the ready transition. Otherwise a sibling
			// can bind to a ready environment whose canonical rows are incomplete.
			if finalizer, ok := e.repo.(taskEnvironmentMaterializationFinalizer); ok {
				existingEnv.Status = models.TaskEnvironmentStatusReady
				existingEnv.MaterializationSessionID = ""
				if err := finalizer.FinalizeTaskEnvironmentMaterialization(ctx, existingEnv, repos, materializationSessionID); err != nil {
					existingEnv.Status = previousStatus
					existingEnv.MaterializationSessionID = previousMaterializationSessionID
					e.logger.Warn("failed to finalize task environment materialization",
						zap.String("task_id", taskID), zap.String("env_id", existingEnv.ID), zap.Error(err))
					return fmt.Errorf("finalize task environment materialization: %w", err)
				}
				bindSessionToTaskEnvironment(session, existingEnv)
				e.selfHealTaskRepositoryBaseBranches(ctx, taskID, req, resp)
				return nil
			}
		}
		// Persist per-repo rows for launches that didn't have them yet *before*
		// the environment can be marked ready below. The environment-repository
		// rows are the only physical-worktree record, so single-repo launches
		// write one row here too. Ordering matters: writing status=ready first
		// and these rows second leaves a window where a crash or a concurrent
		// reader observes a ready environment with an empty inventory for a
		// repo-backed task — exactly the state the guards elsewhere in this PR
		// exist to prevent (see validateReuseEnvironmentInventory).
		if err := e.persistTaskEnvironmentRepos(ctx, existingEnv.ID, repos); err != nil {
			existingEnv.Status = previousStatus
			existingEnv.MaterializationSessionID = previousMaterializationSessionID
			return err
		}
		repoBacked := e.taskIsRepoBacked(ctx, taskID)
		// A sibling can finish while the initial materializer still owns a
		// CREATING environment. In that case the owner remains responsible for
		// publishing (or failing) the inventory, so leave the claim untouched
		// instead of turning the sibling's empty result into a launch error. Once
		// the claim is no longer CREATING, an empty repo-backed inventory is a
		// persistence failure and must fail the launch rather than publish an
		// unusable READY/STOPPED environment.
		if len(repos) == 0 && len(existingEnv.Repos) == 0 && repoBacked && previousStatus != models.TaskEnvironmentStatusCreating {
			existingEnv.Status = previousStatus
			existingEnv.MaterializationSessionID = previousMaterializationSessionID
			return fmt.Errorf("persist task environment: ready status requires repository inventory")
		}
		// A non-materializing sibling only advances the environment to ready
		// when inventory is present: already recorded before this launch,
		// about to be written by this launch, or not required because the
		// task has no configured repositories at all. A launch whose prepare
		// step failed produces an empty repos slice here — leaving the
		// environment's existing status untouched (it can only already be
		// ready or stopped by the time this branch runs) keeps that launch
		// from bricking future reuse instead of silently corrupting the row.
		if len(repos) > 0 || len(existingEnv.Repos) > 0 || !repoBacked {
			existingEnv.Status = models.TaskEnvironmentStatusReady
		}
		existingEnv.MaterializationSessionID = ""
		if err := e.repo.UpdateTaskEnvironment(ctx, existingEnv); err != nil {
			existingEnv.Status = previousStatus
			existingEnv.MaterializationSessionID = previousMaterializationSessionID
			e.logger.Warn("failed to update task environment",
				zap.String("task_id", taskID),
				zap.String("env_id", existingEnv.ID),
				zap.Error(err))
			return fmt.Errorf("update task environment: %w", err)
		}
		bindSessionToTaskEnvironment(session, existingEnv)
		e.selfHealTaskRepositoryBaseBranches(ctx, taskID, req, resp)
		return nil
	}

	env := &models.TaskEnvironment{
		ID:                session.TaskEnvironmentID,
		TaskID:            taskID,
		ExecutorType:      req.ExecutorType,
		ExecutorID:        execCfg.ExecutorID,
		ExecutorProfileID: session.ExecutorProfileID,
		// AgentExecutionID is intentionally not set here — see executors_running
		// for the active execution per session.
		Status:                            models.TaskEnvironmentStatusReady,
		WorkspacePath:                     workspacePath,
		ContainerID:                       resp.ContainerID,
		ContainerBootstrapNonceSecretID:   extractContainerBootstrapNonceSecretID(resp.Metadata),
		ContainerControlAuthTokenSecretID: extractContainerControlAuthTokenSecretID(resp.Metadata),
		TaskDirName:                       req.TaskDirName,
		SandboxID:                         extractSandboxID(resp.Metadata),
	}
	// Embed per-repo rows in the same create transaction. Single-repo
	// launches produce one row so the worktree identity is always recorded.
	env.Repos = environmentReposForLaunch(req, resp)
	if err := e.repo.CreateTaskEnvironment(ctx, env); err != nil {
		e.logger.Warn("failed to create task environment",
			zap.String("task_id", taskID),
			zap.Error(err))
		return fmt.Errorf("create task environment: %w", err)
	}
	bindSessionToTaskEnvironment(session, env)
	e.selfHealTaskRepositoryBaseBranches(ctx, taskID, req, resp)
	return nil
}

// taskIsRepoBacked reports whether a task has any configured repositories.
// Fails closed on a read error (treats the task as repo-backed) so a
// transient failure cannot mask the ready-with-empty-inventory bug the
// persistTaskEnvironment status guard exists to prevent.
func (e *Executor) taskIsRepoBacked(ctx context.Context, taskID string) bool {
	if e.repo == nil {
		return false
	}
	repos, err := e.repo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		e.logger.Warn("failed to check task repositories for environment ready-status guard",
			zap.String("task_id", taskID), zap.Error(err))
		return true
	}
	return len(repos) > 0
}

func (e *Executor) selfHealTaskRepositoryBaseBranches(
	ctx context.Context,
	taskID string,
	req *LaunchAgentRequest,
	resp *LaunchAgentResponse,
) {
	if e.taskRepositoryBaseBranchUpdater == nil || req == nil || resp == nil {
		return
	}
	if len(resp.Worktrees) > 0 {
		for _, result := range resp.Worktrees {
			e.persistRecoveredBaseBranch(ctx, taskID, result.TaskRepositoryID, result.RequestedBaseBranch, result.BaseBranch, result.BaseBranchFallbackWarning)
		}
		return
	}
	e.persistRecoveredBaseBranch(ctx, taskID, req.TaskRepositoryID, resp.RequestedBaseBranch, resp.BaseBranch, resp.BaseBranchFallbackWarning)
}

func (e *Executor) persistRecoveredBaseBranch(
	ctx context.Context,
	taskID, taskRepositoryID, requestedBranch, resolvedBranch, warning string,
) {
	if strings.TrimSpace(warning) == "" || taskRepositoryID == "" || resolvedBranch == "" || resolvedBranch == requestedBranch {
		return
	}
	if err := e.taskRepositoryBaseBranchUpdater.UpdateTaskRepositoryBaseBranch(ctx, taskID, taskRepositoryID, resolvedBranch); err != nil {
		e.logger.Warn("failed to persist recovered task repository base branch",
			zap.String("task_id", taskID),
			zap.String("task_repository_id", taskRepositoryID),
			zap.String("base_branch", resolvedBranch),
			zap.Error(err))
	}
}

// environmentReposForLaunch returns the environment-repository rows for a
// launch: one per multi-repo worktree result, or a single repository inventory
// row. Rows without a WorktreeID describe a repository slot only; the worktree
// store excludes them from physical checkout operations.
func environmentReposForLaunch(req *LaunchAgentRequest, resp *LaunchAgentResponse) []*models.TaskEnvironmentRepo {
	if len(resp.Worktrees) > 0 {
		return buildTaskEnvironmentRepos(resp.Worktrees)
	}
	// Clone-based remote executors materialize all repositories inside one
	// task workspace, so they have no host worktree result to project here.
	// Still record every repository/branch slot before the environment becomes
	// reusable; otherwise the next attach-only launch correctly rejects the
	// partial inventory.
	if len(req.Repositories) > 0 {
		repos := make([]*models.TaskEnvironmentRepo, 0, len(req.Repositories))
		for position, spec := range req.Repositories {
			if spec.RepositoryID == "" {
				continue
			}
			repos = append(repos, &models.TaskEnvironmentRepo{
				RepositoryID: spec.RepositoryID,
				BranchSlug:   launchRepoBranchIdentitySlug(spec),
				// Remote clone launches only report an environment-level workspace
				// handle when they have no per-repository result. Do not invent a
				// path or branch for every repository from that shared handle: it
				// is not a canonical repository projection and could point a later
				// attach at the wrong checkout.
				Position: position,
			})
		}
		return repos
	}
	if req.RepositoryID == "" {
		return nil
	}
	branchSlug := req.BranchIdentitySlug
	if branchSlug == "" {
		branchSlug = req.BranchSlug
	}
	if branchSlug == "" {
		branchSlug = topLevelBranchIdentitySlug(req)
	}
	worktreeID, worktreePath, worktreeBranch := "", "", ""
	if resp.WorktreeID != "" {
		worktreeID = resp.WorktreeID
		worktreePath = resp.WorktreePath
		worktreeBranch = resp.WorktreeBranch
	}
	return []*models.TaskEnvironmentRepo{{
		RepositoryID: req.RepositoryID,
		BranchSlug:   worktree.SanitizeBranchSlug(branchSlug),
		// A launch without a concrete worktree still needs an inventory row
		// for reuse validation. It is not a physical worktree, so do not copy
		// the environment-level workspace path (which may be the host's seed
		// checkout) into the physical-worktree fields.
		WorktreeID:     worktreeID,
		WorktreePath:   worktreePath,
		WorktreeBranch: worktreeBranch,
		Position:       0,
	}}
}

// buildTaskEnvironmentRepos converts per-repo worktree results into env-repo rows.
// TaskEnvironmentID is left blank — it is set by the env Create transaction.
func buildTaskEnvironmentRepos(worktrees []RepoWorktreeResult) []*models.TaskEnvironmentRepo {
	out := make([]*models.TaskEnvironmentRepo, 0, len(worktrees))
	for i, w := range worktrees {
		out = append(out, &models.TaskEnvironmentRepo{
			RepositoryID:   w.RepositoryID,
			BranchSlug:     w.BranchSlug,
			WorktreeID:     w.WorktreeID,
			WorktreePath:   w.WorktreePath,
			WorktreeBranch: w.WorktreeBranch,
			Position:       i,
			ErrorMessage:   w.ErrorMessage,
		})
	}
	return out
}

// persistTaskEnvironmentRepos upserts per-repo rows under an existing env id.
// Used when an existing environment is reused (resume / re-launch on the same
// task), including cases where stale or legacy rows need the successful launch
// result written back for the next handoff.
func (e *Executor) persistTaskEnvironmentRepos(ctx context.Context, envID string, repos []*models.TaskEnvironmentRepo) error {
	if envID == "" || len(repos) == 0 {
		return nil
	}
	existing, err := e.repo.ListTaskEnvironmentRepos(ctx, envID)
	if err != nil {
		e.logger.Warn("failed to list existing task_environment_repos before insert",
			zap.String("env_id", envID),
			zap.Error(err))
		return fmt.Errorf("list task environment repositories: %w", err)
	}
	byKey := make(map[string]*models.TaskEnvironmentRepo, len(existing))
	legacyFlatByRepo := make(map[string]*models.TaskEnvironmentRepo)
	for _, row := range existing {
		key := row.RepositoryID + "\x00" + row.BranchSlug
		byKey[key] = row
		if row.RepositoryID != "" && row.BranchSlug == "" {
			legacyFlatByRepo[row.RepositoryID] = row
		}
	}
	for i, w := range repos {
		if w.RepositoryID == "" {
			continue
		}
		key := w.RepositoryID + "\x00" + w.BranchSlug
		if row := byKey[key]; row != nil {
			if err := e.refreshTaskEnvironmentRepo(ctx, row, w, i); err != nil {
				return err
			}
			continue
		}
		if w.BranchSlug != "" {
			if row := legacyFlatByRepo[w.RepositoryID]; row != nil {
				if err := e.refreshTaskEnvironmentRepo(ctx, row, w, i); err != nil {
					return err
				}
				delete(legacyFlatByRepo, w.RepositoryID)
				byKey[key] = row
				continue
			}
		}
		row := &models.TaskEnvironmentRepo{
			TaskEnvironmentID: envID,
			RepositoryID:      w.RepositoryID,
			BranchSlug:        w.BranchSlug,
			WorktreeID:        w.WorktreeID,
			WorktreePath:      w.WorktreePath,
			WorktreeBranch:    w.WorktreeBranch,
			Position:          i,
			ErrorMessage:      w.ErrorMessage,
		}
		if createErr := e.repo.CreateTaskEnvironmentRepo(ctx, row); createErr != nil {
			e.logger.Warn("failed to persist task environment repo",
				zap.String("env_id", envID),
				zap.String("repository_id", w.RepositoryID),
				zap.Error(createErr))
			return fmt.Errorf("persist task environment repository %q: %w", w.RepositoryID, createErr)
		}
	}
	return nil
}

func (e *Executor) refreshTaskEnvironmentRepo(ctx context.Context, row, w *models.TaskEnvironmentRepo, position int) error {
	if !taskEnvironmentRepoNeedsRefresh(row, w, position) {
		return nil
	}
	row.BranchSlug = w.BranchSlug
	// Concrete launch results populate the physical tuple together; inventory-only
	// rows have no WorktreeID and must not replace it.
	if w.WorktreeID != "" {
		row.WorktreeID = w.WorktreeID
		row.WorktreePath = w.WorktreePath
		row.WorktreeBranch = w.WorktreeBranch
	}
	row.Position = position
	row.ErrorMessage = w.ErrorMessage
	if err := e.repo.UpdateTaskEnvironmentRepo(ctx, row); err != nil {
		e.logger.Warn("failed to update task environment repo",
			zap.String("env_id", row.TaskEnvironmentID),
			zap.String("repository_id", row.RepositoryID),
			zap.String("branch_slug", row.BranchSlug),
			zap.Error(err))
		return fmt.Errorf("update task environment repository %q: %w", row.RepositoryID, err)
	}
	return nil
}

func taskEnvironmentRepoNeedsRefresh(row, w *models.TaskEnvironmentRepo, position int) bool {
	return row.BranchSlug != w.BranchSlug ||
		(w.WorktreeID != "" &&
			(row.WorktreeID != w.WorktreeID ||
				row.WorktreePath != w.WorktreePath ||
				row.WorktreeBranch != w.WorktreeBranch)) ||
		row.Position != position ||
		row.ErrorMessage != w.ErrorMessage
}

// extractSandboxID extracts the sandbox identifier from launch response metadata.
// For Sprites executors, this is the sprite_name.
func extractSandboxID(metadata map[string]interface{}) string {
	if metadata == nil {
		return ""
	}
	if name, ok := metadata["sprite_name"].(string); ok {
		return name
	}
	return ""
}
