package service

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestExecutorFailureReconciliationPersistsIdleCauseAndRecovery(t *testing.T) {
	f := newSweepFixture(t, time.Hour)
	repo := f.rawRepo
	require.NoError(t, repo.CreateTaskEnvironment(t.Context(), &models.TaskEnvironment{ID: "env-1", TaskID: "task-1", OwnershipGeneration: 1, ExecutorType: "local_docker", Status: models.TaskEnvironmentStatusReady, ContainerID: "owned"}))
	controller, ok := any(f.svc).(interface {
		SetExecutorInspector(func(context.Context, models.ExecutorObservationTarget) (*models.ExecutorObservation, error))
		ReconcileExecutorFailures(context.Context)
		RecheckExecutorFailure(context.Context, string, string, int64) (*models.ExecutorFailureEpisode, error)
	})
	require.True(t, ok, "task owner must reconcile retained executor failures")
	now := time.Now().UTC()
	outcome := "terminated"
	var inspectErr error
	calls := 0
	controller.SetExecutorInspector(func(ctx context.Context, target models.ExecutorObservationTarget) (*models.ExecutorObservation, error) {
		calls++
		return &models.ExecutorObservation{Outcome: outcome, Runtime: target.Runtime, ResourceKey: target.ResourceKey, ObservedAt: now, Reason: "ContainerExited", Workspace: "unknown"}, inspectErr
	})
	controller.ReconcileExecutorFailures(t.Context())
	episode, err := repo.GetExecutorFailure(t.Context(), "task-1")
	require.NoError(t, err)
	require.NotNil(t, episode)
	require.Equal(t, "active", episode.State)
	firstID := episode.ID
	controller.ReconcileExecutorFailures(t.Context())
	again, err := repo.GetExecutorFailure(t.Context(), "task-1")
	require.NoError(t, err)
	require.Equal(t, firstID, again.ID)
	inspectErr = errors.New("API temporarily unavailable")
	outcome = "unknown"
	now = now.Add(time.Minute)
	_, err = controller.RecheckExecutorFailure(t.Context(), "task-1", episode.ID, episode.Revision)
	require.Error(t, err)
	preserved, err := repo.GetExecutorFailure(t.Context(), "task-1")
	require.NoError(t, err)
	require.Equal(t, "ContainerExited", preserved.Observation.Reason)
	require.Equal(t, "active", preserved.State)
	require.Equal(t, "unknown", preserved.CurrentOutcome, "current uncertainty must survive reload without erasing the cause")
	before := calls
	_, err = controller.RecheckExecutorFailure(t.Context(), "task-1", episode.ID, episode.Revision+100)
	require.Error(t, err)
	require.Equal(t, before, calls, "stale recheck cannot inspect replacement")
	inspectErr = nil
	outcome = "healthy"
	now = now.Add(time.Minute)
	resolved, err := controller.RecheckExecutorFailure(t.Context(), "task-1", episode.ID, preserved.Revision)
	require.NoError(t, err)
	require.Equal(t, "resolved", resolved.State)
	session, err := repo.GetTaskSession(t.Context(), "session-1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State, "observer must not manufacture session completion")
}

func TestExecutorFailureReloadRepairsMissingAndStaleSummary(t *testing.T) {
	f := newSweepFixture(t, time.Hour)
	f.svc.statusSummaries = f.rawRepo
	require.NoError(t, f.rawRepo.CreateTaskEnvironment(t.Context(), &models.TaskEnvironment{ID: "env-1", TaskID: "task-1", OwnershipGeneration: 1, ExecutorType: "local_docker", Status: models.TaskEnvironmentStatusReady, ContainerID: "owned"}))
	target := models.ExecutorObservationTarget{TaskID: "task-1", EnvironmentID: "env-1", OwnershipGeneration: 1, ResourceKey: "owned", Runtime: "docker"}
	observation := &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "owned", Runtime: "docker", ObservedAt: time.Now().UTC(), Reason: "ContainerExited", Workspace: "unknown"}
	episode, _, err := f.rawRepo.ObserveExecutorFailure(t.Context(), target, observation)
	require.NoError(t, err)
	task, err := f.rawRepo.GetTask(t.Context(), "task-1")
	require.NoError(t, err)
	summaries, err := f.svc.ReconcileTaskStatusSummaries(t.Context(), []*models.Task{task}, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, summaries[task.ID])
	require.NotNil(t, summaries[task.ID].ExecutorFailure, "reload must hydrate failure without an event")
	require.Equal(t, episode.ID, summaries[task.ID].ExecutorFailure.ID)
	persisted, err := f.rawRepo.LoadTaskStatusSummaries(t.Context(), []string{task.ID})
	require.NoError(t, err)
	reloaded, err := f.svc.ReconcileTaskStatusSummaries(t.Context(), []*models.Task{task}, nil, nil, persisted)
	require.NoError(t, err, "private observation authority must not make an unchanged public projection require a semantic no-op CAS")
	require.NotNil(t, reloaded[task.ID].ExecutorFailure)

	observation.Outcome = "healthy"
	observation.ObservedAt = observation.ObservedAt.Add(time.Minute)
	_, _, err = f.rawRepo.ObserveExecutorFailure(t.Context(), target, observation)
	require.NoError(t, err)
	summaries, err = f.svc.ReconcileTaskStatusSummaries(t.Context(), []*models.Task{task}, nil, nil, summaries)
	require.NoError(t, err)
	require.Equal(t, "resolved", summaries[task.ID].ExecutorFailure.State)
}

