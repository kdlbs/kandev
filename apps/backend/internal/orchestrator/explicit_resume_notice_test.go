package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type explicitResumeNoticeFixture struct {
	ctx      context.Context
	repo     *sqliterepo.Repository
	session  *models.TaskSession
	registry *resumeAttemptRegistry
	attempt  *resumeAttempt
	service  *Service
	event    watcher.AgentEventData
}

func newExplicitResumeNoticeFixture(t *testing.T, providerRestored bool) explicitResumeNoticeFixture {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "resume-notice-task", "resume-notice-session", "step1")
	session, err := repo.GetTaskSession(ctx, "resume-notice-session")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "resume-notice-execution")

	registry := newResumeAttemptRegistry()
	attempt, owner := registry.begin(ctx, session.TaskID, session.ID)
	require.True(t, owner)
	if providerRestored {
		require.True(t, registry.setSettingsPolicy(attempt, executor.ResumeSettingsPolicyProviderRestored))
		require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyACPModelState, lifecycle.SessionModelsSnapshot{
			CurrentModelID:    "provider-model",
			CurrentModeID:     "provider-mode",
			SettingsAttemptID: attempt.identity(),
			Models:            []streams.SessionModelInfo{{ModelID: "provider-model", Name: "Provider Model"}},
		}))
		lastError := models.LastAgentError{
			Message:          "The saved model could not be applied.",
			OccurredAt:       time.Date(2026, time.September, 30, 10, 0, 0, 0, time.UTC),
			AgentExecutionID: "prior-execution",
			ExecutionID:      "prior-execution",
			AttemptID:        "prior-attempt",
			Phase:            models.LaunchErrorPhaseBootstrap,
			StampValue:       "prior-selection-failure",
			Causes: []models.AgentErrorCause{{
				Operation: models.AgentErrorCauseOperationResume,
				Code:      models.AgentErrorCauseCodeModelUnavailable,
			}},
		}
		require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyLastAgentError, lastError))
		session.Metadata[models.SessionMetaKeyLastAgentError] = lastError
		require.True(t, registry.setRecoveryErrorStamp(attempt, lastError.Stamp()))
	}
	attempt.setExecutionID("resume-notice-execution")

	service := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	service.resumeAttempts = registry
	service.messageCreator = newServiceBackedMessageCreator(repo)
	event := watcher.AgentEventData{
		TaskID:           session.TaskID,
		SessionID:        session.ID,
		AgentExecutionID: "resume-notice-execution",
		AttemptID:        attempt.identity(),
	}
	if providerRestored {
		event.SessionSettingsPolicy = streams.SessionSettingsPolicyProviderRestored
	}
	return explicitResumeNoticeFixture{
		ctx: ctx, repo: repo, session: session, registry: registry,
		attempt: attempt, service: service, event: event,
	}
}

func (f explicitResumeNoticeFixture) messages(t *testing.T) []*models.Message {
	t.Helper()
	messages, err := f.repo.ListMessages(f.ctx, f.session.ID)
	require.NoError(t, err)
	return messages
}

func TestExplicitResumeNoticeAttemptOwnership(t *testing.T) {
	f := newExplicitResumeNoticeFixture(t, true)

	f.service.handleAgentBootReady(f.ctx, f.event)
	f.service.handleAgentBootReady(f.ctx, f.event)

	messages := f.messages(t)
	require.Len(t, messages, 1, "success and replay should resolve to one durable session notice")
	require.Equal(t, "status", string(messages[0].Type))
	require.Equal(t, f.session.ID, messages[0].TaskSessionID)
	require.Equal(t, "resume_settings_provider_restored", messages[0].Metadata["variant"])
	require.Equal(t, f.attempt.identity(), messages[0].Metadata["attempt_id"])
	require.Equal(t, "provider_restored", messages[0].Metadata["settings_policy"])
	require.Equal(t, []interface{}{"mode", "model"}, messages[0].Metadata["skipped_settings"])
	require.Equal(t, []interface{}{"agent_profile", "runtime_config", "workflow_overrides", "provider_config_options"}, messages[0].Metadata["skipped_selection_sources"])
	require.Equal(t, true, messages[0].Metadata["effective_model_known"])
	require.Equal(t, true, messages[0].Metadata["effective_mode_known"])
	require.Equal(t, "provider-model", messages[0].Metadata["effective_model_id"])
	require.Equal(t, "Provider Model", messages[0].Metadata["effective_model_name"])
	require.Equal(t, "provider-mode", messages[0].Metadata["effective_mode_id"])
	require.Equal(t, "prior-selection-failure", messages[0].Metadata["resolved_error_stamp"])
}

