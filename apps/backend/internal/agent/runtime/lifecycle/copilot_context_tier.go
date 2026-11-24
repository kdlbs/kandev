package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
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

func launchedExecutionCopilotContextTier(execution *AgentExecution) string {
	if execution == nil {
		return acpcompat.CopilotContextTierDefault
	}
	if tier := execution.confirmedCopilotContextTier.Load(); tier != nil {
		return *tier
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
	return acpcompat.CopilotContextTierDefault
}

func (execution *AgentExecution) setConfirmedCopilotContextTier(tier string) {
	if execution == nil || !acpcompat.IsValidCopilotContextTier(tier) {
		return
	}
	confirmed := tier
	execution.confirmedCopilotContextTier.Store(&confirmed)
}

func stagedSynthesizedCopilotContextTier(state *CachedModelState) (string, bool) {
	if state == nil {
		return "", false
	}
	for _, option := range state.ConfigOptions {
		if acpcompat.IsSynthesizedCopilotContextOption(option.ID, option.Description) &&
			acpcompat.IsValidCopilotContextTier(option.CurrentValue) {
			return option.CurrentValue, true
		}
	}
	return "", false
}

func effectiveExecutionCopilotContextTier(execution *AgentExecution) string {
	if execution == nil {
		return acpcompat.CopilotContextTierDefault
	}
	if staged, ok := stagedSynthesizedCopilotContextTier(execution.GetModelState()); ok {
		return staged
	}
	return launchedExecutionCopilotContextTier(execution)
}

func preserveStagedCopilotContextTier(
	execution *AgentExecution,
	incoming []streams.ConfigOption,
) []streams.ConfigOption {
	if execution == nil || execution.AgentID != agents.CopilotACPAgentID || len(incoming) == 0 {
		return incoming
	}
	staged, ok := stagedSynthesizedCopilotContextTier(execution.GetModelState())
	if !ok {
		return incoming
	}
	for i := range incoming {
		opt := incoming[i]
		if acpcompat.IsSynthesizedCopilotContextOption(opt.ID, opt.Description) && opt.CurrentValue != staged {
			cloned := append([]streams.ConfigOption(nil), incoming...)
			for j := range cloned {
				cloned[j].Options = append([]streams.ConfigOptionValue(nil), incoming[j].Options...)
			}
			cloned[i].CurrentValue = staged
			return cloned
		}
	}
	return incoming
}

func validateSynthesizedCopilotContextTierReplay(execution *AgentExecution, requested string) error {
	if !acpcompat.IsValidCopilotContextTier(requested) {
		return fmt.Errorf("unsupported Copilot context tier %q", requested)
	}
	launched := launchedExecutionCopilotContextTier(execution)
	if launched != requested {
		return fmt.Errorf("copilot context tier %q does not match startup tier %q", requested, launched)
	}
	return nil
}

func (m *Manager) applySynthesizedCopilotContextTier(
	ctx context.Context,
	execution *AgentExecution,
	configID, value, expectedModelID string,
) (SessionConfigurationResult, error) {
	unknown := SessionConfigurationResult{ConfigID: configID, Requested: value, ExecutionID: execution.ID}
	execution.sessionConfigurationMu.Lock()
	defer execution.sessionConfigurationMu.Unlock()

	if err := ctx.Err(); err != nil || !execution.contextResetAdmissionMu.TryLock() {
		return unknown, fmt.Errorf("set config option failed: cannot change Copilot context length while session is busy")
	}
	defer execution.contextResetAdmissionMu.Unlock()
	if err := execution.contextResetAdmissionError(); err != nil {
		return unknown, fmt.Errorf("set config option failed: cannot change Copilot context length while session is busy")
	}

	if execution.idleSuspensionInProgress.Load() {
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
	acpSessionID, cliTier := m.readExecutionCopilotReloadIdentity(execution)
	if !execution.isSessionInitialized() || acpSessionID == "" {
		return unknown, fmt.Errorf("execution %q ACP session is not ready", execution.ID)
	}
	if !acpcompat.IsValidCopilotContextTier(value) {
		return unknown, fmt.Errorf("set config option failed: unsupported Copilot context tier %q", value)
	}
	if _, err := m.checkExecutionIdleForCopilotContextReload(execution); err != nil {
		return unknown, err
	}
	if value != cliTier {
		runtimeConfig := m.buildCopilotReloadRuntimeConfig(ctx, execution, value)
		if _, err := m.preflightCopilotContextReload(ctx, execution, runtimeConfig); err != nil {
			return unknown, err
		}
	}
	m.stageCopilotContextTierState(execution, acpSessionID, value)
	return SessionConfigurationResult{
		ConfigID:    configID,
		Requested:   value,
		Effective:   value,
		Confirmed:   true,
		ExecutionID: execution.ID,
	}, nil
}

func (m *Manager) readExecutionCopilotReloadIdentity(execution *AgentExecution) (string, string) {
	if execution == nil {
		return "", acpcompat.CopilotContextTierDefault
	}
	var acpSessionID string
	cliTier := acpcompat.CopilotContextTierDefault
	readFn := func(exec *AgentExecution) {
		acpSessionID = exec.ACPSessionID
		cliTier = launchedExecutionCopilotContextTier(exec)
	}
	if m != nil && m.executionStore != nil && m.executionStore.WithLock(execution.ID, readFn) == nil {
		return acpSessionID, cliTier
	}
	readFn(execution)
	return acpSessionID, cliTier
}

func (m *Manager) stageCopilotContextTierState(execution *AgentExecution, acpSessionID, desiredTier string) {
	stageCopilotContextTierOnExecution(execution, acpSessionID, desiredTier, m.eventPublisher)
}

func restageSynthesizedCopilotContextTier(
	execution *AgentExecution,
	acpSessionID, requested string,
	publisher *EventPublisher,
) error {
	if !acpcompat.IsValidCopilotContextTier(requested) {
		return fmt.Errorf("unsupported Copilot context tier %q", requested)
	}
	stageCopilotContextTierOnExecution(execution, acpSessionID, requested, publisher)
	return nil
}

func stageCopilotContextTierOnExecution(
	execution *AgentExecution,
	acpSessionID, desiredTier string,
	publisher *EventPublisher,
) {
	base := execution.GetModelState()
	var updated *CachedModelState
	if base != nil {
		if staged, ok := stagedSynthesizedCopilotContextTier(base); ok && staged == desiredTier {
			return
		}
		updated = cloneCachedModelState(base)
	} else {
		updated = &CachedModelState{}
	}
	updated.ConfigOptions, _ = acpcompat.NormalizeCopilotConfigOptions(updated.ConfigOptions, desiredTier)
	_ = setConfigOptionValue(updated.ConfigOptions, acpcompat.CopilotContextTierOptionID, desiredTier)
	execution.SetModelState(updated)
	if publisher != nil && (updated.CurrentModelID != "" || len(updated.ConfigOptions) > 0) {
		publisher.PublishAgentStreamEvent(execution, agentctl.AgentEvent{
			Type:                  streams.EventTypeSessionModels,
			SessionID:             acpSessionID,
			SessionSettingsPolicy: providerRestoredSettingsPolicy(execution),
			CurrentModelID:        updated.CurrentModelID,
			SessionModels:         updated.Models,
			ConfigOptions:         updated.ConfigOptions,
		})
	}
}

func (m *Manager) needsCopilotContextTierReload(ctx context.Context, execution *AgentExecution) (string, bool) {
	if !isCopilotACPExecution(ctx, m.profileResolver, execution) || execution.PassthroughProcessID != "" {
		return "", false
	}
	acpSessionID, cliTier := m.readExecutionCopilotReloadIdentity(execution)
	if acpSessionID == "" {
		return "", false
	}
	desired := cliTier
	if staged, ok := stagedSynthesizedCopilotContextTier(execution.GetModelState()); ok {
		desired = staged
	} else if _, _, opts := m.sessionRuntimeOverrides(ctx, execution); acpcompat.IsValidCopilotContextTier(opts[acpcompat.CopilotContextTierOptionID]) {
		desired = opts[acpcompat.CopilotContextTierOptionID]
	}
	if desired == cliTier && execution.isSessionInitialized() {
		return desired, false
	}
	return desired, true
}

func (m *Manager) hasActivePromptTurnOnExecution(execution *AgentExecution) bool {
	if !execution.promptLifecycleMu.TryLock() {
		return true
	}
	var status v1.AgentStatus
	if m != nil && m.executionStore != nil && m.executionStore.WithLock(execution.ID, func(exec *AgentExecution) {
		status = exec.Status
	}) != nil {
		status = execution.Status
	}
	active := status == v1.AgentStatusRunning ||
		execution.dispatchedPromptPending.Load() ||
		execution.activeToolSnapshot() != nil
	execution.promptLifecycleMu.Unlock()
	return active
}

func (m *Manager) reconcileCopilotContextTierBeforePrompt(ctx context.Context, execution *AgentExecution) error {
	if _, needed := m.needsCopilotContextTierReload(ctx, execution); !needed || m.hasActivePromptTurnOnExecution(execution) {
		return nil
	}
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()

	operationRelease, err := execution.acquireContextResetExclusive(ctx)
	if err != nil {
		return err
	}
	defer operationRelease()

	if m.hasActivePromptTurnOnExecution(execution) {
		return nil
	}
	desired, needed := m.needsCopilotContextTierReload(ctx, execution)
	if !needed {
		return nil
	}
	execution.promptLifecycleMu.Lock()
	previousPromptGen := execution.promptGeneration
	execution.promptLifecycleMu.Unlock()
	return m.reloadCopilotSessionForContextTier(ctx, execution, desired, previousPromptGen)
}

func (m *Manager) acquirePromptContextOperation(ctx context.Context, execution *AgentExecution) (func(), error) {
	for {
		if err := m.reconcileCopilotContextTierBeforePrompt(ctx, execution); err != nil {
			return nil, err
		}
		release, err := execution.acquireContextResetOperation(ctx)
		if err != nil {
			return nil, err
		}
		if _, needed := m.needsCopilotContextTierReload(ctx, execution); !needed {
			return release, nil
		}
		release()
		if m.hasActivePromptTurnOnExecution(execution) {
			return nil, errors.New("copilot context tier reload is waiting for the active turn")
		}
	}
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
	var status v1.AgentStatus
	if m.executionStore == nil || m.executionStore.WithLock(execution.ID, func(exec *AgentExecution) {
		status = exec.Status
	}) != nil {
		status = execution.Status
	}
	busy := (status != v1.AgentStatusReady && status != v1.AgentStatusCompleted) ||
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
	cfg := m.captureSessionRuntimeConfigForReset(ctx, execution)
	if cfg.ConfigOptions == nil {
		cfg.ConfigOptions = make(map[string]string, 1)
	}
	cfg.ConfigOptions[acpcompat.CopilotContextTierOptionID] = targetContextTier
	return cfg
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
	requestedTier := runtimeConfig.ConfigOptions[acpcompat.CopilotContextTierOptionID]
	launchedTier, _ := acpcompat.ParseCopilotContextTierFromArgs(preparation.commands.args)
	if acpcompat.IsValidCopilotContextTier(requestedTier) && launchedTier != requestedTier {
		return agentRestartPreparation{}, fmt.Errorf(
			"set config option failed: Copilot profile flags select context tier %q, not requested tier %q",
			launchedTier, requestedTier,
		)
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
	value string,
	previousPromptGen uint64,
) error {
	runtimeConfig := m.buildCopilotReloadRuntimeConfig(ctx, execution, value)
	preparation, err := m.preflightCopilotContextReload(ctx, execution, runtimeConfig)
	if err != nil {
		return err
	}

	previousACPSessionID, _ := m.readExecutionCopilotReloadIdentity(execution)
	previousModelState := execution.GetModelState()
	restoreOnError := func() {
		if previousACPSessionID != "" {
			if m.executionStore == nil || m.executionStore.WithLock(execution.ID, func(exec *AgentExecution) {
				if exec.ACPSessionID == "" {
					exec.ACPSessionID = previousACPSessionID
				}
			}) != nil {
				if execution.ACPSessionID == "" {
					execution.ACPSessionID = previousACPSessionID
				}
			}
		}
		if execution.GetModelState() == nil && previousModelState != nil {
			execution.SetModelState(previousModelState)
		}
	}
	canFallbackToNewSession := m.canFallbackToNewCopilotSession(ctx, execution, previousPromptGen)

	operation, err := m.restartCopilotSubprocessForContextReload(ctx, execution, preparation)
	if err != nil {
		restoreOnError()
		return err
	}
	if err := m.restoreCopilotACPSessionAfterReload(
		operation.context(ctx),
		execution,
		preparation.agentConfig,
		operation.servers,
		previousACPSessionID,
		canFallbackToNewSession,
	); err != nil {
		restoreOnError()
		m.updateExecutionError(execution.ID, "failed to restore Copilot ACP session after reload: "+err.Error())
		return err
	}
	if err := m.finalizeCopilotContextReloadState(ctx, execution, previousModelState, preparation.runtimeConfig, value); err != nil {
		restoreOnError()
		return err
	}

	m.logger.Info("copilot ACP process reloaded with updated context length",
		zap.String("execution_id", execution.ID),
		zap.String("session_id", execution.SessionID),
		zap.String("acp_session_id", execution.ACPSessionID),
		zap.String("context_tier", value))

	return nil
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
	if err := client.StopAgentProcess(ctx); err != nil {
		releaseClient()
		return nil, fmt.Errorf("failed to stop Copilot child process for context reload: %w", err)
	}
	client.CloseUpdatesStream()
	client.CloseWorkspaceStream()
	releaseClient()
	m.resetAgentRestartState(execution.ID, preparation)

	client, releaseClient = execution.AcquireAgentCtlClient()
	if client == nil {
		releaseClient()
		return nil, fmt.Errorf("execution %q has no agentctl client", execution.ID)
	}
	var readyErr error
	if client.Health(ctx) != nil {
		readyErr = client.WaitForReady(ctx, 30*time.Second)
	}
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
	if client != nil && client.Health(ctx) != nil {
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

	if err := m.connectReloadStreams(ctx, execution); err != nil {
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

	if m.executionStore == nil || m.executionStore.WithLock(execution.ID, func(exec *AgentExecution) {
		exec.ACPSessionID = restoredSessionID
	}) != nil {
		execution.ACPSessionID = restoredSessionID
	}
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

func (m *Manager) connectReloadStreams(ctx context.Context, execution *AgentExecution) error {
	if m.streamManager == nil {
		return nil
	}
	updatesReady := make(chan struct{})
	m.streamManager.ConnectAll(execution, updatesReady)
	select {
	case <-updatesReady:
		return nil
	case <-ctx.Done():
		return ctx.Err()
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
	if !canFallbackToNewSession || !isEmptyCopilotNativeSessionMiss(loadErr, previousACPSessionID) ||
		m.sessionManager == nil || isTransportDeadErr(loadErr) {
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

func isEmptyCopilotNativeSessionMiss(err error, expectedSessionID string) bool {
	var restoreErr *agentctl.SessionRestoreOperationError
	if expectedSessionID == "" || !errors.As(err, &restoreErr) || restoreErr.Code != "SESSION_RESTORE_REQUIRED" {
		return false
	}
	return restoreErr.Details["reason"] == "native_state_missing" &&
		restoreErr.Details["session_id"] == expectedSessionID
}

func (m *Manager) canFallbackToNewCopilotSession(
	ctx context.Context,
	execution *AgentExecution,
	previousPromptGeneration uint64,
) bool {
	if previousPromptGeneration != 0 || execution == nil || m.executorProfileReader == nil {
		return false
	}
	if m.sessionManager != nil && m.sessionManager.historyManager != nil &&
		m.sessionManager.historyManager.HasHistory(execution.SessionID) {
		return false
	}
	promptHistory, ok := m.executorProfileReader.(interface {
		HasUserPromptHistory(context.Context, string) (bool, error)
	})
	if !ok {
		return false
	}
	hasPrompt, err := promptHistory.HasUserPromptHistory(ctx, execution.SessionID)
	return err == nil && !hasPrompt
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
	acpSessionID, _ := m.readExecutionCopilotReloadIdentity(execution)
	client, releaseClient := execution.AcquireAgentCtlClient()
	decision, err := m.restoreSessionRuntimeConfigWithClientAndDecision(
		ctx, execution, client, acpSessionID, runtimeConfig,
	)
	releaseClient()
	if err != nil {
		execution.setSessionInitialized(false)
		m.updateExecutionError(execution.ID, "failed to restore session runtime configuration after reload: "+err.Error())
		return fmt.Errorf("failed to restore session runtime configuration after reload: %w", err)
	}
	effectiveModel := decision.EffectiveModel
	if effectiveModel == "" {
		if modelState := execution.GetModelState(); modelState != nil {
			effectiveModel = modelState.CurrentModelID
		}
	}
	m.synchronizeReloadedCopilotModelState(
		execution, previousModelState, runtimeConfig, targetContextTier, effectiveModel,
	)
	if err := m.updateStatusAndPersist(ctx, execution.ID, v1.AgentStatusReady); err != nil {
		m.updateExecutionError(execution.ID, "failed to mark reloaded Copilot agent ready: "+err.Error())
		return fmt.Errorf("failed to mark reloaded Copilot agent ready: %w", err)
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
	effectiveModel string,
) {
	base := execution.GetModelState()
	if base == nil {
		base = previousModelState
	}
	updated := cloneModelStateForCopilotReload(base, runtimeConfig, targetContextTier)
	if effectiveModel != "" {
		updated.CurrentModelID = effectiveModel
		_ = setConfigOptionValue(updated.ConfigOptions, modelConfigOptionID, effectiveModel)
	}
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
