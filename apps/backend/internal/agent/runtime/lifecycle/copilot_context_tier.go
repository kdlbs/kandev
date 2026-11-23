package lifecycle

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/settings/profileconfig"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func isCopilotACPExecution(ctx context.Context, resolver ProfileResolver, execution *AgentExecution) bool {
	if execution == nil {
		return false
	}
	if execution.AgentID == agents.CopilotACPAgentID {
		return true
	}
	if execution.AgentID != "" {
		return false
	}
	if resolver != nil && execution.AgentProfileID != "" {
		if profile, err := resolver.ResolveProfile(ctx, execution.AgentProfileID); err == nil && profile != nil {
			return profile.AgentName == agents.CopilotACPAgentID
		}
	}
	return false
}

func isSynthesizedCopilotContextOptionForExecution(
	ctx context.Context,
	resolver ProfileResolver,
	execution *AgentExecution,
	configID string,
) bool {
	if configID != acpcompat.CopilotContextTierOptionID || !isCopilotACPExecution(ctx, resolver, execution) {
		return false
	}
	state := execution.GetModelState()
	if state == nil {
		return true
	}
	for _, option := range state.ConfigOptions {
		if option.ID == acpcompat.CopilotContextTierOptionID {
			return acpcompat.IsSynthesizedCopilotContextOption(option.ID, option.Description)
		}
	}
	return !acpcompat.HasNativeCopilotContextOption(state.ConfigOptions)
}

func effectiveExecutionCopilotContextTier(execution *AgentExecution) string {
	if execution == nil {
		return acpcompat.CopilotContextTierDefault
	}
	if len(execution.AgentArgs) > 0 {
		tier, _ := acpcompat.ParseCopilotContextTierFromArgs(execution.AgentArgs)
		return tier
	}
	if execution.AgentCommand != "" {
		if tier, explicit := acpcompat.ParseCopilotContextTierFromArgs(strings.Fields(execution.AgentCommand)); explicit {
			return tier
		}
	}
	if state := execution.GetModelState(); state != nil {
		for _, option := range state.ConfigOptions {
			if option.ID == acpcompat.CopilotContextTierOptionID && acpcompat.IsValidCopilotContextTier(option.CurrentValue) {
				return option.CurrentValue
			}
		}
	}
	return acpcompat.CopilotContextTierDefault
}

func validateSynthesizedCopilotContextTierReplay(execution *AgentExecution, requested string) error {
	if !acpcompat.IsValidCopilotContextTier(requested) {
		return fmt.Errorf("unsupported Copilot context tier %q", requested)
	}
	effective := effectiveExecutionCopilotContextTier(execution)
	if effective != requested {
		return fmt.Errorf("copilot context tier %q does not match startup tier %q", requested, effective)
	}
	return nil
}

func (m *Manager) applySynthesizedCopilotContextTier(
	ctx context.Context,
	execution *AgentExecution,
	configID, value, expectedModelID string,
) (SessionConfigurationResult, error) {
	unknown := SessionConfigurationResult{ConfigID: configID, Requested: value, ExecutionID: execution.ID}
	if !execution.remoteInstanceLifecycleMu.TryLock() {
		return unknown, fmt.Errorf("set config option failed: cannot change Copilot context length while session is busy")
	}
	defer execution.remoteInstanceLifecycleMu.Unlock()

	if !execution.contextResetAdmissionMu.TryLock() {
		return unknown, fmt.Errorf("set config option failed: cannot change Copilot context length while session is busy")
	}
	defer execution.contextResetAdmissionMu.Unlock()

	execution.sessionConfigurationMu.Lock()
	defer execution.sessionConfigurationMu.Unlock()

	if execution.idleSuspensionInProgress.Load() || execution.contextResetAdmissionError() != nil {
		return unknown, fmt.Errorf("set config option failed: cannot change Copilot context length while session is busy")
	}
	if err := validateExpectedSessionModel(execution, expectedModelID); err != nil {
		return unknown, err
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		releaseClient()
		return unknown, fmt.Errorf("execution %q has no agentctl client", execution.ID)
	}
	releaseClient()
	if !execution.isSessionInitialized() || execution.ACPSessionID == "" {
		return unknown, fmt.Errorf("execution %q ACP session is not ready", execution.ID)
	}
	if !acpcompat.IsValidCopilotContextTier(value) {
		return unknown, fmt.Errorf("set config option failed: unsupported Copilot context tier %q", value)
	}
	previousPromptGen, err := m.checkExecutionIdleForCopilotContextReload(execution)
	if err != nil {
		return unknown, err
	}
	if effectiveExecutionCopilotContextTier(execution) == value {
		return SessionConfigurationResult{
			ConfigID:    configID,
			Requested:   value,
			Effective:   value,
			Confirmed:   true,
			ExecutionID: execution.ID,
		}, nil
	}
	return m.reloadCopilotSessionForContextTier(ctx, execution, configID, value, previousPromptGen)
}

