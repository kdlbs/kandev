package lifecycle

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func assertContainsFlagPair(t *testing.T, args []string, flag, wantValue string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			require.Equal(t, wantValue, args[i+1], "args=%v", args)
			return
		}
	}
	t.Fatalf("expected %s %s in args %v", flag, wantValue, args)
}

// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-002.1
// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-002.2
func TestCopilotLaunchAndRestartCommandsIncludeContextTier(t *testing.T) {
	ctx := context.Background()

	t.Run("launch uses profile context_tier when session has no runtime override", func(t *testing.T) {
		mgr := newTestManager(t)
		profile := &AgentProfileInfo{
			ProfileID: "copilot-profile",
			AgentName: agents.CopilotACPAgentID,
			Model:     "gpt-5.4",
			ConfigOptions: map[string]string{
				"context_tier":     "long_context",
				"reasoning_effort": "high",
			},
		}
		req := &LaunchRequest{
			TaskID:    "task-1",
			SessionID: "session-profile-only",
		}
		cmds, err := mgr.buildAgentCommandWithContext(
			ctx, req, profile, agents.NewCopilotACP(), agentruntime.RuntimeStandalone, true,
		)
		require.NoError(t, err)
		assertContainsFlagPair(t, cmds.args, "--context", "long_context")
		require.Empty(t, cmds.continueArgs)
	})

	t.Run("launch overlays session runtime context_tier over profile and keeps profile context_tier when session overrides another option", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-override-tier": {
					SessionID:               "session-override-tier",
					RuntimeConfigOptionsSet: true,
					RuntimeConfigOptions: map[string]string{
						"context_tier": "long_context",
					},
				},
				"session-override-effort": {
					SessionID:               "session-override-effort",
					RuntimeConfigOptionsSet: true,
					RuntimeConfigOptions: map[string]string{
						"reasoning_effort": "low",
					},
				},
			},
		}
		profile := &AgentProfileInfo{
			ProfileID: "copilot-profile",
			AgentName: agents.CopilotACPAgentID,
			Model:     "gpt-5.4",
			ConfigOptions: map[string]string{
				"context_tier":     "default",
				"reasoning_effort": "high",
			},
		}

		overrideCmds, err := mgr.buildAgentCommandWithContext(
			ctx,
			&LaunchRequest{TaskID: "task-1", SessionID: "session-override-tier"},
			profile,
			agents.NewCopilotACP(),
			agentruntime.RuntimeStandalone,
			true,
		)
		require.NoError(t, err)
		assertContainsFlagPair(t, overrideCmds.args, "--context", "long_context")

		effortOnlyProfile := &AgentProfileInfo{
			ProfileID: "copilot-profile-long",
			AgentName: agents.CopilotACPAgentID,
			Model:     "gpt-5.4",
			ConfigOptions: map[string]string{
				"context_tier":     "long_context",
				"reasoning_effort": "high",
			},
		}
		preservedCmds, err := mgr.buildAgentCommandWithContext(
			ctx,
			&LaunchRequest{TaskID: "task-1", SessionID: "session-override-effort"},
			effortOnlyProfile,
			agents.NewCopilotACP(),
			agentruntime.RuntimeStandalone,
			true,
		)
		require.NoError(t, err)
		assertContainsFlagPair(t, preservedCmds.args, "--context", "long_context")
	})

	t.Run("passthrough launch and resume include merged context_tier", func(t *testing.T) {
		mgr := newTestManager(t)
		profile := &AgentProfileInfo{
			ProfileID: "copilot-profile",
			AgentName: agents.CopilotACPAgentID,
			Model:     "gpt-5.4",
		}
		mgr.profileResolver = &restartProfileResolver{profile: profile}
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-pt": {
					SessionID:               "session-pt",
					RuntimeConfigOptionsSet: true,
					RuntimeConfigOptions: map[string]string{
						"context_tier": "long_context",
					},
				},
			},
		}
		exec := &AgentExecution{
			ID:             "exec-pt",
			TaskID:         "task-pt",
			SessionID:      "session-pt",
			AgentID:        agents.CopilotACPAgentID,
			AgentProfileID: "copilot-profile",
			RuntimeName:    agentruntime.RuntimeStandalone,
			IsPassthrough:  true,
		}

		_, _, _, cmd, err := mgr.passthroughAgentCommand(ctx, exec, profile)
		require.NoError(t, err)
		assertContainsFlagPair(t, cmd.Args(), "--context", "long_context")
	})

	t.Run("prepareAgentRestart applies runtimeConfigOverride context_tier before persisted write", func(t *testing.T) {
		mgr := newTestManager(t)
		profile := &AgentProfileInfo{
			ProfileID: "copilot-profile",
			AgentName: agents.CopilotACPAgentID,
			Model:     "gpt-5.4",
			ConfigOptions: map[string]string{
				"context_tier": "default",
			},
		}
		mgr.profileResolver = &restartProfileResolver{profile: profile}
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-restart": {
					SessionID:               "session-restart",
					RuntimeConfigOptionsSet: true,
					RuntimeConfigOptions: map[string]string{
						"context_tier": "default",
					},
				},
			},
		}
		exec := &AgentExecution{
			ID:             "exec-restart",
			TaskID:         "task-restart",
			SessionID:      "session-restart",
			AgentID:        agents.CopilotACPAgentID,
			AgentProfileID: "copilot-profile",
			RuntimeName:    agentruntime.RuntimeStandalone,
		}
		override := &models.SessionRuntimeConfig{
			Model: "gpt-5.4",
			ConfigOptions: map[string]string{
				"context_tier": "long_context",
			},
		}

		prep, err := mgr.prepareAgentRestart(ctx, exec, override)
		require.NoError(t, err)
		assertContainsFlagPair(t, prep.commands.args, "--context", "long_context")
		require.False(t, slices.Contains(prep.commands.args, "default"), "args=%v", prep.commands.args)
	})
}

