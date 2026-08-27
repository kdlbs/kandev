package lifecycle

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func runtimeConfigResetModelState() *streams.SessionModelState {
	return &streams.SessionModelState{
		CurrentModelID: "mock-fast",
		Models: []streams.SessionModelInfo{
			{ModelID: "mock-fast"},
			{ModelID: "mock-smart"},
		},
	}
}

func runtimeConfigResetWorkspaceInfo() *mockWorkspaceInfoProvider {
	return &mockWorkspaceInfoProvider{
		infos: map[string]*WorkspaceInfo{
			"session-1": {
				SessionID:               "session-1",
				RuntimeModel:            "mock-smart",
				SessionMode:             "acceptEdits",
				RuntimeConfigOptions:    map[string]string{"zeta": "max", "alpha": "enabled"},
				RuntimeConfigOptionsSet: true,
			},
		},
	}
}

func runtimeConfigResetExecution(client *agentctl.Client, initialized bool) *AgentExecution {
	return &AgentExecution{
		ID:                 "exec-runtime-config",
		TaskID:             "task-1",
		SessionID:          "session-1",
		AgentProfileID:     "profile-1",
		ACPSessionID:       "old-session",
		AgentCommand:       "auggie --model test",
		Status:             v1.AgentStatusRunning,
		WorkspacePath:      "/workspace",
		sessionInitialized: initialized,
		agentctl:           client,
		promptDoneCh:       make(chan PromptCompletionSignal, 1),
	}
}

func TestManager_ResetAgentContext_ReappliesSessionRuntimeConfig(t *testing.T) {
	mgr := newTestManager(t)
	provider := runtimeConfigResetWorkspaceInfo()
	mgr.workspaceInfoProvider = provider
	mock := newRestartMockAgentctlServer(t, false, false)
	mock.modelState = runtimeConfigResetModelState()
	mock.onReset = func() {
		provider.infos["session-1"].RuntimeModel = "mock-fast"
		provider.infos["session-1"].SessionMode = "default"
		provider.infos["session-1"].RuntimeConfigOptions = map[string]string{
			"zeta":  "low",
			"alpha": "disabled",
		}
	}

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, nil, nil))

	exec := runtimeConfigResetExecution(client, true)
	exec.SetModelState(&CachedModelState{CurrentModelID: "mock-smart"})
	require.NoError(t, mgr.executionStore.Add(exec))

	require.NoError(t, mgr.ResetAgentContext(ctx, exec.ID))

	actions := mock.getWSActions()
	require.Equal(t, []string{
		"agent.session.reset",
		"agent.session.set_model",
		"agent.session.set_mode",
		"agent.session.set_config_option",
		"agent.session.set_config_option",
	}, actions)
	require.Equal(t, []string{"mock-smart"}, mock.getSetModelIDs())
	require.Equal(t, []string{"acceptEdits"}, mock.getSetModeIDs())
	require.Equal(t, []restartConfigOption{
		{ID: "alpha", Value: "enabled"},
		{ID: "zeta", Value: "max"},
	}, mock.getSetOptions())
	require.Equal(t, v1.AgentStatusReady, exec.Status)
}

func TestManager_RestartAgentProcess_ReappliesSessionRuntimeConfig(t *testing.T) {
	mgr := newTestManager(t)
	provider := runtimeConfigResetWorkspaceInfo()
	mgr.workspaceInfoProvider = provider
	mock := newRestartMockAgentctlServer(t, false, false)
	mock.newModelState = runtimeConfigResetModelState()
	mock.onSessionNew = func() {
		provider.infos["session-1"].RuntimeModel = "mock-fast"
		provider.infos["session-1"].SessionMode = "default"
		provider.infos["session-1"].RuntimeConfigOptions = map[string]string{
			"zeta":  "low",
			"alpha": "disabled",
		}
	}

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	exec := runtimeConfigResetExecution(client, false)
	exec.SetModeState(&CachedModeState{CurrentModeID: "acceptEdits"})
	exec.SetModelState(&CachedModelState{CurrentModelID: "mock-smart"})
	require.NoError(t, mgr.executionStore.Add(exec))

	require.NoError(t, mgr.RestartAgentProcess(ctx, exec.ID))

	actions := mock.getWSActions()
	require.Equal(t, []string{
		"agent.initialize",
		"agent.session.new",
		"agent.session.set_model",
		"agent.session.set_mode",
		"agent.session.set_config_option",
		"agent.session.set_config_option",
	}, actions)
	require.Equal(t, []string{"mock-smart"}, mock.getSetModelIDs())
	require.Equal(t, []string{"acceptEdits"}, mock.getSetModeIDs())
	require.Equal(t, []restartConfigOption{
		{ID: "alpha", Value: "enabled"},
		{ID: "zeta", Value: "max"},
	}, mock.getSetOptions())
	require.Equal(t, v1.AgentStatusReady, exec.Status)
}