func validateExpectedSessionModel(execution *AgentExecution, expectedModelID string) error {
	if expectedModelID == "" {
		return nil
	}
	modelState := execution.GetModelState()
	if modelState == nil || modelState.CurrentModelID != expectedModelID {
		actual := ""
		if modelState != nil {
			actual = modelState.CurrentModelID
		}
		return fmt.Errorf("%w: expected %q, found %q", ErrSessionConfigurationModelChanged, expectedModelID, actual)
	}
	return nil
}

func (m *Manager) checkExecutionIdleForCopilotContextReload(execution *AgentExecution) (uint64, error) {
	if !execution.promptLifecycleMu.TryLock() {
		return 0, fmt.Errorf("set config option failed: cannot change Copilot context length while a turn is running")
	}
	previousPromptGen := execution.promptGeneration
	busy := (execution.Status != v1.AgentStatusReady && execution.Status != v1.AgentStatusCompleted) ||
		execution.dispatchedPromptPending.Load() ||
		execution.activeToolSnapshot() != nil ||
		m.hasTrackedExecutionActivity(execution.ID)
	execution.promptLifecycleMu.Unlock()
	if busy {
		return 0, fmt.Errorf("set config option failed: cannot change Copilot context length while a turn is running")
	}
	return previousPromptGen, nil
}

func (m *Manager) buildCopilotReloadRuntimeConfig(
	ctx context.Context,
	execution *AgentExecution,
	targetContextTier string,
) models.SessionRuntimeConfig {
	previousModelState := execution.GetModelState()
	profileModel, profileMode, profileOptions := m.resolveProfileSessionConfig(ctx, execution.AgentProfileID)
	runtimeModel, runtimeMode, runtimeOptions := m.sessionRuntimeOverrides(ctx, execution)

	mergedOptions := make(map[string]string)
	maps.Copy(mergedOptions, profileconfig.SanitizeConfigOptions(profileOptions))
	if liveStateOptions, ok := liveSessionRuntimeConfigOptions(previousModelState); ok {
		maps.Copy(mergedOptions, liveStateOptions)
	}
	maps.Copy(mergedOptions, profileconfig.SanitizeConfigOptions(runtimeOptions))
	delete(mergedOptions, acpcompat.CopilotContextTierOptionID)
	sanitized := sanitizeRuntimeConfigOptionsWithCatalog(mergedOptions, previousModelState)
	if sanitized == nil {
		sanitized = make(map[string]string, 1)
	}
	sanitized[acpcompat.CopilotContextTierOptionID] = targetContextTier

	model := profileModel
	if previousModelState != nil && strings.TrimSpace(previousModelState.CurrentModelID) != "" {
		model = previousModelState.CurrentModelID
	}
	if strings.TrimSpace(runtimeModel) != "" {
		model = runtimeModel
	}

	mode := profileMode
	if modeState := execution.GetModeState(); modeState != nil && strings.TrimSpace(modeState.CurrentModeID) != "" {
		mode = modeState.CurrentModeID
	}
	if strings.TrimSpace(runtimeMode) != "" {
		mode = runtimeMode
	}

	return models.SessionRuntimeConfig{
		Model:         model,
		Mode:          mode,
		ConfigOptions: sanitized,
	}
}

