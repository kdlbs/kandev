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

func TestCursorRuntimeConfigReconcilesVerifiedCatalog(t *testing.T) {
	t.Run("live switch", func(t *testing.T) {
		repo, svc, eventBus := newCursorRuntimeConfigFixture(t, "opus")
		svc.handleSessionModelsEvent(t.Context(), cursorModelReport(t, 1, false, false))
		require.Equal(t, "false", cursorRuntimeConfig(t, repo).ConfigOptions["fast"])
		require.Equal(t, "medium", cursorRuntimeConfig(t, repo).ConfigOptions["reasoning_effort"])
		svc.handleSessionModelsEvent(t.Context(), cursorModelReport(t, 2, true, false))
		want := map[string]string{
			"model": "sonnet", "context": "300k", "reasoning_effort": "medium", "supported_toggle": "false",
		}
		require.Equal(t, want, cursorRuntimeConfig(t, repo).ConfigOptions)

		// A synthetic convergence report can echo old option IDs under the new model.
		synthetic := cursorModelReport(t, 3, false, false)
		synthetic.Data.Data = nil
		synthetic.Data.ConfigOptions = append(synthetic.Data.ConfigOptions,
			streams.ConfigOption{ID: "fast", CurrentValue: "false"})
		svc.handleSessionModelsEvent(t.Context(), synthetic)
		svc.handleSessionModelsEvent(t.Context(), cursorModelReport(t, 2, true, false))
		require.Equal(t, want, cursorRuntimeConfig(t, repo).ConfigOptions)

		session, err := repo.GetTaskSession(t.Context(), "s1")
		require.NoError(t, err)
		overrides, ok := models.LoadSessionRuntimeConfigOverrides(session.Metadata)
		require.True(t, ok)
		require.Equal(t, map[string]string{"fast": "false"}, overrides.ConfigOptions)
		projected := eventBus.events[1].event.Data.(lifecycle.SessionModelsEventPayload)
		require.Equal(t, "sonnet", projected.CurrentModelID)
		require.Equal(t, cursorModelReport(t, 2, true, false).Data.ConfigOptions, projected.ConfigOptions)
	})

	t.Run("legacy row when startup settles", func(t *testing.T) {
		repo, svc, _ := newCursorRuntimeConfigFixture(t, "sonnet")
		ctx := t.Context()
		require.NoError(t, repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateStarting, ""))
		require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyOrigin, models.SessionOriginTaskInitial))
		svc.handleSessionModelsEvent(ctx, cursorModelReport(t, 1, true, false))
		require.Contains(t, cursorRuntimeConfig(t, repo).ConfigOptions, "fast")
		svc.handleSessionModelsEvent(ctx, cursorModelReport(t, 1, true, true))
		require.NotContains(t, cursorRuntimeConfig(t, repo).ConfigOptions, "fast")
	})

	t.Run("new report can retry a runtime write", func(t *testing.T) {
		repo, svc, _ := newCursorRuntimeConfigFixture(t, "sonnet")
		failing := &cursorRuntimeFailingRepo{Repository: repo}
		svc.repo = failing
		report := cursorModelReport(t, 1, true, false)
		svc.handleSessionModelsEvent(t.Context(), report)
		require.Contains(t, cursorRuntimeConfig(t, repo).ConfigOptions, "fast")
		svc.handleSessionModelsEvent(t.Context(), report)
		require.NotContains(t, cursorRuntimeConfig(t, repo).ConfigOptions, "fast")
	})
}

func newCursorRuntimeConfigFixture(t *testing.T, model string) (*sqliterepo.Repository, *Service, *recordingEventBus) {
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

func cursorModelReport(t *testing.T, generation uint64, complete, settled bool) *lifecycle.AgentStreamEventPayload {
	t.Helper()
	eventBus := &recordingEventBus{}
	publisher := lifecycle.NewEventPublisher(eventBus, testLogger())
	execution := &lifecycle.AgentExecution{ID: "cursor-exec", TaskID: "t1", SessionID: "s1", AgentID: acpcompat.CursorAgentID}
	publisher.PublishAgentStreamEvent(execution, streams.AgentEvent{
		Type: streams.EventTypeSessionModels, SessionID: "cursor-native",
		CurrentModelID: "sonnet", SessionSettingsGeneration: generation, SessionSettingsSourceGeneration: 1,
		SessionModels: []streams.SessionModelInfo{{ModelID: "opus"}, {ModelID: "sonnet"}},
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

func cursorRuntimeConfig(t *testing.T, repo *sqliterepo.Repository) models.SessionRuntimeConfig {
	t.Helper()
	session, err := repo.GetTaskSession(t.Context(), "s1")
	require.NoError(t, err)
	cfg, ok := models.LoadSessionRuntimeConfig(session.Metadata)
	require.True(t, ok)
	return cfg
}

type cursorRuntimeFailingRepo struct {
	*sqliterepo.Repository
	failed bool
}

func (r *cursorRuntimeFailingRepo) SetSessionMetadataKeyIfJSONValue(ctx context.Context, sessionID, key string, expected, value any) (bool, error) {
	if !r.failed && key == models.SessionMetaKeyRuntimeConfig {
		r.failed = true
		return false, errors.New("runtime config write failed")
	}
	return r.Repository.SetSessionMetadataKeyIfJSONValue(ctx, sessionID, key, expected, value)
}
