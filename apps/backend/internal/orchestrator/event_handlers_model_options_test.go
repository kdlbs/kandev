package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestRuntimeConfigReconcilesVerifiedCatalog(t *testing.T) {
	for _, agentID := range []string{acpcompat.CursorAgentID, "generic-acp"} {
		t.Run(agentID, func(t *testing.T) {
			testRuntimeConfigReconcilesVerifiedCatalog(t, agentID)
		})
	}
}

func testRuntimeConfigReconcilesVerifiedCatalog(t *testing.T, agentID string) {
	t.Helper()
	t.Run("live switch", func(t *testing.T) {
		repo, svc, eventBus := newRuntimeOptionsFixture(t, "opus")
		svc.handleSessionModelsEvent(t.Context(), modelOptionReport(t, agentID, 1, false, false))
		require.Equal(t, "false", readRuntimeOptions(t, repo).ConfigOptions["fast"])
		require.Equal(t, "medium", readRuntimeOptions(t, repo).ConfigOptions["reasoning_effort"])
		svc.handleSessionModelsEvent(t.Context(), modelOptionReport(t, agentID, 2, true, false))
		want := map[string]string{
			"model": "sonnet", "context": "300k", "reasoning_effort": "medium", "supported_toggle": "false",
		}
		require.Equal(t, want, readRuntimeOptions(t, repo).ConfigOptions)

		// A synthetic convergence report can echo old option IDs under the new model.
		synthetic := modelOptionReport(t, agentID, 3, false, false)
		synthetic.Data.Data = nil
		synthetic.Data.ConfigOptions = append(synthetic.Data.ConfigOptions,
			streams.ConfigOption{ID: "fast", CurrentValue: "false"})
		svc.handleSessionModelsEvent(t.Context(), synthetic)
		svc.handleSessionModelsEvent(t.Context(), modelOptionReport(t, agentID, 2, true, false))
		require.Equal(t, want, readRuntimeOptions(t, repo).ConfigOptions)

		session, err := repo.GetTaskSession(t.Context(), "s1")
		require.NoError(t, err)
		overrides, ok := models.LoadSessionRuntimeConfigOverrides(session.Metadata)
		require.True(t, ok)
		require.Equal(t, map[string]string{"fast": "false"}, overrides.ConfigOptions)
		projected := eventBus.events[1].event.Data.(lifecycle.SessionModelsEventPayload)
		require.Equal(t, "sonnet", projected.CurrentModelID)
		require.Equal(t, modelOptionReport(t, agentID, 2, true, false).Data.ConfigOptions, projected.ConfigOptions)
	})

	t.Run("legacy row when startup settles", func(t *testing.T) {
		repo, svc, _ := newRuntimeOptionsFixture(t, "sonnet")
		ctx := t.Context()
		require.NoError(t, repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateStarting, ""))
		require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyOrigin, models.SessionOriginTaskInitial))
		svc.handleSessionModelsEvent(ctx, modelOptionReport(t, agentID, 1, true, false))
		require.Contains(t, readRuntimeOptions(t, repo).ConfigOptions, "fast")
		settled := modelOptionReport(t, agentID, 1, true, true)
		settled.Data.ConfigOptions = nil
		svc.handleSessionModelsEvent(ctx, settled)
		require.Empty(t, readRuntimeOptions(t, repo).ConfigOptions)
	})

	t.Run("redelivery can retry a runtime write", func(t *testing.T) {
		testRuntimeOptionWriteRetry(t, agentID)
	})
}

func testRuntimeOptionWriteRetry(t *testing.T, agentID string) {
	t.Helper()
	for _, tc := range []struct {
		name     string
		complete bool
	}{
		{name: "complete", complete: true},
		{name: "verified subset"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, svc, _ := newRuntimeOptionsFixture(t, "sonnet")
			svc.repo = &runtimeOptionsFailingRepo{Repository: repo}
			report := modelOptionReport(t, agentID, 1, tc.complete, false, "reasoning_effort")
			svc.handleSessionModelsEvent(t.Context(), report)
			require.Equal(t, "low", readRuntimeOptions(t, repo).ConfigOptions["reasoning_effort"])

			svc.handleSessionModelsEvent(t.Context(), report)
			options := readRuntimeOptions(t, repo).ConfigOptions
			require.Equal(t, "medium", options["reasoning_effort"])
			_, retained := options["fast"]
			require.Equal(t, !tc.complete, retained)
		})
	}
}

