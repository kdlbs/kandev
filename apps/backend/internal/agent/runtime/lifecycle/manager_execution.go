package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/agentctl/tracing"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/appctx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// ErrSessionWorkspaceNotReady indicates the task session exists but does not yet
// have a resolved workspace path (typically while worktree preparation is in progress).
var ErrSessionWorkspaceNotReady = errors.New("session workspace not ready")

// ErrSessionTerminal indicates the task session has reached a terminal state
// (cancelled/completed/failed) and no execution can be created for it. User-facing
// workspace handlers treat this like ErrSessionWorkspaceNotReady: a graceful
// not-ready envelope rather than an ERROR-logged failure, since a terminal session
// will never recover an execution.
var ErrSessionTerminal = errors.New("session is terminal")

// ResolveSessionRuntime returns the runtime selected for a session without
// creating or resuming its execution. Session-scoped handlers can use this to
// reject unsupported runtimes before GetOrEnsureExecution starts resources.
func (m *Manager) ResolveSessionRuntime(ctx context.Context, sessionID string) (agentruntime.Runtime, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session_id is required")
	}
	// Execution surfaces require session.exec, not mere reach. This is the
	// chokepoint every workspace-oriented handler (shell, files, ports, VS
	// Code, LSP) goes through, so gating it here covers them all rather than
	// relying on each handler to remember.
	if check := m.execAccessCheck(); check != nil {
		if err := check(ctx, sessionID); err != nil {
			return "", err
		}
	}
	if execution, exists := m.executionStore.GetBySessionID(sessionID); exists {
		return execution.RuntimeName, nil
	}
	if m.workspaceInfoProvider == nil {
		return "", fmt.Errorf("workspace info provider not configured")
	}
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, "", sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve runtime for session %s: %w", sessionID, err)
	}
	if info == nil {
		return "", fmt.Errorf("session %s not found", sessionID)
	}
	if info.ExecutorType != "" {
		return models.ExecutorType(info.ExecutorType).Runtime(), nil
	}
	if info.RuntimeName != "" {
		return info.RuntimeName, nil
	}
	return agentruntime.RuntimeStandalone, nil
}

// GetOrEnsureExecution returns an existing execution or creates one on-demand.
// Use this for workspace-oriented operations (files, shell, inference, ports, vscode, LSP)
// that should survive backend restarts. For operations requiring a running agent
// process (prompt, cancel, mode), use GetExecutionBySessionID instead.
//
// Concurrent calls for the same sessionID are deduplicated via singleflight.
func (m *Manager) GetOrEnsureExecution(ctx context.Context, sessionID string) (*AgentExecution, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	// Per-user workspace scoping (opt-in auth): user-facing session surfaces
	// funnel through here; internal callers pass a ctx without an identity
	// and are unaffected.
	// Execution surfaces require session.exec, not mere reach. This is the
	// chokepoint every workspace-oriented handler (shell, files, ports, VS
	// Code, LSP) goes through, so gating it here covers them all rather than
	// relying on each handler to remember.
	if check := m.execAccessCheck(); check != nil {
		if err := check(ctx, sessionID); err != nil {
			return nil, err
		}
	}

	// Fast path: execution already in memory
	if execution, exists := m.executionStore.GetBySessionID(sessionID); exists {
		return execution, nil
	}

	// Slow path: create on-demand, deduplicated by sessionID-keyed singleflight.
	// Use ensureWorkspaceExecutionLocked (not EnsureWorkspaceExecutionForSession)
	// to avoid recursing into the same singleflight slot we already hold.
	value, err := m.doCoalescedExecution(ctx, sessionID, func(sharedCtx context.Context) (interface{}, error) {
		return m.ensureWorkspaceExecutionLocked(sharedCtx, "", sessionID)
	})
	if err != nil {
		return nil, err
	}
	return value.(*AgentExecution), nil
}