func TestExplicitResumeNoticeRetriesAfterFailedWriteAndAttemptCleanup(t *testing.T) {
	f := newExplicitResumeNoticeFixture(t, true)
	f.service.messageCreator = &failOnceBootstrapMessageCreator{
		serviceBackedMessageCreator: newServiceBackedMessageCreator(f.repo),
		err:                         errors.New("temporary message write failure"),
	}

	f.service.handleAgentBootReady(f.ctx, f.event)
	require.Empty(t, f.messages(t))
	f.attempt.finish(f.registry)
	f.service.handleAgentBootReady(f.ctx, f.event)

	messages := f.messages(t)
	require.Len(t, messages, 1, "replayed boot-ready should retry a failed idempotent write after attempt cleanup")
	require.Equal(t, f.attempt.identity(), messages[0].Metadata["attempt_id"])
	require.Equal(t, "prior-selection-failure", messages[0].Metadata["resolved_error_stamp"])
}

func TestExplicitResumeNoticeKeepsSelectorsUnknownWhenSnapshotBelongsToAnotherAttempt(t *testing.T) {
	f := newExplicitResumeNoticeFixture(t, true)
	require.NoError(t, f.repo.SetSessionMetadataKey(f.ctx, f.session.ID, models.SessionMetaKeyACPModelState, lifecycle.SessionModelsSnapshot{
		CurrentModelID:    "previous-attempt-model",
		CurrentModeID:     "previous-attempt-mode",
		SettingsAttemptID: "resume-previous",
	}))
	f.service.handleAgentBootReady(f.ctx, f.event)

	messages := f.messages(t)
	require.Len(t, messages, 1)
	require.Equal(t, false, messages[0].Metadata["effective_model_known"])
	require.Equal(t, false, messages[0].Metadata["effective_mode_known"])
	require.NotContains(t, messages[0].Metadata, "effective_model_id")
	require.NotContains(t, messages[0].Metadata, "effective_mode_id")
}

func TestExplicitResumeNoticeKeepsPartialProviderReport(t *testing.T) {
	f := newExplicitResumeNoticeFixture(t, true)
	require.NoError(t, f.repo.SetSessionMetadataKey(f.ctx, f.session.ID, models.SessionMetaKeyACPModelState, lifecycle.SessionModelsSnapshot{
		CurrentModelID:    "provider-model",
		SettingsAttemptID: f.attempt.identity(),
		Models:            []streams.SessionModelInfo{{ModelID: "provider-model", Name: "Provider Model"}},
	}))
	f.service.handleAgentBootReady(f.ctx, f.event)

	messages := f.messages(t)
	require.Len(t, messages, 1)
	require.Equal(t, true, messages[0].Metadata["effective_model_known"])
	require.Equal(t, "provider-model", messages[0].Metadata["effective_model_id"])
	require.Equal(t, "Provider Model", messages[0].Metadata["effective_model_name"])
	require.Equal(t, false, messages[0].Metadata["effective_mode_known"])
	require.NotContains(t, messages[0].Metadata, "effective_mode_id")
}

