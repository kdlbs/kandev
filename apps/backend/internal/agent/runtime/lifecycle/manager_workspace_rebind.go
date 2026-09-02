package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const agentctlProcessStatusRunning = "running"

var (
	workspaceRebindReadyTimeout  = 10 * time.Second
	workspaceRebindReadyPoll     = 500 * time.Millisecond
	workspaceRebindStreamTimeout = 10 * time.Second
)

// RebindWorkspaceForSession moves an idle native host execution to a prepared
// task root without changing its Kandev session. Providers that support loading
// a conversation under a new CWD keep their ACP session; incompatible providers
// get a fresh ACP session whose next prompt receives recorded Kandev history.
// The caller must only invoke this after the complete attachment batch is
// ready; this operation publishes no materialization event itself.
func (m *Manager) RebindWorkspaceForSession(ctx context.Context, sessionID, workspacePath string, sourceRoots ...[]string) error {
	execution, ok := m.executionStore.GetBySessionID(sessionID)
	if !ok {
		// There is no child to adopt. The persisted environment root is enough
		// for the next launch, so materialization remains durable rather than
		// failing a batch because an old session is no longer live.
		return nil
	}
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	client, releaseClient := execution.AcquireAgentCtlClient()
	releaseClient()
	if execution.IsPassthrough || client == nil || execution.ACPSessionID == "" {
		return fmt.Errorf("workspace rebind is unsupported for this session; start a new session after attaching sources")
	}
	execution.promptLifecycleMu.Lock()
	defer execution.promptLifecycleMu.Unlock()
	if execution.Status != v1.AgentStatusReady {
		return fmt.Errorf("workspace rebind requires an idle ready execution")
	}
	oldPath, acpID := execution.WorkspacePath, execution.ACPSessionID
	oldRoots := append([]string(nil), execution.WorkspaceSourceRoots...)
	newRoots := optionalWorkspaceSourceRoots(oldRoots, sourceRoots)
	startNewSession := m.startsNewSessionOnWorkspaceRebind(execution)
	execution.Status = v1.AgentStatusStarting

	// Stop before changing agentctl's workdir: a successful rebind must never
	// leave a child running in the old CWD. If stop cannot be proven, do not
	// attempt adoption or start a duplicate process.
	client, releaseClient = execution.AcquireAgentCtlClient()
	err := client.Stop(ctx)
	releaseClient()
	if err != nil {
		execution.Status = v1.AgentStatusReady
		return fmt.Errorf("stop agent before workspace rebind: %w", err)
	}
	execution.WorkspaceSourceRoots = newRoots
	client, releaseClient = execution.AcquireAgentCtlClient()
	err = client.RebindWorkspace(ctx, workspacePath, newRoots)
	releaseClient()
	if err != nil {
		return m.rollbackWorkspaceRebind(ctx, execution, oldPath, oldRoots, acpID, fmt.Errorf("rebind agentctl workspace: %w", err))
	}
	execution.WorkspacePath = workspacePath
	client, releaseClient = execution.AcquireAgentCtlClient()
	_, err = client.Start(ctx)
	releaseClient()
	if err != nil {
		return m.rollbackWorkspaceRebind(ctx, execution, oldPath, oldRoots, acpID, fmt.Errorf("restart agent after workspace rebind: %w", err))
	}
	if err := m.restoreReboundACPSession(ctx, execution, acpID, startNewSession); err != nil {
		return m.rollbackWorkspaceRebind(ctx, execution, oldPath, oldRoots, acpID, fmt.Errorf("restore ACP session after workspace rebind: %w", err))
	}
	execution.Status = v1.AgentStatusReady
	return nil
}