// GetOrEnsureExecutionForEnvironment returns an execution for a task environment,
// creating one on-demand from the workspace info provider when needed.
//
// Important: this MUST share the session-keyed singleflight bucket with
// GetOrEnsureExecution(sessionID) and EnsureWorkspaceExecutionForSession.
// A previous version keyed by `"env:" + envID`, which let a concurrent
// session-keyed call race past it (each path observed "no execution" for its
// own key, both called createExecution, both ExecutionStore.Add, the second
// silently overwrote the bySession index, and the first execution's
// agent subprocess was orphaned). See `ErrExecutionAlreadyExistsForSession`.
func (m *Manager) GetOrEnsureExecutionForEnvironment(ctx context.Context, taskEnvironmentID string) (*AgentExecution, error) {
	if taskEnvironmentID == "" {
		return nil, fmt.Errorf("task_environment_id is required")
	}
	// Per-user scoping (opt-in auth) — before the cache short-circuit so a
	// cached execution cannot be reached by a non-owner.
	if check := m.environmentAccessCheck; check != nil {
		if err := check(ctx, taskEnvironmentID); err != nil {
			return nil, err
		}
	}

	if execution, exists := m.executionStore.GetByTaskEnvironmentID(taskEnvironmentID); exists {
		return execution, nil
	}

	if m.workspaceInfoProvider == nil {
		return nil, fmt.Errorf("workspace info provider not configured")
	}
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForEnvironment(ctx, taskEnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace info for environment %s: %w", taskEnvironmentID, err)
	}
	if info == nil {
		return nil, fmt.Errorf("task environment %s not found", taskEnvironmentID)
	}
	if info.TaskEnvironmentID == "" {
		return nil, fmt.Errorf("task environment %s has no task_environment_id", taskEnvironmentID)
	}
	if info.TaskEnvironmentID != taskEnvironmentID {
		return nil, fmt.Errorf("workspace info resolved environment %s, want %s", info.TaskEnvironmentID, taskEnvironmentID)
	}
	if info.WorkspacePath == "" {
		return nil, fmt.Errorf("%w: task environment %s has no workspace path yet", ErrSessionWorkspaceNotReady, taskEnvironmentID)
	}
	if info.SessionID == "" {
		return nil, fmt.Errorf("task environment %s has no task session", taskEnvironmentID)
	}

	// Share the sessionID-keyed bucket so we deduplicate against any concurrent
	// GetOrEnsureExecution(sessionID) / EnsureWorkspaceExecutionForSession for
	// the same session.
	value, err := m.doCoalescedExecution(ctx, info.SessionID, func(sharedCtx context.Context) (interface{}, error) {
		if execution, exists := m.executionStore.GetBySessionID(info.SessionID); exists {
			return execution, nil
		}
		if execution, exists := m.executionStore.GetByTaskEnvironmentID(taskEnvironmentID); exists {
			return execution, nil
		}
		// createExecution publishes AgentctlStarting before spawning the
		// waitForAgentctlReady goroutine, so frontend gates flip out of
		// `undefined` even on this lazy-create path.
		execution, err := m.createExecution(sharedCtx, info.TaskID, info)
		if err != nil {
			return nil, err
		}
		return execution, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*AgentExecution), nil
}

// EnsureWorkspaceExecutionForSession ensures an agentctl execution exists for a specific task session.
// This is used when the frontend provides a session ID (e.g., from URL path /task/[id]/[sessionId]).
// If an execution already exists for the session, it returns it. Otherwise, it creates a new execution
// using the session's workspace configuration from the database.
//
// Concurrent calls (including from GetOrEnsureExecution and
// GetOrEnsureExecutionForEnvironment) are deduplicated via the same
// sessionID-keyed singleflight bucket so they cannot race past their
// individual check-then-act guards and create duplicate executions.
func (m *Manager) EnsureWorkspaceExecutionForSession(ctx context.Context, taskID, sessionID string) (*AgentExecution, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Fast path: execution already in memory
	if execution, exists := m.executionStore.GetBySessionID(sessionID); exists {
		return execution, nil
	}

	value, err := m.doCoalescedExecution(ctx, sessionID, func(sharedCtx context.Context) (interface{}, error) {
		return m.ensureWorkspaceExecutionLocked(sharedCtx, taskID, sessionID)
	})
	if err != nil {
		return nil, err
	}
	return value.(*AgentExecution), nil
}

func (m *Manager) doCoalescedExecution(
	ctx context.Context,
	key string,
	operation func(context.Context) (interface{}, error),
) (interface{}, error) {
	result := m.ensureExecutionGroup.DoChan(key, func() (interface{}, error) {
		sharedCtx, cancel := m.coalescedExecutionContext(ctx)
		defer cancel()
		return operation(sharedCtx)
	})
	return awaitCoalescedResult(ctx, result)
}

func (m *Manager) coalescedExecutionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	// The shared context owns caller-independent cancellation only. Runtime
	// launch phases start their own deadlines after environment resolution, and
	// setup scripts derive a separate preparation budget inside those phases.
	sharedCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	if m.stopCh == nil {
		return sharedCtx, cancel
	}
	go func() {
		select {
		case <-m.stopCh:
			cancel()
		case <-sharedCtx.Done():
		}
	}()
	return sharedCtx, cancel
}

func awaitCoalescedResult(
	ctx context.Context,
	result <-chan singleflight.Result,
) (interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case completed := <-result:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if completed.Err != nil {
			return nil, completed.Err
		}
		return completed.Val, nil
	}
}

