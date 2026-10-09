package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/common/appctx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

const defaultRemoteContributionPreflightTimeout = 2 * time.Minute

func (m *Manager) contributionPreflightTimeout() time.Duration {
	if m.remoteContributionPreflightTimeout > 0 {
		return m.remoteContributionPreflightTimeout
	}
	return defaultRemoteContributionPreflightTimeout
}

// startPassthroughExecution dispatches a passthrough-routed execution to the
// resume or fresh launch path. profileInfo may be nil — see routePassthrough's
// contract.
func (m *Manager) startPassthroughExecution(ctx context.Context, execution *AgentExecution, profileInfo *AgentProfileInfo) error {
	if execution.isResumedSession {
		// Resume reuses the launched session id; ResumePassthroughSession does
		// its own profile lookup for command building.
		return m.ResumePassthroughSession(ctx, execution.SessionID)
	}
	if profileInfo == nil {
		// Snapshot-driven routing leaves profileInfo nil so we resolve fresh
		// here for command building.
		if execution.AgentProfileID == "" || m.profileResolver == nil {
			return fmt.Errorf("execution %q is passthrough but has no resolvable profile", execution.ID)
		}
		resolved, err := m.profileResolver.ResolveProfile(ctx, execution.AgentProfileID)
		if err != nil {
			return fmt.Errorf("resolve profile for passthrough execution %q: %w", execution.ID, err)
		}
		profileInfo = resolved
	}
	if err := validatePassthroughProvider(profileInfo); err != nil {
		return err
	}
	return m.startPassthroughSession(ctx, execution, profileInfo)
}

// routePassthrough decides whether an execution should launch on the passthrough
// PTY path or the ACP path. The session-creation snapshot
// (execution.IsPassthrough) is authoritative for session-backed launches.
// Sessionless launches (legacy controller.LaunchAgent) fall back to live profile
// state so first-time launches reflect the current mode. A profile resolve
// failure on the fallback path is treated as "not passthrough" — callers that
// need profile info for command building handle the resolve error themselves.
//
// When the sessionless fallback path resolved the profile, the resolved
// *AgentProfileInfo is returned so the caller can reuse it for command building
// without a second round-trip. Snapshot-driven routing returns nil for the
// profile because the caller must resolve fresh — the snapshot deliberately
// outlives any particular live profile state.
func (m *Manager) routePassthrough(ctx context.Context, execution *AgentExecution) (bool, *AgentProfileInfo) {
	if execution.IsPassthrough {
		return true, nil
	}
	if execution.SessionID != "" || execution.AgentProfileID == "" || m.profileResolver == nil {
		return false, nil
	}
	profileInfo, err := m.profileResolver.ResolveProfile(ctx, execution.AgentProfileID)
	if err != nil || profileInfo == nil {
		return false, nil
	}
	if !profileInfo.CLIPassthrough {
		return false, nil
	}
	return true, profileInfo
}

// StartAgentProcess configures and starts the agent subprocess for an execution.
// This must be called after Launch() to actually start the agent (e.g., auggie, codex).
// The command is built internally based on the execution's agent profile.
func (m *Manager) StartAgentProcess(ctx context.Context, executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	if execution.SessionID == "" {
		return m.startAgentProcess(ctx, executionID)
	}
	_, err := m.doCoalescedExecution(ctx, execution.SessionID, func(sharedCtx context.Context) (interface{}, error) {
		return nil, m.startAgentProcess(sharedCtx, executionID)
	})
	return err
}