func TestExecutorFailureConfirmedLossSettlesExactTurnWithoutCompletion(t *testing.T) {
	f := newSweepFixture(t, time.Hour)
	controller, ok := any(f.svc).(interface {
		SetExecutorLossRetirer(func(context.Context, models.ExecutorObservationTarget, *models.ExecutorObservation) (bool, error))
	})
	require.True(t, ok, "only confirmed lost executions can release recovery ownership")
	require.NoError(t, f.rawRepo.CreateTaskEnvironment(t.Context(), &models.TaskEnvironment{ID: "env-1", TaskID: "task-1", OwnershipGeneration: 1, ExecutorType: "local_docker", Status: models.TaskEnvironmentStatusReady, ContainerID: "owned"}))
	session, err := f.rawRepo.GetTaskSession(t.Context(), "session-1")
	require.NoError(t, err)
	session.TaskEnvironmentID = "env-1"
	require.NoError(t, f.rawRepo.UpdateTaskSession(t.Context(), session))
	require.NoError(t, f.rawRepo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "running", TaskID: "task-1", SessionID: "session-1", AgentExecutionID: "lost", ContainerID: "owned", Status: "running"}))
	require.NoError(t, f.rawRepo.CreateTurn(t.Context(), &models.Turn{ID: "lost-turn", TaskID: "task-1", TaskSessionID: "session-1", StartedAt: time.Now().UTC()}))
	now := time.Now().UTC()
	canRetire := false
	f.svc.SetExecutorInspector(func(ctx context.Context, target models.ExecutorObservationTarget) (*models.ExecutorObservation, error) {
		return &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "owned", Runtime: "docker", ObservedAt: now, Reason: "ContainerExited", Workspace: "unknown"}, nil
	})
	controller.SetExecutorLossRetirer(func(ctx context.Context, target models.ExecutorObservationTarget, obs *models.ExecutorObservation) (bool, error) {
		require.Equal(t, "lost", target.ExecutionID)
		return canRetire, nil
	})
	f.svc.ReconcileExecutorFailures(t.Context())
	historyPublished := false
	for _, event := range f.eventBus.GetPublishedEvents() {
		if event.Type == "message.added" {
			historyPublished = true
		}
	}
	require.True(t, historyPublished, "committed incident history must reach the open transcript without reload")
	still, err := f.rawRepo.GetTaskSession(t.Context(), "session-1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, still.State)
	canRetire = true
	now = now.Add(time.Minute)
	f.svc.ReconcileExecutorFailures(t.Context())
	recovered, err := f.rawRepo.GetTaskSession(t.Context(), "session-1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, recovered.State)
	turn, err := f.rawRepo.GetTurn(t.Context(), "lost-turn")
	require.NoError(t, err)
	require.NotNil(t, turn.CompletedAt)
	require.True(t, turn.CompletedAt.Equal(turn.StartedAt), "interrupted turn is abandoned with zero duration")
	for _, event := range f.eventBus.GetPublishedEvents() {
		require.NotEqual(t, "turn.completed", event.Type)
	}
	refs, err := f.rawRepo.ListExecutorFailureAffectedSessions(t.Context(), "task-1")
	require.NoError(t, err)
	require.Empty(t, refs)
}

func TestExecutorFailureUnavailableWorkerReconciliationDoesNotSettleSession(t *testing.T) {
	f := newSweepFixture(t, time.Hour)
	repo := f.rawRepo
	require.NoError(t, repo.CreateTaskEnvironment(t.Context(), &models.TaskEnvironment{ID: "env-outage", TaskID: "task-1", OwnershipGeneration: 1, ExecutorType: string(models.ExecutorTypeKubernetes), Status: models.TaskEnvironmentStatusReady}))
	inventory, err := repo.ClaimKubernetesEnvironment(t.Context(), "env-outage", "task-1", 1, "test-inventory")
	require.NoError(t, err)
	inventory.Metadata = map[string]interface{}{"kubernetes_pod_uid": "owned-pod"}
	require.NoError(t, repo.SaveKubernetesEnvironment(t.Context(), inventory, true))
	session, err := repo.GetTaskSession(t.Context(), "session-1")
	require.NoError(t, err)
	session.TaskEnvironmentID = "env-outage"
	require.NoError(t, repo.UpdateTaskSession(t.Context(), session))
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "running", TaskID: "task-1", SessionID: "session-1", AgentExecutionID: "uncertain", Status: "running", Runtime: "k8s", Metadata: inventory.Metadata}))
	f.svc.SetExecutorInspector(func(_ context.Context, target models.ExecutorObservationTarget) (*models.ExecutorObservation, error) {
		return &models.ExecutorObservation{Runtime: "k8s", ResourceKey: target.ResourceKey, Outcome: "unknown", Reason: "WorkerUnavailable", ObservedAt: time.Now().UTC(), Workspace: "retained"}, nil
	})
	retired := false
	f.svc.SetExecutorLossRetirer(func(context.Context, models.ExecutorObservationTarget, *models.ExecutorObservation) (bool, error) {
		retired = true
		return true, nil
	})
	f.svc.ReconcileExecutorFailures(t.Context())
	episode, err := repo.GetExecutorFailure(t.Context(), "task-1")
	require.NoError(t, err)
	require.NotNil(t, episode)
	require.Equal(t, "unknown", episode.CurrentOutcome)
	require.False(t, retired, "uncertainty must retain execution capacity and recovery ownership")
	session, err = repo.GetTaskSession(t.Context(), "session-1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	row, err := repo.GetExecutorRunningBySessionID(t.Context(), "session-1")
	require.NoError(t, err)
	require.Equal(t, "uncertain", row.AgentExecutionID)
}