func TestManager_ResetAgentContext_FailsClosedOnRuntimeConfigRestore(t *testing.T) {
	mgr := newTestManager(t)
	writer := &captureExecutorRunningWriter{}
	mgr.SetExecutorRunningWriter(writer)
	provider := runtimeConfigResetWorkspaceInfo()
	mgr.workspaceInfoProvider = provider
	mock := newRestartMockAgentctlServer(t, false, false)
	mock.modelState = runtimeConfigResetModelState()
	mock.failConfigOptionID = "alpha"

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, nil, nil))

	exec := runtimeConfigResetExecution(client, true)
	require.NoError(t, mgr.executionStore.Add(exec))

	err := mgr.ResetAgentContext(ctx, exec.ID)
	require.Error(t, err)
	require.Equal(t, v1.AgentStatusFailed, exec.Status)
	require.NotEmpty(t, exec.ErrorMessage)
	require.NotNil(t, writer.running)
	require.Equal(t, models.ExecutorRunningStatusFailed, writer.running.Status)
	require.Equal(t, []string{"agent.session.reset", "agent.session.set_model", "agent.session.set_mode", "agent.session.set_config_option"}, mock.getWSActions())

	bus := mgr.eventBus.(*MockEventBus)
	for _, event := range bus.PublishedEvents {
		require.NotEqual(t, events.AgentBootReady, event.Type)
		require.NotEqual(t, events.AgentContextReset, event.Type)
	}
}

func TestManager_ResetAgentContext_FailsClosedOnSessionModelRestore(t *testing.T) {
	mgr := newTestManager(t)
	mgr.workspaceInfoProvider = runtimeConfigResetWorkspaceInfo()
	mock := newRestartMockAgentctlServer(t, false, false)
	mock.modelState = runtimeConfigResetModelState()
	mock.failModel = true

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, nil, nil))

	exec := runtimeConfigResetExecution(client, true)
	require.NoError(t, mgr.executionStore.Add(exec))

	err := mgr.ResetAgentContext(ctx, exec.ID)
	require.Error(t, err)
	require.Equal(t, v1.AgentStatusFailed, exec.Status)
	require.Equal(t, []string{"agent.session.reset", "agent.session.set_model"}, mock.getWSActions())
	require.Empty(t, mock.getSetModeIDs())
	require.Empty(t, mock.getSetOptions())

	bus := mgr.eventBus.(*MockEventBus)
	for _, event := range bus.PublishedEvents {
		require.NotEqual(t, events.AgentBootReady, event.Type)
		require.NotEqual(t, events.AgentContextReset, event.Type)
	}
}

func TestManager_RestartAgentProcess_FailsClosedOnRuntimeConfigRestore(t *testing.T) {
	mgr := newTestManager(t)
	mgr.workspaceInfoProvider = runtimeConfigResetWorkspaceInfo()
	mock := newRestartMockAgentctlServer(t, false, false)
	mock.newModelState = runtimeConfigResetModelState()
	mock.failConfigOptionID = "alpha"

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	exec := runtimeConfigResetExecution(client, false)
	exec.SetModeState(&CachedModeState{CurrentModeID: "acceptEdits"})
	require.NoError(t, mgr.executionStore.Add(exec))

	err := mgr.RestartAgentProcess(ctx, exec.ID)
	require.Error(t, err)
	require.Equal(t, v1.AgentStatusFailed, exec.Status)
	require.NotEmpty(t, exec.ErrorMessage)

	bus := mgr.eventBus.(*MockEventBus)
	for _, event := range bus.PublishedEvents {
		require.NotEqual(t, events.AgentBootReady, event.Type)
		require.NotEqual(t, events.AgentContextReset, event.Type)
	}
}