func (m *Manager) rollbackWorkspaceRebind(ctx context.Context, execution *AgentExecution, oldPath string, oldRoots []string, acpID string, cause error) error {
	rollbackCtx := context.WithoutCancel(ctx)
	// Restore the authoritative in-memory policy first, even if an I/O failure
	// prevents the best-effort child rollback below.
	execution.WorkspaceSourceRoots = append([]string(nil), oldRoots...)
	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		return fmt.Errorf("%w; rollback agentctl client is unavailable", cause)
	}
	err := client.Stop(rollbackCtx)
	releaseClient()
	if err != nil {
		m.executionStore.UpdateError(execution.ID, fmt.Sprintf("%v; rollback stop failed: %v", cause, err))
		return fmt.Errorf("%w; rollback stop failed: %v", cause, err)
	}
	client, releaseClient = execution.AcquireAgentCtlClient()
	if client == nil {
		return fmt.Errorf("%w; rollback agentctl client is unavailable", cause)
	}
	err = client.RebindWorkspace(rollbackCtx, oldPath, oldRoots)
	releaseClient()
	if err != nil {
		m.executionStore.UpdateError(execution.ID, fmt.Sprintf("%v; rollback rebind failed: %v", cause, err))
		return fmt.Errorf("%w; rollback rebind failed: %v", cause, err)
	}
	execution.WorkspacePath = oldPath
	client, releaseClient = execution.AcquireAgentCtlClient()
	if client == nil {
		return fmt.Errorf("%w; rollback agentctl client is unavailable", cause)
	}
	_, err = client.Start(rollbackCtx)
	releaseClient()
	if err != nil {
		m.executionStore.UpdateError(execution.ID, fmt.Sprintf("%v; rollback restart failed: %v", cause, err))
		return fmt.Errorf("%w; rollback restart failed: %v", cause, err)
	}
	if err := m.restoreReboundACPSession(rollbackCtx, execution, acpID, false); err != nil {
		m.executionStore.UpdateError(execution.ID, fmt.Sprintf("%v; rollback session/load failed: %v", cause, err))
		return fmt.Errorf("%w; rollback session/load failed: %v", cause, err)
	}
	execution.Status = v1.AgentStatusReady
	return cause
}

// restoreReboundACPSession restores agent continuity once the restarted child
// has published a running status and its replacement updates stream is
// connected. Start can acknowledge before the ACP adapter is usable.
func (m *Manager) restoreReboundACPSession(ctx context.Context, execution *AgentExecution, acpID string, startNewSession bool) error {
	if err := waitForReboundAgentReady(ctx, execution); err != nil {
		return err
	}
	updatesReady := make(chan struct{})
	m.streamManager.ConnectAll(execution, updatesReady)
	if err := waitForReboundUpdatesStream(ctx, updatesReady); err != nil {
		return err
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		return fmt.Errorf("agentctl client is unavailable")
	}
	_, err := client.Initialize(ctx, "kandev", "1.0.0")
	releaseClient()
	if err != nil {
		return fmt.Errorf("initialize restarted ACP adapter: %w", err)
	}
	if startNewSession {
		return m.createReboundACPSession(ctx, execution)
	}
	client, releaseClient = execution.AcquireAgentCtlClient()
	if client == nil {
		return fmt.Errorf("agentctl client is unavailable")
	}
	err = client.LoadSession(ctx, acpID, nil)
	releaseClient()
	if err != nil {
		return err
	}
	return nil
}

func (m *Manager) startsNewSessionOnWorkspaceRebind(execution *AgentExecution) bool {
	if m.registry == nil || execution.AgentID == "" {
		return false
	}
	agentConfig, ok := m.registry.Get(execution.AgentID)
	if !ok || agentConfig.Runtime() == nil {
		return false
	}
	return agentConfig.Runtime().SessionConfig.NewSessionOnWorkspaceRebind
}