// ensureWorkspaceExecutionLocked is the body of EnsureWorkspaceExecutionForSession
// run inside the sessionID-keyed singleflight bucket. Callers other than
// EnsureWorkspaceExecutionForSession must already hold the singleflight slot.
func (m *Manager) ensureWorkspaceExecutionLocked(ctx context.Context, taskID, sessionID string) (*AgentExecution, error) {
	// Double-check after acquiring the slot — a peer in the same group may have
	// finished while we were waiting.
	if execution, exists := m.executionStore.GetBySessionID(sessionID); exists {
		return execution, nil
	}

	if m.workspaceInfoProvider == nil {
		return nil, fmt.Errorf("workspace info provider not configured")
	}

	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, taskID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace info for session %s: %w", sessionID, err)
	}
	if info == nil {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	// Resolve taskID from provider when caller doesn't have it (e.g., GetOrEnsureExecution)
	if taskID == "" {
		taskID = info.TaskID
	}

	if info.TaskEnvironmentID != "" {
		if execution, exists := m.executionStore.GetByTaskEnvironmentID(info.TaskEnvironmentID); exists {
			m.logger.Info("reusing existing execution for task environment",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("task_environment_id", info.TaskEnvironmentID),
				zap.String("execution_id", execution.ID))
			return execution, nil
		}
	}

	if info.WorkspacePath == "" {
		return nil, fmt.Errorf("%w: session %s has no workspace path yet", ErrSessionWorkspaceNotReady, sessionID)
	}

	m.logger.Info("creating execution for task session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("workspace_path", info.WorkspacePath),
		zap.String("acp_session_id", info.ACPSessionID))

	// createExecution publishes AgentctlStarting before spawning the
	// waitForAgentctlReady goroutine, so workspace-only executions also
	// notify the frontend without racing the readiness event.
	execution, err := m.createExecution(ctx, taskID, info)
	if err != nil {
		return nil, err
	}

	// For workspace-only executions (no agent), wait for agentctl to be ready
	// then connect the workspace stream so process output can be received.
	// Note: AgentctlReady/Error events are already handled by waitForAgentctlReady
	// (started by createExecution), so this goroutine only connects the stream.
	go func() {
		// Use detached context that respects stopCh for graceful shutdown
		waitCtx, cancel := appctx.Detached(ctx, m.stopCh, 60*time.Second)
		defer cancel()

		if err := execution.agentctl.WaitForReady(waitCtx, 60*time.Second); err != nil {
			m.logger.Error("agentctl not ready for workspace stream connection",
				zap.String("execution_id", execution.ID),
				zap.Error(err))
			return
		}

		// Connect workspace stream for process output (agent stream not needed for workspace-only)
		if m.streamManager != nil {
			m.logger.Info("connecting workspace stream for workspace-only execution",
				zap.String("execution_id", execution.ID))
			m.streamManager.ConnectWorkspaceStream(execution, nil)
		}
	}()

	return execution, nil
}

// GetExecutionIDForSession returns the execution ID for a session from the in-memory
// execution store. Returns empty string and error if no execution is found.
func (m *Manager) GetExecutionIDForSession(_ context.Context, sessionID string) (string, error) {
	if execution, exists := m.executionStore.GetBySessionID(sessionID); exists {
		return execution.ID, nil
	}
	return "", fmt.Errorf("%w: %s", ErrNoExecutionForSession, sessionID)
}

// GetACPSessionIDForSession returns the ACP conversation currently owned by a
// live execution. The orchestrator uses this optional accessor after a context
// reset to persist the new conversation immediately, instead of depending on
// an asynchronous session-created event arriving before a backend restart.
func (m *Manager) GetACPSessionIDForSession(sessionID string) (string, bool) {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists || execution == nil {
		return "", false
	}
	var acpSessionID string
	if err := m.executionStore.WithRLock(execution.ID, func(exec *AgentExecution) {
		acpSessionID = exec.ACPSessionID
	}); err != nil || acpSessionID == "" {
		return "", false
	}
	return acpSessionID, true
}

// IsAgentCommandConfigured reports whether an execution has been promoted from
// workspace-only infrastructure to an agent execution ready to start.
func (m *Manager) IsAgentCommandConfigured(executionID string) bool {
	configured := false
	_ = m.executionStore.WithRLock(executionID, func(execution *AgentExecution) {
		configured = execution.AgentCommand != ""
	})
	return configured
}

// EnsurePassthroughExecution ensures an execution exists for a passthrough session
// and starts the passthrough process if needed. This is called when the terminal
// handler receives a connection for a session that might need recovery after backend restart.
//
// The sessionID is required. If taskID is empty, it will be looked up from:
// 1. The existing execution (if any)
// 2. The workspace info provider
//
// Returns the execution with a running passthrough process, or an error.
func (m *Manager) EnsurePassthroughExecution(ctx context.Context, sessionID string) (*AgentExecution, error) {
	// Per-user scoping (opt-in auth) — before the cache short-circuit so a
	// cached execution cannot be reached by a non-owner.
	// Execution surfaces require session.exec, not mere reach. This is the
	// chokepoint every workspace-oriented handler (shell, files, ports, VS
	// Code, LSP) goes through, so gating it here covers them all rather than
	// relying on each handler to remember.
	if check := m.execAccessCheck(); check != nil {
		if err := check(ctx, sessionID); err != nil {
			return nil, err
		}
	}
	// Check if execution already exists with a running passthrough process.
	// PassthroughProcessID is not cleared on exit, so a stale ID can point at
	// a dead process; verify the runner still has it before short-circuiting,
	// otherwise a fast-failed resume launch would keep returning the dead ID
	// and the WS handler's IsProcessReadyOrPending check would 503 forever.
	if execution, exists := m.executionStore.GetBySessionID(sessionID); exists {
		if execution.PassthroughProcessID != "" {
			if runner := m.GetInteractiveRunner(); runner != nil && runner.IsProcessReadyOrPending(execution.PassthroughProcessID) {
				return execution, nil
			}
			m.logger.Info("execution has stale passthrough process ID, relaunching",
				zap.String("session_id", sessionID),
				zap.String("execution_id", execution.ID),
				zap.String("stale_process_id", execution.PassthroughProcessID))
		}
		return m.resumeExistingExecution(ctx, sessionID, execution)
	}

	// No execution exists - need to create one from session info
	return m.createExecutionFromSessionInfo(ctx, sessionID)
}