func TestManager_ResetAgentContext_FailsClosedOnSessionModeRestore(t *testing.T) {
	mgr := newTestManager(t)
	mgr.workspaceInfoProvider = runtimeConfigResetWorkspaceInfo()
	mock := newRestartMockAgentctlServer(t, false, false)
	mock.modelState = runtimeConfigResetModelState()
	mock.failMode = true

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, nil, nil))

	exec := runtimeConfigResetExecution(client, true)
	require.NoError(t, mgr.executionStore.Add(exec))

	err := mgr.ResetAgentContext(ctx, exec.ID)
	require.Error(t, err)
	require.Equal(t, v1.AgentStatusFailed, exec.Status)
	require.Equal(t, []string{
		"agent.session.reset",
		"agent.session.set_model",
		"agent.session.set_mode",
	}, mock.getWSActions())

	bus := mgr.eventBus.(*MockEventBus)
	for _, event := range bus.PublishedEvents {
		require.NotEqual(t, events.AgentBootReady, event.Type)
		require.NotEqual(t, events.AgentContextReset, event.Type)
	}
}

func TestRuntimeConfigResetModelStateHelperUsesStableValues(t *testing.T) {
	state := runtimeConfigResetModelState()
	require.True(t, slices.ContainsFunc(state.Models, func(model streams.SessionModelInfo) bool {
		return model.ModelID == "mock-smart"
	}))
}

func TestSelectedRuntimeConfigOptionsFiltersInvalidEntries(t *testing.T) {
	tests := []struct {
		name  string
		state *CachedModelState
		want  map[string]string
	}{
		{name: "nil state"},
		{name: "empty state", state: &CachedModelState{}},
		{
			name: "model-shaped entries",
			state: &CachedModelState{ConfigOptions: []streams.ConfigOption{
				{ID: "model", Category: "model", CurrentValue: "mock-smart"},
				{ID: "primary", Category: "model", CurrentValue: "mock-smart"},
			}},
		},
		{
			name: "mode-shaped entries",
			state: &CachedModelState{ConfigOptions: []streams.ConfigOption{
				{ID: "mode", Category: "mode", CurrentValue: "acceptEdits"},
				{ID: "permission", Category: "mode", CurrentValue: "acceptEdits"},
			}},
		},
		{
			name: "provider entries",
			state: &CachedModelState{ConfigOptions: []streams.ConfigOption{
				{ID: "effort", CurrentValue: "high"},
				{ID: "effort", CurrentValue: "max"},
				{ID: "blank", CurrentValue: " "},
			}},
			want: map[string]string{"effort": "max"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, selectedRuntimeConfigOptions(test.state))
		})
	}
}

func TestSanitizeRuntimeConfigOptionsFiltersInvalidEntries(t *testing.T) {
	options := sanitizeRuntimeConfigOptions(map[string]string{
		"":       "value",
		"model":  "mock-fast",
		"mode":   "default",
		"effort": " max ",
		"blank":  " ",
	})

	require.Equal(t, map[string]string{"effort": "max"}, options)
}

func TestManager_CaptureSessionRuntimeConfigOverlaysPersistedStateAfterLiveState(t *testing.T) {
	mgr := newTestManager(t)
	mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{
		infos: map[string]*WorkspaceInfo{
			"session-1": {
				SessionID:               "session-1",
				RuntimeModel:            "mock-smart",
				SessionMode:             "acceptEdits",
				RuntimeConfigOptions:    map[string]string{"effort": "low", "removed": "stale"},
				RuntimeConfigOptionsSet: true,
			},
		},
	}
	exec := runtimeConfigResetExecution(nil, true)
	exec.SetModelState(&CachedModelState{
		CurrentModelID: "mock-smart",
		ConfigOptions: []streams.ConfigOption{
			{ID: "effort", CurrentValue: "max"},
		},
	})

	config := mgr.captureSessionRuntimeConfigForReset(context.Background(), exec)

	require.Equal(t, map[string]string{"effort": "low"}, config.ConfigOptions)
}

