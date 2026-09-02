package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const (
	sessionMCPFailureResume = "session_resume_failed"
	sessionMCPFailureLoad   = "session_load_failed"
)

// RequestSessionMCPReconfiguration schedules an MCP update for an existing
// task session. The request is detached from the HTTP lifetime because the
// provider call must finish even after the selection response is returned.
func (m *Manager) RequestSessionMCPReconfiguration(ctx context.Context, sessionID string) {
	if m == nil || m.mcpStateRepo == nil || sessionID == "" {
		return
	}
	go func() {
		if err := m.applyPendingSessionMCP(context.WithoutCancel(ctx), sessionID); err != nil {
			m.logger.Warn("failed to apply session MCP selection",
				zap.String("session_id", sessionID), zap.Error(err))
		}
	}()
}

// applyPendingSessionMCP applies the newest desired selection only when the
// ACP session is idle. promptLifecycleMu is held across the provider call so
// no turn can start while the agent changes its session configuration.
func (m *Manager) applyPendingSessionMCP(ctx context.Context, sessionID string) error {
	state, err := m.mcpStateRepo.GetMCPSelectionState(ctx, sessionID)
	if errors.Is(err, mcpconfig.ErrMCPSelectionStateNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if state.DesiredRevision <= state.AppliedRevision {
		return nil
	}
	execution, ok := m.executionStore.GetBySessionID(sessionID)
	if !ok || execution == nil {
		return m.saveDeferredSessionMCP(ctx, sessionID)
	}

	execution.promptLifecycleMu.Lock()
	defer execution.promptLifecycleMu.Unlock()
	if execution.Status != v1.AgentStatusReady {
		return m.savePendingSessionMCP(ctx, sessionID)
	}
	if execution.IsPassthrough || execution.agentctl == nil || execution.ACPSessionID == "" {
		return m.saveDeferredSessionMCP(ctx, sessionID)
	}

	if mcpErr := m.applySessionMCPWithAgent(ctx, execution); mcpErr != nil {
		return m.saveFailedSessionMCP(ctx, sessionID, state.DesiredRevision, mcpErr)
	}
	return m.saveAppliedSessionMCP(ctx, sessionID, state.DesiredRevision, execution.agentctl.GetLastAttachmentAttemptID())
}

func (m *Manager) applySessionMCPWithAgent(ctx context.Context, execution *AgentExecution) error {
	agentConfig, err := m.getAgentConfigForExecution(execution)
	if err != nil {
		return err
	}
	servers, err := m.resolveMcpServers(ctx, execution, agentConfig)
	if err != nil {
		return err
	}
	client := execution.agentctl
	if client.SupportsSessionResume() {
		err = client.ResumeSession(ctx, execution.ACPSessionID, execution.WorkspacePath, servers)
		if err == nil {
			return nil
		}
		if !client.SupportsSessionLoad() {
			return fmt.Errorf("%s: %w", sessionMCPFailureResume, err)
		}
	}
	if client.SupportsSessionLoad() {
		if err := client.LoadSession(ctx, execution.ACPSessionID, servers); err != nil {
			return fmt.Errorf("%s: %w", sessionMCPFailureLoad, err)
		}
		return nil
	}
	return errors.New("agent does not support session resume or load")
}

func (m *Manager) savePendingSessionMCP(ctx context.Context, sessionID string) error {
	return m.updateSessionMCPState(ctx, sessionID, func(state *mcpconfig.SessionMCPSelectionState) {
		state.ApplyState = mcpconfig.SessionMCPApplyStatePendingIdle
	})
}

func (m *Manager) saveDeferredSessionMCP(ctx context.Context, sessionID string) error {
	return m.updateSessionMCPState(ctx, sessionID, func(state *mcpconfig.SessionMCPSelectionState) {
		state.ApplyState = mcpconfig.SessionMCPApplyStateDeferredRestart
	})
}

func (m *Manager) saveFailedSessionMCP(ctx context.Context, sessionID string, operationRevision int64, cause error) error {
	code := sessionMCPFailureResume
	if strings.Contains(cause.Error(), sessionMCPFailureLoad) {
		code = sessionMCPFailureLoad
	}
	return m.updateSessionMCPState(ctx, sessionID, func(state *mcpconfig.SessionMCPSelectionState) {
		if state.DesiredRevision != operationRevision {
			state.ApplyState = mcpconfig.SessionMCPApplyStatePendingIdle
			state.FailureCode = ""
			state.FailureSummary = ""
			return
		}
		state.ApplyState = mcpconfig.SessionMCPApplyStateFailed
		state.FailureCode = code
		state.FailureSummary = "The agent did not accept the MCP update."
	})
}

func (m *Manager) saveAppliedSessionMCP(
	ctx context.Context,
	sessionID string,
	operationRevision int64,
	attachmentAttemptID string,
) error {
	return m.updateSessionMCPState(ctx, sessionID, func(state *mcpconfig.SessionMCPSelectionState) {
		state.AppliedRevision = operationRevision
		state.AttachmentAttemptID = attachmentAttemptID
		state.FailureCode = ""
		state.FailureSummary = ""
		if state.DesiredRevision == operationRevision {
			state.ApplyState = mcpconfig.SessionMCPApplyStateApplied
			return
		}
		state.ApplyState = mcpconfig.SessionMCPApplyStatePendingIdle
	})
}

func (m *Manager) updateSessionMCPState(
	ctx context.Context,
	sessionID string,
	mutate func(*mcpconfig.SessionMCPSelectionState),
) error {
	state, err := m.mcpStateRepo.GetMCPSelectionState(ctx, sessionID)
	if errors.Is(err, mcpconfig.ErrMCPSelectionStateNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	mutate(&state)
	return m.mcpStateRepo.SaveMCPSelectionState(ctx, sessionID, state)
}