// resumeExistingExecution starts the passthrough process for an existing execution
// that has no running process (e.g., after backend restart).
func (m *Manager) resumeExistingExecution(ctx context.Context, sessionID string, execution *AgentExecution) (*AgentExecution, error) {
	m.logger.Info("execution exists but passthrough process not running, starting",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID))

	if err := m.ResumePassthroughSession(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("resume passthrough session %s: %w", sessionID, err)
	}

	// Get updated execution with process ID
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil, fmt.Errorf("execution disappeared after resuming passthrough session %s", sessionID)
	}
	return execution, nil
}

// createExecutionFromSessionInfo creates a new execution for a passthrough session
// when no execution exists — either because the session has never run (a task
// created with start_agent:false, started later) or because a backend restart
// cleared the execution store. The two are distinguished by applyResumeIntent,
// which decides fresh launch vs resume.
//
// Terminal sessions are rejected by the guard at the top of createExecution, the
// same one every other creation path goes through, so this recovery path never
// spawns a runtime for a session that ended before the restart.
func (m *Manager) createExecutionFromSessionInfo(ctx context.Context, sessionID string) (*AgentExecution, error) {
	if m.workspaceInfoProvider == nil {
		return nil, fmt.Errorf("cannot restore session %s: workspace info provider not configured", sessionID)
	}

	// Get workspace info from the provider (looks up session to get taskID, workspace path, etc.)
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, "", sessionID)
	if err != nil {
		return nil, fmt.Errorf("get workspace info for session %s: %w", sessionID, err)
	}

	if info.WorkspacePath == "" {
		return nil, fmt.Errorf("%w: session %s has no workspace path configured", ErrSessionWorkspaceNotReady, sessionID)
	}

	if info.TaskID == "" {
		return nil, fmt.Errorf("session %s has no associated task ID", sessionID)
	}

	// Verify this session should use passthrough mode
	profileInfo, err := m.verifyPassthroughEnabled(ctx, sessionID, workspaceExecutionProfileID(info))
	if err != nil {
		return nil, err
	}

	// If agent ID not in workspace info (snapshot missing/empty), resolve from profile
	executionProfileID := workspaceExecutionProfileID(info)
	if info.AgentID == "" && executionProfileID != "" && m.profileResolver != nil {
		// Resolve only to backfill info.AgentID — keep the name distinct from the
		// outer profileInfo so it's clear this one is not what reaches
		// startPassthroughExecution below. Both resolve from the same profile ID,
		// so the content is identical, but avoiding the shadow keeps ownership
		// unambiguous for future readers.
		agentProfile, err := m.profileResolver.ResolveProfile(ctx, executionProfileID)
		if err != nil {
			return nil, fmt.Errorf("resolve agent for session %s: %w", sessionID, err)
		}
		info.AgentID = agentProfile.AgentName
	}

	// Create the execution
	m.logger.Info("creating execution for passthrough session",
		zap.String("task_id", info.TaskID),
		zap.String("session_id", sessionID),
		zap.String("workspace_path", info.WorkspacePath))

	execution, err := m.createExecution(ctx, info.TaskID, info)
	if err != nil {
		return nil, fmt.Errorf("create execution for session %s: %w", sessionID, err)
	}

	// createExecution derived the resume intent (applyResumeIntent): a session
	// with no prior agent execution has never run, so there is nothing for the
	// CLI's resume flag to attach to and it launches fresh (issue #2330).
	m.logger.Info("starting passthrough process for session",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID),
		zap.Bool("resumed_session", execution.isResumedSession))

	if err := m.startPassthroughExecution(ctx, execution, profileInfo); err != nil {
		return nil, fmt.Errorf("start passthrough process for session %s: %w", sessionID, err)
	}

	// Get updated execution with process ID
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil, fmt.Errorf("execution disappeared after starting passthrough session %s", sessionID)
	}

	return execution, nil
}

// applyResumeIntent marks whether a freshly built execution should launch as a
// resume, from the same PreviousExecutionID buildExecutionFromInstance uses on
// the ACP launch path (populated here from info.AgentExecutionID).
//
// An empty PreviousExecutionID means no agent execution has ever been recorded
// for this session — the state a task created with start_agent:false is in.
// startPassthroughExecution reads the flag: such a session has no CLI-side
// conversation for `-c` / `--resume` to attach to, and its stored prompt has
// never been delivered, so it must take the fresh-launch path. Only a session
// that previously ran — one whose execution was lost from the in-memory store
// by a backend restart — is a genuine recovery.
func applyResumeIntent(execution *AgentExecution, req *ExecutorCreateRequest) {
	execution.isResumedSession = req.PreviousExecutionID != ""
}

