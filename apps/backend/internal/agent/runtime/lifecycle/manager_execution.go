package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// coalescedExecutionCreationTimeout matches the runtime's 60-second agentctl
// startup window while preventing blocked instance I/O from owning the shared
// session slot and its activity lease for the lifetime of the manager.
const coalescedExecutionCreationTimeout = time.Minute

const taskHostRuntimeSessionPrefix = "task-host-"

// ResolveSessionRuntime returns the runtime selected for a session without
// creating or resuming its execution. Session-scoped handlers can use this to
// reject unsupported runtimes before GetOrEnsureExecution starts resources.
func (m *Manager) ResolveSessionRuntime(ctx context.Context, sessionID string) (agentruntime.Runtime, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session_id is required")
	}
	if check := m.sessionAccessCheck; check != nil {
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
	if check := m.sessionAccessCheck; check != nil {
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

// GetExecutionForEnvironment returns an already-running task-host execution
// without creating or resuming resources. Authorization deliberately runs
// before the in-memory cache lookup.
func (m *Manager) GetExecutionForEnvironment(
	ctx context.Context,
	taskEnvironmentID string,
) (*AgentExecution, bool, error) {
	if taskEnvironmentID == "" {
		return nil, false, fmt.Errorf("task_environment_id is required")
	}
	if check := m.environmentAccessCheck; check != nil {
		if err := check(ctx, taskEnvironmentID); err != nil {
			return nil, false, err
		}
	}
	execution, exists := m.executionStore.GetByTaskEnvironmentID(taskEnvironmentID)
	return execution, exists, nil
}

// GetOrEnsureTaskHostForEnvironment returns the one internal task-host
// execution owned by a task environment. Session executions are deliberately
// ignored: they may stop independently while task services remain warm.
func (m *Manager) GetOrEnsureTaskHostForEnvironment(
	ctx context.Context,
	taskEnvironmentID string,
) (*AgentExecution, error) {
	if taskEnvironmentID == "" {
		return nil, fmt.Errorf("task_environment_id is required")
	}
	if check := m.environmentAccessCheck; check != nil {
		if err := check(ctx, taskEnvironmentID); err != nil {
			return nil, err
		}
	}
	if execution, exists := m.executionStore.GetTaskHostByEnvironmentID(taskEnvironmentID); exists {
		return execution, nil
	}

	value, err := m.doCoalescedExecution(ctx, taskHostRuntimeSessionPrefix+taskEnvironmentID,
		func(sharedCtx context.Context) (interface{}, error) {
			if execution, exists := m.executionStore.GetTaskHostByEnvironmentID(taskEnvironmentID); exists {
				return execution, nil
			}
			if m.workspaceInfoProvider == nil {
				return nil, fmt.Errorf("workspace info provider not configured")
			}
			info, err := m.workspaceInfoProvider.GetWorkspaceInfoForEnvironment(sharedCtx, taskEnvironmentID)
			if err != nil {
				return nil, fmt.Errorf("failed to get workspace info for environment %s: %w", taskEnvironmentID, err)
			}
			if info == nil {
				return nil, fmt.Errorf("task environment %s not found", taskEnvironmentID)
			}
			if info.TaskEnvironmentID != taskEnvironmentID {
				return nil, fmt.Errorf("workspace info resolved environment %s, want %s", info.TaskEnvironmentID, taskEnvironmentID)
			}
			if info.TaskID == "" {
				return nil, fmt.Errorf("task environment %s has no task_id", taskEnvironmentID)
			}
			if info.WorkspacePath == "" {
				return nil, fmt.Errorf("%w: task environment %s has no workspace path yet", ErrSessionWorkspaceNotReady, taskEnvironmentID)
			}
			if err := m.ensureTaskHostTaskActive(sharedCtx, info.TaskID); err != nil {
				return nil, err
			}
			return m.createTaskHostExecution(sharedCtx, info.TaskID, info)
		})
	if err != nil {
		return nil, err
	}
	return value.(*AgentExecution), nil
}

// GetTaskHostForEnvironment returns only the dedicated task-host execution.
// Authorization runs before the cache lookup.
func (m *Manager) GetTaskHostForEnvironment(
	ctx context.Context,
	taskEnvironmentID string,
) (*AgentExecution, bool, error) {
	if taskEnvironmentID == "" {
		return nil, false, fmt.Errorf("task_environment_id is required")
	}
	if check := m.environmentAccessCheck; check != nil {
		if err := check(ctx, taskEnvironmentID); err != nil {
			return nil, false, err
		}
	}
	execution, exists := m.executionStore.GetTaskHostByEnvironmentID(taskEnvironmentID)
	return execution, exists, nil
}

// StopTaskHostForEnvironment reaps the task-owned agentctl process tree without
// touching any session execution sharing the same task environment.
func (m *Manager) StopTaskHostForEnvironment(
	ctx context.Context,
	taskEnvironmentID, reason string,
) error {
	execution, exists, err := m.GetTaskHostForEnvironment(ctx, taskEnvironmentID)
	if err != nil || !exists {
		return err
	}
	return m.StopAgentWithReason(ctx, execution.ID, reason, false)
}

func (m *Manager) ensureTaskHostTaskActive(ctx context.Context, taskID string) error {
	if m.executorProfileReader == nil {
		return nil
	}
	cleanupActive, err := m.executorProfileReader.HasActiveTaskResourceCleanupJob(ctx, taskID)
	if err != nil {
		return fmt.Errorf("verify task-host cleanup admission: %w", err)
	}
	if cleanupActive {
		return fmt.Errorf("verify task-host cleanup admission: %w for task %q", errTaskCleanupActive, taskID)
	}
	return nil
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
	sharedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), coalescedExecutionCreationTimeout)
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
	if check := m.sessionAccessCheck; check != nil {
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

// createExecution creates a session-owned agentctl execution.
// The agent subprocess is NOT started - call ConfigureAgent + Start explicitly.
func (m *Manager) createExecution(ctx context.Context, taskID string, info *WorkspaceInfo) (*AgentExecution, error) {
	return m.createExecutionForPurpose(ctx, taskID, info, false)
}

// createTaskHostExecution creates the internal agentctl execution used by
// task-owned services. It does not inherit session runtime identity, resume
// state, persistence, traces, lifecycle events, or cleanup ownership.
func (m *Manager) createTaskHostExecution(
	ctx context.Context,
	taskID string,
	info *WorkspaceInfo,
) (*AgentExecution, error) {
	return m.createExecutionForPurpose(ctx, taskID, info, true)
}

func (m *Manager) createExecutionForPurpose(
	ctx context.Context,
	taskID string,
	info *WorkspaceInfo,
	isTaskHost bool,
) (*AgentExecution, error) {
	if info == nil {
		return nil, fmt.Errorf("workspace info is required")
	}
	// Session-owned executions must reject terminal sessions before workspace
	// reconciliation or runtime creation. Task hosts use task-level admission
	// and deliberately have no owning session.
	if !isTaskHost {
		if err := m.ensureLaunchSessionStillActive(ctx, info.SessionID); err != nil {
			return nil, err
		}
	}
	if err := m.prepareExecutionWorkspace(ctx, taskID, info); err != nil {
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

	plan, err := m.buildExecutionCreatePlan(ctx, taskID, info, isTaskHost)
	if err != nil {
		return nil, err
	}
	req := plan.request

	if err := resumeRemoteInstancePreflight(ctx, rt, req); err != nil {
		return nil, err
	}

	runtimeInstance, err := rt.CreateInstance(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	execution := runtimeInstance.ToAgentExecution(req)
	execution.RuntimeName = rt.Name()
	// Set before executionStore.Add: concurrent passthrough callers must not
	// observe a half-initialized session resume intent. Task-host reuse uses
	// PreviousExecutionID only as a runtime transport detail.
	if !isTaskHost {
		applyResumeIntent(execution, req)
	}
	return m.finishCreatedExecution(ctx, taskID, info, isTaskHost, plan, rt, runtimeInstance, execution)
}

func (m *Manager) prepareExecutionWorkspace(ctx context.Context, taskID string, info *WorkspaceInfo) error {
	owner := ownedDirectoryLinkOwner(taskID, info.TaskDirName)
	if err := reconcileWorkspaceSources(ctx, info.WorkspacePath, info.WorkspaceFolders, owner); err != nil {
		return err
	}
	if info.ExecutorType == string(models.ExecutorTypeLocal) || info.ExecutorType == legacyExecutorTypeLocalPC {
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

type executionCreatePlan struct {
	executionID string
	request     *ExecutorCreateRequest
	profileInfo *AgentProfileInfo
}

type executionEnvironmentPlan struct {
	env                       map[string]string
	profileInfo               *AgentProfileInfo
	executionProfileID        string
	autoApprove               bool
	autoApproveOverride       *bool
	approvedSecretEnvironment []string
	managedGoCachePath        string
}

func (m *Manager) buildExecutionCreatePlan(
	ctx context.Context,
	taskID string,
	info *WorkspaceInfo,
	isTaskHost bool,
) (*executionCreatePlan, error) {
	executionID := uuid.New().String()
	if isTaskHost {
		executionID = taskHostExecutionID(info.TaskEnvironmentID)
	}

	agentConfig, err := m.executionAgentConfig(info, isTaskHost)
	if err != nil {
		return nil, err
	}
	environment, err := m.buildExecutionEnvironmentPlan(ctx, taskID, info, isTaskHost, executionID, agentConfig)
	if err != nil {
		return nil, err
	}
	metadata := executionMetadata(info.Metadata, isTaskHost)
	if environment.managedGoCachePath != "" {
		metadata[managedGoCacheMetadataKey] = environment.managedGoCachePath
	}
	remoteContributions, err := remoteContributionsFromMetadata(metadata)
	if err != nil {
		return nil, err
	}
	runtimeDetails := m.executionRuntimeDetails(ctx, info, isTaskHost, executionID, metadata)
	protocol := ""
	if agentConfig != nil && agentConfig.Runtime() != nil {
		protocol = string(agentConfig.Runtime().Protocol)
	}
	req := &ExecutorCreateRequest{
		InstanceID:                     executionID,
		TaskID:                         taskID,
		SessionID:                      runtimeDetails.sessionID,
		TaskEnvironmentID:              info.TaskEnvironmentID,
		IsTaskHost:                     isTaskHost,
		AgentProfileID:                 environment.executionProfileID,
		OfficeAgentProfileID:           runtimeDetails.officeAgentProfileID,
		WorkspacePath:                  info.WorkspacePath,
		WorkspaceSourceRoots:           workspaceSourceRoots(info.WorkspaceFolders, info.WorkspaceRepositories),
		Protocol:                       protocol,
		Env:                            environment.env,
		AutoApprovePermissions:         environment.autoApprove,
		AutoApprovePermissionsOverride: environment.autoApproveOverride,
		AgentConfig:                    agentConfig,
		Metadata:                       metadata,
		ApprovedSecretEnvKeys:          append([]string(nil), environment.approvedSecretEnvironment...),
		RemoteContributions:            remoteContributions,
		PreviousExecutionID:            runtimeDetails.previousExecutionID,
		AuthToken:                      runtimeDetails.authToken,
		BootstrapNonce:                 runtimeDetails.bootstrapNonce,
	}
	return &executionCreatePlan{executionID: executionID, request: req, profileInfo: environment.profileInfo}, nil
}

func (m *Manager) buildExecutionEnvironmentPlan(
	ctx context.Context,
	taskID string,
	info *WorkspaceInfo,
	isTaskHost bool,
	executionID string,
	agentConfig agents.Agent,
) (*executionEnvironmentPlan, error) {
	profileInfo, executionProfileID := m.executionProfile(ctx, info, isTaskHost)
	managedAgentProfileID := info.AgentProfileID
	managedSessionID := info.SessionID
	if isTaskHost {
		managedAgentProfileID = ""
		managedSessionID = taskHostRuntimeSessionPrefix + info.TaskEnvironmentID
	}
	managedReq := &LaunchRequest{
		TaskID:             taskID,
		WorkspaceID:        info.WorkspaceID,
		SessionID:          managedSessionID,
		AgentProfileID:     managedAgentProfileID,
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
	plan := &executionEnvironmentPlan{
		env: env, profileInfo: profileInfo, executionProfileID: executionProfileID,
		approvedSecretEnvironment: managedReq.ApprovedSecretEnvKeys,
		managedGoCachePath:        managedReq.managedGoCachePath,
	}
	if profileInfo != nil {
		plan.autoApprove = profileInfo.AutoApprove
		plan.autoApproveOverride = boolPtr(profileInfo.AutoApprove)
	}
	if len(plan.env) == 0 {
		plan.env = nil
	}
	return plan, nil
}

func (m *Manager) executionAgentConfig(info *WorkspaceInfo, isTaskHost bool) (agents.Agent, error) {
	if isTaskHost {
		return nil, nil
	}
	if info.AgentID == "" {
		return nil, fmt.Errorf("agent ID is required in WorkspaceInfo")
	}
	agentConfig, ok := m.registry.Get(info.AgentID)
	if !ok {
		return nil, fmt.Errorf("agent type %q not found in registry", info.AgentID)
	}
	return agentConfig, nil
}

func (m *Manager) executionProfile(
	ctx context.Context,
	info *WorkspaceInfo,
	isTaskHost bool,
) (*AgentProfileInfo, string) {
	profileID := workspaceExecutionProfileID(info)
	if isTaskHost {
		return nil, ""
	}
	if profileID == "" || m.profileResolver == nil {
		return nil, profileID
	}
	profile, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil {
		m.logger.Warn("failed to resolve profile for workspace execution",
			zap.String("execution_profile_id", profileID), zap.Error(err))
		return nil, profileID
	}
	return profile, profileID
}

type executionRuntimeIdentity struct {
	sessionID            string
	previousExecutionID  string
	officeAgentProfileID string
	authToken            string
	bootstrapNonce       string
}

func (m *Manager) executionRuntimeDetails(
	ctx context.Context,
	info *WorkspaceInfo,
	isTaskHost bool,
	executionID string,
	metadata map[string]interface{},
) executionRuntimeIdentity {
	details := executionRuntimeIdentity{
		sessionID: info.SessionID, previousExecutionID: info.AgentExecutionID,
		officeAgentProfileID: info.AgentProfileID,
	}
	if !isTaskHost {
		details.authToken = m.revealRuntimeSecret(ctx, info.Metadata, MetadataKeyAuthTokenSecret)
		details.bootstrapNonce = m.revealRuntimeSecret(ctx, info.Metadata, MetadataKeyBootstrapNonceSecret)
		return details
	}
	details.sessionID = taskHostRuntimeSessionPrefix + info.TaskEnvironmentID
	details.previousExecutionID = ""
	details.officeAgentProfileID = ""
	metadata["task_host"] = true
	if info.ExecutorType == string(models.ExecutorTypeLocalDocker) {
		// A synthetic instance inside the existing task container avoids
		// reusing a session-owned agentctl while retaining container reconnect.
		details.previousExecutionID = executionID
		details.authToken = m.revealRuntimeSecret(ctx, info.Metadata, MetadataKeyAuthTokenSecret)
		details.bootstrapNonce = m.revealRuntimeSecret(ctx, info.Metadata, MetadataKeyBootstrapNonceSecret)
	}
	return details
}

func (m *Manager) finishCreatedExecution(
	ctx context.Context,
	taskID string,
	info *WorkspaceInfo,
	isTaskHost bool,
	plan *executionCreatePlan,
	rt ExecutorBackend,
	runtimeInstance *ExecutorInstance,
	execution *AgentExecution,
) (*AgentExecution, error) {
	if err := m.prepareCreatedExecution(ctx, taskID, info, isTaskHost, plan, rt, runtimeInstance, execution); err != nil {
		return nil, err
	}
	winner, registered, err := m.registerCreatedExecution(ctx, info, rt, runtimeInstance, execution)
	if err != nil || !registered {
		return winner, err
	}
	if isTaskHost {
		return m.finishTaskHostExecution(ctx, taskID, info, plan.executionID, rt, runtimeInstance, execution)
	}
	return m.finishSessionExecution(ctx, taskID, info, plan.executionID, rt, runtimeInstance, execution)
}

func (m *Manager) prepareCreatedExecution(
	ctx context.Context,
	taskID string,
	info *WorkspaceInfo,
	isTaskHost bool,
	plan *executionCreatePlan,
	rt ExecutorBackend,
	runtimeInstance *ExecutorInstance,
	execution *AgentExecution,
) error {
	// Cache only agent-profile values for the best-effort configure fallback.
	// The effective runtime snapshot (including repository secrets) is already
	// captured by ToAgentExecution and must not be mislabeled as profile data.
	if plan.profileInfo != nil && len(plan.profileInfo.EnvVars) > 0 {
		m.cacheResolvedProfileEnv(execution, m.resolveAgentProfileEnvVars(ctx, plan.profileInfo.EnvVars))
	}
	if isTaskHost {
		return nil
	}
	execution.ACPSessionID = info.ACPSessionID
	_, sessionSpan := tracing.TraceSessionStart(
		context.Background(), taskID, info.SessionID, plan.executionID,
	)
	execution.SetSessionSpan(sessionSpan)
	if execution.agentctl != nil {
		execution.agentctl.SetTraceContext(execution.SessionTraceContext())
	}
	if err := m.ensureLaunchSessionStillActive(ctx, info.SessionID); err != nil {
		m.rollbackLaunchExecution(ctx, rt, runtimeInstance, execution, "session ended during runtime creation")
		return err
	}
	return nil
}

func (m *Manager) registerCreatedExecution(
	ctx context.Context,
	info *WorkspaceInfo,
	rt ExecutorBackend,
	runtimeInstance *ExecutorInstance,
	execution *AgentExecution,
) (*AgentExecution, bool, error) {
	addErr := m.executionStore.Add(execution)
	if addErr == nil {
		return execution, true, nil
	}
	// Lost a race after instance creation. Reap the loser and return the
	// already-registered task-host or session winner.
	if errors.Is(addErr, ErrExecutionAlreadyExistsForSession) {
		m.rollbackRacedExecution(ctx, rt, runtimeInstance, execution)
		if existing, ok := m.executionStore.GetBySessionID(info.SessionID); ok {
			return existing, false, nil
		}
	}
	if errors.Is(addErr, ErrExecutionAlreadyExistsForTaskHost) {
		m.rollbackRacedExecution(ctx, rt, runtimeInstance, execution)
		if existing, ok := m.executionStore.GetTaskHostByEnvironmentID(info.TaskEnvironmentID); ok {
			return existing, false, nil
		}
	}
	return nil, false, fmt.Errorf("failed to register execution: %w", addErr)
}

func (m *Manager) finishTaskHostExecution(
	ctx context.Context,
	taskID string,
	info *WorkspaceInfo,
	executionID string,
	rt ExecutorBackend,
	runtimeInstance *ExecutorInstance,
	execution *AgentExecution,
) (*AgentExecution, error) {
	if err := m.ensureTaskHostTaskActive(ctx, taskID); err != nil {
		m.rollbackTaskHostExecution(rt, runtimeInstance, execution, "task cleanup won task-host registration")
		return nil, err
	}
	if execution.agentctl == nil {
		m.rollbackTaskHostExecution(rt, runtimeInstance, execution, "task host has no control client")
		return nil, fmt.Errorf("task-host execution has no agentctl client")
	}
	if err := execution.agentctl.WaitForReady(ctx, coalescedExecutionCreationTimeout); err != nil {
		m.rollbackTaskHostExecution(rt, runtimeInstance, execution, "task host did not become ready")
		return nil, fmt.Errorf("task-host agentctl not ready: %w", err)
	}
	execution.MarkAgentctlReady()
	m.logger.Info("task-host execution created",
		zap.String("execution_id", executionID),
		zap.String("task_id", taskID),
		zap.String("task_environment_id", info.TaskEnvironmentID),
		zap.Stringer("runtime", execution.RuntimeName))
	return execution, nil
}

func (m *Manager) finishSessionExecution(
	ctx context.Context,
	taskID string,
	info *WorkspaceInfo,
	executionID string,
	rt ExecutorBackend,
	runtimeInstance *ExecutorInstance,
	execution *AgentExecution,
) (*AgentExecution, error) {
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
		zap.String("workspace_path", info.WorkspacePath),
		zap.Stringer("runtime", execution.RuntimeName))

	return execution, nil
}

func taskHostExecutionID(taskEnvironmentID string) string {
	return uuid.NewSHA1(
		uuid.NameSpaceOID,
		[]byte("kandev/task-host/"+taskEnvironmentID),
	).String()
}

func executionMetadata(source map[string]interface{}, taskHost bool) map[string]interface{} {
	metadata := make(map[string]interface{}, len(source)+1)
	for key, value := range source {
		if taskHost && (strings.HasPrefix(key, "env_secret_id_") || IsSessionScopedMetadataKey(key)) {
			continue
		}
		metadata[key] = value
	}
	return metadata
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
			BranchSlug: repository.BranchSlug, BranchIdentitySlug: repository.BranchIdentitySlug,
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

func (m *Manager) rollbackTaskHostExecution(
	rt ExecutorBackend,
	runtimeInstance *ExecutorInstance,
	execution *AgentExecution,
	reason string,
) {
	m.logger.Warn("rolling back task-host execution",
		zap.String("execution_id", execution.ID),
		zap.String("task_environment_id", execution.TaskEnvironmentID),
		zap.String("reason", reason))
	m.executionStore.Remove(execution.ID)
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if rt != nil && runtimeInstance != nil {
		if err := rt.StopInstance(cleanupCtx, runtimeInstance, true); err != nil {
			m.logger.Warn("failed to stop task-host runtime during rollback",
				zap.String("execution_id", execution.ID),
				zap.Error(err))
		}
	}
	if execution.agentctl != nil {
		execution.agentctl.Close()
	}
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

	if execution.Metadata == nil {
		execution.Metadata = make(map[string]interface{})
	}
	execution.Metadata[metadataKey] = secret.ID

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