func (m *Manager) startAgentProcess(ctx context.Context, executionID string) (retErr error) {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	startupDisposition := execution.startupDispositionSnapshot()
	defer func() {
		if retErr != nil {
			_, _, onInitialPromptFailure := execution.takeInitialPromptDispatchCallbacks()
			if onInitialPromptFailure != nil {
				onInitialPromptFailure()
			}
			if startupDisposition == AgentStartupReattachedExisting {
				retErr = &AgentReattachmentFailure{
					ExecutionID:     execution.ID,
					SessionID:       execution.SessionID,
					ResumeAttemptID: ResumeAttemptIDFromContext(ctx),
					Cause:           retErr,
				}
			}
		}
		retErr = wrapBootstrapFailure(execution, retErr)
	}()
	if err := execution.contextResetAdmissionError(); err != nil {
		return err
	}
	if err := m.admitExecutionOwner(ctx, &LaunchRequest{
		Owner:          execution.Owner,
		OwnerAdmission: execution.OwnerAdmission,
	}); err != nil {
		return err
	}
	if err := m.ensureLaunchSessionStillActive(ctx, execution.SessionID, executionAdmissionAgent); err != nil {
		return err
	}
	activityClaim, err := m.ensureExecutionActivity(ctx, executionID, activity.KindExecutionPreparing)
	if err != nil {
		return err
	}
	operationCtx := activityClaim.Context(ctx)
	operationRelease, err := execution.acquireContextResetOperation(operationCtx)
	if err != nil {
		activityClaim.Release()
		return err
	}
	defer operationRelease()
	defer func() {
		if retErr != nil {
			activityClaim.Release()
			return
		}
		activityClaim.Commit()
	}()

	// Decide passthrough vs ACP. The session-creation snapshot
	// (execution.IsPassthrough, sourced from TaskSession.IsPassthrough) is
	// the source of truth: a profile that toggles CLIPassthrough after the
	// session was created must not strand existing sessions in the wrong
	// launch path. Non-session launches (e.g. the low-level
	// controller.LaunchAgent path) leave IsPassthrough false and fall back to
	// live profile resolution so first-time launches still pick the current
	// mode.
	//
	// Profile resolution is needed only for *command building* in the
	// non-resume passthrough path; it is not part of the routing decision. A
	// transient resolve error must not silently route a passthrough session
	// to ACP — the resume branch routes on the snapshot alone, and the fresh
	// branch surfaces the resolve error explicitly.
	isPassthrough, profileInfo := m.routePassthrough(operationCtx, execution)
	if err := context.Cause(operationCtx); err != nil {
		return err
	}
	if isPassthrough {
		return m.startPassthroughExecution(operationCtx, execution, profileInfo)
	}
	execution.beginStartupAttemptWithID(ResumeAttemptIDFromContext(operationCtx))
	client, releaseClient := execution.AcquireAgentCtlClient()
	releaseClient()
	if client == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}

	// The executor selected this instance because its existing process already
	// owns the task session. The immutable disposition fences all later cleanup.
	reuseExisting := startupDisposition == AgentStartupReattachedExisting

	if !reuseExisting && execution.AgentCommand == "" {
		return fmt.Errorf("execution %q has no agent command configured", executionID)
	}

	// Wait for agentctl to be ready
	client, releaseClient = execution.AcquireAgentCtlClient()
	if client == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}
	err = client.WaitForReady(operationCtx, 60*time.Second)
	releaseClient()
	if err != nil {
		m.updateExecutionError(executionID, "agentctl not ready: "+err.Error())
		return fmt.Errorf("agentctl not ready: %w", err)
	}
	var taskDescription, agentDisplayName string
	if !reuseExisting {
		err = m.preflightRemoteContributionPushes(operationCtx, execution)
		if err != nil {
			m.updateExecutionError(executionID, "contribution push preflight failed: "+err.Error())
			return err
		}
		taskDescription = getTaskDescriptionFromMetadata(execution)
		agentDisplayName = m.resolveAgentDisplayName(operationCtx, execution)
	}

	execution.remoteInstanceLifecycleMu.Lock()
	if err := m.admitExecutionOwner(operationCtx, &LaunchRequest{
		Owner:          execution.Owner,
		OwnerAdmission: execution.OwnerAdmission,
	}); err != nil {
		execution.remoteInstanceLifecycleMu.Unlock()
		return err
	}
	if err := m.ensureLaunchSessionStillActive(operationCtx, execution.SessionID, executionAdmissionAgent); err != nil {
		execution.remoteInstanceLifecycleMu.Unlock()
		return err
	}
	var bootCommand string
	if reuseExisting {
		m.logger.Info("reattaching to existing agent session without ACP initialization",
			zap.String("execution_id", executionID),
			zap.String("task_id", execution.TaskID))
		err := m.adoptExistingAgentSession(operationCtx, execution, client)
		execution.remoteInstanceLifecycleMu.Unlock()
		return err
	} else {
		m.logger.Info("StartAgentProcess: starting subprocess",
			zap.String("execution_id", executionID),
			zap.String("task_id", execution.TaskID),
			zap.String("agent_command", execution.AgentCommand),
			zap.String("acp_session_id", execution.ACPSessionID))

		var err error
		bootCommand, err = m.configureAndStartAgent(operationCtx, execution)
		if err != nil {
			execution.remoteInstanceLifecycleMu.Unlock()
			return routingerr.NewAgentStartupFailure(routingerr.PhaseProcessStart, execution.AgentID, err)
		}

		m.logger.Info("agent process started",
			zap.String("execution_id", executionID),
			zap.String("task_id", execution.TaskID),
			zap.String("command", bootCommand))
	}
	execution.remoteInstanceLifecycleMu.Unlock()

	if err := m.initializeAgentSession(operationCtx, execution, bootCommand, agentDisplayName, taskDescription); err != nil {
		return routingerr.NewAgentStartupFailure(routingerr.PhaseSessionInit, execution.AgentID, err)
	}
	return nil
}