// verifyPassthroughEnabled checks if the session's profile has CLI passthrough
// enabled, returning the resolved profile so callers can reuse it for command
// building instead of resolving twice.
func (m *Manager) verifyPassthroughEnabled(ctx context.Context, sessionID, profileID string) (*AgentProfileInfo, error) {
	if m.profileResolver == nil || profileID == "" {
		return nil, fmt.Errorf("session %s has no profile configured for passthrough mode", sessionID)
	}

	profileInfo, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil {
		m.logger.Warn("failed to resolve profile for passthrough check",
			zap.String("session_id", sessionID),
			zap.String("profile_id", profileID),
			zap.Error(err))
		return nil, fmt.Errorf("session %s: failed to resolve profile %s: %w", sessionID, profileID, err)
	}

	if profileInfo == nil || !profileInfo.CLIPassthrough {
		return nil, fmt.Errorf("session %s is not configured for CLI passthrough mode", sessionID)
	}

	return profileInfo, nil
}

// createExecution creates an agentctl execution.
// The agent subprocess is NOT started - call ConfigureAgent + Start explicitly.
func (m *Manager) createExecution(ctx context.Context, taskID string, info *WorkspaceInfo) (*AgentExecution, error) {
	if info == nil {
		return nil, fmt.Errorf("workspace info is required")
	}
	// A terminal session can never gain an execution, so reject it before
	// reconciling the workspace, taking an activity lease, or creating a
	// runtime instance. User-facing panels (terminal, git, files) reconnect on
	// a timer; without this every retry paid for a full instance creation that
	// the post-creation check below tore straight back down. That check stays —
	// it guards the session that terminalizes *during* creation.
	if err := m.ensureLaunchSessionStillActive(ctx, info.SessionID); err != nil {
		return nil, err
	}
	if err := m.reconcileExecutionWorkspace(ctx, taskID, info); err != nil {
		return nil, err
	}
	activityLease, err := m.acquireActivity(ctx, activity.KindExecutionStarting)
	if err != nil {
		return nil, err
	}
	defer activityLease.Release()
	activityLease.SetKind(activity.KindExecutionPreparing)

	// Select runtime based on executor type; falls back to standalone if empty/unavailable
	rt, err := m.getExecutorBackend(info.ExecutorType)
	if err != nil {
		return nil, fmt.Errorf("no runtime configured: %w", err)
	}

	executionID := uuid.New().String()
	preparation, err := m.prepareExecutionCreateRequest(ctx, taskID, info, executionID)
	if err != nil {
		return nil, err
	}
	launchCtx, launchCancel := withLaunchPhaseTimeout(ctx)
	defer launchCancel()
	if err := resumeRemoteInstancePreflight(launchCtx, rt, preparation.request); err != nil {
		return nil, err
	}

	runtimeInstance, err := rt.CreateInstance(launchCtx, preparation.request)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	execution := m.initializeCreatedExecution(ctx, taskID, info, executionID, rt, runtimeInstance, preparation)

	if err := m.ensureLaunchSessionStillActive(ctx, info.SessionID); err != nil {
		m.rollbackLaunchExecution(ctx, rt, runtimeInstance, execution, "session ended during runtime creation")
		return nil, err
	}

	if addErr := m.executionStore.Add(execution); addErr != nil {
		// Lost a race: another path created an execution for this session
		// between our check and our Add. Roll back the runtime instance we
		// just spawned (otherwise its subprocess is orphaned) and return the
		// winner so the caller observes a single execution per session.
		if errors.Is(addErr, ErrExecutionAlreadyExistsForSession) {
			m.rollbackRacedExecution(ctx, rt, runtimeInstance, execution)
			if existing, ok := m.executionStore.GetBySessionID(info.SessionID); ok {
				return existing, nil
			}
		}
		return nil, fmt.Errorf("failed to register execution: %w", addErr)
	}
	// Persist before the final session read so concurrent deletion cleanup can
	// inventory this execution even if it started between Add and validation.
	if err := m.persistExecutorRunningResult(ctx, execution); err != nil {
		m.rollbackRegisteredLaunchAfterPersistFailure(rt, runtimeInstance, execution)
		return nil, fmt.Errorf("persist execution registration: %w", err)
	}
	if err := m.ensureLaunchSessionStillActive(ctx, info.SessionID); err != nil {
		if errors.Is(err, errTaskCleanupActive) {
			m.rollbackRegisteredLaunchForTaskCleanup(rt, runtimeInstance, execution)
		} else {
			m.rollbackRegisteredLaunch(rt, runtimeInstance, execution, "session ended during execution registration")
		}
		return nil, err
	}
	m.publishCreatedExecution(ctx, runtimeInstance, execution, executionID, taskID)

	return execution, nil
}

type executionCreatePreparation struct {
	request     *ExecutorCreateRequest
	profileInfo *AgentProfileInfo
}

