package lifecycle

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

var (
	workspaceRebindReadyTimeout  = 10 * time.Second
	workspaceRebindReadyPoll     = 500 * time.Millisecond
	workspaceRebindStreamTimeout = 10 * time.Second
)

// RebindWorkspaceForSession moves an idle native host execution to a prepared
// task root without changing its Kandev session or ACP conversation. The
// caller must only invoke this after the complete attachment batch is ready;
// this operation publishes no materialization event itself.
func (m *Manager) RebindWorkspaceForSession(ctx context.Context, sessionID, workspacePath string, sourceRoots ...[]string) error {
	execution, ok := m.executionStore.GetBySessionID(sessionID)
	if !ok {
		// There is no child to adopt. The persisted environment root is enough
		// for the next launch, so materialization remains durable rather than
		// failing a batch because an old session is no longer live.
		return nil
	}
	if execution.IsPassthrough || execution.agentctl == nil || execution.ACPSessionID == "" {
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
	execution.Status = v1.AgentStatusStarting

	// Stop before changing agentctl's workdir: a successful rebind must never
	// leave a child running in the old CWD. If stop cannot be proven, do not
	// attempt adoption or start a duplicate process.
	if err := execution.agentctl.Stop(ctx); err != nil {
		execution.Status = v1.AgentStatusReady
		return fmt.Errorf("stop agent before workspace rebind: %w", err)
	}
	execution.WorkspaceSourceRoots = newRoots
	if err := execution.agentctl.RebindWorkspace(ctx, workspacePath, newRoots); err != nil {
		return m.rollbackWorkspaceRebind(ctx, execution, oldPath, oldRoots, acpID, fmt.Errorf("rebind agentctl workspace: %w", err))
	}
	execution.WorkspacePath = workspacePath
	if _, err := execution.agentctl.Start(ctx); err != nil {
		return m.rollbackWorkspaceRebind(ctx, execution, oldPath, oldRoots, acpID, fmt.Errorf("restart agent after workspace rebind: %w", err))
	}
	if err := m.restoreReboundACPSession(ctx, execution, acpID); err != nil {
		return m.rollbackWorkspaceRebind(ctx, execution, oldPath, oldRoots, acpID, fmt.Errorf("load existing ACP session after workspace rebind: %w", err))
	}
	execution.Status = v1.AgentStatusReady
	return nil
}

func (m *Manager) rollbackWorkspaceRebind(ctx context.Context, execution *AgentExecution, oldPath string, oldRoots []string, acpID string, cause error) error {
	rollbackCtx := context.WithoutCancel(ctx)
	// Restore the authoritative in-memory policy first, even if an I/O failure
	// prevents the best-effort child rollback below.
	execution.WorkspaceSourceRoots = append([]string(nil), oldRoots...)
	if err := execution.agentctl.Stop(rollbackCtx); err != nil {
		m.executionStore.UpdateError(execution.ID, fmt.Sprintf("%v; rollback stop failed: %v", cause, err))
		return fmt.Errorf("%w; rollback stop failed: %v", cause, err)
	}
	if err := execution.agentctl.RebindWorkspace(rollbackCtx, oldPath, oldRoots); err != nil {
		m.executionStore.UpdateError(execution.ID, fmt.Sprintf("%v; rollback rebind failed: %v", cause, err))
		return fmt.Errorf("%w; rollback rebind failed: %v", cause, err)
	}
	execution.WorkspacePath = oldPath
	if _, err := execution.agentctl.Start(rollbackCtx); err != nil {
		m.executionStore.UpdateError(execution.ID, fmt.Sprintf("%v; rollback restart failed: %v", cause, err))
		return fmt.Errorf("%w; rollback restart failed: %v", cause, err)
	}
	if err := m.restoreReboundACPSession(rollbackCtx, execution, acpID); err != nil {
		m.executionStore.UpdateError(execution.ID, fmt.Sprintf("%v; rollback session/load failed: %v", cause, err))
		return fmt.Errorf("%w; rollback session/load failed: %v", cause, err)
	}
	execution.Status = v1.AgentStatusReady
	return cause
}

// restoreReboundACPSession only resumes the existing ACP conversation once the
// restarted child has published a running status and its replacement updates
// stream is connected. Start can acknowledge before the ACP adapter is usable.
func (m *Manager) restoreReboundACPSession(ctx context.Context, execution *AgentExecution, acpID string) error {
	if err := waitForReboundAgentReady(ctx, execution); err != nil {
		return err
	}
	updatesReady := make(chan struct{})
	m.streamManager.ConnectAll(execution, updatesReady)
	if err := waitForReboundUpdatesStream(ctx, updatesReady); err != nil {
		return err
	}
	if _, err := execution.agentctl.Initialize(ctx, "kandev", "1.0.0"); err != nil {
		return fmt.Errorf("initialize restarted ACP adapter: %w", err)
	}
	if err := execution.agentctl.LoadSession(ctx, acpID, nil); err != nil {
		return err
	}
	return nil
}

func waitForReboundAgentReady(ctx context.Context, execution *AgentExecution) error {
	readyCtx, cancel := context.WithTimeout(ctx, workspaceRebindReadyTimeout)
	defer cancel()
	ticker := time.NewTicker(workspaceRebindReadyPoll)
	defer ticker.Stop()
	var lastErr error
	for {
		status, err := execution.agentctl.GetStatus(readyCtx)
		if err == nil && status.AgentStatus == "running" {
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
