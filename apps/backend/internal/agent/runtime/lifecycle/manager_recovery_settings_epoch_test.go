package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/agentruntime"
	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestRestoreRecoveredSessionSettingsSourceWithoutSessionKeepsLegacyDefaults(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID: "execution-without-session", SessionSettingsPolicy: SessionSettingsPolicyStrict,
		SessionSettingsProjectionPolicy: SessionSettingsPolicyProviderRestored,
	}

	require.NoError(t, mgr.restoreRecoveredSessionSettingsSource(context.Background(), execution))
	require.Zero(t, execution.startupAttemptSnapshot())
	require.Empty(t, execution.ResumeAttemptID)
	require.Equal(t, SessionSettingsPolicyStrict, execution.SessionSettingsPolicy)
	require.Equal(t, SessionSettingsPolicyStrict, execution.SessionSettingsProjectionPolicy)
}

func TestManagerStartRestoresSettingsSourceForAdoptedExecution(t *testing.T) {
	const (
		executionID = "execution-adopted"
		sessionID   = "session-adopted"
		attemptID   = "resume-opaque-attempt"
		taskID      = "task-adopted"
	)
	ctx := context.Background()
	repo := newSettingsSourceRecoveryRepository(t, taskID, sessionID)
	persistedSnapshot := SessionModelsSnapshot{
		CurrentModelID:            "provider-model",
		CurrentModeID:             "provider-mode",
		SettingsAttemptID:         attemptID,
		SettingsPolicy:            streams.SessionSettingsPolicyProviderRestored,
		SettingsSourceExecutionID: executionID,
		SettingsSourceGeneration:  9,
		CurrentModelGeneration:    6,
		CurrentModeGeneration:     8,
	}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyACPModelState, persistedSnapshot))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeySessionMode, "saved-mode"))
	savedRuntimeConfig := models.SessionRuntimeConfig{
		Model: "saved-model", Mode: "saved-mode", ConfigOptions: map[string]string{"reasoning_effort": "saved-effort"},
	}
	savedRuntimeOverrides := models.SessionRuntimeConfig{
		Model: "saved-override-model", Mode: "saved-override-mode",
		ConfigOptions: map[string]string{"reasoning_effort": "saved-override-effort"},
	}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyRuntimeConfig, savedRuntimeConfig))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyRuntimeConfigOverrides, savedRuntimeOverrides))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		TaskID: taskID, SessionID: sessionID, Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, Resumable: true, AgentExecutionID: executionID,
		ExecutionProfileID: recoveryTestAgentProfileID, ResumeToken: "native-session-token",
	}))

	eventBus := &MockEventBus{}
	mgr := newSettingsSourceRecoveryManager(t, eventBus, repo, executionID, sessionID, taskID)
	t.Cleanup(func() { _ = mgr.Stop() })

	require.NoError(t, mgr.Start(ctx))

	recovered, ok := mgr.GetExecutionBySessionID(sessionID)
	require.True(t, ok)
	require.Equal(t, uint64(10), recovered.startupAttemptSnapshot(),
		"the adopted callbacks must use the durably reserved source epoch")
	require.Equal(t, attemptID, recovered.ResumeAttemptID,
		"backend reconstruction must preserve the source attempt identity")
	require.Equal(t, SessionSettingsPolicyStrict, recovered.SessionSettingsPolicy,
		"reconstruction must not restore provider-report policy as startup authority")
	require.Equal(t, SessionSettingsPolicyProviderRestored, recovered.SessionSettingsProjectionPolicy,
		"the adopted conversation retains provider-restored report provenance")

	reserved := readSettingsSnapshot(t, repo, sessionID)
	require.Equal(t, uint64(10), reserved.SettingsSourceGeneration)
	require.Equal(t, executionID, reserved.SettingsSourceExecutionID)
	require.Equal(t, attemptID, reserved.SettingsAttemptID)
	require.Equal(t, "provider-model", reserved.CurrentModelID)
	require.Equal(t, "provider-mode", reserved.CurrentModeID)
	require.Zero(t, reserved.CurrentModelGeneration)
	require.Zero(t, reserved.CurrentModeGeneration)
	assertSavedSettingsUnchanged(t, repo, sessionID, savedRuntimeConfig, savedRuntimeOverrides)

	// The first manager is discarded without publishing any settings frame.
	// A second reconstruction must reserve another durable epoch rather than
	// restarting from the prior adapter's zeroed local counter.
	mgr.executionStore.Remove(executionID)
	require.NoError(t, mgr.Stop())

	secondBus := &MockEventBus{}
	secondManager := newSettingsSourceRecoveryManager(t, secondBus, repo, executionID, sessionID, taskID)
	require.NoError(t, secondManager.Start(ctx))
	t.Cleanup(func() { _ = secondManager.Stop() })
	secondRecovered, ok := secondManager.GetExecutionBySessionID(sessionID)
	require.True(t, ok)
	require.Equal(t, uint64(11), secondRecovered.startupAttemptSnapshot())
	require.Equal(t, attemptID, secondRecovered.ResumeAttemptID)
	require.Equal(t, SessionSettingsPolicyStrict, secondRecovered.SessionSettingsPolicy)
	require.Equal(t, SessionSettingsPolicyProviderRestored, secondRecovered.SessionSettingsProjectionPolicy)

	secondManager.eventPublisher.PublishAgentStreamEvent(secondRecovered, agentctl.AgentEvent{
		Type: streams.EventTypeSessionModels, SessionSettingsPolicy: providerRestoredSettingsPolicy(secondRecovered),
	})
	var published AgentStreamEventPayload
	for _, event := range secondBus.PublishedEvents {
		if payload, ok := event.Data.(AgentStreamEventPayload); ok {
			published = payload
			break
		}
	}
	require.Equal(t, attemptID, published.AttemptID)
	require.Equal(t, uint64(11), published.SessionSettingsSourceGeneration)
	require.Equal(t, streams.SessionSettingsPolicyProviderRestored, published.Data.SessionSettingsPolicy)

	reserved = readSettingsSnapshot(t, repo, sessionID)
	require.Equal(t, uint64(11), reserved.SettingsSourceGeneration)
	require.Equal(t, "provider-model", reserved.CurrentModelID)
	require.Equal(t, "provider-mode", reserved.CurrentModeID)
	require.Zero(t, reserved.CurrentModelGeneration)
	require.Zero(t, reserved.CurrentModeGeneration)
	assertSavedSettingsUnchanged(t, repo, sessionID, savedRuntimeConfig, savedRuntimeOverrides)
	running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, "native-session-token", running.ResumeToken)
}

