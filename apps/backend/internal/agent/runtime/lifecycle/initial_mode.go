package lifecycle

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// initialModeOutcome records what happened when Kandev tried to give an agent
// process the mode it should start in.
type initialModeOutcome struct {
	// Mode is the effective session mode Kandev wanted to deliver.
	Mode string
	// Delivered is true when the launched process is configured with Mode from
	// its first turn, rather than only switched afterwards.
	Delivered bool
	// Reason explains a mode that could not be delivered.
	Reason string
	// Request is installed by the executor after its selected settings are ready.
	Request *AgentInitialModeRequest
}

// applyInitialMode configures the launch environment so the agent process
// starts in the effective session mode.
//
// Applying a mode after session/new is not equivalent: the agent is told about
// the new mode while the process keeps enforcing what it was launched with. The
// post-creation switch stays in place for agents without a declared channel and
// for later mode changes; this is what makes the mode true at the first turn.
//
// Nothing happens when no mode is requested. A profile that leaves the mode
// empty keeps today's launch environment byte for byte, which bounds this to
// the sessions that actually ask for a mode.
func (m *Manager) applyInitialMode(
	env map[string]string,
	executionID string,
	agentConfig agents.Agent,
	mode string,
	executorType string,
) initialModeOutcome {
	if mode == "" {
		return initialModeOutcome{}
	}
	outcome := initialModeOutcome{Mode: mode}
	executor := models.ExecutorType(executorType)
	runtimeCfg, reason := initialModeRuntimeConfig(env, agentConfig, mode, executor)
	if reason != "" {
		outcome.Reason = reason
		return outcome
	}
	delivery := runtimeCfg.InitialMode
	if executor != models.ExecutorTypeSSH {
		env[delivery.ConfigDirEnvVar] = runtimeCfg.SessionConfig.SessionDirTarget
	}
	if containerRuntimeNeedsSandboxDeclaration(executorType) {
		applyInitialModeSandboxEnv(env, delivery.SandboxEnv)
	}
	outcome.Request = &AgentInitialModeRequest{Mode: mode}
	m.logger.Debug("prepared session mode for executor installation",
		zap.String("execution_id", executionID),
		zap.String("mode", mode))
	return outcome
}

func initialModeRuntimeConfig(
	env map[string]string,
	agentConfig agents.Agent,
	mode string,
	executor models.ExecutorType,
) (*agents.RuntimeConfig, string) {
	if agentConfig == nil || env == nil {
		return nil, "no agent configuration for this launch"
	}
	runtimeCfg := agentConfig.Runtime()
	if runtimeCfg == nil {
		return nil, "no agent configuration for this launch"
	}
	delivery := runtimeCfg.InitialMode
	if !delivery.Delivers(mode) {
		return nil, "agent declares no start-mode channel for " + mode
	}
	if configuredDir := env[delivery.ConfigDirEnvVar]; strings.TrimSpace(configuredDir) != "" {
		return nil, "agent configuration directory is explicitly configured and cannot be replaced safely"
	}
	if delivery.ConfigDirEnvVar == "" {
		return nil, "agent declares no configuration directory for start-mode delivery"
	}
	if !executorSupportsInitialMode(executor) {
		return nil, "executor cannot provide an isolated configuration directory without changing authentication"
	}
	if executor != models.ExecutorTypeSSH && runtimeCfg.SessionConfig.SessionDirTarget == "" {
		return nil, "agent declares no isolated configuration directory for this executor"
	}
	return runtimeCfg, ""
}

func executorSupportsInitialMode(executor models.ExecutorType) bool {
	switch executor {
	case models.ExecutorTypeLocalDocker, models.ExecutorTypeRemoteDocker,
		models.ExecutorTypeSSH, models.ExecutorTypeKubernetes:
		return true
	default:
		return false
	}
}

func applyInitialModeSandboxEnv(env, sandboxEnv map[string]string) {
	for key, value := range sandboxEnv {
		if _, present := env[key]; present {
			continue
		}
		env[key] = value
	}
}

func (m *Manager) reportInitialModeWarning(
	onProgress PrepareProgressCallback,
	taskID, sessionID, mode, reason string,
) {
	if onProgress == nil {
		if m == nil || m.eventPublisher == nil {
			return
		}
		onProgress = m.newProgressCallback(taskID, sessionID)
	}
	step := beginStep("Configure start permission mode")
	step.Warning = "The requested permission mode was not installed before the first turn."
	step.WarningDetail = "Requested mode: " + mode
	if strings.TrimSpace(reason) != "" {
		step.WarningDetail += ". Reason: " + reason
	}
	completeStepSuccess(&step)
	onProgress(step, 0, 0)
}

// containerRuntimeNeedsSandboxDeclaration reports whether this executor runs
// the agent inside container isolation, where a permissive mode would otherwise
// be disabled for the container's root process identity.
func containerRuntimeNeedsSandboxDeclaration(executorType string) bool {
	return models.ExecutorType(executorType).Runtime().IsContainerized()
}

// launchSessionMode resolves the mode this launch should start in.

// It mirrors effectiveSessionMode's precedence: a persisted session mode — set
// by the user's toggle or a set_session_mode workflow action — wins over the
// agent profile's mode. Reading it here rather than after session/new is the
// point: the value has to be known before the process starts.
func (m *Manager) launchSessionMode(ctx context.Context, req *LaunchRequest, profileInfo *AgentProfileInfo) string {
	profileMode := ""
	if profileInfo != nil {
		profileMode = profileInfo.Mode
	}
	if req == nil || m.workspaceInfoProvider == nil || req.SessionID == "" {
		return profileMode
	}
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, req.TaskID, req.SessionID)
	if err != nil || info == nil || info.SessionMode == "" {
		return profileMode
	}
	return info.SessionMode
}

// reportModeOutcome records what a live mode change achieved. A clamped or
// unobserved mode must not read as a clean apply in the log.
func (m *Manager) reportModeOutcome(execution *AgentExecution, result agentctlclient.ModeResult) {
	if execution == nil || result.Requested == "" {
		return
	}
	if result.Applied() {
		m.logger.Info("session mode applied",
			zap.String("execution_id", execution.ID),
			zap.String("mode", result.Effective))
		return
	}
	m.logger.Warn("session mode was not confirmed by the agent",
		zap.String("execution_id", execution.ID),
		zap.String("requested_mode", result.Requested),
		zap.String("effective_mode", result.Effective),
		zap.Bool("confirmed", result.Confirmed))
}