type executionEnvironmentPreparation struct {
	env                   map[string]string
	approvedSecretEnvKeys []string
	managedGoCachePath    string
}

func (m *Manager) reconcileExecutionWorkspace(ctx context.Context, taskID string, info *WorkspaceInfo) error {
	owner := ownedDirectoryLinkOwner(taskID, info.TaskDirName)
	if err := reconcileWorkspaceSources(ctx, info.WorkspacePath, info.WorkspaceFolders, owner); err != nil {
		return err
	}
	if info.ExecutorType == string(models.ExecutorTypeLocal) || info.ExecutorType == "local_pc" {
		if err := reconcileWorkspaceRepositories(info.WorkspacePath, info.WorkspaceRepositories, m.logger, owner); err != nil {
			return err
		}
	}
	if info.ExecutorType == string(models.ExecutorTypeWorktree) {
		if err := m.reconcileWorkspaceWorktrees(ctx, taskID, info); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) prepareExecutionCreateRequest(
	ctx context.Context,
	taskID string,
	info *WorkspaceInfo,
	executionID string,
) (*executionCreatePreparation, error) {
	if info.AgentID == "" {
		return nil, fmt.Errorf("agent ID is required in WorkspaceInfo")
	}
	agentConfig, ok := m.registry.Get(info.AgentID)
	if !ok {
		return nil, fmt.Errorf("agent type %q not found in registry", info.AgentID)
	}

	executionProfileID := workspaceExecutionProfileID(info)
	profileInfo := m.resolveWorkspaceExecutionProfile(ctx, executionProfileID)
	envPreparation, err := m.prepareExecutionEnvironment(
		ctx, taskID, info, executionID, executionProfileID, agentConfig, profileInfo,
	)
	if err != nil {
		return nil, err
	}

	metadata := make(map[string]interface{}, len(info.Metadata)+1)
	for key, value := range info.Metadata {
		metadata[key] = value
	}
	if envPreparation.managedGoCachePath != "" {
		metadata[managedGoCacheMetadataKey] = envPreparation.managedGoCachePath
	}
	remoteContributions, err := remoteContributionsFromMetadata(metadata)
	if err != nil {
		return nil, err
	}
	contributionDestinations, err := contributionDestinationsFromMetadata(metadata)
	if err != nil {
		return nil, err
	}
	comparisonTargets, err := comparisonTargetsFromMetadata(metadata)
	if err != nil {
		return nil, err
	}
	if len(comparisonTargets) == 0 {
		comparisonTargets, err = comparisonTargetsFromWorkspaceRepositories(info.WorkspaceRepositories)
		if err != nil {
			return nil, err
		}
	}
	autoApprove := false
	var autoApproveOverride *bool
	if profileInfo != nil {
		autoApprove = profileInfo.AutoApprove
		autoApproveOverride = boolPtr(profileInfo.AutoApprove)
	}

	return &executionCreatePreparation{
		request: &ExecutorCreateRequest{
			InstanceID:                     executionID,
			TaskID:                         taskID,
			SessionID:                      info.SessionID,
			TaskEnvironmentID:              info.TaskEnvironmentID,
			AgentProfileID:                 executionProfileID,
			OfficeAgentProfileID:           info.AgentProfileID,
			WorkspacePath:                  info.WorkspacePath,
			WorkspaceSourceRoots:           workspaceSourceRoots(info.WorkspaceFolders, info.WorkspaceRepositories),
			Protocol:                       string(agentConfig.Runtime().Protocol),
			Env:                            envPreparation.env,
			AutoApprovePermissions:         autoApprove,
			AutoApprovePermissionsOverride: autoApproveOverride,
			AgentConfig:                    agentConfig,
			Metadata:                       metadata,
			ApprovedSecretEnvKeys:          append([]string(nil), envPreparation.approvedSecretEnvKeys...),
			PreviousExecutionID:            info.AgentExecutionID,
			AuthToken:                      m.revealRuntimeSecret(ctx, info.Metadata, MetadataKeyAuthTokenSecret),
			BootstrapNonce:                 m.revealRuntimeSecret(ctx, info.Metadata, MetadataKeyBootstrapNonceSecret),
			AgentctlStartupConfig:          m.agentctlStartupConfig,
			RemoteContributions:            remoteContributions,
			ContributionDestinations:       contributionDestinations,
			ComparisonTargets:              comparisonTargets,
		},
		profileInfo: profileInfo,
	}, nil
}

func (m *Manager) resolveWorkspaceExecutionProfile(ctx context.Context, profileID string) *AgentProfileInfo {
	if profileID == "" || m.profileResolver == nil {
		return nil
	}
	profileInfo, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil {
		m.logger.Warn("failed to resolve profile for workspace execution",
			zap.String("execution_profile_id", profileID),
			zap.Error(err))
		return nil
	}
	return profileInfo
}

func (m *Manager) prepareExecutionEnvironment(
	ctx context.Context,
	taskID string,
	info *WorkspaceInfo,
	executionID string,
	executionProfileID string,
	agentConfig agents.Agent,
	profileInfo *AgentProfileInfo,
) (*executionEnvironmentPreparation, error) {
	managedReq := &LaunchRequest{
		TaskID:             taskID,
		WorkspaceID:        info.WorkspaceID,
		SessionID:          info.SessionID,
		AgentProfileID:     info.AgentProfileID,
		ExecutionProfileID: executionProfileID,
		ExecutorType:       info.ExecutorType,
		Env:                make(map[string]string),
	}
	if err := m.prepareManagedGoCacheEnvironment(ctx, managedReq); err != nil {
		return nil, err
	}
	definitions, err := m.repositoryEnvironmentDefinitions(ctx, taskID, info.WorkspaceID)
	if err != nil {
		return nil, err
	}
	managedReq.EnvironmentDefinitions = append(managedReq.EnvironmentDefinitions, definitions...)
	executorDefinitions, err := m.executorProfileEnvironmentDefinitions(ctx, workspaceExecutorProfileID(info))
	if err != nil {
		return nil, err
	}
	managedReq.EnvironmentDefinitions = append(managedReq.EnvironmentDefinitions, executorDefinitions...)
	managedReq.ApprovedSecretEnvKeys = approvedSecretEnvironmentKeys(managedReq.EnvironmentDefinitions)
	managedReq.EnvironmentResolutionRequired = true
	env, err := m.buildEnvForExecution(ctx, executionID, managedReq, agentConfig, profileInfo)
	if err != nil {
		return nil, fmt.Errorf("build recovered environment: %w", err)
	}
	if len(env) == 0 {
		env = nil
	}
	return &executionEnvironmentPreparation{
		env:                   env,
		approvedSecretEnvKeys: managedReq.ApprovedSecretEnvKeys,
		managedGoCachePath:    managedReq.managedGoCachePath,
	}, nil
}

func (m *Manager) initializeCreatedExecution(
	ctx context.Context,
	taskID string,
	info *WorkspaceInfo,
	executionID string,
	rt ExecutorBackend,
	runtimeInstance *ExecutorInstance,
	preparation *executionCreatePreparation,
) *AgentExecution {
	execution := runtimeInstance.ToAgentExecution(preparation.request)
	execution.RuntimeName = rt.Name()
	// Set before executionStore.Add: once the execution is registered, a
	// concurrent EnsurePassthroughExecution can reach it, and it must never
	// observe a half-initialised resume intent.
	applyResumeIntent(execution, preparation.request)

	if info.ACPSessionID != "" {
		execution.ACPSessionID = info.ACPSessionID
	}
	_, sessionSpan := tracing.TraceSessionStart(context.Background(), taskID, info.SessionID, executionID)
	execution.SetSessionSpan(sessionSpan)
	if execution.agentctl != nil {
		execution.agentctl.SetTraceContext(execution.SessionTraceContext())
	}
	return execution
}

func (m *Manager) publishCreatedExecution(
	ctx context.Context,
	runtimeInstance *ExecutorInstance,
	execution *AgentExecution,
	executionID string,
	taskID string,
) {
	m.setRuntimeInterest(execution.SessionID, true)

	// Persist agentctl auth token only after the execution is tracked, so a
	// race-lost rollback never leaves an orphaned secret in the store.
	m.persistRuntimeSecrets(ctx, runtimeInstance, execution)
	go m.pollOneRemoteStatus(context.Background(), execution)

	// Publish Starting BEFORE spawning waitForAgentctlReady so subscribers
	// always observe Starting → Ready/Error in order. Doing it after the go
	// call would race: if Health succeeds before this line runs, Ready could
	// be published first and the frontend gate would briefly flicker.
	m.eventPublisher.PublishAgentctlEvent(ctx, events.AgentctlStarting, execution, "")
	go m.waitForAgentctlReady(execution)

	m.logger.Info("execution created",
		zap.String("execution_id", executionID),
		zap.String("task_id", taskID),
		zap.String("workspace_path", execution.WorkspacePath),
		zap.Stringer("runtime", execution.RuntimeName))
}

func (m *Manager) reconcileWorkspaceWorktrees(ctx context.Context, taskID string, info *WorkspaceInfo) error {
	if len(info.WorkspaceRepositories) == 0 || m.worktreeMgr == nil {
		return nil
	}
	if info.SessionID == "" || info.TaskDirName == "" {
		return fmt.Errorf("worktree workspace is missing durable session or task directory")
	}
	for _, repository := range info.WorkspaceRepositories {
		if repository.RepositoryPath == "" {
			return fmt.Errorf("workspace repository %q source path is missing", repository.RepoName)
		}
		if _, err := m.worktreeMgr.Create(ctx, worktree.CreateRequest{
			TaskID: taskID, SessionID: info.SessionID, RepositoryID: repository.RepositoryID,
			RepositoryPath: repository.RepositoryPath, BaseBranch: repository.BaseBranch,
			FallbackBaseBranch: repository.DefaultBranch, CheckoutBranch: repository.CheckoutBranch,
			WorktreeID: repository.WorktreeID, TaskDirName: info.TaskDirName, WorkspaceID: info.WorkspaceID,
			RepoName: repository.RepoName, WorktreeBranchPrefix: repository.WorktreeBranchPrefix,
			WorktreeBranchTemplate: repository.WorktreeBranchTemplate, PullBeforeWorktree: repository.PullBeforeWorktree,
			RemoteSyncHandled: repository.RemoteSyncHandled,
			BranchSlug:        repository.BranchSlug, BranchIdentitySlug: repository.BranchIdentitySlug,
		}); err != nil {
			return fmt.Errorf("recreate workspace worktree %q: %w", repository.RepoName, err)
		}
	}
	return nil
}

func workspaceExecutionProfileID(info *WorkspaceInfo) string {
	if info == nil {
		return ""
	}
	if info.ExecutionProfileID != "" {
		return info.ExecutionProfileID
	}
	return info.AgentProfileID
}

func workspaceExecutorProfileID(info *WorkspaceInfo) string {
	if info == nil {
		return ""
	}
	return info.ExecutorProfileID
}

// rollbackRacedExecution tears down an execution that lost a session-conflict
// race in the store. Without this the runtime instance (agentctl + agent
// subprocess if any) keeps running with no tracking entry, and no cleanup path
// will ever find it.
func (m *Manager) rollbackRacedExecution(ctx context.Context, rt ExecutorBackend, runtimeInstance *ExecutorInstance, execution *AgentExecution) {
	m.logger.Warn("rolling back duplicate execution after session-conflict race",
		zap.String("execution_id", execution.ID),
		zap.String("session_id", execution.SessionID))
	if rt != nil && runtimeInstance != nil {
		if stopErr := rt.StopInstance(ctx, runtimeInstance, true); stopErr != nil {
			m.logger.Warn("failed to stop raced runtime instance during rollback",
				zap.String("execution_id", execution.ID),
				zap.Error(stopErr))
		}
	}
	if execution.agentctl != nil {
		execution.agentctl.Close()
	}
	execution.EndSessionSpan()
}

const (
	// MetadataKeyAuthTokenSecret is the metadata key for the encrypted agentctl auth token secret ID.
	MetadataKeyAuthTokenSecret = "env_secret_id_AGENTCTL_AUTH_TOKEN"
	// MetadataKeyBootstrapNonceSecret stores the encrypted Docker bootstrap nonce.
	// It lets the backend re-handshake after a container restart starts a new
	// agentctl process with a fresh auth token.
	MetadataKeyBootstrapNonceSecret = "env_secret_id_AGENTCTL_BOOTSTRAP_NONCE"
)

func (m *Manager) persistRuntimeSecrets(ctx context.Context, instance *ExecutorInstance, execution *AgentExecution) {
	m.persistAuthToken(ctx, instance, execution)
	m.persistBootstrapNonce(ctx, instance, execution)
}

// persistAuthToken stores the agentctl handshake auth token in SecretStore
// and saves the secret ID in the execution's metadata for recovery after restart.
func (m *Manager) persistAuthToken(ctx context.Context, instance *ExecutorInstance, execution *AgentExecution) {
	m.persistRuntimeSecret(ctx, instance, execution, MetadataKeyAuthTokenSecret, "agentctl-auth", instance.AuthToken)
}

func (m *Manager) persistBootstrapNonce(ctx context.Context, instance *ExecutorInstance, execution *AgentExecution) {
	m.persistRuntimeSecret(ctx, instance, execution, MetadataKeyBootstrapNonceSecret, "agentctl-bootstrap", instance.BootstrapNonce)
}

func (m *Manager) persistRuntimeSecret(
	ctx context.Context,
	instance *ExecutorInstance,
	execution *AgentExecution,
	metadataKey string,
	secretNamePrefix string,
	value string,
) {
	if value == "" || m.secretStore == nil {
		return
	}

	secret := &secrets.SecretWithValue{
		Secret: secrets.Secret{
			Name: fmt.Sprintf("%s-%s", secretNamePrefix, truncateID(instance.InstanceID, 12)),
		},
		Value: value,
	}
	if err := m.secretStore.Create(ctx, secret); err != nil {
		m.logger.Error("failed to persist runtime secret",
			zap.String("instance_id", instance.InstanceID),
			zap.String("metadata_key", metadataKey),
			zap.Error(err))
		return
	}

	execution.setMetadataValue(metadataKey, secret.ID)

	m.logger.Debug("persisted runtime secret in secret store",
		zap.String("instance_id", instance.InstanceID),
		zap.String("metadata_key", metadataKey))
}

func (m *Manager) revealRuntimeSecret(ctx context.Context, metadata map[string]interface{}, metadataKey string) string {
	if m.secretStore == nil {
		return ""
	}
	secretID := getMetadataString(metadata, metadataKey)
	if secretID == "" {
		return ""
	}
	value, err := revealGlobalSecret(ctx, m.secretStore, secretID)
	if err != nil {
		m.logger.Warn("failed to reveal runtime secret",
			zap.String("metadata_key", metadataKey),
			zap.Error(err))
		return ""
	}
	return value
}

// truncateID safely truncates an ID string to maxLen characters.
func truncateID(id string, maxLen int) string {
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen]
}
