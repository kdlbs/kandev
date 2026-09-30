package orchestrator

import (
	"context"
	"errors"
	"testing"

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
		}))
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
	require.Equal(t, "provider-mode", messages[0].Metadata["effective_mode_id"])
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
