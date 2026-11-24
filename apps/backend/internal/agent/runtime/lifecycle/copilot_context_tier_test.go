package lifecycle

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/agentruntime"
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

		resolved, err := mgr.resolvePassthroughAgent(ctx, exec)
		require.NoError(t, err)
		resumeCmd, err := mgr.resumePassthroughCommandWithPreparation(ctx, exec, resolved, true, nil)
		require.NoError(t, err)
		assertContainsFlagPair(t, resumeCmd.Args(), "--context", "long_context")
		require.Contains(t, resumeCmd.Args(), "--continue")
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

func TestConfirmedCopilotLaunchTierIsSeparateFromStagedPreference(t *testing.T) {
	execution := &AgentExecution{
		AgentID:   agents.CopilotACPAgentID,
		AgentArgs: []string{"copilot", "--acp", "--context", acpcompat.CopilotContextTierDefault},
	}
	execution.setConfirmedCopilotContextTier(acpcompat.CopilotContextTierLong)
	execution.SetModelState(synthesizedCopilotModelState(acpcompat.CopilotContextTierDefault, "medium"))

	require.Equal(t, acpcompat.CopilotContextTierLong, launchedExecutionCopilotContextTier(execution))
	require.Equal(t, acpcompat.CopilotContextTierDefault, effectiveExecutionCopilotContextTier(execution))

	incoming, _ := acpcompat.NormalizeCopilotConfigOptions(nil, acpcompat.CopilotContextTierLong)
	preserved := preserveStagedCopilotContextTier(execution, incoming)
	require.Equal(t, acpcompat.CopilotContextTierDefault, currentContextTierFromOptions(preserved))
}

func TestCopilotReloadModelStateKeepsEffectiveFallback(t *testing.T) {
	base := &CachedModelState{
		CurrentModelID:       "provider-default",
		Models:               []streams.SessionModelInfo{{ModelID: "provider-default"}},
		ConfigOptions:        []streams.ConfigOption{{ID: modelConfigOptionID, CurrentValue: "requested-model"}},
		ConfigOptionsSettled: true,
	}
	runtimeConfig := models.SessionRuntimeConfig{Model: "requested-model"}
	cloned := cloneModelStateForCopilotReload(base, runtimeConfig, acpcompat.CopilotContextTierLong)
	require.Equal(t, "provider-default", cloned.CurrentModelID)

	manager := newTestManager(t)
	execution := &AgentExecution{AgentID: agents.CopilotACPAgentID, ACPSessionID: "native"}
	execution.SetModelState(cloned)
	manager.synchronizeReloadedCopilotModelState(
		execution, base, runtimeConfig, acpcompat.CopilotContextTierLong, "provider-default",
	)
	state := execution.GetModelState()
	require.Equal(t, "provider-default", state.CurrentModelID)
	require.Equal(t, "provider-default", configOptionValue(state.ConfigOptions, modelConfigOptionID))
}

func configOptionValue(options []streams.ConfigOption, id string) string {
	for _, option := range options {
		if option.ID == id {
			return option.CurrentValue
		}
	}
	return ""
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

		// Under ProviderRestored (adopted surviving agentctl), a staged context_tier restages cleanly.
		exec.SessionSettingsPolicy = SessionSettingsPolicyProviderRestored
		_, restoredFailed := mgr.sessionManager.applyRuntimeSessionLayers(
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
		require.Empty(t, restoredFailed)
		require.Equal(t, "default", currentContextTierFromModelState(exec.GetModelState()))

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