func (m *Manager) createReboundACPSession(ctx context.Context, execution *AgentExecution) error {
	agentConfig, ok := m.registry.Get(execution.AgentID)
	if !ok {
		return fmt.Errorf("agent type not found: %s", execution.AgentID)
	}
	mcpServers, err := m.resolveMcpServers(ctx, execution, agentConfig)
	if err != nil {
		return fmt.Errorf("resolve MCP servers for rebound session: %w", err)
	}
	previousMode := execution.GetModeState()
	previousModel := execution.GetModelState()
	previousModelID := m.effectiveSessionModelForReset(ctx, execution)
	execution.SetModelState(nil)
	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		return fmt.Errorf("agentctl client is unavailable")
	}
	newSessionID, err := client.NewSession(ctx, execution.WorkspacePath, mcpServers)
	releaseClient()
	if err != nil {
		execution.SetModelState(previousModel)
		return fmt.Errorf("create ACP session in rebound workspace: %w", err)
	}
	execution.ACPSessionID = newSessionID
	execution.setSessionInitialized(true)
	execution.resumeContextInjected = false
	execution.needsResumeContext = m.historyManager != nil &&
		m.historyManager.HasHistory(execution.SessionID)
	if !cacheFreshSessionModelState(execution) && previousModelID != "" {
		waitForFreshSessionModelState(ctx, m.logger, execution)
	}
	if err := m.reapplyReboundSessionConfig(ctx, execution, newSessionID, previousModel, previousModelID, previousMode); err != nil {
		return err
	}
	if m.eventPublisher != nil {
		m.eventPublisher.PublishACPSessionCreated(execution, newSessionID)
	}
	return nil
}

func (m *Manager) reapplyReboundSessionConfig(
	ctx context.Context,
	execution *AgentExecution,
	sessionID string,
	model *CachedModelState,
	modelID string,
	mode *CachedModeState,
) error {
	if modelID != "" {
		client, releaseClient := execution.AcquireAgentCtlClient()
		if client == nil {
			return fmt.Errorf("agentctl client is unavailable")
		}
		policy := m.resolveStartModelPolicy(ctx, execution.AgentProfileID)
		policy.Model = modelID
		decision, err := applyStartModelPolicy(ctx, m.logger, client, execution.GetModelState(), policy)
		releaseClient()
		if err != nil {
			m.logger.Warn("failed to re-apply model after workspace rebind",
				zap.String("execution_id", execution.ID),
				zap.String("model", modelID),
				zap.Error(err))
			return err
		}
		if decision.Warning && m.sessionManager != nil {
			m.sessionManager.publishModelSelectionWarningEvent(execution, sessionID, decision)
		}
	}
	if model != nil {
		for _, option := range model.ConfigOptions {
			if option.ID == "" || option.CurrentValue == "" ||
				strings.EqualFold(option.Category, "model") ||
				strings.EqualFold(option.Category, "mode") {
				continue
			}
			client, releaseClient := execution.AcquireAgentCtlClient()
			if client == nil {
				return fmt.Errorf("agentctl client is unavailable")
			}
			err := client.SetConfigOption(ctx, option.ID, option.CurrentValue)
			releaseClient()
			if err != nil {
				m.logger.Warn("failed to re-apply config option after workspace rebind",
					zap.String("execution_id", execution.ID),
					zap.String("config_id", option.ID),
					zap.Error(err))
			}
		}
	}
	m.reapplySessionModeAfterReset(ctx, execution, sessionID, mode)
	return nil
}

func waitForReboundAgentReady(ctx context.Context, execution *AgentExecution) error {
	readyCtx, cancel := context.WithTimeout(ctx, workspaceRebindReadyTimeout)
	defer cancel()
	ticker := time.NewTicker(workspaceRebindReadyPoll)
	defer ticker.Stop()
	var lastErr error
	for {
		client, releaseClient := execution.AcquireAgentCtlClient()
		if client == nil {
			return fmt.Errorf("wait for restarted agent readiness: agentctl client is unavailable")
		}
		status, err := client.GetStatus(readyCtx)
		releaseClient()
		if err == nil && status.AgentStatus == agentctlProcessStatusRunning {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("agent status is %q", status.AgentStatus)
		}
		select {
		case <-readyCtx.Done():
			return fmt.Errorf("wait for restarted agent readiness: %w", lastErr)
		case <-ticker.C:
		}
	}
}

func waitForReboundUpdatesStream(ctx context.Context, updatesReady <-chan struct{}) error {
	timer := time.NewTimer(workspaceRebindStreamTimeout)
	defer timer.Stop()
	select {
	case <-updatesReady:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return fmt.Errorf("timeout waiting for agent stream after workspace rebind")
	}
}
