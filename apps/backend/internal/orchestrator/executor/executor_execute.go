package executor

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"maps"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
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
func (e *Executor) resolveTaskSessionMCPMode(ctx context.Context, taskID string, session *models.TaskSession) (string, error) {
	if isConfigModeSession(session) {
		return McpModeConfig, nil
	}
	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("load task for MCP mode: %w", err)
	}
	if task != nil && task.IsFromOffice {
		return McpModeOffice, nil
	}
	return "", nil
}

// isContainerizedExecutor returns true for executor types that run agents in
// containers or remote sandboxes (Docker variants + Sprites). These executors
// need GitHub token injection for git operations since they don't have access
// to the host's git credentials, and they're the same set that needs the
// kandev-managed feature branch propagated through env metadata.
func isContainerizedExecutor(executorType string) bool {
	switch models.ExecutorType(executorType) {
	case models.ExecutorTypeLocalDocker, models.ExecutorTypeRemoteDocker, models.ExecutorTypeSprites:
		return true
	default:
		return false
	}
}

// executorNeedsResolvedCredentials reports whether an executor runs the agent
// off the control-plane host and therefore needs credentials resolved into
// req.Env (rather than inherited from the kandev process environment). This is
// every containerized executor plus SSH, whose remote agentctl only receives
// the credential keys we forward in req.Env.
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
	e.logger.Error("failed to start agent process",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("agent_execution_id", agentExecutionID),
		zap.Error(startErr))

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