func (m *Manager) preflightCopilotContextReload(
	ctx context.Context,
	execution *AgentExecution,
	runtimeConfig models.SessionRuntimeConfig,
) (agentRestartPreparation, error) {
	preparation, err := m.prepareAgentRestart(ctx, execution, &runtimeConfig)
	if err != nil {
		return agentRestartPreparation{}, fmt.Errorf("set config option failed: %w", err)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		releaseClient()
		return agentRestartPreparation{}, fmt.Errorf("execution %q has no agentctl client", execution.ID)
	}
	wrapperErr := client.ValidateLaunchWrapper(ctx, preparation.commands.wrapper)
	releaseClient()
	if wrapperErr != nil {
		return agentRestartPreparation{}, fmt.Errorf("set config option failed: %w", wrapperErr)
	}
	if runtime := preparation.agentConfig.Runtime(); runtime != nil {
		if err := m.prepareCursorMCPAuth(
			execution,
			preparation.profileInfo,
			execution.ExecutorType,
			runtime.ProjectMCPStrategy,
		); err != nil {
			return agentRestartPreparation{}, fmt.Errorf("set config option failed: %w", err)
		}
	}
	return preparation, nil
}

func (m *Manager) reloadCopilotSessionForContextTier(
	ctx context.Context,
	execution *AgentExecution,
	configID, value string,
	previousPromptGen uint64,
) (SessionConfigurationResult, error) {
	unknown := SessionConfigurationResult{ConfigID: configID, Requested: value, ExecutionID: execution.ID}
	runtimeConfig := m.buildCopilotReloadRuntimeConfig(ctx, execution, value)
	preparation, err := m.preflightCopilotContextReload(ctx, execution, runtimeConfig)
	if err != nil {
		return unknown, err
	}

	previousACPSessionID := execution.ACPSessionID
	previousModelState := execution.GetModelState()
	canFallbackToNewSession := previousPromptGen == 0 &&
		(m.sessionManager == nil || m.sessionManager.historyManager == nil || !m.sessionManager.historyManager.HasHistory(execution.SessionID))

	operation, err := m.restartCopilotSubprocessForContextReload(ctx, execution, preparation)
	if err != nil {
		return unknown, err
	}
	if err := m.restoreCopilotACPSessionAfterReload(
		operation.context(ctx),
		execution,
		preparation.agentConfig,
		operation.servers,
		previousACPSessionID,
		canFallbackToNewSession,
	); err != nil {
		m.updateExecutionError(execution.ID, "failed to restore Copilot ACP session after reload: "+err.Error())
		return unknown, err
	}
	if err := m.finalizeCopilotContextReloadState(ctx, execution, previousModelState, preparation.runtimeConfig, value); err != nil {
		return unknown, err
	}

	m.logger.Info("copilot ACP process reloaded with updated context length",
		zap.String("execution_id", execution.ID),
		zap.String("session_id", execution.SessionID),
		zap.String("acp_session_id", execution.ACPSessionID),
		zap.String("context_tier", value))

	return SessionConfigurationResult{
		ConfigID:    configID,
		Requested:   value,
		Effective:   value,
		Confirmed:   true,
		ExecutionID: execution.ID,
	}, nil
}

func (m *Manager) restartCopilotSubprocessForContextReload(
	ctx context.Context,
	execution *AgentExecution,
	preparation agentRestartPreparation,
) (*preparedMCPOperation, error) {
	if preparation.profileInfo != nil {
		_ = m.executionStore.WithLock(execution.ID, func(current *AgentExecution) {
			if preparation.profileInfo.ProfileID != "" {
				current.ExecutionProfileID = preparation.profileInfo.ProfileID
			}
			current.CopilotSelectedPlugins = append([]string{}, preparation.profileInfo.CopilotSelectedPlugins...)
		})
	}
	execution.beginStartupAttemptPreservingIdentity()

	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		releaseClient()
		return nil, fmt.Errorf("execution %q has no agentctl client", execution.ID)
	}
	client.CloseUpdatesStream()
	client.CloseWorkspaceStream()
	releaseClient()

	m.stopAgentProcessForRestart(ctx, execution)
	m.resetAgentRestartState(execution.ID, preparation)

	client, releaseClient = execution.AcquireAgentCtlClient()
	if client == nil {
		releaseClient()
		return nil, fmt.Errorf("execution %q has no agentctl client", execution.ID)
	}
	readyErr := client.WaitForReady(ctx, 30*time.Second)
	releaseClient()
	if readyErr != nil {
		m.updateExecutionError(execution.ID, "agentctl not ready after reload: "+readyErr.Error())
		return nil, fmt.Errorf("agentctl not ready after reload: %w", readyErr)
	}

	operation, err := m.prepareExecutionMCP(ctx, execution, preparation.agentConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve MCP config for reload: %w", err)
	}

	var configureErr error
	if preparation.copilotPluginArgsPrepared {
		_, configureErr = m.configureAndStartAgentWithPreparedArgs(ctx, execution, preparation.copilotPluginArgs)
	} else {
		_, configureErr = m.configureAndStartAgentWithMCP(ctx, execution, preparation.agentConfig, operation)
	}
	if configureErr != nil {
		m.updateExecutionError(execution.ID, "failed to restart Copilot agent: "+configureErr.Error())
		return nil, fmt.Errorf("failed to restart Copilot agent: %w", configureErr)
	}

	client, releaseClient = execution.AcquireAgentCtlClient()
	if client != nil {
		if waitErr := client.WaitForReady(ctx, 10*time.Second); waitErr != nil {
			m.logger.Warn("copilot agent slow to initialize after context length reload",
				zap.String("execution_id", execution.ID),
				zap.Error(waitErr))
		}
	}
	releaseClient()
	return operation, nil
}

