package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-EXECUTORS-FAILURE-VISIBILITY-001.3, .5, .6, .8
func TestExecutorFailureSuppressesSessionOpenRecovery(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy session", true: "shared environment"}[shared], func(t *testing.T) {
			repo, target := seedPassiveExecutorFailure(t, shared)
			calls := 0
			agent := &mockAgentManager{repoForExecutionLookup: repo, launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				calls++
				return nil, errors.New("unexpected launch")
			}}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
			svc.executor = executor.NewExecutor(agent, repo, testLogger(), executor.ExecutorConfig{})
			status, err := svc.GetTaskSessionStatus(t.Context(), target.TaskID, "session-reused")
			require.NoError(t, err)
			require.False(t, status.AutoResumeAllowed)
			require.Equal(t, "executor_failure", status.AutoResumeBlockedReason)
			req := &LaunchSessionRequest{TaskID: target.TaskID, SessionID: "session-reused", Intent: IntentResume, ActivationSource: LaunchActivationSourceSessionOpen}
			response, err := svc.LaunchSession(t.Context(), req)
			require.NoError(t, err)
			require.Equal(t, activationDispositionSuppressed, response.ActivationDisposition)
			require.Zero(t, calls)
			session, err := repo.GetTaskSession(t.Context(), req.SessionID)
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
			req.ActivationSource = LaunchActivationSourceUserAction
			require.Nil(t, svc.passiveLaunchResponse(t.Context(), req, IntentResume), "explicit recovery retains its own admission")
		})
	}
}

// @covers AC-EXECUTORS-FAILURE-VISIBILITY-001.5, .6
func TestExecutorFailureAdmissionUsesCurrentScope(t *testing.T) {
	repo, target := seedPassiveExecutorFailure(t, true)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	task, err := repo.GetTask(t.Context(), target.TaskID)
	require.NoError(t, err)
	session, err := repo.GetTaskSession(t.Context(), "session-reused")
	require.NoError(t, err)
	allowed, reason := svc.autoResumeEligibility(t.Context(), task, session)
	require.False(t, allowed)
	require.Equal(t, "executor_failure", reason)
	store := &passiveFailureReadStore{Repository: repo, nextGeneration: true}
	svc.repo = store
	allowed, reason = svc.autoResumeEligibility(t.Context(), task, session)
	require.True(t, allowed, "an old failure cannot fence a successor ownership generation")
	require.Empty(t, reason)
	store.nextGeneration = false
	store.readError = errors.New("inventory unavailable")
	allowed, reason = svc.autoResumeEligibility(t.Context(), task, session)
	require.False(t, allowed)
	require.Equal(t, autoResumeBlockedOwnershipUnavailable, reason)
	store.readError = nil
	store.environmentError = errors.New("environment unavailable")
	allowed, reason = svc.autoResumeEligibility(t.Context(), task, session)
	require.False(t, allowed)
	require.Equal(t, autoResumeBlockedOwnershipUnavailable, reason)
	store.environmentError = nil
	svc.repo = repo
	_, _, err = repo.ObserveExecutorFailure(t.Context(), target, &models.ExecutorObservation{Outcome: models.ExecutorOutcomeHealthy, ResourceKey: target.ResourceKey, ObservedAt: time.Now().UTC().Add(time.Minute)})
	require.NoError(t, err)
	allowed, reason = svc.autoResumeEligibility(t.Context(), task, session)
	require.True(t, allowed, "verified recovery releases passive admission")
	require.Empty(t, reason)
}