func (m *Manager) preflightRemoteContributionPushes(ctx context.Context, execution *AgentExecution) error {
	preflightCtx, cancel := context.WithTimeout(ctx, m.contributionPreflightTimeout())
	defer cancel()

	if execution == nil {
		return nil
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return nil
	}
	bindings, err := remoteContributionsFromMetadata(execution.MetadataSnapshot())
	if err != nil {
		return err
	}
	if len(bindings) == 0 {
		return nil
	}
	keys := make([]string, 0, len(bindings))
	for key := range bindings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result, err := client.GitPushPreflight(preflightCtx, key, agentctl.PushOptions{})
		if err != nil {
			return &BootstrapFailure{
				Operation: bootstrapOperation(execution),
				Code:      classifyBootstrapCause(err),
				Detail:    bootstrapFailureDetail(classifyBootstrapCause(err)),
				Cause:     fmt.Errorf("repository %q: %w", contributionPreflightDisplayLabel(key), err),
			}
		}
		if result == nil || !result.Success {
			if execution.isResumedSession && result != nil &&
				result.PreflightReason == agentctl.GitPushPreflightHistoryUpdateRequired {
				continue
			}
			message := "remote contribution push is not writable"
			code := models.AgentErrorCauseCodeUnknown
			if result != nil && result.Error != "" {
				message = result.Error
				code = classifyBootstrapCauseCode(result.ErrorCode)
			}
			return &BootstrapFailure{
				Operation: bootstrapOperation(execution),
				Code:      code,
				Detail:    bootstrapFailureDetail(code),
				Cause:     fmt.Errorf("repository %q: %s", contributionPreflightDisplayLabel(key), message),
			}
		}
	}
	return nil
}

func classifyBootstrapCauseCode(code string) string {
	if isKnownBootstrapCauseCode(code) {
		return code
	}
	return models.AgentErrorCauseCodeUnknown
}