func TestExplicitResumeNoticeSanitizesProviderModelEvidence(t *testing.T) {
	tests := []struct {
		name      string
		modelID   string
		modelName string
		wantKnown bool
		wantID    string
		wantName  string
		forbidden string
	}{
		{
			name:      "safe friendly label",
			modelID:   "vendor/model-5",
			modelName: "Gemini 3.7 Flash",
			wantKnown: true,
			wantID:    "vendor/model-5",
			wantName:  "Gemini 3.7 Flash",
		},
		{
			name:      "credential assignment label falls back to confirmed ID",
			modelID:   "vendor/model-5",
			modelName: "token=synthetic-private-value",
			wantKnown: true,
			wantID:    "vendor/model-5",
			forbidden: "synthetic-private-value",
		},
		{
			name:      "authenticated URL label falls back to confirmed ID",
			modelID:   "vendor/model-5",
			modelName: "https://alice:synthetic-password@private.example/models/key",
			wantKnown: true,
			wantID:    "vendor/model-5",
			forbidden: "synthetic-password",
		},
		{
			name:      "private path label falls back to confirmed ID",
			modelID:   "vendor/model-5",
			modelName: "/home/alice/synthetic-private-model",
			wantKnown: true,
			wantID:    "vendor/model-5",
			forbidden: "synthetic-private-model",
		},
		{
			name:      "control label falls back to confirmed ID",
			modelID:   "vendor/model-5",
			modelName: "Gemini\x1b[31msynthetic-control",
			wantKnown: true,
			wantID:    "vendor/model-5",
			forbidden: "synthetic-control",
		},
		{
			name:      "unsafe model ID uses unknown success",
			modelID:   "token=synthetic-private-value",
			modelName: "Synthetic Private Model",
			forbidden: "synthetic-private-value",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newExplicitResumeNoticeFixture(t, true)
			require.NoError(t, f.repo.SetSessionMetadataKey(f.ctx, f.session.ID, models.SessionMetaKeyACPModelState, lifecycle.SessionModelsSnapshot{
				CurrentModelID:    test.modelID,
				SettingsAttemptID: f.attempt.identity(),
				Models:            []streams.SessionModelInfo{{ModelID: test.modelID, Name: test.modelName}},
			}))
			f.service.handleAgentBootReady(f.ctx, f.event)

			messages := f.messages(t)
			require.Len(t, messages, 1)
			metadata := messages[0].Metadata
			require.Equal(t, test.wantKnown, metadata["effective_model_known"])
			if test.wantID == "" {
				require.NotContains(t, metadata, "effective_model_id")
			} else {
				require.Equal(t, test.wantID, metadata["effective_model_id"])
			}
			if test.wantName == "" {
				require.NotContains(t, metadata, "effective_model_name")
			} else {
				require.Equal(t, test.wantName, metadata["effective_model_name"])
			}
			if test.forbidden != "" {
				encoded, err := json.Marshal(metadata)
				require.NoError(t, err)
				require.NotContains(t, string(encoded), test.forbidden)
			}
		})
	}
}

func TestSuccessfulResumePersistsExactResolutionWithoutTranscriptNotice(t *testing.T) {
	f := newExplicitResumeNoticeFixture(t, false)
	lastError := models.LastAgentError{
		Message:    "The selected model could not be applied.",
		OccurredAt: time.Date(2026, time.September, 30, 10, 0, 0, 0, time.UTC),
		StampValue: "strict-selection-failure",
		Phase:      models.LaunchErrorPhaseBootstrap,
		Causes: []models.AgentErrorCause{{
			Operation: models.AgentErrorCauseOperationResume,
			Code:      models.AgentErrorCauseCodeModelSelectionFailed,
			Reason:    models.AgentErrorCauseReasonApplicationFailed,
		}},
	}
	require.NoError(t, f.repo.SetSessionMetadataKey(f.ctx, f.session.ID, models.SessionMetaKeyLastAgentError, lastError))
	require.True(t, f.registry.setRecoveryErrorStamp(f.attempt, lastError.Stamp()))
	f.service.messageCreator = &failOnceBootstrapMessageCreator{
		serviceBackedMessageCreator: newServiceBackedMessageCreator(f.repo),
		err:                         errors.New("temporary transcript write failure"),
	}
	f.service.handleAgentBootReady(f.ctx, f.event)

	reloaded, err := f.repo.GetTaskSession(f.ctx, f.session.ID)
	require.NoError(t, err)
	encoded, err := json.Marshal(reloaded.Metadata["recovery_resolutions"])
	require.NoError(t, err)
	var resolutions []map[string]any
	require.NoError(t, json.Unmarshal(encoded, &resolutions))
	require.Len(t, resolutions, 1)
	require.Equal(t, lastError.Stamp(), resolutions[0]["error_stamp"])
	require.Equal(t, f.attempt.identity(), resolutions[0]["attempt_id"])
	require.NotEmpty(t, resolutions[0]["resolved_at"])
	require.Empty(t, f.messages(t), "strict recovery must not depend on a provider-restored transcript row")
}

