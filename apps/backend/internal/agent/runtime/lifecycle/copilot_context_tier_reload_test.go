package lifecycle

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func newCopilotLiveReloadFixture(
	t *testing.T,
	status v1.AgentStatus,
	currentTier string,
	profilePrefix string,
) (*Manager, *restartMockAgentctlServer, *AgentExecution, *mockWorkspaceInfoProvider) {
	t.Helper()

	mgr := newTestManager(t)
	profile := &AgentProfileInfo{
		ProfileID:     "copilot-profile",
		AgentName:     agents.CopilotACPAgentID,
		Model:         "gpt-5.4",
		CommandPrefix: profilePrefix,
		ConfigOptions: map[string]string{
			"context_tier":     currentTier,
			"reasoning_effort": "medium",
		},
	}
	mgr.profileResolver = &restartProfileResolver{profile: profile}
	provider := &mockWorkspaceInfoProvider{
		infos: map[string]*WorkspaceInfo{
			"session-live": {
				SessionID:               "session-live",
				RuntimeModel:            "claude-sonnet-4.6",
				RuntimeConfigOptionsSet: true,
				RuntimeConfigOptions: map[string]string{
					"reasoning_effort": "high",
				},
			},
		},
	}
	mgr.workspaceInfoProvider = provider

	mock := newRestartMockAgentctlServer(t, false, false)
	mock.supportsSessionLoad = true
	mock.failConfigOptionID = acpcompat.CopilotContextTierOptionID
	loadedOpts, _ := acpcompat.NormalizeCopilotConfigOptions([]streams.ConfigOption{
		{
			ID:           "reasoning_effort",
			Name:         "Reasoning Effort",
			Category:     "thought_level",
			Type:         "select",
			CurrentValue: "medium",
		},
	}, "long_context")
	mock.modelState = &streams.SessionModelState{
		CurrentModelID: "gpt-5.4",
		Models: []streams.SessionModelInfo{
			{ModelID: "gpt-5.4"},
			{ModelID: "claude-sonnet-4.6"},
		},
		ConfigOptions:        loadedOpts,
		ConfigOptionsSettled: true,
	}
	mock.newModelState = mock.modelState

	t.Cleanup(mgr.streamManager.Wait)
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)

	exec := &AgentExecution{
		ID:                 "exec-copilot-live",
		TaskID:             "task-live",
		SessionID:          "session-live",
		AgentID:            agents.CopilotACPAgentID,
		AgentProfileID:     "copilot-profile",
		RuntimeName:        agentruntime.RuntimeStandalone,
		ACPSessionID:       "copilot-acp-session-1",
		AgentCommand:       "copilot --acp --stdio --context " + currentTier,
		AgentArgs:          []string{"copilot", "--acp", "--stdio", "--context", currentTier},
		Status:             status,
		WorkspacePath:      t.TempDir(),
		sessionInitialized: true,
		agentctl:           client,
		promptDoneCh:       make(chan PromptCompletionSignal, 1),
	}
	exec.beginStartupAttempt()
	initialAttempt := exec.beginDeliveryStartupAttempt(context.Background())
	initialAttempt.finishNegotiation(nil)
	initialAttempt.finishNative(nil)
	streamCtx, cancelStream := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStream()
	require.NoError(t, mgr.streamManager.ConnectAllAndWait(streamCtx, exec))
	initialOpts, _ := acpcompat.NormalizeCopilotConfigOptions([]streams.ConfigOption{
		{
			ID:           "reasoning_effort",
			Name:         "Reasoning Effort",
			Category:     "thought_level",
			Type:         "select",
			CurrentValue: "high",
		},
	}, currentTier)
	exec.SetModelState(&CachedModelState{
		CurrentModelID: "claude-sonnet-4.6",
		Models: []streams.SessionModelInfo{
			{ModelID: "gpt-5.4"},
			{ModelID: "claude-sonnet-4.6"},
		},
		ConfigOptions:        initialOpts,
		ConfigOptionsSettled: true,
	})
	require.NoError(t, mgr.executionStore.Add(exec))
	return mgr, mock, exec, provider
}

// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-001.3
// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-003.1
// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-003.2
// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-003.3
// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-003.4
// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-003.5
func TestCopilotIdleLiveSessionContextTierReload(t *testing.T) {
	t.Run("idle session stages context_tier without restarting and reloads once before next prompt", func(t *testing.T) {
		mgr, mock, exec, provider := newCopilotLiveReloadFixture(t, v1.AgentStatusReady, "default", "")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		res, err := mgr.SetSessionConfigOptionBySessionIDWithExpectedModelWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "long_context", "claude-sonnet-4.6",
		)
		require.NoError(t, err)
		require.True(t, res.Confirmed)
		require.Equal(t, "long_context", res.Effective)
		require.Equal(t, exec.ID, res.ExecutionID)

		require.Empty(t, mock.getHTTPActions())
		require.Equal(t, "long_context", currentContextTierFromModelState(exec.GetModelState()))

		followRes, err := mgr.SetSessionConfigOptionBySessionIDWithExpectedModelWithResult(
			ctx, exec.SessionID, "reasoning_effort", "low", "claude-sonnet-4.6",
		)
		require.NoError(t, err)
		require.Equal(t, exec.ID, followRes.ExecutionID)
		provider.infos[exec.SessionID].RuntimeConfigOptions["reasoning_effort"] = "low"
		provider.infos[exec.SessionID].RuntimeConfigOptions[acpcompat.CopilotContextTierOptionID] = "long_context"

		staleOpts, _ := acpcompat.NormalizeCopilotConfigOptions([]streams.ConfigOption{
			{
				ID:           "reasoning_effort",
				Name:         "Reasoning Effort",
				Category:     "thought_level",
				Type:         "select",
				CurrentValue: "low",
			},
		}, "default")
		processed := mgr.handleAgentEventState(exec, agentctl.AgentEvent{
			Type:           streams.EventTypeSessionModels,
			SessionID:      exec.ACPSessionID,
			CurrentModelID: "claude-sonnet-4.6",
			ConfigOptions:  staleOpts,
		})
		require.Equal(t, "long_context", currentContextTierFromOptions(processed.ConfigOptions))
		require.Equal(t, "long_context", currentContextTierFromModelState(exec.GetModelState()))

		noopRes, err := mgr.SetSessionConfigOptionBySessionIDWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "long_context",
		)
		require.NoError(t, err)
		require.True(t, noopRes.Confirmed)
		require.Equal(t, "long_context", noopRes.Effective)
		require.Empty(t, mock.getHTTPActions())

		mock.mu.Lock()
		mock.emitPromptComplete = true
		mock.mu.Unlock()
		promptCtx, promptCancel := context.WithTimeout(ctx, 5*time.Second)
		defer promptCancel()
		_, err = mgr.PromptAgent(promptCtx, exec.ID, "second-turn after lazy reload", nil, false)
		require.NoError(t, err, "follow-up prompt must reload once and settle")

		require.Equal(t, []string{"stop", "configure", "start"}, mock.getHTTPActions())
		_, configuredArgs, _ := mock.getConfiguredProcess()
		assertContainsFlagPair(t, configuredArgs, "--context", "long_context")
		require.False(t, slices.Contains(configuredArgs, "default"), "configuredArgs=%v", configuredArgs)

		wsActions := mock.getWSActions()
		require.Contains(t, wsActions, "agent.initialize")
		require.Contains(t, wsActions, "agent.session.load")
		require.Contains(t, wsActions, "agent.prompt")
		require.NotContains(t, wsActions, "agent.session.new")
		require.NotContains(t, wsActions, "agent.session.reset")
		mock.mu.Lock()
		loadedIDs := append([]string(nil), mock.loadedSessionIDs...)
		mock.mu.Unlock()
		require.Equal(t, []string{"copilot-acp-session-1"}, loadedIDs)

		require.Equal(t, []string{"claude-sonnet-4.6"}, mock.getSetModelIDs())
		require.Contains(t, mock.getSetOptions(), restartConfigOption{ID: "reasoning_effort", Value: "low"})

		require.Equal(t, "copilot-acp-session-1", exec.ACPSessionID)
		require.Equal(t, v1.AgentStatusReady, exec.Status)
		require.True(t, mgr.WasSessionInitialized(exec.ID))

		state := exec.GetModelState()
		require.NotNil(t, state)
		require.Equal(t, "claude-sonnet-4.6", state.CurrentModelID)
		require.Equal(t, "long_context", effectiveExecutionCopilotContextTier(exec))

		bus := mgr.eventBus.(*MockEventBus)
		eventTypes := publishedEventTypes(bus)
		require.NotContains(t, eventTypes, events.AgentBootReady)
		require.NotContains(t, eventTypes, events.AgentContextReset)
		require.Contains(t, eventTypes, events.AgentReady)

		_, err = mgr.PromptAgent(promptCtx, exec.ID, "third-turn already converged", nil, false)
		require.NoError(t, err)
		require.Equal(t, []string{"stop", "configure", "start"}, mock.getHTTPActions())
	})

	t.Run("toggling context_tier away and back before prompt performs zero restarts", func(t *testing.T) {
		mgr, mock, exec, _ := newCopilotLiveReloadFixture(t, v1.AgentStatusReady, "default", "")
		mock.mu.Lock()
		mock.emitPromptComplete = true
		mock.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := mgr.SetSessionConfigOptionBySessionIDWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "long_context",
		)
		require.NoError(t, err)
		require.Equal(t, "long_context", currentContextTierFromModelState(exec.GetModelState()))

		revertRes, err := mgr.SetSessionConfigOptionBySessionIDWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "default",
		)
		require.NoError(t, err)
		require.True(t, revertRes.Confirmed)
		require.Equal(t, "default", revertRes.Effective)
		require.Equal(t, "default", currentContextTierFromModelState(exec.GetModelState()))

		_, err = mgr.PromptAgent(ctx, exec.ID, "turn with unchanged effective tier", nil, false)
		require.NoError(t, err)
		require.Empty(t, mock.getHTTPActions())
		require.Contains(t, mock.getWSActions(), "agent.prompt")
	})

	t.Run("zero-turn session falls back to session/new on pre-prompt reload when session/load misses", func(t *testing.T) {
		mgr, mock, exec, _ := newCopilotLiveReloadFixture(t, v1.AgentStatusReady, "default", "")
		mock.sessionLoadError = "session not found"
		mock.mu.Lock()
		mock.emitPromptComplete = true
		mock.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		res, err := mgr.SetSessionConfigOptionBySessionIDWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "long_context",
		)
		require.NoError(t, err)
		require.True(t, res.Confirmed)
		require.Empty(t, mock.getHTTPActions())

		_, err = mgr.PromptAgent(ctx, exec.ID, "first turn on zero-turn session", nil, false)
		require.NoError(t, err)
		require.Equal(t, "new-session-123", exec.ACPSessionID)
		require.True(t, mgr.WasSessionInitialized(exec.ID))
		require.Equal(t, v1.AgentStatusReady, exec.Status)
		wsActions := mock.getWSActions()
		require.Contains(t, wsActions, "agent.session.load")
		require.Contains(t, wsActions, "agent.session.new")
	})

	t.Run("multi-turn session refuses session/new fallback when session/load fails and retries cleanly", func(t *testing.T) {
		mgr, mock, exec, _ := newCopilotLiveReloadFixture(t, v1.AgentStatusReady, "default", "")
		mock.sessionLoadError = "session not found"
		exec.promptGeneration = 1

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		res, err := mgr.SetSessionConfigOptionBySessionIDWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "long_context",
		)
		require.NoError(t, err)
		require.True(t, res.Confirmed)
		require.Empty(t, mock.getHTTPActions())

		_, err = mgr.PromptAgent(ctx, exec.ID, "blocked turn when load fails", nil, false)
		require.Error(t, err)
		wsActions := mock.getWSActions()
		require.Contains(t, wsActions, "agent.session.load")
		require.NotContains(t, wsActions, "agent.session.new")
		require.NotContains(t, wsActions, "agent.prompt")

		mock.mu.Lock()
		mock.sessionLoadError = ""
		mock.emitPromptComplete = true
		mock.mu.Unlock()
		_, err = mgr.PromptAgent(ctx, exec.ID, "recovered turn", nil, false)
		require.NoError(t, err)
		require.Contains(t, mock.getWSActions(), "agent.prompt")
	})

	t.Run("active turn and invalid values are rejected without stopping process", func(t *testing.T) {
		mgr, mock, exec, _ := newCopilotLiveReloadFixture(t, v1.AgentStatusRunning, "default", "")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := mgr.SetSessionConfigOptionBySessionIDWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "long_context",
		)
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "set config option failed:"), "err=%v", err)
		require.Empty(t, mock.getHTTPActions())
		require.Equal(t, v1.AgentStatusRunning, exec.Status)
		require.Equal(t, "default", effectiveExecutionCopilotContextTier(exec))

		exec.Status = v1.AgentStatusReady
		exec.dispatchedPromptPending.Store(true)
		_, err = mgr.SetSessionConfigOptionBySessionIDWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "long_context",
		)
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "set config option failed:"), "err=%v", err)
		require.Empty(t, mock.getHTTPActions())
		exec.dispatchedPromptPending.Store(false)

		_, err = mgr.SetSessionConfigOptionBySessionIDWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "2m_invalid",
		)
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "set config option failed:"), "err=%v", err)
		require.Empty(t, mock.getHTTPActions())
	})

	t.Run("preflight failure before stop leaves live subprocess untouched", func(t *testing.T) {
		mgr, mock, exec, _ := newCopilotLiveReloadFixture(t, v1.AgentStatusReady, "default", "\"unterminated")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := mgr.SetSessionConfigOptionBySessionIDWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "long_context",
		)
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "set config option failed:"), "err=%v", err)
		require.Empty(t, mock.getHTTPActions())
		require.Equal(t, "copilot-acp-session-1", exec.ACPSessionID)
		require.Equal(t, v1.AgentStatusReady, exec.Status)
		require.True(t, mgr.WasSessionInitialized(exec.ID))
	})

	t.Run("durable admission prompt reconciles staged context_tier before invoking admission callback", func(t *testing.T) {
		mgr, mock, exec, _ := newCopilotLiveReloadFixture(t, v1.AgentStatusReady, "default", "")
		mock.sessionLoadError = "session not found"
		exec.promptGeneration = 1
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		_, err := mgr.SetSessionConfigOptionBySessionIDWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "long_context",
		)
		require.NoError(t, err)

		admitted := false
		_, err = mgr.PromptAgentWithAdmissionCallbackAndSubmissionID(
			ctx, exec.ID, "durable turn blocked on reload failure", nil, false,
			func(context.Context, string, []v1.MessageAttachment) error {
				admitted = true
				return nil
			},
			nil,
			"sub-1",
		)
		require.Error(t, err)
		require.False(t, admitted, "admission callback must not run when pre-prompt reload fails")

		mock.mu.Lock()
		mock.sessionLoadError = ""
		mock.emitPromptComplete = true
		mock.mu.Unlock()

		var configuredArgsAtAdmission []string
		_, err = mgr.PromptAgentWithAdmissionCallbackAndSubmissionID(
			ctx, exec.ID, "durable turn after recovery", nil, false,
			func(context.Context, string, []v1.MessageAttachment) error {
				admitted = true
				_, configuredArgsAtAdmission, _ = mock.getConfiguredProcess()
				return nil
			},
			nil,
			"sub-2",
		)
		require.NoError(t, err)
		require.True(t, admitted)
		assertContainsFlagPair(t, configuredArgsAtAdmission, "--context", "long_context")
	})

	t.Run("ResetAgentContext with staged context_tier restarts subprocess with new --context flag", func(t *testing.T) {
		mgr, mock, exec, provider := newCopilotLiveReloadFixture(t, v1.AgentStatusReady, "default", "")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		_, err := mgr.SetSessionConfigOptionBySessionIDWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "long_context",
		)
		require.NoError(t, err)
		provider.infos[exec.SessionID].RuntimeConfigOptions[acpcompat.CopilotContextTierOptionID] = "long_context"
		require.Empty(t, mock.getHTTPActions())

		require.NoError(t, mgr.ResetAgentContext(ctx, exec.ID))
		require.Equal(t, []string{"stop", "configure", "start"}, mock.getHTTPActions())
		_, configuredArgs, _ := mock.getConfiguredProcess()
		assertContainsFlagPair(t, configuredArgs, "--context", "long_context")
		require.Equal(t, "new-session-123", exec.ACPSessionID)
		require.Contains(t, mock.getWSActions(), "agent.session.new")
		require.NotContains(t, mock.getWSActions(), "agent.session.reset")
	})

	t.Run("in-place session MCP reconfiguration preserves staged context_tier until next prompt", func(t *testing.T) {
		mgr, mock, exec, provider := newCopilotLiveReloadFixture(t, v1.AgentStatusReady, "default", "")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		_, err := exec.agentctl.Initialize(ctx, "test", "1.0.0")
		require.NoError(t, err)
		mgr.mcpResolver = &sessionMCPResolverFake{}
		stateRepo := newSessionMCPStateFake()
		stateRepo.state[exec.SessionID] = mcpconfig.SessionMCPSelectionState{
			DesiredRevision: 1,
			AppliedRevision: 0,
			ApplyState:      mcpconfig.SessionMCPApplyStatePendingIdle,
		}
		mgr.SetMCPSelectionStateRepository(stateRepo)

		_, err = mgr.SetSessionConfigOptionBySessionIDWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "long_context",
		)
		require.NoError(t, err)
		provider.infos[exec.SessionID].RuntimeConfigOptions[acpcompat.CopilotContextTierOptionID] = "long_context"

		inPlaceOpts, _ := acpcompat.NormalizeCopilotConfigOptions([]streams.ConfigOption{
			{
				ID:           "reasoning_effort",
				Name:         "Reasoning Effort",
				Category:     "thought_level",
				Type:         "select",
				CurrentValue: "medium",
			},
		}, "default")
		mock.mu.Lock()
		mock.modelState = &streams.SessionModelState{
			CurrentModelID:       "gpt-5.4",
			Models:               []streams.SessionModelInfo{{ModelID: "gpt-5.4"}, {ModelID: "claude-sonnet-4.6"}},
			ConfigOptions:        inPlaceOpts,
			ConfigOptionsSettled: true,
		}
		mock.mu.Unlock()

		require.NoError(t, mgr.applyPendingSessionMCP(ctx, exec.SessionID))
		mcpState, err := stateRepo.GetMCPSelectionState(ctx, exec.SessionID)
		require.NoError(t, err)
		require.Equal(t, mcpconfig.SessionMCPApplyStateApplied, mcpState.ApplyState)
		require.Equal(t, "long_context", currentContextTierFromModelState(exec.GetModelState()))
		require.Empty(t, mock.getHTTPActions(), "MCP in-place reload must not restart subprocess yet")

		reloadedOpts, _ := acpcompat.NormalizeCopilotConfigOptions([]streams.ConfigOption{
			{
				ID:           "reasoning_effort",
				Name:         "Reasoning Effort",
				Category:     "thought_level",
				Type:         "select",
				CurrentValue: "high",
			},
		}, "long_context")
		mock.mu.Lock()
		mock.modelState = &streams.SessionModelState{
			CurrentModelID:       "claude-sonnet-4.6",
			Models:               []streams.SessionModelInfo{{ModelID: "gpt-5.4"}, {ModelID: "claude-sonnet-4.6"}},
			ConfigOptions:        reloadedOpts,
			ConfigOptionsSettled: true,
		}
		mock.emitPromptComplete = true
		mock.mu.Unlock()

		_, err = mgr.PromptAgent(ctx, exec.ID, "turn after mcp update", nil, false)
		require.NoError(t, err)
		require.Equal(t, []string{"stop", "configure", "start"}, mock.getHTTPActions())
		_, configuredArgs, _ := mock.getConfiguredProcess()
		assertContainsFlagPair(t, configuredArgs, "--context", "long_context")
	})
}

func currentContextTierFromModelState(state *CachedModelState) string {
	if state == nil {
		return ""
	}
	return currentContextTierFromOptions(state.ConfigOptions)
}

func currentContextTierFromOptions(options []streams.ConfigOption) string {
	for _, opt := range options {
		if opt.ID == acpcompat.CopilotContextTierOptionID {
			return opt.CurrentValue
		}
	}
	return ""
}

func publishedEventTypes(bus *MockEventBus) []string {
	bus.mu.Lock()
	defer bus.mu.Unlock()
	eventTypes := make([]string, 0, len(bus.PublishedEvents))
	for _, ev := range bus.PublishedEvents {
		eventTypes = append(eventTypes, ev.Type)
	}
	return eventTypes
}
