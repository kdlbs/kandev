package lifecycle

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/events"
	"go.uber.org/zap"
)

// restartPassthroughProcess kills the current PTY process and relaunches a fresh one
// without --resume, effectively clearing the agent's conversation context.
// The workflow step prompt is delivered afterwards via stdin (autoStartPassthroughPrompt).
func (m *Manager) restartPassthroughProcess(ctx context.Context, execution *AgentExecution) error {
	interactiveRunner := m.passthroughRunnerForExecution(execution)
	if interactiveRunner == nil {
		return fmt.Errorf("interactive runner not available")
	}
	execution.passthroughLifecycleMu.Lock()
	processID, err := m.replacePassthroughProcess(ctx, execution, interactiveRunner)
	execution.passthroughLifecycleMu.Unlock()
	if err != nil {
		return err
	}

	// First-idle callbacks acquire the lifecycle lock. Settle startup outside
	// that lock before a workflow prompt can claim the next running turn.
	readyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := interactiveRunner.WaitForFirstIdle(readyCtx, processID); err != nil {
		return fmt.Errorf("wait for restarted passthrough readiness: %w", err)
	}
	return nil
}

func (m *Manager) replacePassthroughProcess(ctx context.Context, execution *AgentExecution, interactiveRunner executionPassthroughRunner) (string, error) {
	m.logger.Info("restarting passthrough process for context reset",
		zap.String("execution_id", execution.ID),
		zap.String("session_id", execution.SessionID),
		zap.String("old_process_id", execution.PassthroughProcessID))

	preparation, err := m.preparePassthroughReset(ctx, execution, interactiveRunner)
	if err != nil {
		return "", err
	}

	processInfo, err := m.startPassthroughWithMCP(
		ctx, execution, interactiveRunner, preparation.startRequest, preparation.operation,
	)
	if err != nil {
		m.updateExecutionError(execution.ID, "failed to restart passthrough session: "+err.Error())
		return "", fmt.Errorf("failed to restart passthrough session: %w", err)
	}

	if preparation.profile != nil && preparation.profile.AgentName == agents.CopilotACPAgentID {
		execution.ExecutionProfileID = preparation.profile.ProfileID
		execution.CopilotSelectedPlugins = append([]string{}, preparation.profile.CopilotSelectedPlugins...)
	}
	execution.PassthroughProcessID = processInfo.ID
	execution.PassthroughStartedAt = time.Now()
	execution.passthroughLaunchUsedResume = false
	execution.passthroughInitialPromptProcessID = ""

	m.logger.Info("passthrough process restarted with fresh context",
		zap.String("execution_id", execution.ID),
		zap.String("session_id", execution.SessionID),
		zap.String("new_process_id", processInfo.ID))

	m.observeRemotePassthrough(execution)
	if interactiveRunner.ConnectSessionWebSocket(processInfo.ID) {
		m.logger.Debug("reconnected WebSocket to restarted passthrough process",
			zap.String("session_id", execution.SessionID),
			zap.String("process_id", processInfo.ID))
	}
	m.eventPublisher.PublishAgentEvent(ctx, events.AgentContextReset, execution)
	return processInfo.ID, nil
}

type passthroughResetPreparation struct {
	startRequest process.InteractiveStartRequest
	operation    *preparedMCPOperation
	profile      *AgentProfileInfo
}

func (m *Manager) preparePassthroughReset(ctx context.Context, execution *AgentExecution, runner executionPassthroughRunner) (passthroughResetPreparation, error) {
	// Resolve profile credentials before stopping the current CLI. Remote MCP
	// preparation follows a confirmed stop so it can retire the old file claims.
	resolved, err := m.resolvePassthroughAgent(ctx, execution)
	if err != nil {
		return passthroughResetPreparation{}, err
	}
	copilotCommand, copilotEnv, copilotPrepared, err := m.prepareCopilotPassthroughReset(ctx, execution, resolved)
	if err != nil {
		return passthroughResetPreparation{}, err
	}
	var env map[string]string
	if isSSHPassthrough(execution) {
		env, err = m.buildPassthroughEnv(ctx, execution, resolved.rt.RequiredEnv)
		if err != nil {
			return passthroughResetPreparation{}, err
		}
		if err := m.stopPassthroughForReset(ctx, execution, runner); err != nil {
			return passthroughResetPreparation{}, err
		}
	}
	progress := m.newCursorPassthroughPreparationRecorder(execution, resolved)
	pt, rt, cmd, err := m.preparePassthroughResetCommand(
		ctx, execution, resolved, progress, copilotCommand, copilotPrepared,
	)
	if err != nil {
		return passthroughResetPreparation{}, err
	}
	env, err = m.preparePassthroughResetEnvironment(
		ctx, execution, runner, resolved, rt, cmd, copilotEnv, copilotPrepared, env,
	)
	if err != nil {
		return passthroughResetPreparation{}, err
	}
	profile := (*AgentProfileInfo)(nil)
	if resolved.agentID == agents.CopilotACPAgentID {
		profile = resolved.profile
	}
	return passthroughResetPreparation{
		startRequest: buildInteractiveStartRequest(execution.SessionID, execution, pt, env, cmd, rt.StripEnv, true),
		operation:    resolved.mcpOperation,
		profile:      profile,
	}, nil
}