func TestManager_CaptureSessionRuntimeConfigLayersLiveStateOverProfileDefaults(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &restartProfileResolver{profile: &AgentProfileInfo{
		Model:         "mock-fast",
		Mode:          "default",
		ConfigOptions: map[string]string{"effort": "low"},
	}}
	exec := runtimeConfigResetExecution(nil, true)
	exec.SetModelState(&CachedModelState{
		CurrentModelID: "mock-smart",
		ConfigOptions:  []streams.ConfigOption{{ID: "effort", CurrentValue: "max"}},
	})
	exec.SetModeState(&CachedModeState{CurrentModeID: "acceptEdits"})

	config := mgr.captureSessionRuntimeConfigForReset(context.Background(), exec)

	require.Equal(t, "mock-smart", config.Model)
	require.Equal(t, "acceptEdits", config.Mode)
	require.Equal(t, map[string]string{"effort": "max"}, config.ConfigOptions)
}

func TestManager_ResetAgentContext_FallbackReusesCapturedRuntimeConfig(t *testing.T) {
	mgr := newTestManager(t)
	provider := runtimeConfigResetWorkspaceInfo()
	mgr.workspaceInfoProvider = provider
	mock := newRestartMockAgentctlServer(t, false, false)
	mock.failSessionReset = true
	mock.newModelState = runtimeConfigResetModelState()
	mock.onReset = func() {
		provider.infos["session-1"].RuntimeModel = "mock-fast"
		provider.infos["session-1"].SessionMode = "default"
		provider.infos["session-1"].RuntimeConfigOptions = map[string]string{
			"effort": "low",
		}
	}

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	exec := runtimeConfigResetExecution(client, true)
	exec.SetModelState(&CachedModelState{CurrentModelID: "mock-smart"})
	require.NoError(t, mgr.executionStore.Add(exec))

	require.NoError(t, mgr.ResetAgentContext(ctx, exec.ID))

	require.Equal(t, []string{"mock-smart"}, mock.getSetModelIDs())
	require.Equal(t, []string{"acceptEdits"}, mock.getSetModeIDs())
	require.Equal(t, []restartConfigOption{
		{ID: "alpha", Value: "enabled"},
		{ID: "zeta", Value: "max"},
	}, mock.getSetOptions())
}

func TestManager_CaptureSessionRuntimeConfigFiltersPersistedCategorizedOptions(t *testing.T) {
	mgr := newTestManager(t)
	mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{
		infos: map[string]*WorkspaceInfo{
			"session-1": {
				SessionID: "session-1",
				RuntimeConfigOptions: map[string]string{
					"primary_model":   "mock-smart",
					"permission_mode": "acceptEdits",
					"effort":          "max",
				},
				RuntimeConfigOptionsSet: true,
			},
		},
	}
	exec := runtimeConfigResetExecution(nil, true)
	exec.SetModelState(&CachedModelState{
		CurrentModelID: "mock-smart",
		ConfigOptions: []streams.ConfigOption{
			{ID: "primary_model", Category: "model", CurrentValue: "mock-smart"},
			{ID: "permission_mode", Category: "mode", CurrentValue: "acceptEdits"},
			{ID: "effort", CurrentValue: "max"},
		},
	})

	config := mgr.captureSessionRuntimeConfigForReset(context.Background(), exec)

	require.Equal(t, map[string]string{"effort": "max"}, config.ConfigOptions)
}

func TestSanitizeRuntimeConfigOptionsUsesCapturedOptionCategories(t *testing.T) {
	options := sanitizeRuntimeConfigOptionsWithCatalog(map[string]string{
		"primary_model":   "mock-smart",
		"permission_mode": "acceptEdits",
		"effort":          "max",
	}, &CachedModelState{
		ConfigOptions: []streams.ConfigOption{
			{ID: "primary_model", Category: "model"},
			{ID: "permission_mode", Category: "mode"},
			{ID: "effort", Category: ""},
		},
	})

	require.Equal(t, map[string]string{"effort": "max"}, options)
}