func TestManagerStartRefusesAdoptionWhenSettingsSourceReservationFails(t *testing.T) {
	const (
		executionID = "execution-reservation-failure"
		sessionID   = "session-reservation-failure"
		taskID      = "task-reservation-failure"
	)
	ctx := context.Background()
	repo := newSettingsSourceRecoveryRepository(t, taskID, sessionID)
	original := SessionModelsSnapshot{
		CurrentModelID:            "provider-model",
		CurrentModeID:             "provider-mode",
		SettingsAttemptID:         "attempt-reservation-failure",
		SettingsPolicy:            streams.SessionSettingsPolicyProviderRestored,
		SettingsSourceExecutionID: executionID,
		SettingsSourceGeneration:  9,
	}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyACPModelState, original))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		TaskID: taskID, SessionID: sessionID, Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, Resumable: true, AgentExecutionID: executionID,
		ExecutionProfileID: recoveryTestAgentProfileID, ResumeToken: "native-session-token",
	}))

	eventBus := &MockEventBus{}
	mgr := newSettingsSourceRecoveryManager(t, eventBus, repo, executionID, sessionID, taskID)
	mgr.SetSessionSettingsSnapshotWriter(failingSettingsSnapshotWriter{err: errors.New("injected metadata failure")})
	t.Cleanup(func() { _ = mgr.Stop() })
	require.NoError(t, mgr.Start(ctx))

	_, tracked := mgr.GetExecutionBySessionID(sessionID)
	require.False(t, tracked, "failed epoch reservation must refuse execution tracking")
	backend, err := mgr.executorRegistry.GetBackend(executor.NameStandalone)
	require.NoError(t, err)
	require.Len(t, backend.(*MockExecutor).stopInstanceCalls, 1,
		"a live runtime instance refused during reconstruction must use the recovery stop path")
	require.Equal(t, original, readSettingsSnapshot(t, repo, sessionID),
		"a failed reservation must leave the existing durable snapshot intact")
	running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, "native-session-token", running.ResumeToken)
	for _, event := range eventBus.PublishedEvents {
		if _, ok := event.Data.(AgentStreamEventPayload); ok {
			t.Fatalf("unexpected session settings publication after failed reservation: %#v", event.Data)
		}
	}
}

