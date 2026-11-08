package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const (
	sessionMCPFailureResume             = "session_resume_failed"
	sessionMCPFailureLoad               = "session_load_failed"
	sessionMCPReconfigurationTimeout    = 2 * time.Minute
	sessionMCPFailurePersistenceTimeout = 5 * time.Second
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
	ctx, cancel := context.WithTimeout(ctx, sessionMCPReconfigurationTimeout)
	defer cancel()
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

	generation := execution.startupAttemptSnapshot()
	var applyErr error
	accepted := execution.withStartupAttempt(generation, func(string) { applyErr = m.applyPendingSessionMCPLeased(ctx, execution) })
	if !accepted {
		return m.savePendingSessionMCP(ctx, sessionID)
	}
	return applyErr
}

// Startup callbacks take their lease before the prompt lock. Keep that order
// across the bounded reconnect and acceptance, without acquiring it again.
func (m *Manager) applyPendingSessionMCPLeased(ctx context.Context, execution *AgentExecution) error {
	sessionID := execution.SessionID

	execution.promptLifecycleMu.Lock()
	defer execution.promptLifecycleMu.Unlock()
	// The selection can change while the execution lock is being acquired.
	// Re-read it here so this operation cannot apply an older revision and
	// overwrite the newer request's state when it completes.
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
	if execution.Status != v1.AgentStatusReady {
		return m.savePendingSessionMCP(ctx, sessionID)
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if execution.IsPassthrough || client == nil || execution.ACPSessionID == "" {
		return m.saveDeferredSessionMCP(ctx, sessionID)
	}
	if !client.SupportsSessionResume() && !client.SupportsSessionLoad() {
		return m.saveDeferredSessionMCP(ctx, sessionID)
	}

	return m.reconfigureIdleSessionMCP(ctx, execution, state, client)
}

func (m *Manager) reconfigureIdleSessionMCP(ctx context.Context, execution *AgentExecution, state mcpconfig.SessionMCPSelectionState, client *agentctl.Client) error {
	sessionID := execution.SessionID

	// session/load and session/resume can reset provider-owned runtime settings.
	// Capture the effective Kandev runtime configuration before reconnecting so
	// MCP selection changes do not silently replace a user's model, mode, or
	// config options with provider defaults.
	runtimeConfig := m.captureSessionRuntimeConfigForReset(ctx, execution)
	agentConfig, err := m.getAgentConfigForExecution(execution)
	if err != nil {
		return m.saveFailedSessionMCP(ctx, sessionID, state.DesiredRevision, err)
	}
	if runtime := agentConfig.Runtime(); runtime != nil && runtime.ProjectMCPStrategy != nil {
		return m.saveDeferredSessionMCP(ctx, sessionID)
	}

	operation, err := m.prepareExecutionMCP(ctx, execution, agentConfig)
	if err != nil {
		return m.saveFailedSessionMCP(ctx, sessionID, state.DesiredRevision, err)
	}
	setup, revision, captured := operation.setup, operation.revision, operation.captured
	if !captured {
		revision = state.DesiredRevision
	}
	attemptID, mcpErr := m.applySessionMCPWithAgent(operation.context(ctx), client, execution, operation.servers)
	if mcpErr != nil {
		return m.saveFailedSessionMCP(ctx, sessionID, revision, mcpErr)
	}
	if err := m.restoreSessionRuntimeConfigWithClient(ctx, execution, client, execution.ACPSessionID, runtimeConfig); err != nil {
		return m.saveFailedSessionMCP(ctx, sessionID, state.DesiredRevision, err)
	}
	return m.saveAppliedSessionMCP(ctx, sessionID, revision, attemptID, setup)
}

func (m *Manager) applySessionMCPWithAgent(ctx context.Context, client *agentctl.Client, execution *AgentExecution, servers []agentctltypes.McpServer) (string, error) {
	var err error
	var attemptID string
	if client.SupportsSessionResume() {
		attemptID, err = client.ResumeSessionWithResult(ctx, execution.ACPSessionID, execution.WorkspacePath, servers)
		if err == nil {
			return attemptID, nil
		}
		if !client.SupportsSessionLoad() {
			return "", fmt.Errorf("%s: %w", sessionMCPFailureResume, err)
		}
	}
	if client.SupportsSessionLoad() {
		attemptID, err = client.LoadSessionWithResult(ctx, execution.ACPSessionID, servers, streams.SessionSettingsPolicyStrict)
		if err != nil {
			return "", fmt.Errorf("%s: %w", sessionMCPFailureLoad, err)
		}
		return attemptID, nil
	}
	return "", errors.New("agent does not support session resume or load")
}

func (m *Manager) savePendingSessionMCP(ctx context.Context, sessionID string) error {
	return m.updateSessionMCPState(ctx, sessionID, func(state *mcpconfig.SessionMCPSelectionState) {
		if state.AppliedRevision >= state.DesiredRevision {
			return
		}
		state.ApplyState = mcpconfig.SessionMCPApplyStatePendingIdle
	})
}

func (m *Manager) saveDeferredSessionMCP(ctx context.Context, sessionID string) error {
	return m.updateSessionMCPState(ctx, sessionID, func(state *mcpconfig.SessionMCPSelectionState) {
		if state.AppliedRevision >= state.DesiredRevision {
			return
		}
		state.ApplyState = mcpconfig.SessionMCPApplyStateDeferredRestart
		state.FailureCode = ""
		state.FailureSummary = ""
	})
}

func (m *Manager) saveFailedSessionMCP(ctx context.Context, sessionID string, operationRevision int64, cause error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sessionMCPFailurePersistenceTimeout)
	defer cancel()
	code := sessionMCPFailureResume
	if strings.Contains(cause.Error(), sessionMCPFailureLoad) {
		code = sessionMCPFailureLoad
	}
	return m.updateSessionMCPState(ctx, sessionID, func(state *mcpconfig.SessionMCPSelectionState) {
		if state.AppliedRevision >= operationRevision {
			return
		}
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
	setup *mcpconfig.MCPSetupSnapshot,
) error {
	if m.mcpStateRepo == nil || sessionID == "" {
		return nil
	}
	if writer, ok := m.mcpStateRepo.(mcpconfig.AcceptedMCPSetupRepository); ok {
		return writer.SaveAcceptedMCPSetup(ctx, sessionID, operationRevision, attachmentAttemptID, setup)
	}
	if _, err := m.mcpStateRepo.GetMCPSelectionState(ctx, sessionID); errors.Is(err, mcpconfig.ErrMCPSelectionStateNotFound) {
		return m.mcpStateRepo.SaveMCPSelectionState(ctx, sessionID, mcpconfig.SessionMCPSelectionState{DesiredRevision: operationRevision, AppliedRevision: operationRevision, ApplyState: mcpconfig.SessionMCPApplyStateApplied, AttachmentAttemptID: attachmentAttemptID, AcceptedSetup: setup})
	}
	return m.updateSessionMCPState(ctx, sessionID, func(state *mcpconfig.SessionMCPSelectionState) {
		if state.AppliedRevision > operationRevision {
			return
		}
		state.AppliedRevision = operationRevision
		state.AcceptedSetup = setup
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
	expectedDesiredRevision := state.DesiredRevision
	mutate(&state)
	if compareAndSwap, ok := m.mcpStateRepo.(mcpconfig.CompareAndSwapMCPSelectionStateRepository); ok {
		_, err := compareAndSwap.CompareAndSwapMCPSelectionState(
			ctx, sessionID, expectedDesiredRevision, state,
		)
		return err
	}
	return m.mcpStateRepo.SaveMCPSelectionState(ctx, sessionID, state)
}
