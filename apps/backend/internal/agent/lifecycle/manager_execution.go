package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/tracing"
	"github.com/kandev/kandev/internal/common/appctx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/secrets"
)

// ErrSessionWorkspaceNotReady indicates the task session exists but does not yet
// have a resolved workspace path (typically while worktree preparation is in progress).
var ErrSessionWorkspaceNotReady = errors.New("session workspace not ready")

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

	// Fast path: execution already in memory
	if execution, exists := m.executionStore.GetBySessionID(sessionID); exists {
		return execution, nil
	}

	// Slow path: create on-demand, deduplicated by sessionID-keyed singleflight.
	// Use ensureWorkspaceExecutionLocked (not EnsureWorkspaceExecutionForSession)
	// to avoid recursing into the same singleflight slot we already hold.
	v, err, _ := m.ensureExecutionGroup.Do(sessionID, func() (interface{}, error) {
		return m.ensureWorkspaceExecutionLocked(ctx, "", sessionID)
	})
	if err != nil {
		return nil, err
	}
	return v.(*AgentExecution), nil
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
	v, err, _ := m.ensureExecutionGroup.Do(info.SessionID, func() (interface{}, error) {
		if execution, exists := m.executionStore.GetBySessionID(info.SessionID); exists {
			return execution, nil
		}
		if execution, exists := m.executionStore.GetByTaskEnvironmentID(taskEnvironmentID); exists {
			return execution, nil
		}
		// createExecution publishes AgentctlStarting before spawning the
		// waitForAgentctlReady goroutine, so frontend gates flip out of
		// `undefined` even on this lazy-create path.
		execution, err := m.createExecution(ctx, info.TaskID, info)
		if err != nil {
			return nil, err
		}
		return execution, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*AgentExecution), nil
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

	v, err, _ := m.ensureExecutionGroup.Do(sessionID, func() (interface{}, error) {
		return m.ensureWorkspaceExecutionLocked(ctx, taskID, sessionID)
	})
	if err != nil {
		return nil, err
	}
	return v.(*AgentExecution), nil
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
			go m.streamManager.connectWorkspaceStream(execution, nil)
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
	return "", fmt.Errorf("no execution found for session %s", sessionID)
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
// when no execution exists (e.g., backend restarted and execution store was cleared).
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
	if err := m.verifyPassthroughEnabled(ctx, sessionID, info.AgentProfileID); err != nil {
		return nil, err
	}

	// If agent ID not in workspace info (snapshot missing/empty), resolve from profile
	if info.AgentID == "" && info.AgentProfileID != "" && m.profileResolver != nil {
		profileInfo, err := m.profileResolver.ResolveProfile(ctx, info.AgentProfileID)
		if err != nil {
			return nil, fmt.Errorf("resolve agent for session %s: %w", sessionID, err)
		}
		info.AgentID = profileInfo.AgentName
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

	// Start the passthrough process using resume command (recovery after restart)
	m.logger.Info("starting passthrough process for session",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID))

	if err := m.ResumePassthroughSession(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("start passthrough process for session %s: %w", sessionID, err)
	}

	// Get updated execution with process ID
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil, fmt.Errorf("execution disappeared after starting passthrough session %s", sessionID)
	}

	return execution, nil
}

// verifyPassthroughEnabled checks if the session's profile has CLI passthrough enabled.
func (m *Manager) verifyPassthroughEnabled(ctx context.Context, sessionID, profileID string) error {
	if m.profileResolver == nil || profileID == "" {
		return fmt.Errorf("session %s has no profile configured for passthrough mode", sessionID)
	}

	profileInfo, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil {
		m.logger.Warn("failed to resolve profile for passthrough check",
			zap.String("session_id", sessionID),
			zap.String("profile_id", profileID),
			zap.Error(err))
		return fmt.Errorf("session %s: failed to resolve profile %s: %w", sessionID, profileID, err)
	}

	if profileInfo == nil || !profileInfo.CLIPassthrough {
		return fmt.Errorf("session %s is not configured for CLI passthrough mode", sessionID)
	}

	return nil
}

// createExecution creates an agentctl execution.
// The agent subprocess is NOT started - call ConfigureAgent + Start explicitly.
func (m *Manager) createExecution(ctx context.Context, taskID string, info *WorkspaceInfo) (*AgentExecution, error) {
	// Select runtime based on executor type; falls back to standalone if empty/unavailable
	rt, err := m.getExecutorBackend(info.ExecutorType)
	if err != nil {
		return nil, fmt.Errorf("no runtime configured: %w", err)
	}

	if info.AgentID == "" {
		return nil, fmt.Errorf("agent ID is required in WorkspaceInfo")
	}

	executionID := uuid.New().String()

	agentConfig, ok := m.registry.Get(info.AgentID)
	if !ok {
		return nil, fmt.Errorf("agent type %q not found in registry", info.AgentID)
	}

	req := &ExecutorCreateRequest{
		InstanceID:          executionID,
		TaskID:              taskID,
		SessionID:           info.SessionID,
		TaskEnvironmentID:   info.TaskEnvironmentID,
		AgentProfileID:      info.AgentProfileID,
		WorkspacePath:       info.WorkspacePath,
		Protocol:            string(agentConfig.Runtime().Protocol),
		AgentConfig:         agentConfig,
		Metadata:            info.Metadata,
		PreviousExecutionID: info.AgentExecutionID,
		AuthToken:           m.revealRuntimeSecret(ctx, info.Metadata, MetadataKeyAuthTokenSecret),
		BootstrapNonce:      m.revealRuntimeSecret(ctx, info.Metadata, MetadataKeyBootstrapNonceSecret),
	}

	runtimeInstance, err := rt.CreateInstance(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	execution := runtimeInstance.ToAgentExecution(req)
	execution.RuntimeName = string(rt.Name())

	// Set the ACP session ID for session resumption
	if info.ACPSessionID != "" {
		execution.ACPSessionID = info.ACPSessionID
	}

	// Create trace span for workspace-only execution
	_, sessionSpan := tracing.TraceSessionStart(
		context.Background(), taskID, info.SessionID, executionID,
	)
	execution.SetSessionSpan(sessionSpan)
	if execution.agentctl != nil {
		execution.agentctl.SetTraceContext(execution.SessionTraceContext())
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

	// Persist executors_running row in lockstep with the in-memory Add so the
	// DB never holds an execution_id the store doesn't know about. This is the
	// structural fix for the divergence bug — pre-refactor, the orchestrator
	// wrote the row later via a full-row UPDATE that could race with the store.
	m.persistExecutorRunning(ctx, execution)

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
		zap.String("runtime", execution.RuntimeName))

	return execution, nil
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
		if stopErr := rt.StopInstance(ctx, runtimeInstance, false); stopErr != nil {
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

	if execution.Metadata == nil {
		execution.Metadata = make(map[string]interface{})
	}
	execution.Metadata[metadataKey] = secret.ID

	m.logger.Debug("persisted runtime secret in secret store",
		zap.String("instance_id", instance.InstanceID),
		zap.String("metadata_key", metadataKey),
		zap.String("secret_id", secret.ID))
}

func (m *Manager) revealRuntimeSecret(ctx context.Context, metadata map[string]interface{}, metadataKey string) string {
	if m.secretStore == nil {
		return ""
	}
	secretID := getMetadataString(metadata, metadataKey)
	if secretID == "" {
		return ""
	}
	value, err := m.secretStore.Reveal(ctx, secretID)
	if err != nil {
		m.logger.Warn("failed to reveal runtime secret",
			zap.String("metadata_key", metadataKey),
			zap.String("secret_id", secretID),
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