func TestSuccessfulResumeResolutionDoesNotDismissSuccessorFailure(t *testing.T) {
	f := newExplicitResumeNoticeFixture(t, true)
	successor := models.LastAgentError{
		Message:    "A later resumed attempt failed.",
		OccurredAt: time.Date(2026, time.September, 30, 10, 1, 0, 0, time.UTC),
		StampValue: "successor-failure",
		Phase:      models.LaunchErrorPhaseBootstrap,
	}
	require.NoError(t, f.repo.SetSessionMetadataKey(f.ctx, f.session.ID, models.SessionMetaKeyLastAgentError, successor))
	f.service.messageCreator = &failOnceBootstrapMessageCreator{
		serviceBackedMessageCreator: newServiceBackedMessageCreator(f.repo),
		err:                         errors.New("temporary transcript write failure"),
	}
	f.service.handleAgentBootReady(f.ctx, f.event)

	reloaded, err := f.repo.GetTaskSession(f.ctx, f.session.ID)
	require.NoError(t, err)
	storedError, ok := models.LoadLastAgentError(reloaded.Metadata)
	require.True(t, ok)
	require.Equal(t, successor.Stamp(), storedError.Stamp())
	require.False(t, storedError.IsDismissed(), "an older resume must not dismiss its successor failure")
	encoded, err := json.Marshal(reloaded.Metadata["recovery_resolutions"])
	require.NoError(t, err)
	var resolutions []map[string]any
	require.NoError(t, json.Unmarshal(encoded, &resolutions))
	require.Len(t, resolutions, 1)
	require.Equal(t, "prior-selection-failure", resolutions[0]["error_stamp"])
}

func TestExplicitResumeNoticeDoesNotUseAnUnrelatedCatalogLabel(t *testing.T) {
	f := newExplicitResumeNoticeFixture(t, true)
	require.NoError(t, f.repo.SetSessionMetadataKey(f.ctx, f.session.ID, models.SessionMetaKeyACPModelState, lifecycle.SessionModelsSnapshot{
		CurrentModelID:    "provider-model",
		SettingsAttemptID: f.attempt.identity(),
		Models:            []streams.SessionModelInfo{{ModelID: "another-model", Name: "Provider Model"}},
	}))
	f.service.handleAgentBootReady(f.ctx, f.event)

	messages := f.messages(t)
	require.Len(t, messages, 1)
	require.Equal(t, true, messages[0].Metadata["effective_model_known"])
	require.Equal(t, "provider-model", messages[0].Metadata["effective_model_id"])
	require.NotContains(t, messages[0].Metadata, "effective_model_name")
}

func TestExplicitResumeNoticeResolvesOnlyTheFailureCapturedByItsAttempt(t *testing.T) {
	f := newExplicitResumeNoticeFixture(t, true)
	newerError := models.LastAgentError{
		Message:          "A newer startup attempt failed.",
		OccurredAt:       time.Date(2026, time.September, 30, 10, 5, 0, 0, time.UTC),
		AgentExecutionID: "resume-notice-execution",
		ExecutionID:      "resume-notice-execution",
		AttemptID:        f.attempt.identity(),
		Phase:            models.LaunchErrorPhaseBootstrap,
		StampValue:       "newer-selection-failure",
		Causes: []models.AgentErrorCause{{
			Operation: models.AgentErrorCauseOperationResume,
			Code:      models.AgentErrorCauseCodeModelUnavailable,
		}},
	}
	require.NoError(t, f.repo.SetSessionMetadataKey(f.ctx, f.session.ID, models.SessionMetaKeyLastAgentError, newerError))
	f.session.Metadata[models.SessionMetaKeyLastAgentError] = newerError

	f.service.handleAgentBootReady(f.ctx, f.event)

	messages := f.messages(t)
	require.Len(t, messages, 1)
	require.Equal(t, "prior-selection-failure", messages[0].Metadata["resolved_error_stamp"])
	stored, err := f.repo.GetTaskSession(f.ctx, f.session.ID)
	require.NoError(t, err)
	storedError, found := models.LoadLastAgentError(stored.Metadata)
	require.True(t, found)
	require.Equal(t, "newer-selection-failure", storedError.Stamp())
	require.False(t, storedError.IsDismissed(), "a prior attempt must not retire a newer failure")
}

func TestExplicitResumeNoticeDoesNotClaimUncapturedFailureAsRecovered(t *testing.T) {
	f := newExplicitResumeNoticeFixture(t, true)
	require.True(t, f.registry.setRecoveryErrorStamp(f.attempt, ""))
	f.service.handleAgentBootReady(f.ctx, f.event)

	messages := f.messages(t)
	require.Len(t, messages, 1)
	require.NotContains(t, messages[0].Metadata, "resolved_error_stamp")
}