func classifyBootstrapCause(err error) string {
	if err == nil {
		return models.AgentErrorCauseCodeUnknown
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return models.AgentErrorCauseCodeTimeout
	}
	normalized := strings.ToLower(err.Error())
	switch {
	case strings.Contains(normalized, "authentication"), strings.Contains(normalized, "login"):
		return models.AgentErrorCauseCodeAuthenticationRequired
	case strings.Contains(normalized, "permission denied"), strings.Contains(normalized, "access denied"):
		return models.AgentErrorCauseCodePermissionDenied
	case strings.Contains(normalized, "destination"), strings.Contains(normalized, "push url"):
		return models.AgentErrorCauseCodeDestinationInvalid
	case strings.Contains(normalized, "could not resolve host"), strings.Contains(normalized, "unable to access"), strings.Contains(normalized, "connection refused"):
		return models.AgentErrorCauseCodeTransportUnavailable
	default:
		return models.AgentErrorCauseCodeUnknown
	}
}

func contributionPreflightDisplayLabel(key string) string {
	if strings.TrimSpace(key) == "" {
		return "default repository"
	}
	return key
}

func cloneBootMessageForUpdate(message *models.Message) *models.Message {
	clone := *message
	clone.Metadata = make(map[string]interface{}, len(message.Metadata))
	for key, value := range message.Metadata {
		clone.Metadata[key] = value
	}
	return &clone
}

func (m *Manager) updateBootMessage(
	execution *AgentExecution,
	message *models.Message,
	final bool,
	update func(*models.Message),
) {
	if message == nil || m.bootMessageService == nil {
		return
	}
	if execution != nil {
		execution.bootMessageMu.Lock()
		defer execution.bootMessageMu.Unlock()
		if execution.bootMessageFinalized && !final {
			return
		}
		if final {
			execution.bootMessageFinalized = true
		}
	}
	if message.Metadata == nil {
		message.Metadata = make(map[string]interface{})
	}
	update(message)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.bootMessageService.UpdateMessage(ctx, cloneBootMessageForUpdate(message)); err != nil {
		if final {
			m.logger.Warn("failed to update agent boot message",
				zap.String("message_id", message.ID),
				zap.Error(err))
		} else {
			m.logger.Debug("failed to update agent boot message",
				zap.String("message_id", message.ID),
				zap.Error(err))
		}
	}
}

func (m *Manager) updateBootMessageStartupRetryProgress(execution *AgentExecution, message *models.Message) {
	m.updateBootMessage(execution, message, false, func(message *models.Message) {
		message.Metadata["startup_retrying"] = true
		message.Metadata["startup_retry_attempt"] = 2
		message.Metadata["startup_retry_max_attempts"] = 2
	})
}

// pollAgentStderr polls the agent's stderr buffer every 2 seconds and updates the boot message.
func (m *Manager) pollAgentStderr(execution *AgentExecution, msg *models.Message, stopCh chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastLineCount int

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			baseCtx := execution.SessionTraceContext()
			ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
			client, releaseClient := execution.AcquireAgentCtlClient()
			if client == nil {
				cancel()
				continue
			}
			lines, err := client.GetAgentStderr(ctx)
			releaseClient()
			cancel()
			if err != nil {
				m.logger.Debug("failed to poll agent stderr", zap.Error(err))
				continue
			}

			if len(lines) > lastLineCount {
				lastLineCount = len(lines)
				content := strings.Join(lines, "\n")
				m.updateBootMessage(execution, msg, false, func(message *models.Message) {
					message.Content = content
				})
			}
		}
	}
}