// startAgentProcessAsync starts the agent subprocess and transitions the task to IN_PROGRESS on success.
func (e *Executor) startAgentProcessAsync(ctx context.Context, taskID, sessionID, agentExecutionID string) {
	e.runAgentProcessAsync(ctx, taskID, sessionID, agentExecutionID, func(updCtx context.Context) {
		if updateErr := e.writeTaskInProgressForRuntime(updCtx, taskID, sessionID); updateErr != nil {
			e.logger.Warn("failed to update task state to IN_PROGRESS after agent start",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(updateErr))
		}
	}, true, false)
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

// updateSessionStarting persists a full session-row STARTING transition, using
// the orchestrator callback when present so task/session runtime state stays
// serialized with guarded REVIEW reconciliation.
func (e *Executor) updateSessionStarting(ctx context.Context, taskID string, session *models.TaskSession, promoteTask bool) error {
	if e.onSessionStarting != nil {
		return e.onSessionStarting(ctx, taskID, session, promoteTask)
	}
	current, err := e.repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		return err
	}
	if current == nil {
		return fmt.Errorf("%w: agent session not found: %s", models.ErrTaskSessionNotFound, session.ID)
	}
	allowedTerminalRecovery := !promoteTask &&
		session.State == models.TaskSessionStateStarting &&
		(current.State == models.TaskSessionStateFailed ||
			(current.State == models.TaskSessionStateCancelled &&
				models.IsArchiveCancelReason(current.ErrorMessage)))
	if isStopTerminalSessionState(current.State) && !allowedTerminalRecovery {
		return &SessionStateSupersededError{SessionID: session.ID, State: current.State}
	}
	return e.persistSessionFullRowIfCurrentState(ctx, session, current.State)
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
	if repo.LocalPath == "" {
		return ""
	}
	cmd := exec.Command("git", "-C", repo.LocalPath, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// getSessionLock returns a per-session mutex, creating one if it doesn't exist.
// This serializes concurrent resume/launch operations on the same session to prevent
// duplicate agent processes after backend restart.
func (e *Executor) getSessionLock(sessionID string) *sync.Mutex {
	val, _ := e.sessionLocks.LoadOrStore(sessionID, &sync.Mutex{})
	return val.(*sync.Mutex)
}

func (e *Executor) applyPreferredShellEnv(ctx context.Context, executorType string, env map[string]string) map[string]string {
	if e.capabilities == nil || !e.capabilities.ShouldApplyPreferredShell(executorType) {
		return env
	}
	if e.shellPrefs == nil {
		return env
	}
	preferred, err := e.shellPrefs.PreferredShell(ctx)
	if err != nil {
		return env
	}
	preferred = strings.TrimSpace(preferred)
	if preferred == "" {
		return env
	}
	if env == nil {
		env = make(map[string]string)
	}
	env["AGENTCTL_SHELL_COMMAND"] = preferred
	env["SHELL"] = preferred
	return env
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

	// Launch the agent for the prepared session
	return e.LaunchPreparedSession(ctx, task, sessionID, LaunchOptions{
		AgentProfileID: agentProfileID,
		ExecutorID:     executorID,
		Prompt:         prompt,
		WorkflowStepID: workflowStepID,
		StartAgent:     true,
	})
}

// PrepareSession creates a session entry in the database without launching the agent.
// This allows the caller to get the session ID immediately and launch the agent later.
// Returns the session ID.
func (e *Executor) PrepareSession(ctx context.Context, task *v1.Task, agentProfileID string, executorID string, executorProfileID string, workflowStepID string) (string, error) {
	if agentProfileID == "" {
		e.logger.Error("task has no agent_profile_id configured", zap.String("task_id", task.ID))
		return "", ErrNoAgentProfileID
	}

	metadata := cloneMetadata(task.Metadata)
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

	// Determine if this new session should become primary.
	// Only the first session for a task is primary by default; subsequent sessions
	// leave the existing primary unchanged so the user's explicit choice is preserved.
	existingSessions, _ := e.repo.ListTaskSessions(ctx, task.ID)
	hasPrimary := false
	for _, s := range existingSessions {
		if s.IsPrimary {
			hasPrimary = true
			break
		}
	}
	isFirstSession := !hasPrimary

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
		IsPrimary:            isFirstSession,
		IsPassthrough:        isPassthrough,
		Metadata:             metadata,
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

	if err := e.repo.CreateTaskSession(ctx, session); err != nil {
		e.logger.Error("failed to persist agent session",
			zap.String("task_id", task.ID),
			zap.Error(err))
		return "", err
	}

	// Set primary flag only for the first session (no existing primary).
	// Subsequent sessions do not override the established primary.
	if isFirstSession {
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
	if opts.McpMode == "" {
		opts.McpMode, err = e.resolveTaskSessionMCPMode(ctx, task.ID, session)
		if err != nil {
			return nil, err
		}
	}

	running, _ := e.repo.GetExecutorRunningBySessionID(ctx, sessionID)
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

	// Fast path: workspace already launched (executors_running row exists).
	// Only start the agent subprocess if requested; otherwise return early.
	// If startAgentOnExistingWorkspace returns ErrStaleExecution, the in-memory
	// execution was lost (e.g. backend restart). The full LaunchAgent path below
	// will create a new execution and lifecycle.persistExecutorRunning will
	// overwrite the stale row.
	hasRunning, _ := e.repo.HasExecutorRunningRow(ctx, sessionID)
	if hasRunning {
		result, err := e.startAgentOnExistingWorkspace(ctx, task, session, prompt, startAgent, opts.McpMode, opts.Env)
		if !errors.Is(err, ErrStaleExecution) && !errors.Is(err, ErrAgentCommandMissing) {
			return result, err
		}
		e.logger.Info("falling through to full LaunchAgent for existing workspace",
			zap.String("task_id", task.ID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	allRepos, err := e.resolveAllRepoInfo(ctx, task.ID)
	if err != nil {
		return nil, err
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

	// Resolve the env ID before LaunchAgent so the in-memory AgentExecution
	// is env-scoped from the first shell/layout request, not only after DB
	// persistence succeeds. GetTaskEnvironmentByTaskID returns (nil, nil)
	// when no row exists; a real DB error must propagate so the launch
	// fails closed instead of silently launching a fresh environment that
	// orphans the existing container/sandbox/worktree.
	existingEnv, err := e.repo.GetTaskEnvironmentByTaskID(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("lookup existing task environment: %w", err)
	}
	// Child tasks created by office task-handoffs may have had
	// session.TaskEnvironmentID rewritten to point at the parent's /
	// shared group's env (see internal/orchestrator/handoff_inheritance.go).
	// The by-task-id lookup misses that row because it indexes by the
	// child task id, so without this fallback the launch path creates a
	// fresh worktree and the inheritance contract silently breaks.
	if existingEnv == nil && session.TaskEnvironmentID != "" {
		if inherited, err := e.repo.GetTaskEnvironment(ctx, session.TaskEnvironmentID); err == nil {
			existingEnv = inherited
		}
	}
	assignLaunchTaskEnvironmentID(session, existingEnv)

	req, execCfg, err := e.buildLaunchAgentRequest(ctx, task, session, agentProfileID, executorID, prompt, primaryRepo, allRepos)
	if err != nil {
		return nil, err
	}
	req.OfficeAgentProfileID = opts.OfficeAgentProfileID
	if req.OfficeAgentProfileID == "" && session.AgentProfileID != "" {
		req.OfficeAgentProfileID = session.AgentProfileID
	}
	req.StartAgent = startAgent
	mergeEnv(req, opts.Env)
	if opts.RouteOverride != nil {
		req.RouteOverride = opts.RouteOverride
	}

	// Apply McpMode from options (takes precedence over session metadata check in buildLaunchAgentRequest)
	if opts.McpMode != "" {
		req.McpMode = opts.McpMode
	}

	// Carry the prior ACP session id forward so the agent CLI resumes the
	// existing conversation (session/load) instead of opening a fresh one.
	// Reading from executors_running covers both:
	//   - office wakeups where startAgentOnExistingWorkspace returned
	//     ErrStaleExecution (in-memory exec gone after IDLE), and
	//   - kanban / quick-chat re-launches that hit the full path.
	// Unlike ResumeSession we do NOT clear req.TaskDescription — wakeups
	// deliver the new comment / event as the prompt.
	if startAgent {
		if token := resumeTokenForExecutionProfile(running, agentProfileID); token != "" {
			req.ACPSessionID = token
			e.logger.Info("resuming ACP session via stored resume token",
				zap.String("task_id", task.ID),
				zap.String("session_id", sessionID),
				zap.String("acp_session_id", token))
		}
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

	req.Env = e.applyPreferredShellEnv(ctx, req.ExecutorType, req.Env)

	// Call the AgentManager to launch the container
	resp, err := e.agentManager.LaunchAgent(ctx, req)
	if err != nil {
		return nil, e.handleLaunchFailure(ctx, task.ID, sessionID, failingLaunchRepositoryID(req, err), err)
	}

	// Create or update the task environment with launch results
	e.persistTaskEnvironment(ctx, task.ID, session, existingEnv, req, resp, execCfg)

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

// failingLaunchRepositoryID identifies the repository that caused a
// multi-repository branch-fetch failure. A lifecycle launch error only carries
// the failed branch name, so correlate it with the unique per-repository
// checkout branch in the request. Ambiguous or unrecognized branches fail
// closed: no repository-scoped destructive guidance can be offered.
func failingLaunchRepositoryID(req *LaunchAgentRequest, launchErr error) string {
	if req == nil {
		return ""
	}
	if len(req.Repositories) == 0 {
		return req.RepositoryID
	}

	branch := extractLaunchFailureBranch(launchErr)
	if branch == "" {
		return ""
	}
	var repositoryID string
	for _, spec := range req.Repositories {
		if strings.TrimSpace(spec.CheckoutBranch) != branch {
			continue
		}
		if repositoryID != "" {
			return ""
		}
		repositoryID = spec.RepositoryID
	}
	return repositoryID
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

// handleLaunchFailure marks the session and task as FAILED and returns the original error.
func (e *Executor) handleLaunchFailure(ctx context.Context, taskID, sessionID, repositoryID string, launchErr error) error {
	// Detach from caller context so failure bookkeeping completes even if the
	// original request context was cancelled.
	failCtx := context.WithoutCancel(ctx)
	e.logger.Error("failed to launch agent",
		zap.String("task_id", taskID),
		zap.Error(launchErr))
	var onChanged func()
	if e.onLaunchFailed != nil {
		onChanged = func() {
			e.onLaunchFailed(failCtx, taskID, sessionID, repositoryID, launchErr)
		}
	}
	changed, _, updateErr := e.transitionSessionStateWithHook(
		failCtx, taskID, sessionID, models.TaskSessionStateFailed, launchErr.Error(), onChanged,
	)
	if updateErr != nil {
		e.logger.Warn("failed to mark session as failed after launch error",
			zap.String("session_id", sessionID),
			zap.Error(updateErr))
	}
	if changed {
		if updateErr := e.updateTaskState(failCtx, taskID, v1.TaskStateFailed); updateErr != nil {
			e.logger.Warn("failed to mark task as failed after launch error",
				zap.String("task_id", taskID),
				zap.Error(updateErr))
		}
	}
	return launchErr
}

// finalizeLaunch persists launch state and returns the resulting TaskExecution.
func (e *Executor) finalizeLaunch(ctx context.Context, task *v1.Task, session *models.TaskSession, agentProfileID, sessionID string, repoInfo *repoInfo, resp *LaunchAgentResponse, startAgent bool, execCfg executorConfig) (*TaskExecution, error) {
	now := time.Now().UTC()
	if err := e.persistLaunchState(ctx, task.ID, sessionID, session, resp, startAgent, now); err != nil {
		e.cleanupUnstartedExecutionAfterPersistError(ctx, sessionID, resp.AgentExecutionID, err)
		return nil, err
	}
	e.persistWorktreeAssociation(ctx, task.ID, session, repoInfo.RepositoryID, resp)

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
func (e *Executor) buildLaunchAgentRequest(ctx context.Context, task *v1.Task, session *models.TaskSession, agentProfileID, executorID, prompt string, repoInfo *repoInfo, allRepos []*repoInfo) (*LaunchAgentRequest, executorConfig, error) {
	metadata := cloneMetadata(task.Metadata)
	if session.ExecutorProfileID != "" {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["executor_profile_id"] = session.ExecutorProfileID
	}
	sessionID := session.ID
	req := &LaunchAgentRequest{
		TaskID:            task.ID,
		WorkspaceID:       task.WorkspaceID,
		TaskTitle:         task.Title,
		AgentProfileID:    agentProfileID,
		TaskDescription:   prompt,
		Priority:          task.Priority,
		SessionID:         sessionID,
		TaskEnvironmentID: session.TaskEnvironmentID,
		IsEphemeral:       task.IsEphemeral,
		IsPassthrough:     session.IsPassthrough,
		WorkspacePath:     session.WorkspacePath,
	}

	execConfig := e.resolveExecutorConfig(ctx, executorID, task.WorkspaceID, metadata)
	if execConfig.ExecutorID != "" {
		metadata = execConfig.Metadata
		req.ExecutorType = execConfig.ExecutorType
		req.ExecutorConfig = execConfig.ExecutorCfg
		req.SetupScript = execConfig.SetupScript
		// Merge profile env vars into request env
		if len(execConfig.ProfileEnv) > 0 {
			if req.Env == nil {
				req.Env = make(map[string]string)
			}
			for k, v := range execConfig.ProfileEnv {
				req.Env[k] = v
			}
		}
	}

	// For remote executors (containerized *and* SSH), resolve credentials into
	// req.Env in this order:
	// 1. Profile remote_auth_secrets (e.g., gh_cli_env method with secret)
	// 2. Profile remote_credentials with gh_cli_token (extract from local gh CLI)
	// 3. Global GITHUB_TOKEN secret (fallback)
	// 4. Auto-extract from local gh CLI (final fallback)
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

	// Activate config-mode MCP tools when config_mode is set in session metadata.
	if isConfigModeSession(session) {
		req.McpMode = McpModeConfig
	}

	if len(metadata) > 0 {
		req.Metadata = metadata
	}

	return req, execConfig, nil
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

// applyContainerCredentials resolves and injects credentials for containerized executors.
func (e *Executor) applyContainerCredentials(ctx context.Context, req *LaunchAgentRequest, metadata map[string]interface{}) {
	e.resolveRemoteCredentials(ctx, req, metadata)
	e.injectGitHubToken(ctx, req)        // Fallback to global secret
	e.injectGitHubTokenFromCLI(ctx, req) // Final fallback to local gh CLI
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
			RepositoryID:           info.RepositoryID,
			RepositoryPath:         info.RepositoryPath,
			BaseBranch:             info.BaseBranch,
			CheckoutBranch:         info.CheckoutBranch,
			PRNumber:               info.PRNumber,
			WorktreeBranchPrefix:   info.WorktreeBranchPrefix,
			WorktreeBranchTemplate: info.WorktreeBranchTemplate,
			PullBeforeWorktree:     info.PullBeforeWorktree,
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
	groups := make(map[string][]*repoInfo, len(allRepos))
	for _, info := range allRepos {
		if info == nil || info.RepositoryID == "" {
			continue
		}
		groups[info.RepositoryID] = append(groups[info.RepositoryID], info)
	}

	plans := make(map[*repoInfo]repoBranchPlan, len(allRepos))
	for repoID, group := range groups {
		identities := branchIdentitySlugsForGroup(repoID, group)
		if len(group) < 2 {
			for _, info := range group {
				plans[info] = repoBranchPlan{identitySlug: identities[info]}
			}
			continue
		}
		flatIdentity := selectFlatBranchIdentity(group, identities)
		for _, info := range group {
			identity := identities[info]
			pathSlug := identity
			if identity == flatIdentity {
				pathSlug = ""
			}
			plans[info] = repoBranchPlan{identitySlug: identity, pathSlug: pathSlug}
		}
	}
	return plans
}

func branchIdentitySlugsForGroup(repoID string, group []*repoInfo) map[*repoInfo]string {
	raw := make(map[*repoInfo]string, len(group))
	counts := make(map[string]int, len(group))
	for _, info := range group {
		slug := preferredBranchIdentitySlug(info)
		if slug == "" {
			slug = "branch-" + branchIdentityHash(repoID, info)
		}
		raw[info] = slug
		counts[slug]++
	}

	out := make(map[*repoInfo]string, len(group))
	for _, info := range group {
		slug := raw[info]
		if counts[slug] > 1 {
			slug += "-" + branchIdentityHash(repoID, info)
		}
		slug = worktree.SanitizeBranchSlug(slug)
		if slug == "" {
			slug = "branch-" + branchIdentityHash(repoID, info)
		}
		out[info] = slug
	}
	return out
}

func preferredBranchIdentitySlug(info *repoInfo) string {
	branch := info.CheckoutBranch
	if branch == "" {
		branch = info.BaseBranch
	}
	if branch == "" && info.Repository != nil {
		branch = info.Repository.DefaultBranch
	}
	return worktree.SanitizeBranchSlug(branch)
}

func branchIdentityHash(repoID string, info *repoInfo) string {
	seed := strings.Join([]string{
		repoID,
		info.BaseBranch,
		info.CheckoutBranch,
		fmt.Sprintf("%d", info.PRNumber),
	}, "\x00")
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	return fmt.Sprintf("%08x", h.Sum32())
}

func selectFlatBranchIdentity(group []*repoInfo, identities map[*repoInfo]string) string {
	candidates := make([]*repoInfo, 0, len(group))
	candidates = append(candidates, group...)
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Position != candidates[j].Position {
			return candidates[i].Position < candidates[j].Position
		}
		leftRank := flatBranchRank(candidates[i])
		rightRank := flatBranchRank(candidates[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return identities[candidates[i]] < identities[candidates[j]]
	})
	return identities[candidates[0]]
}

func flatBranchRank(info *repoInfo) int {
	if info.CheckoutBranch != "" {
		return 3
	}
	if info.Repository != nil && info.Repository.DefaultBranch != "" && info.BaseBranch == info.Repository.DefaultBranch {
		return 0
	}
	if info.BaseBranch == defaultBaseBranch {
		return 1
	}
	return 2
}

// applyRepositoryConfig sets repository-related fields on the request and resolves clone URLs.
func (e *Executor) applyRepositoryConfig(req *LaunchAgentRequest, task *v1.Task, repoInfo *repoInfo, execConfig executorConfig, metadata map[string]interface{}) (map[string]interface{}, error) {
	if repoInfo.RepositoryPath != "" {
		req.UseWorktree = shouldUseWorktree(execConfig.ExecutorType)
		req.RepositoryID = repoInfo.RepositoryID
		req.RepositoryPath = repoInfo.RepositoryPath
		req.BaseBranch = repoInfo.BaseBranch
		req.CheckoutBranch = repoInfo.CheckoutBranch
		req.PRNumber = repoInfo.PRNumber
		req.WorktreeBranchPrefix = repoInfo.WorktreeBranchPrefix
		req.WorktreeBranchTemplate = repoInfo.WorktreeBranchTemplate
		req.PullBeforeWorktree = repoInfo.PullBeforeWorktree
		if repoInfo.Repository != nil {
			req.DefaultBranch = repoInfo.Repository.DefaultBranch
		}
		// Task directory mode: place worktree inside per-task directory
		if req.UseWorktree && repoInfo.Repository != nil && repoInfo.Repository.Name != "" {
			req.TaskDirName = worktree.SemanticWorktreeName(task.Title, worktree.SmallSuffix(3))
			req.RepoName = repoInfo.Repository.Name
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
	if e.capabilities != nil && e.capabilities.RequiresCloneURL(execConfig.ExecutorType) && repoInfo.Repository != nil {
		cloneURL := repositoryCloneURL(repoInfo.Repository)
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

// startAgentOnExistingWorkspace handles the case where LaunchPreparedSession is called on a session
// whose workspace (agentctl) was already launched. It optionally starts just the agent subprocess.
//
// The in-memory ExecutionStore is the single source of truth here: if no execution
// exists for this session in the store, the workspace is gone (or was never
// created in this process — e.g. after restart) and the caller must take the full
// re-launch path. Pre-refactor this also consulted session.AgentExecutionID and
// reconciled DB drift; that's now structurally impossible because executors_running
// is owned by the lifecycle manager and writes are atomic with executionStore.Add.
func (e *Executor) startAgentOnExistingWorkspace(ctx context.Context, task *v1.Task, session *models.TaskSession, prompt string, startAgent bool, mcpMode string, env map[string]string) (*TaskExecution, error) {
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
	if effectiveMcpMode != "" {
		if err := e.agentManager.SetMcpMode(ctx, executionID, effectiveMcpMode); err != nil {
			e.logger.Error("failed to set MCP mode for existing workspace",
				zap.String("session_id", session.ID),
				zap.String("agent_execution_id", executionID),
				zap.String("mcp_mode", effectiveMcpMode),
				zap.Error(err))
			return nil, fmt.Errorf("set MCP mode %q: %w", effectiveMcpMode, err)
		}
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
	now := time.Now().UTC()
	session.State = models.TaskSessionStateStarting
	session.ErrorMessage = ""
	session.UpdatedAt = now
	if err := e.updateSessionStarting(ctx, task.ID, session, true); err != nil {
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
// resp.WorktreePath here already mirrors what executor_standalone.go writes
// into metadata["worktree_path"] (= req.WorkspacePath from the env preparer),
// which is also what becomes cmd.Dir of the agent process. So persisting it
// as-is keeps a single source of truth.
func computeWorkspacePath(req *LaunchAgentRequest, resp *LaunchAgentResponse) string {
	if resp.WorktreePath != "" {
		return resp.WorktreePath
	}
	if req.RepositoryPath != "" {
		return req.RepositoryPath
	}
	// Quick-chat sessions have no worktree/repo but the lifecycle manager
	// creates a workspace directory — use it as fallback.
	return resp.WorkspacePath
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
) {
	mu := e.taskEnvLock(taskID)
	mu.Lock()
	defer mu.Unlock()

	// Re-fetch under the lock — a sibling launch for the same task may have
	// just created the env and released the lock. Without this we'd still
	// see existingEnv == nil from the original call and try to create a
	// duplicate.
	if existingEnv == nil {
		if fresh, err := e.repo.GetTaskEnvironmentByTaskID(ctx, taskID); err == nil && fresh != nil {
			existingEnv = fresh
		}
	}

	workspacePath := computeWorkspacePath(req, resp)

	if existingEnv != nil {
		// agent_execution_id is no longer stored on task_environments — the column
		// is being dropped (executors_running is the single source of truth).
		// Status, worktree, and container fields are still env-row-owned.
		existingEnv.Status = models.TaskEnvironmentStatusReady
		// Refresh worktree + workspace + container/sandbox fields. The original
		// update branch only touched AgentExecutionID/Status, so envs created
		// with empty paths (e.g. before the worktree resolved) stayed
		// permanently broken. Sandbox ID gets refreshed too in case a fallback
		// created a new sprite.
		if resp.WorktreeID != "" {
			existingEnv.WorktreeID = resp.WorktreeID
		}
		if resp.WorktreePath != "" {
			existingEnv.WorktreePath = resp.WorktreePath
		}
		if resp.WorktreeBranch != "" {
			existingEnv.WorktreeBranch = resp.WorktreeBranch
		}
		if workspacePath != "" {
			existingEnv.WorkspacePath = workspacePath
		}
		if resp.ContainerID != "" {
			existingEnv.ContainerID = resp.ContainerID
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
		if err := e.repo.UpdateTaskEnvironment(ctx, existingEnv); err != nil {
			e.logger.Warn("failed to update task environment",
				zap.String("task_id", taskID),
				zap.String("env_id", existingEnv.ID),
				zap.Error(err))
		}
		session.TaskEnvironmentID = existingEnv.ID
		// Persist per-repo rows for multi-repo launches that didn't have them yet.
		e.persistTaskEnvironmentRepos(ctx, existingEnv.ID, resp.Worktrees)
		return
	}

	env := &models.TaskEnvironment{
		ID:                session.TaskEnvironmentID,
		TaskID:            taskID,
		RepositoryID:      req.RepositoryID,
		ExecutorType:      req.ExecutorType,
		ExecutorID:        execCfg.ExecutorID,
		ExecutorProfileID: session.ExecutorProfileID,
		// AgentExecutionID is intentionally not set here — see executors_running
		// for the active execution per session.
		Status:         models.TaskEnvironmentStatusReady,
		WorktreeID:     resp.WorktreeID,
		WorktreePath:   resp.WorktreePath,
		WorktreeBranch: resp.WorktreeBranch,
		WorkspacePath:  workspacePath,
		ContainerID:    resp.ContainerID,
		TaskDirName:    req.TaskDirName,
		SandboxID:      extractSandboxID(resp.Metadata),
	}
	// Embed per-repo rows in the same create transaction when multi-repo.
	if len(resp.Worktrees) > 0 {
		env.Repos = buildTaskEnvironmentRepos(resp.Worktrees)
	}
	if err := e.repo.CreateTaskEnvironment(ctx, env); err != nil {
		e.logger.Warn("failed to create task environment",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	session.TaskEnvironmentID = env.ID
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
func (e *Executor) persistTaskEnvironmentRepos(ctx context.Context, envID string, worktrees []RepoWorktreeResult) {
	if envID == "" || len(worktrees) == 0 {
		return
	}
	existing, err := e.repo.ListTaskEnvironmentRepos(ctx, envID)
	if err != nil {
		e.logger.Warn("failed to list existing task_environment_repos before insert",
			zap.String("env_id", envID),
			zap.Error(err))
		return
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
	for i, w := range worktrees {
		if w.RepositoryID == "" {
			continue
		}
		key := w.RepositoryID + "\x00" + w.BranchSlug
		if row := byKey[key]; row != nil {
			e.refreshTaskEnvironmentRepo(ctx, row, w, i)
			continue
		}
		if w.BranchSlug != "" {
			if row := legacyFlatByRepo[w.RepositoryID]; row != nil {
				e.refreshTaskEnvironmentRepo(ctx, row, w, i)
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
		}
	}
}

func (e *Executor) refreshTaskEnvironmentRepo(ctx context.Context, row *models.TaskEnvironmentRepo, w RepoWorktreeResult, position int) {
	if !taskEnvironmentRepoNeedsRefresh(row, w, position) {
		return
	}
	row.BranchSlug = w.BranchSlug
	row.WorktreeID = w.WorktreeID
	row.WorktreePath = w.WorktreePath
	row.WorktreeBranch = w.WorktreeBranch
	row.Position = position
	row.ErrorMessage = w.ErrorMessage
	if err := e.repo.UpdateTaskEnvironmentRepo(ctx, row); err != nil {
		e.logger.Warn("failed to update task environment repo",
			zap.String("env_id", row.TaskEnvironmentID),
			zap.String("repository_id", row.RepositoryID),
			zap.String("branch_slug", row.BranchSlug),
			zap.Error(err))
	}
}

func taskEnvironmentRepoNeedsRefresh(row *models.TaskEnvironmentRepo, w RepoWorktreeResult, position int) bool {
	return row.BranchSlug != w.BranchSlug ||
		row.WorktreeID != w.WorktreeID ||
		row.WorktreePath != w.WorktreePath ||
		row.WorktreeBranch != w.WorktreeBranch ||
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