func (m *Manager) restoreCopilotACPSessionAfterReload(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	mcpServers []agentctltypes.McpServer,
	previousACPSessionID string,
	canFallbackToNewSession bool,
) error {
	setup, revision, captured := capturedMCPSetup(ctx)
	mcpStartupGeneration := execution.startupAttemptSnapshot()
	attempt := execution.beginDeliveryStartupAttempt(ctx)
	defer execution.releaseDeliveryStartupAttempt(attempt)
	defer attempt.fail(ErrDurableAdoptionBlocked)

	if err := m.connectReloadStreams(execution); err != nil {
		attempt.fail(err)
		return err
	}

	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		releaseClient()
		err := fmt.Errorf("agentctl client is unavailable")
		attempt.fail(err)
		return err
	}
	defer releaseClient()

	if _, err := client.Initialize(ctx, "kandev", "1.0.0"); err != nil {
		wrappedErr := fmt.Errorf("initialize reloaded Copilot ACP adapter: %w", err)
		attempt.fail(wrappedErr)
		return wrappedErr
	}
	if m.sessionManager != nil {
		if err := m.sessionManager.publishNegotiatedDeliveryMode(client, execution, attempt, agentConfig.ID()); err != nil {
			attempt.fail(err)
			return err
		}
	} else {
		attempt.finishNegotiation(nil)
	}

	restoredSessionID, attemptID, err := m.loadOrCreateCopilotReloadSession(
		ctx,
		execution,
		client,
		agentConfig,
		mcpServers,
		previousACPSessionID,
		canFallbackToNewSession,
	)
	if err != nil {
		attempt.fail(err)
		return err
	}

	execution.ACPSessionID = restoredSessionID
	execution.setSessionInitialized(true)
	if captured {
		execution.withStartupAttempt(mcpStartupGeneration, func(string) {
			m.recordAcceptedMCPSetupOperation(ctx, execution, setup, revision, attemptID)
		})
	}
	if restoredSessionID != previousACPSessionID && m.sessionManager != nil && m.sessionManager.eventPublisher != nil {
		m.sessionManager.eventPublisher.PublishACPSessionCreatedWithAttempt(
			execution,
			restoredSessionID,
			ResumeAttemptIDFromContext(ctx),
		)
	}
	attempt.finishNative(nil)
	return nil
}

func (m *Manager) connectReloadStreams(execution *AgentExecution) error {
	if m.streamManager == nil {
		return nil
	}
	updatesReady := make(chan struct{})
	m.streamManager.ConnectAll(execution, updatesReady)
	select {
	case <-updatesReady:
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timeout waiting for agent stream to connect after reload")
	}
}

func (m *Manager) loadOrCreateCopilotReloadSession(
	ctx context.Context,
	execution *AgentExecution,
	client *agentctl.Client,
	agentConfig agents.Agent,
	mcpServers []agentctltypes.McpServer,
	previousACPSessionID string,
	canFallbackToNewSession bool,
) (string, string, error) {
	attemptID, loadErr := client.LoadSessionWithResult(
		ctx,
		previousACPSessionID,
		mcpServers,
		streams.SessionSettingsPolicyStrict,
	)
	if loadErr == nil {
		return previousACPSessionID, attemptID, nil
	}
	if !canFallbackToNewSession || m.sessionManager == nil || isTransportDeadErr(loadErr) {
		return "", "", fmt.Errorf("failed to restore Copilot ACP session %q: %w", previousACPSessionID, loadErr)
	}
	created, newErr := m.sessionManager.createNewSession(
		ctx,
		client,
		agentConfig,
		execution.WorkspacePath,
		mcpServers,
	)
	if newErr != nil {
		return "", "", fmt.Errorf("failed to create fresh Copilot ACP session after empty load miss: %w", newErr)
	}
	return created.SessionID, created.AttachmentAttemptID, nil
}