// finalizeBootMessage stops the polling goroutine and updates the boot message with final status.
func (m *Manager) finalizeBootMessage(execution *AgentExecution, msg *models.Message, stopCh chan struct{}, status string) {
	if msg == nil || m.bootMessageService == nil {
		return
	}

	// Stop the polling goroutine
	if stopCh != nil {
		close(stopCh)
	}

	// Final stderr fetch
	if execution != nil {
		baseCtx := execution.SessionTraceContext()
		ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
		client, releaseClient := execution.AcquireAgentCtlClient()
		var lines []string
		var err error
		stderrSnapshotAvailable := client != nil
		if client != nil {
			lines, err = client.GetAgentStderr(ctx)
			releaseClient()
		}
		cancel()
		if stderrSnapshotAvailable && err == nil {
			content := strings.Join(lines, "\n")
			m.updateBootMessage(execution, msg, true, func(message *models.Message) {
				message.Content = content
			})
		}
	}

	m.updateBootMessage(execution, msg, true, func(message *models.Message) {
		message.Metadata["status"] = status
		message.Metadata["completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		delete(message.Metadata, "startup_retrying")
		if status == containerStateExited {
			message.Metadata["exit_code"] = 0
		}
	})
}

// buildEnvForExecution builds environment variables for any runtime.
// This is the unified method used by the runtime interface.
func (m *Manager) buildEnvForExecution(ctx context.Context, executionID string, req *LaunchRequest, agentConfig agents.Agent, profileInfo *AgentProfileInfo) (map[string]string, error) {
	if req.EnvironmentFinalized {
		env := cloneStringMap(req.Env)
		if err := spillLargeWakePayloadEnv(env, req.WorkspacePath, m.logger.Zap()); err != nil {
			return nil, err
		}
		return env, nil
	}
	if req.EnvironmentResolutionRequired {
		env, err := m.resolveStrictEnvironment(ctx, executionID, req, agentConfig, profileInfo)
		if err != nil {
			return nil, err
		}
		if err := spillLargeWakePayloadEnv(env, req.WorkspacePath, m.logger.Zap()); err != nil {
			return nil, err
		}
		return env, nil
	}

	env := make(map[string]string)

	// Copy request environment
	for k, v := range req.Env {
		env[k] = v
	}

	// partial=true: a broken secret reference on one profile env var drops that
	// var (with a warn log) rather than blanking the agent's whole environment
	// at session launch (AC-004.1). Required keys (the OpenAI-compatible
	// provider key) are checked fail-closed upstream in resolveProviderGatewayAuth.
	if profileInfo != nil {
		if err := m.mergeAgentProfileEnvFromInfoWithPartial(ctx, profileInfo, env, true); err != nil {
			return nil, fmt.Errorf("resolve agent profile environment: %w", err)
		}
	} else {
		if err := m.mergeAgentProfileEnvWithPartial(ctx, executionProfileID(req), env, true); err != nil {
			return nil, fmt.Errorf("resolve agent profile environment: %w", err)
		}
	}

	// Add standard variables for recovery after backend restart
	env["KANDEV_INSTANCE_ID"] = executionID
	env["KANDEV_TASK_ID"] = req.TaskID
	env["KANDEV_SESSION_ID"] = req.SessionID
	env["KANDEV_AGENT_PROFILE_ID"] = req.AgentProfileID
	env["KANDEV_EXECUTION_PROFILE_ID"] = executionProfileID(req)

	// Add agent runtime default env vars (e.g., MCP_TIMEOUT for Claude Code)
	if agentConfig != nil {
		if rt := agentConfig.Runtime(); rt != nil {
			for k, v := range rt.Env {
				if _, exists := env[k]; !exists {
					env[k] = v
				}
			}
		}
	}

	// Add required credentials from agent config
	if m.credsMgr != nil && agentConfig != nil {
		for _, credKey := range agentConfig.Runtime().RequiredEnv {
			if value, err := m.credsMgr.GetCredentialValue(ctx, credKey); err == nil && value != "" {
				env[credKey] = value
			}
		}
	}
	if req.managedGoCachePath != "" {
		env["GOCACHE"] = req.managedGoCachePath
	}

	if err := spillLargeWakePayloadEnv(env, req.WorkspacePath, m.logger.Zap()); err != nil {
		return nil, err
	}

	return env, nil
}

func (m *Manager) prepareManagedGoCacheEnvironment(ctx context.Context, req *LaunchRequest) error {
	if req == nil {
		return nil
	}
	if req.Metadata != nil {
		managedMetadata := make(map[string]interface{}, len(req.Metadata))
		for key, value := range req.Metadata {
			managedMetadata[key] = value
		}
		delete(managedMetadata, managedGoCacheMetadataKey)
		req.Metadata = managedMetadata
	}
	req.managedGoCachePath = ""
	warnFallback := func(reason string) {
		fields := []zap.Field{zap.String("reason", reason)}
		if req.TaskID != "" {
			fields = append(fields, zap.String("task_id", req.TaskID))
		}
		if req.SessionID != "" {
			fields = append(fields, zap.String("session_id", req.SessionID))
		}
		m.logger.Warn("managed Go cache preparation skipped; continuing without managed override", fields...)
	}
	if m.managedGoCache == nil || !isHostLocalExecutor(req.ExecutorType) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	env, err := m.managedGoCache.ExecutionEnvironment(ctx)
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("prepare managed Go cache: %w", err)
		}
		warnFallback("preparation_failed")
		return nil
	}
	path := env["GOCACHE"]
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) {
		warnFallback("invalid_output")
		return nil
	}
	path = filepath.Clean(path)
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	req.Metadata[managedGoCacheMetadataKey] = path
	req.managedGoCachePath = path
	return nil
}