func TestBeginResumeAttemptCapturesOnlyTheActiveRecoveryFailure(t *testing.T) {
	t.Run("active failure", func(t *testing.T) {
		f := newExplicitResumeNoticeFixture(t, false)
		lastError := models.LastAgentError{
			Message:    "The saved model could not be applied.",
			OccurredAt: time.Date(2026, time.September, 30, 10, 0, 0, 0, time.UTC),
			StampValue: "captured-selection-failure",
			Causes: []models.AgentErrorCause{{
				Operation: models.AgentErrorCauseOperationResume,
				Code:      models.AgentErrorCauseCodeModelUnavailable,
			}},
		}
		require.NoError(t, f.repo.SetSessionMetadataKey(f.ctx, f.session.ID, models.SessionMetaKeyLastAgentError, lastError))
		f.attempt.finish(f.registry)

		attempt, owner, err := f.service.beginResumeAttempt(f.ctx, f.session.TaskID, f.session.ID)
		require.NoError(t, err)
		require.True(t, owner)
		require.Equal(t, lastError.Stamp(), f.registry.recoveryErrorStamp(f.session.ID, attempt.identity()))
	})

	t.Run("manually dismissed failure", func(t *testing.T) {
		f := newExplicitResumeNoticeFixture(t, false)
		dismissedAt := time.Date(2026, time.September, 30, 10, 1, 0, 0, time.UTC)
		lastError := models.LastAgentError{
			Message:     "The saved model could not be applied.",
			OccurredAt:  time.Date(2026, time.September, 30, 10, 0, 0, 0, time.UTC),
			StampValue:  "dismissed-selection-failure",
			DismissedAt: &dismissedAt,
		}
		require.NoError(t, f.repo.SetSessionMetadataKey(f.ctx, f.session.ID, models.SessionMetaKeyLastAgentError, lastError))
		f.attempt.finish(f.registry)

		attempt, owner, err := f.service.beginResumeAttempt(f.ctx, f.session.TaskID, f.session.ID)
		require.NoError(t, err)
		require.True(t, owner)
		require.Empty(t, f.registry.recoveryErrorStamp(f.session.ID, attempt.identity()))
	})
}

func TestExplicitResumeNoticeRejectsSuccessorAndOrdinaryResume(t *testing.T) {
	t.Run("older attempt after successor finishes", func(t *testing.T) {
		f := newExplicitResumeNoticeFixture(t, true)
		f.service.messageCreator = &failOnceBootstrapMessageCreator{
			serviceBackedMessageCreator: newServiceBackedMessageCreator(f.repo),
			err:                         errors.New("temporary message write failure"),
		}
		f.service.handleAgentBootReady(f.ctx, f.event)
		require.Empty(t, f.messages(t))
		f.attempt.finish(f.registry)

		successor, owner := f.registry.begin(f.ctx, f.session.TaskID, f.session.ID)
		require.True(t, owner)
		successor.setExecutionID("resume-notice-execution")
		successor.finish(f.registry)
		f.service.handleAgentBootReady(f.ctx, f.event)

		require.Empty(t, f.messages(t), "an older attempt must not publish success after its successor")
	})

	t.Run("ordinary or already-running strict startup", func(t *testing.T) {
		f := newExplicitResumeNoticeFixture(t, true)
		f.event.SessionSettingsPolicy = streams.SessionSettingsPolicyStrict
		f.service.handleAgentBootReady(f.ctx, f.event)
		require.Empty(t, f.messages(t), "a provider-restored request bound to a strict existing execution must not claim success")
	})
}