func (m *Manager) finalizeCopilotContextReloadState(
	ctx context.Context,
	execution *AgentExecution,
	previousModelState *CachedModelState,
	runtimeConfig models.SessionRuntimeConfig,
	targetContextTier string,
) error {
	if !cacheFreshSessionModelState(execution) {
		if previousModelState != nil {
			execution.SetModelState(cloneModelStateForCopilotReload(previousModelState, runtimeConfig, targetContextTier))
		} else {
			waitForFreshSessionModelState(ctx, m.logger, execution)
		}
	}
	if err := m.restoreSessionRuntimeConfig(ctx, execution, execution.ACPSessionID, runtimeConfig); err != nil {
		m.updateExecutionError(execution.ID, "failed to restore session runtime configuration after reload: "+err.Error())
		return fmt.Errorf("failed to restore session runtime configuration after reload: %w", err)
	}
	m.synchronizeReloadedCopilotModelState(execution, previousModelState, runtimeConfig, targetContextTier)
	if err := m.updateStatusAndPersist(ctx, execution.ID, v1.AgentStatusReady); err != nil {
		m.updateExecutionError(execution.ID, "failed to mark reloaded Copilot agent ready: "+err.Error())
		return fmt.Errorf("failed to mark reloaded Copilot agent ready: %w", err)
	}
	if m.eventPublisher != nil {
		m.eventPublisher.PublishAgentEvent(ctx, events.AgentBootReady, execution)
	}
	m.RequestSessionMCPReconfiguration(context.WithoutCancel(ctx), execution.SessionID)
	return nil
}

func cloneModelStateForCopilotReload(
	base *CachedModelState,
	runtimeConfig models.SessionRuntimeConfig,
	targetContextTier string,
) *CachedModelState {
	var currentModel string
	var modelsList []streams.SessionModelInfo
	var options []streams.ConfigOption
	var settled bool
	if base != nil {
		cloned := cloneCachedModelState(base)
		currentModel = cloned.CurrentModelID
		modelsList = cloned.Models
		options = cloned.ConfigOptions
		settled = cloned.ConfigOptionsSettled
	}
	if runtimeConfig.Model != "" {
		currentModel = runtimeConfig.Model
	}
	options, _ = acpcompat.NormalizeCopilotConfigOptions(options, targetContextTier)
	_ = setConfigOptionValue(options, acpcompat.CopilotContextTierOptionID, targetContextTier)
	if currentModel != "" {
		_ = setConfigOptionValue(options, modelConfigOptionID, currentModel)
	}
	if runtimeConfig.Mode != "" {
		_ = setConfigOptionValue(options, "mode", runtimeConfig.Mode)
	}
	for id, val := range runtimeConfig.ConfigOptions {
		_ = setConfigOptionValue(options, id, val)
	}
	return &CachedModelState{
		CurrentModelID:       currentModel,
		Models:               modelsList,
		ConfigOptions:        options,
		ConfigOptionsSettled: settled || len(options) > 0,
	}
}

func (m *Manager) synchronizeReloadedCopilotModelState(
	execution *AgentExecution,
	previousModelState *CachedModelState,
	runtimeConfig models.SessionRuntimeConfig,
	targetContextTier string,
) {
	base := execution.GetModelState()
	if base == nil {
		base = previousModelState
	}
	updated := cloneModelStateForCopilotReload(base, runtimeConfig, targetContextTier)
	execution.SetModelState(updated)
	if m.eventPublisher != nil && (updated.CurrentModelID != "" || len(updated.ConfigOptions) > 0) {
		m.eventPublisher.PublishAgentStreamEvent(execution, agentctl.AgentEvent{
			Type:                  streams.EventTypeSessionModels,
			SessionID:             execution.ACPSessionID,
			SessionSettingsPolicy: providerRestoredSettingsPolicy(execution),
			CurrentModelID:        updated.CurrentModelID,
			SessionModels:         updated.Models,
			ConfigOptions:         updated.ConfigOptions,
		})
	}
}