func TestManagerStartReplacesOldExecutionProjectionWithoutChangingSavedSettings(t *testing.T) {
	const (
		executionID = "execution-replacement"
		sessionID   = "session-replacement"
		taskID      = "task-replacement"
	)
	ctx := context.Background()
	repo := newSettingsSourceRecoveryRepository(t, taskID, sessionID)
	oldSnapshot := SessionModelsSnapshot{
		CurrentModelID:            "old-provider-model",
		CurrentModeID:             "old-provider-mode",
		SettingsAttemptID:         "old-attempt",
		SettingsPolicy:            streams.SessionSettingsPolicyProviderRestored,
		SettingsSourceExecutionID: "old-execution",
		SettingsSourceGeneration:  12,
		CurrentModelGeneration:    4,
		CurrentModeGeneration:     7,
	}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyACPModelState, oldSnapshot))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeySessionMode, "saved-mode"))
	savedRuntimeConfig := models.SessionRuntimeConfig{
		Model: "saved-model", Mode: "saved-mode", ConfigOptions: map[string]string{"reasoning_effort": "saved-effort"},
	}
	savedRuntimeOverrides := models.SessionRuntimeConfig{Model: "saved-override-model"}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyRuntimeConfig, savedRuntimeConfig))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyRuntimeConfigOverrides, savedRuntimeOverrides))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		TaskID: taskID, SessionID: sessionID, Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, Resumable: true, AgentExecutionID: "old-execution",
		ExecutionProfileID: recoveryTestAgentProfileID, ResumeToken: "native-session-token",
	}))

	withoutWriter := newSettingsSourceRecoveryManager(t, &MockEventBus{}, repo, executionID, sessionID, taskID)
	withoutWriter.SetSessionSettingsSnapshotWriter(nil)
	require.NoError(t, withoutWriter.Start(ctx))
	_, tracked := withoutWriter.GetExecutionBySessionID(sessionID)
	require.False(t, tracked, "a known old source epoch cannot be bypassed when the writer is missing")
	backend, err := withoutWriter.executorRegistry.GetBackend(executor.NameStandalone)
	require.NoError(t, err)
	require.Len(t, backend.(*MockExecutor).stopInstanceCalls, 1)
	require.Equal(t, oldSnapshot, readSettingsSnapshot(t, repo, sessionID))
	require.NoError(t, withoutWriter.Stop())

	manager := newSettingsSourceRecoveryManager(t, &MockEventBus{}, repo, executionID, sessionID, taskID)
	t.Cleanup(func() { _ = manager.Stop() })
	require.NoError(t, manager.Start(ctx))

	recovered, ok := manager.GetExecutionBySessionID(sessionID)
	require.True(t, ok)
	require.Equal(t, uint64(1), recovered.startupAttemptSnapshot())
	require.Empty(t, recovered.ResumeAttemptID)
	require.Equal(t, SessionSettingsPolicyStrict, recovered.SessionSettingsPolicy)
	require.Equal(t, SessionSettingsPolicyStrict, recovered.SessionSettingsProjectionPolicy)
	newSnapshot := readSettingsSnapshot(t, repo, sessionID)
	require.Equal(t, executionID, newSnapshot.SettingsSourceExecutionID)
	require.Equal(t, uint64(1), newSnapshot.SettingsSourceGeneration)
	require.Empty(t, newSnapshot.CurrentModelID)
	require.Empty(t, newSnapshot.CurrentModeID)
	require.Empty(t, newSnapshot.SettingsAttemptID)
	require.Empty(t, newSnapshot.SettingsPolicy)
	require.Zero(t, newSnapshot.CurrentModelGeneration)
	require.Zero(t, newSnapshot.CurrentModeGeneration)
	assertSavedSettingsUnchanged(t, repo, sessionID, savedRuntimeConfig, savedRuntimeOverrides)
	running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, "native-session-token", running.ResumeToken)
}