func isHostLocalExecutor(executorType string) bool {
	switch models.ExecutorType(executorType) {
	case "", models.ExecutorTypeLocal, "local_pc", models.ExecutorTypeWorktree:
		return true
	default:
		return false
	}
}

// waitForAgentctlReady waits for the agentctl HTTP server to be ready.
// This enables shell/workspace features without starting the agent process.
func (m *Manager) waitForAgentctlReady(execution *AgentExecution) {
	opStart := time.Now()
	// Use detached context that respects stopCh for graceful shutdown
	ctx, cancel := appctx.Detached(context.Background(), m.stopCh, 60*time.Second)
	defer cancel()

	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		m.updateExecutionError(execution.ID, "agentctl client is unavailable")
		return
	}
	m.logger.Debug("waiting for agentctl to be ready",
		zap.String("execution_id", execution.ID),
		zap.String("url", client.BaseURL()))

	err := client.WaitForReady(ctx, 60*time.Second)
	releaseClient()
	if err != nil {
		m.logger.Error("agentctl not ready",
			zap.String("execution_id", execution.ID),
			zap.Duration("duration", time.Since(opStart)),
			zap.Error(err))
		m.updateExecutionError(execution.ID, "agentctl not ready: "+err.Error())
		// Use the timeout context for event publishing instead of a fresh Background context
		m.eventPublisher.PublishAgentctlEvent(ctx, events.AgentctlError, execution, err.Error())
		return
	}
	// Comparison configuration must arrive before readiness exposes workspace
	// access or the cached poll mode starts fresh Git scans.
	client, releaseClient = execution.AcquireAgentCtlClient()
	if client != nil {
		m.pushTaskBaseBranches(ctx, execution.TaskID, execution.ID, client)
		m.pushTaskComparisonTargets(ctx, execution.TaskID, execution.ID, client)
		releaseClient()
	}
	execution.MarkAgentctlReady()

	elapsed := time.Since(opStart)
	if elapsed > 10*time.Second {
		m.logger.Warn("agentctl ready took longer than expected",
			zap.String("execution_id", execution.ID),
			zap.Duration("duration", elapsed))
	} else {
		m.logger.Debug("agentctl ready - shell/workspace access available",
			zap.String("execution_id", execution.ID),
			zap.Duration("duration", elapsed))
	}
	// Flush any session mode the gateway cached before this execution was
	// ready (pre-execution-focus race). Without this, agentctl stays in its
	// default slow poll mode even though the frontend already sent focus,
	// and git state updates take up to 30s to reach the UI.
	m.flushCachedPollMode(execution.SessionID)
	// Use the timeout context for event publishing instead of a fresh Background context
	m.eventPublisher.PublishAgentctlEvent(ctx, events.AgentctlReady, execution, "")
}