func (m *Manager) preparePassthroughResetCommand(
	ctx context.Context,
	execution *AgentExecution,
	resolved *resolvedPassthrough,
	progress *prepareProgressRecorder,
	copilotCommand passthroughCommand,
	copilotPrepared bool,
) (agents.PassthroughConfig, *agents.RuntimeConfig, passthroughCommand, error) {
	pt, rt, cmd := resolved.pt, resolved.rt, copilotCommand
	var err error
	if !copilotPrepared {
		pt, rt, cmd, err = m.freshPassthroughCommandForResolved(ctx, execution, resolved, progress)
	}
	if err != nil {
		if isSSHPassthrough(execution) {
			m.updateExecutionError(execution.ID, "failed to prepare replacement passthrough session: "+err.Error())
		}
		if progress != nil {
			m.publishExecutionPrepareCompleted(execution, progress, err)
		}
		return agents.PassthroughConfig{}, nil, passthroughCommand{}, err
	}
	if progress != nil {
		m.publishExecutionPrepareCompleted(execution, progress, nil)
	}
	return pt, rt, cmd, nil
}

func (m *Manager) preparePassthroughResetEnvironment(
	ctx context.Context,
	execution *AgentExecution,
	runner executionPassthroughRunner,
	resolved *resolvedPassthrough,
	rt *agents.RuntimeConfig,
	cmd passthroughCommand,
	copilotEnv map[string]string,
	copilotPrepared bool,
	env map[string]string,
) (map[string]string, error) {
	if isSSHPassthrough(execution) {
		maps.Copy(env, getPassthroughMCPEnv(execution))
		return env, nil
	}
	if copilotPrepared {
		env = copilotEnv
	} else {
		var err error
		env, err = m.buildPassthroughEnv(ctx, execution, rt.RequiredEnv)
		if err != nil {
			return nil, err
		}
	}
	if err := m.stopPassthroughForReset(ctx, execution, runner); err != nil {
		return nil, err
	}
	if !copilotPrepared {
		var selection []string
		if resolved.profile != nil {
			selection = resolved.profile.CopilotSelectedPlugins
		}
		if err := m.applyPassthroughCopilotPluginsForSelection(ctx, execution, &cmd, env, selection); err != nil {
			return nil, err
		}
	}
	return env, nil
}

func (m *Manager) prepareCopilotPassthroughReset(
	ctx context.Context,
	execution *AgentExecution,
	resolved *resolvedPassthrough,
) (passthroughCommand, map[string]string, bool, error) {
	if resolved.agentID != agents.CopilotACPAgentID || resolved.profile == nil ||
		len(resolved.profile.CopilotSelectedPlugins) == 0 {
		return passthroughCommand{}, nil, false, nil
	}
	if !copilotPluginHostExecutor(execution) {
		return passthroughCommand{}, nil, false, &copilotplugins.Error{
			Reason: copilotplugins.ReasonUnsupportedLaunch,
		}
	}
	cmd, err := m.freshCopilotPassthroughCommandForResolved(ctx, execution, resolved)
	if err != nil {
		return passthroughCommand{}, nil, false, err
	}
	env, err := m.buildPassthroughEnvForProfile(ctx, execution, resolved.rt.RequiredEnv, resolved.profile)
	if err != nil {
		return passthroughCommand{}, nil, false, err
	}
	if err := m.applyPassthroughCopilotPluginsForSelection(
		ctx, execution, &cmd, env, resolved.profile.CopilotSelectedPlugins,
	); err != nil {
		return passthroughCommand{}, nil, false, err
	}
	return cmd, env, true, nil
}

func (m *Manager) stopPassthroughForReset(ctx context.Context, execution *AgentExecution, runner executionPassthroughRunner) error {
	// Clear the process identity before stopping so its final status cannot
	// trigger an automatic restart during an explicit context reset.
	oldProcessID := execution.PassthroughProcessID
	oldStartedAt := execution.PassthroughStartedAt
	oldInitialPromptID := execution.passthroughInitialPromptProcessID
	execution.PassthroughProcessID = ""
	execution.passthroughInitialPromptProcessID = ""
	execution.PassthroughStartedAt = time.Time{}

	if err := runner.Stop(ctx, oldProcessID); err != nil {
		if isSSHPassthrough(execution) {
			execution.PassthroughProcessID = oldProcessID
			execution.PassthroughStartedAt = oldStartedAt
			execution.passthroughInitialPromptProcessID = oldInitialPromptID
			return fmt.Errorf("stop remote passthrough before reset: %w", err)
		}
		m.logger.Warn("failed to stop passthrough process during context reset",
			zap.String("execution_id", execution.ID),
			zap.String("process_id", oldProcessID),
			zap.Error(err))
	}

	return nil
}