func TestIndependentStartupResetsRecoveredReportProjectionToStrict(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID: "execution-independent-start", SessionSettingsPolicy: SessionSettingsPolicyStrict,
		SessionSettingsProjectionPolicy: SessionSettingsPolicyProviderRestored,
	}
	require.NoError(t, mgr.executionStore.Add(execution))
	err := mgr.StartAgentProcess(context.Background(), execution.ID)
	require.ErrorContains(t, err, "no agentctl client")
	require.Equal(t, SessionSettingsPolicyStrict, execution.SessionSettingsPolicy,
		"a new start must retain its strict settings request")
	require.Equal(t, SessionSettingsPolicyStrict, execution.SessionSettingsProjectionPolicy,
		"a new strict start must not inherit provider-restored report provenance")
}

func newSettingsSourceRecoveryRepository(t *testing.T, taskID, sessionID string) *tasksqlite.Repository {
	t.Helper()
	conn, err := kandevdb.OpenSQLite(filepath.Join(t.TempDir(), "settings-source.db"))
	require.NoError(t, err)
	db := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	repo, err := tasksqlite.NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-" + taskID, Name: "Recovery"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "workspace-" + taskID, Title: "Recovery"}))
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
		StartedAt: now, UpdatedAt: now,
	}))
	return repo
}

func newSettingsSourceRecoveryManager(
	t *testing.T,
	eventBus *MockEventBus,
	repo *tasksqlite.Repository,
	executionID, sessionID, taskID string,
) *Manager {
	t.Helper()
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID: executionID, TaskID: taskID, SessionID: sessionID,
			RuntimeName: executor.NameStandalone, AgentProfileID: recoveryTestAgentProfileID,
		}},
	})
	mgr := NewManager(newTestRegistry(), eventBus, registry,
		&MockCredentialsManager{}, &MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log)
	registerRecoveryTestAgentProfile(t, mgr)
	mgr.SetExecutorProfileReader(repo)
	mgr.SetSessionSettingsSnapshotWriter(repo)
	mgr.SetExecutorRunningWriter(repo)
	return mgr
}

func readSettingsSnapshot(t *testing.T, repo *tasksqlite.Repository, sessionID string) SessionModelsSnapshot {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	snapshot, ok := LoadSessionModelsSnapshot(session.Metadata[models.SessionMetaKeyACPModelState])
	require.True(t, ok)
	return snapshot
}

type failingSettingsSnapshotWriter struct{ err error }

func (w failingSettingsSnapshotWriter) SetSessionMetadataKey(context.Context, string, string, interface{}) error {
	return w.err
}

func assertSavedSettingsUnchanged(
	t *testing.T,
	repo *tasksqlite.Repository,
	sessionID string,
	wantRuntime models.SessionRuntimeConfig,
	wantOverrides models.SessionRuntimeConfig,
) {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, "saved-mode", models.StringFromAny(session.Metadata[models.SessionMetaKeySessionMode]))
	runtime, ok := models.LoadSessionRuntimeConfig(session.Metadata)
	require.True(t, ok)
	require.Equal(t, wantRuntime, runtime)
	overrides, ok := models.LoadSessionRuntimeConfigOverrides(session.Metadata)
	require.True(t, ok)
	require.Equal(t, wantOverrides, overrides)
}