func synthesizedCopilotModelState(tier, effort string) *CachedModelState {
	opts, _ := acpcompat.NormalizeCopilotConfigOptions([]streams.ConfigOption{
		{
			ID:           "reasoning_effort",
			Name:         "Reasoning Effort",
			Category:     "thought_level",
			Type:         "select",
			CurrentValue: effort,
		},
	}, tier)
	return &CachedModelState{
		CurrentModelID:       "gpt-5.4",
		Models:               []streams.SessionModelInfo{{ModelID: "gpt-5.4"}},
		ConfigOptions:        opts,
		ConfigOptionsSettled: true,
	}
}

// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-001.3
// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-002.4
func TestCopilotStartupRestoreAndRebindSkipSynthesizedContextTierSetConfigOption(t *testing.T) {
	t.Run("startup profile and runtime layers skip synthesized context_tier RPC while forwarding reasoning_effort", func(t *testing.T) {
		mgr := newTestManager(t)
		mock := newRestartMockAgentctlServer(t, false, false)
		mock.failConfigOptionID = acpcompat.CopilotContextTierOptionID
		client := createTestClient(t, mock.server.URL)
		t.Cleanup(client.Close)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		require.NoError(t, client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, nil, nil))

		exec := &AgentExecution{
			ID:                 "exec-copilot-startup",
			TaskID:             "task-1",
			SessionID:          "session-1",
			AgentID:            agents.CopilotACPAgentID,
			AgentProfileID:     "copilot-profile",
			ACPSessionID:       "acp-1",
			AgentArgs:          []string{"copilot", "--acp", "--stdio", "--context", "long_context"},
			Status:             v1.AgentStatusReady,
			sessionInitialized: true,
			agentctl:           client,
		}
		exec.SetModelState(synthesizedCopilotModelState("long_context", "medium"))

		_, _, applied := mgr.sessionManager.applyProfileSessionLayers(
			ctx,
			exec,
			"acp-1",
			"gpt-5.4",
			map[string]string{
				"context_tier":     "long_context",
				"reasoning_effort": "high",
			},
			true,
			"gpt-5.4",
		)
		require.Equal(t, map[string]string{
			"context_tier":     "long_context",
			"reasoning_effort": "high",
		}, applied)

		_, failed := mgr.sessionManager.applyRuntimeSessionLayers(
			ctx,
			exec,
			"acp-1",
			"",
			"gpt-5.4",
			"",
			"gpt-5.4",
			"",
			map[string]string{
				"context_tier":     "long_context",
				"reasoning_effort": "low",
			},
		)
		require.Empty(t, failed)

		// Mismatched runtime context_tier is reported in failed without sending an unsupported RPC.
		_, mismatchFailed := mgr.sessionManager.applyRuntimeSessionLayers(
			ctx,
			exec,
			"acp-1",
			"",
			"gpt-5.4",
			"",
			"gpt-5.4",
			"",
			map[string]string{
				"context_tier": "default",
			},
		)
		require.Equal(t, []string{"context_tier"}, mismatchFailed)
		require.Equal(t, []restartConfigOption{
			{ID: "reasoning_effort", Value: "high"},
			{ID: "reasoning_effort", Value: "low"},
		}, mock.getSetOptions())
	})

	t.Run("restore and workspace rebind skip synthesized context_tier RPC", func(t *testing.T) {
		mgr := newTestManager(t)
		mock := newRestartMockAgentctlServer(t, false, false)
		mock.failConfigOptionID = acpcompat.CopilotContextTierOptionID
		client := createTestClient(t, mock.server.URL)
		t.Cleanup(client.Close)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		require.NoError(t, client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, nil, nil))

		exec := &AgentExecution{
			ID:                 "exec-copilot-restore",
			TaskID:             "task-1",
			SessionID:          "session-1",
			AgentID:            agents.CopilotACPAgentID,
			AgentProfileID:     "copilot-profile",
			ACPSessionID:       "acp-1",
			AgentArgs:          []string{"copilot", "--acp", "--stdio", "--context", "long_context"},
			Status:             v1.AgentStatusReady,
			sessionInitialized: true,
			agentctl:           client,
		}
		exec.SetModelState(synthesizedCopilotModelState("long_context", "high"))

		require.NoError(t, mgr.restoreSessionRuntimeConfigWithClient(
			ctx,
			exec,
			client,
			"acp-1",
			models.SessionRuntimeConfig{
				Model: "gpt-5.4",
				ConfigOptions: map[string]string{
					"context_tier":     "long_context",
					"reasoning_effort": "high",
				},
			},
		))
		require.NoError(t, mgr.reapplyReboundSessionConfig(
			ctx,
			exec,
			"acp-1",
			exec.GetModelState(),
			"",
			nil,
		))
		require.Equal(t, []restartConfigOption{
			{ID: "reasoning_effort", Value: "high"},
			{ID: "reasoning_effort", Value: "high"},
		}, mock.getSetOptions())
	})

	t.Run("native wire context_tier option is forwarded via SetConfigOption", func(t *testing.T) {
		mgr := newTestManager(t)
		mock := newRestartMockAgentctlServer(t, false, false)
		client := createTestClient(t, mock.server.URL)
		t.Cleanup(client.Close)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		require.NoError(t, client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, nil, nil))

		exec := &AgentExecution{
			ID:                 "exec-copilot-native-wire",
			TaskID:             "task-1",
			SessionID:          "session-1",
			AgentID:            agents.CopilotACPAgentID,
			AgentProfileID:     "copilot-profile",
			ACPSessionID:       "acp-1",
			AgentArgs:          []string{"copilot", "--acp", "--stdio"},
			Status:             v1.AgentStatusReady,
			sessionInitialized: true,
			agentctl:           client,
		}
		exec.SetModelState(&CachedModelState{
			CurrentModelID: "gpt-5.4",
			Models:         []streams.SessionModelInfo{{ModelID: "gpt-5.4"}},
			ConfigOptions: []streams.ConfigOption{
				{
					ID:           "context_tier",
					Name:         "Context Tier",
					Description:  "Provider-native runtime context tier",
					Category:     "context",
					Type:         "select",
					CurrentValue: "long_context",
				},
			},
			ConfigOptionsSettled: true,
		})

		require.NoError(t, mgr.restoreSessionRuntimeConfigWithClient(
			ctx,
			exec,
			client,
			"acp-1",
			models.SessionRuntimeConfig{
				Model: "gpt-5.4",
				ConfigOptions: map[string]string{
					"context_tier": "long_context",
				},
			},
		))
		require.NoError(t, mgr.reapplyReboundSessionConfig(
			ctx,
			exec,
			"acp-1",
			exec.GetModelState(),
			"",
			nil,
		))
		require.Equal(t, []restartConfigOption{
			{ID: "context_tier", Value: "long_context"},
			{ID: "context_tier", Value: "long_context"},
		}, mock.getSetOptions())
	})
}

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

		// Lazy reload: changing context_tier while idle does NOT stop or restart the subprocess.
		require.Empty(t, mock.getHTTPActions())
		require.Equal(t, "long_context", currentContextTierFromModelState(exec.GetModelState()))

		// Live option updates before the next prompt hit the still-running process directly,
		// and any session_models event emitted by that old --context default process keeps
		// the staged long_context value instead of flickering back to default.
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

		// Re-applying the already-staged context_tier remains an instant no-op.
		noopRes, err := mgr.SetSessionConfigOptionBySessionIDWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "long_context",
		)
		require.NoError(t, err)
		require.True(t, noopRes.Confirmed)
		require.Equal(t, "long_context", noopRes.Effective)
		require.Empty(t, mock.getHTTPActions())

		// Sending the next prompt reconciles the subprocess once via session/load before dispatch,
		// without emitting AgentBootReady (which would reset orchestrator RUNNING state) or AgentContextReset.
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

		// Subsequent prompt with no context_tier change performs zero additional restarts.
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

		// Pending dispatched prompt while status appears Ready is also rejected.
		exec.Status = v1.AgentStatusReady
		exec.dispatchedPromptPending.Store(true)
		_, err = mgr.SetSessionConfigOptionBySessionIDWithResult(
			ctx, exec.SessionID, acpcompat.CopilotContextTierOptionID, "long_context",
		)
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "set config option failed:"), "err=%v", err)
		require.Empty(t, mock.getHTTPActions())
		exec.dispatchedPromptPending.Store(false)

		// Invalid value is rejected without stopping process.
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