func TestProviderRestoredRecoveryIdentitySurvivesRegistryRestart(t *testing.T) {
	f := newExplicitResumeNoticeFixture(t, true)
	f.service.messageCreator = newServiceBackedMessageCreator(f.repo)

	// Model data and a notice written by a previous version, whose first
	// process-local recovery identity was resume-1.
	f.attempt.id = 1
	f.registry.nextID = 1
	f.event.AttemptID = "resume-1"

	session, err := f.repo.GetTaskSession(f.ctx, f.session.ID)
	require.NoError(t, err)
	snapshot, ok := lifecycle.LoadSessionModelsSnapshot(session.Metadata[models.SessionMetaKeyACPModelState])
	require.True(t, ok)
	snapshot.SettingsAttemptID = "resume-1"
	snapshot.CurrentModelGeneration = 9
	snapshot.CurrentModeGeneration = 8
	require.NoError(t, f.repo.SetSessionMetadataKey(f.ctx, session.ID, models.SessionMetaKeyACPModelState, snapshot))
	require.NoError(t, f.service.persistProviderRestoredResumeNotice(f.ctx, session, f.event))
	oldNotice := f.messages(t)[0]
	f.attempt.finish(f.registry)

	// A restarted service has a fresh registry, but its identity range must not
	// collide with a legacy resume-1 snapshot or notice.
	newRegistry := newResumeAttemptRegistry()
	newAttempt, owner := newRegistry.begin(f.ctx, session.TaskID, session.ID)
	require.True(t, owner)
	require.NotEqual(t, "resume-1", newAttempt.identity())
	require.NotEqual(t, f.attempt.identity(), newAttempt.identity())
	nextAttempt, owner := newRegistry.begin(f.ctx, session.TaskID, session.ID+"-next")
	require.True(t, owner)
	require.Equal(t, newAttempt.id+1, nextAttempt.id, "identities remain monotonic within one process")
	require.True(t, newRegistry.setSettingsPolicy(newAttempt, executor.ResumeSettingsPolicyProviderRestored))
	newAttempt.setExecutionID("resume-notice-execution-new")

	newService := createTestService(f.repo, newMockStepGetter(), newMockTaskRepo())
	newService.resumeAttempts = newRegistry
	newService.messageCreator = newServiceBackedMessageCreator(f.repo)
	newService.eventBus = &recordingEventBus{}
	newAttemptID := newAttempt.identity()
	providerEvent := func(generation uint64, data *lifecycle.AgentStreamEventData) *lifecycle.AgentStreamEventPayload {
		data.SessionSettingsPolicy = streams.SessionSettingsPolicyProviderRestored
		data.SessionSettingsGeneration = generation
		return &lifecycle.AgentStreamEventPayload{
			Type: "agent/event", AttemptID: newAttemptID, ExecutionID: "resume-notice-execution-new",
			TaskID: session.TaskID, SessionID: session.ID, Data: data,
		}
	}
	newService.handleSessionModeEvent(f.ctx, providerEvent(1, &lifecycle.AgentStreamEventData{CurrentModeID: "new-provider-mode"}))
	newService.handleSessionModelsEvent(f.ctx, providerEvent(1, &lifecycle.AgentStreamEventData{
		CurrentModelID: "new-provider-model",
		SessionModels:  []streams.SessionModelInfo{{ModelID: "new-provider-model", Name: "New provider model"}},
	}))

	updated, err := f.repo.GetTaskSession(f.ctx, session.ID)
	require.NoError(t, err)
	newSnapshot, ok := lifecycle.LoadSessionModelsSnapshot(updated.Metadata[models.SessionMetaKeyACPModelState])
	require.True(t, ok)
	require.Equal(t, newAttemptID, newSnapshot.SettingsAttemptID)
	require.Equal(t, "new-provider-mode", newSnapshot.CurrentModeID)
	require.Equal(t, "new-provider-model", newSnapshot.CurrentModelID)
	require.Equal(t, uint64(1), newSnapshot.CurrentModeGeneration)
	require.Equal(t, uint64(1), newSnapshot.CurrentModelGeneration)

	newEvent := watcher.AgentEventData{
		TaskID: session.TaskID, SessionID: session.ID, AgentExecutionID: "resume-notice-execution-new",
		AttemptID: newAttemptID, SessionSettingsPolicy: streams.SessionSettingsPolicyProviderRestored,
	}
	require.NoError(t, newService.persistProviderRestoredResumeNotice(f.ctx, updated, newEvent))
	require.NoError(t, newService.persistProviderRestoredResumeNotice(f.ctx, updated, newEvent))

	messages := f.messages(t)
	require.Len(t, messages, 2, "the post-restart attempt gets one notice distinct from its predecessor")
	require.NotEqual(t, oldNotice.ID, messages[1].ID)
	require.Equal(t, newAttemptID, messages[1].Metadata["attempt_id"])
}