// @covers AC-EXECUTORS-FAILURE-VISIBILITY-001.5, .6
func TestExecutorFailureRecheckedAtSessionOpenAdmission(t *testing.T) {
	repo, target := seedPassiveExecutorFailure(t, true)
	_, _, err := repo.ObserveExecutorFailure(t.Context(), target, &models.ExecutorObservation{Outcome: models.ExecutorOutcomeHealthy, ResourceKey: target.ResourceKey, ObservedAt: time.Now().UTC().Add(time.Minute)})
	require.NoError(t, err)
	calls := 0
	agent := &mockAgentManager{repoForExecutionLookup: repo, launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		calls++
		return nil, errors.New("unexpected launch")
	}}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	svc.executor = executor.NewExecutor(agent, repo, testLogger(), executor.ExecutorConfig{})
	svc.sessionCeiling = newSessionCeilingController(unlimitedSessionCeiling, nil, nil)
	store := &latePassiveFailureStore{Repository: repo, target: target}
	svc.repo = store
	response, err := svc.LaunchSession(t.Context(), &LaunchSessionRequest{TaskID: target.TaskID, SessionID: "session-reused", Intent: IntentResume, ActivationSource: LaunchActivationSourceSessionOpen})
	require.NoError(t, err)
	require.GreaterOrEqual(t, store.reads, 2, "launch must recheck the initial passive status")
	require.Equal(t, activationDispositionSuppressed, response.ActivationDisposition)
	require.Equal(t, "executor_failure", response.ActivationReason)
	require.Zero(t, calls)
}

type latePassiveFailureStore struct {
	*sqliterepo.Repository
	target models.ExecutorObservationTarget
	reads  int
}

func (r *latePassiveFailureStore) GetActiveExecutorFailure(ctx context.Context, target models.ExecutorObservationTarget) (*models.ExecutorFailureEpisode, error) {
	r.reads++
	if r.reads == 2 {
		if _, _, err := r.ObserveExecutorFailure(ctx, r.target, &models.ExecutorObservation{Outcome: models.ExecutorOutcomeTerminated, ResourceKey: r.target.ResourceKey, ObservedAt: time.Now().UTC().Add(2 * time.Minute)}); err != nil {
			return nil, err
		}
	}
	return r.Repository.GetActiveExecutorFailure(ctx, target)
}

type passiveFailureReadStore struct {
	*sqliterepo.Repository
	nextGeneration   bool
	readError        error
	environmentError error
}

func (r *passiveFailureReadStore) GetTaskEnvironment(ctx context.Context, id string) (*models.TaskEnvironment, error) {
	if r.environmentError != nil {
		return nil, r.environmentError
	}
	env, err := r.Repository.GetTaskEnvironment(ctx, id)
	if env != nil && r.nextGeneration {
		env.OwnershipGeneration++
	}
	return env, err
}

func (r *passiveFailureReadStore) GetActiveExecutorFailure(ctx context.Context, target models.ExecutorObservationTarget) (*models.ExecutorFailureEpisode, error) {
	if r.readError != nil {
		return nil, r.readError
	}
	return r.Repository.GetActiveExecutorFailure(ctx, target)
}

func seedPassiveExecutorFailure(t *testing.T, shared bool) (*sqliterepo.Repository, models.ExecutorObservationTarget) {
	t.Helper()
	repo := setupTestRepo(t)
	seedSessionOpenRecoveryState(t, repo, models.TaskSessionStateWaitingForInput)
	target := models.ExecutorObservationTarget{TaskID: "task-reused", SessionID: "session-reused", ExecutionID: "execution-reused", Runtime: "docker", ResourceKey: "retained-container"}
	if shared {
		env := &models.TaskEnvironment{ID: "retained-environment", TaskID: target.TaskID, ExecutorType: "docker", Status: models.TaskEnvironmentStatusReady, ContainerID: target.ResourceKey}
		require.NoError(t, repo.CreateTaskEnvironment(t.Context(), env))
		session, err := repo.GetTaskSession(t.Context(), target.SessionID)
		require.NoError(t, err)
		session.TaskEnvironmentID = env.ID
		require.NoError(t, repo.UpdateTaskSession(t.Context(), session))
		target.EnvironmentID, target.OwnershipGeneration, target.SessionID = env.ID, env.OwnershipGeneration, ""
	} else {
		running, err := repo.GetExecutorRunningBySessionID(t.Context(), target.SessionID)
		require.NoError(t, err)
		running.Runtime, running.ContainerID = "docker", target.ResourceKey
		require.NoError(t, repo.UpsertExecutorRunning(t.Context(), running))
	}
	_, _, err := repo.ObserveExecutorFailure(t.Context(), target, &models.ExecutorObservation{Outcome: models.ExecutorOutcomeTerminated, ResourceKey: target.ResourceKey, ObservedAt: time.Now().UTC()})
	require.NoError(t, err)
	return repo, target
}