func TestRuntimeConfigUpdatesOnlyVerifiedPartialOptions(t *testing.T) {
	repo, svc, _ := newRuntimeOptionsFixture(t, "opus")
	report := modelOptionReport(t, "generic-acp", 1, false, false, "reasoning_effort")
	report.Data.Data = map[string]any{streams.ConfigOptionsDisplayOnlyKey: true}
	svc.handleSessionModelsEvent(t.Context(), report)
	options := readRuntimeOptions(t, repo).ConfigOptions
	require.Equal(t, "medium", options["reasoning_effort"])
	require.Equal(t, "false", options["fast"])
	require.NotContains(t, options, "supported_toggle")

	copied := modelOptionReport(t, "generic-acp", 2, false, false)
	copied.Data.Data = map[string]any{
		streams.ConfigOptionsDisplayOnlyKey: true,
		streams.ConfigOptionsConfigIDKey:    "supported_toggle",
	}
	copied.Data.ConfigOptions[2].CurrentValue = "high"
	svc.handleSessionModelsEvent(t.Context(), copied)
	options = readRuntimeOptions(t, repo).ConfigOptions
	require.Equal(t, "false", options["supported_toggle"])
	require.Equal(t, "medium", options["reasoning_effort"])
}

func newRuntimeOptionsFixture(t *testing.T, model string) (*sqliterepo.Repository, *Service, *recordingEventBus) {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "s1", models.SessionMetaKeyRuntimeConfig, models.SessionRuntimeConfig{
		Model: model,
		ConfigOptions: map[string]string{
			"model": model, "fast": "false", "context": "300k", "reasoning_effort": "low",
		},
	}))
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "s1", models.SessionMetaKeyRuntimeConfigOverrides, models.SessionRuntimeConfig{
		Model: "sonnet", ConfigOptions: map[string]string{"fast": "false"},
	}))
	bus := &recordingEventBus{}
	return repo, &Service{logger: testLogger(), repo: repo, eventBus: bus}, bus
}

func modelOptionReport(t *testing.T, agentID string, generation uint64, complete, settled bool, verifiedIDs ...string) *lifecycle.AgentStreamEventPayload {
	t.Helper()
	eventBus := &recordingEventBus{}
	publisher := lifecycle.NewEventPublisher(eventBus, testLogger())
	execution := &lifecycle.AgentExecution{ID: "model-settings-exec", TaskID: "t1", SessionID: "s1", AgentID: agentID}
	publisher.PublishAgentStreamEvent(execution, streams.AgentEvent{
		Type: streams.EventTypeSessionModels, SessionID: "model-settings-native",
		CurrentModelID: "sonnet", SessionSettingsGeneration: generation, SessionSettingsSourceGeneration: 1,
		SessionModels:           []streams.SessionModelInfo{{ModelID: "opus"}, {ModelID: "sonnet"}},
		VerifiedConfigOptionIDs: verifiedIDs,
		ConfigOptions: []streams.ConfigOption{
			{ID: "model", Category: "model", CurrentValue: "sonnet"},
			{ID: "context"},
			{ID: "reasoning_effort", CurrentValue: "medium"},
			{ID: "supported_toggle", CurrentValue: "false"},
		},
		Data: map[string]any{
			streams.ConfigOptionsSourceKey:   "provider_response",
			streams.ConfigOptionsCompleteKey: complete,
			"config_options_settled":         settled,
		},
	})
	require.Len(t, eventBus.events, 1)
	payload := eventBus.events[0].event.Data.(lifecycle.AgentStreamEventPayload)
	return &payload
}

func readRuntimeOptions(t *testing.T, repo *sqliterepo.Repository) models.SessionRuntimeConfig {
	t.Helper()
	session, err := repo.GetTaskSession(t.Context(), "s1")
	require.NoError(t, err)
	cfg, ok := models.LoadSessionRuntimeConfig(session.Metadata)
	require.True(t, ok)
	return cfg
}

type runtimeOptionsFailingRepo struct {
	*sqliterepo.Repository
	failed bool
}

func (r *runtimeOptionsFailingRepo) SetSessionMetadataKeyIfJSONValue(ctx context.Context, sessionID, key string, expected, value any) (bool, error) {
	if !r.failed && key == models.SessionMetaKeyRuntimeConfig {
		r.failed = true
		return false, errors.New("runtime config write failed")
	}
	return r.Repository.SetSessionMetadataKeyIfJSONValue(ctx, sessionID, key, expected, value)
}
