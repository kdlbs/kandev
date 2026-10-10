package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type silentRestoreCheckpointFixture struct {
	ctx        context.Context
	service    *Service
	repo       *sqliterepo.Repository
	manager    *mockAgentManager
	store      taskrepo.SilentRestoreRepository
	inputs     *silentRestoreInputs
	attemptID  string
	session    *models.TaskSession
	checkpoint models.SilentRestoreCheckpoint
}

func newSilentRestoreCheckpointFixture(t *testing.T) *silentRestoreCheckpointFixture {
	return newSilentRestoreCheckpointFixtureWithLaunching(t, true)
}

func newSilentRestoreCheckpointFixtureWithLaunching(
	t *testing.T,
	launching bool,
) *silentRestoreCheckpointFixture {
	t.Helper()
	ctx := context.Background()
	service, repo, manager, session, store := newSilentRestoreEligibilityFixture(t, "provider-session", "silent")
	workspace, err := repo.GetWorkspace(ctx, "ws1")
	require.NoError(t, err)
	inputs, err := service.loadSilentRestoreInputs(ctx, models.SilentRestoreCandidate{
		TaskID: session.TaskID, SessionID: session.ID, WorkspaceID: workspace.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, inputs)
	attemptID := silentRestoreAttemptID(session.ID, inputs.recovery.IncarnationID, 1)
	attempt, err := service.ensureSilentRestoreAttempt(ctx, store, attemptID, inputs)
	require.NoError(t, err)
	require.NotNil(t, attempt)
	allocated := *attempt.Checkpoint
	allocated.CandidateExecutionID = "candidate-execution"
	allocated.Stage = models.SilentRestoreStageCandidateAllocated
	changed, err := store.CompareAndSwapSilentRestoreCheckpoint(ctx, attemptID, *attempt.Checkpoint, allocated)
	require.NoError(t, err)
	require.True(t, changed)
	checkpoint := allocated
	if launching {
		require.NoError(t, service.markSilentRestoreCandidateLaunching(
			ctx, store, attemptID, allocated, allocated.CandidateExecutionID, inputs,
		))
		checkpoint.Stage = models.SilentRestoreStageCandidateLaunching
	}
	return &silentRestoreCheckpointFixture{
		ctx: ctx, service: service, repo: repo, manager: manager, store: store,
		inputs: inputs, attemptID: attemptID, session: session, checkpoint: checkpoint,
	}
}

func newSilentRestoreEligibilityFixture(
	t *testing.T,
	nativeSessionID, prefix string,
) (*Service, *sqliterepo.Repository, *mockAgentManager, *models.TaskSession, taskrepo.SilentRestoreRepository) {
	t.Helper()
	ctx := context.Background()
	service, _, _ := continuationFailureFixture(t)
	repo := service.repo.(*sqliterepo.Repository)
	manager := service.agentManager.(*mockAgentManager)
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateFailed
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	seedSilentRestoreCheckpointSource(t, ctx, repo, session, prefix, nativeSessionID)
	workspace, err := repo.GetWorkspace(ctx, "ws1")
	require.NoError(t, err)
	service.SetStartupRecoveryOwnerContextResolver(func(ownerCtx context.Context, ownerID, orgID string) (context.Context, error) {
		if ownerID != workspace.OwnerID || orgID != workspace.OrgID {
			return nil, errors.New("workspace owner changed")
		}
		return ownerCtx, nil
	})
	return service, repo, manager, session, service.repo.(taskrepo.SilentRestoreRepository)
}

func seedSilentRestoreCheckpointSource(
	t *testing.T,
	ctx context.Context,
	repo *sqliterepo.Repository,
	session *models.TaskSession,
	prefix string,
	nativeSessionID string,
) {
	t.Helper()
	executionID := prefix + "-execution"
	submissionID := prefix + "-submission"
	streamID := prefix + "-stream"
	incarnationID := session.QueueIncarnationID
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: incarnationID, Generation: 1,
		NativeSessionID: nativeSessionID, CreationReason: "initial",
	}))
	_, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: session.ID, IncarnationID: incarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: prefix + "-source-hash",
		Payload: []byte("original prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	})
	require.NoError(t, err)
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: executionID, TaskID: session.TaskID, SessionID: session.ID,
		AgentExecutionID: executionID, Status: "running",
	}))
	recovery := models.AgentDeliveryRecovery{
		SessionID: session.ID, AgentExecutionID: executionID, IncarnationID: incarnationID,
		HarnessGeneration: 1, SubmissionID: submissionID, StreamID: streamID,
		PromptGeneration: 1, Phase: models.AgentDeliveryRecoveryUncertain,
	}
	opened, err := repo.UpsertAgentDeliveryRecovery(ctx, &recovery, &models.SessionRecoveryBlock{
		SessionID: session.ID, IncarnationID: incarnationID, ExpectedGeneration: 1,
		Reason: "unknown_prompt_outcome", ConsumerReference: agentDeliveryConsumer,
		DeliverySubmissionID: recovery.SubmissionID, DeliveryStreamID: recovery.StreamID,
		State: models.RecoveryBlockOpen,
	})
	require.NoError(t, err)
	require.True(t, opened)
}

func seedBlockedSilentRestoreCandidate(t *testing.T, ctx context.Context, repo *sqliterepo.Repository) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-blocked", Name: "Foreign workspace", OwnerID: "foreign-owner", OrgID: "foreign-org",
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-blocked", WorkspaceID: "ws-blocked", Title: "Blocked task",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "a-blocked", TaskID: "task-blocked", State: models.TaskSessionStateFailed,
		StartedAt: now, UpdatedAt: now,
	}))
	session, err := repo.GetTaskSession(ctx, "a-blocked")
	require.NoError(t, err)
	seedSilentRestoreCheckpointSource(t, ctx, repo, session, "blocked", "provider-session")
}

func addSilentRestoreCandidateExecution(t *testing.T, fixture *silentRestoreCheckpointFixture) *models.ExecutorRunning {
	t.Helper()
	running := &models.ExecutorRunning{
		ID: fixture.checkpoint.CandidateExecutionID, TaskID: fixture.session.TaskID,
		SessionID: fixture.session.ID, AgentExecutionID: fixture.checkpoint.CandidateExecutionID,
		ResumeToken: "provider-session", Resumable: true, Status: "running",
	}
	require.NoError(t, fixture.repo.UpsertExecutorRunning(fixture.ctx, running))
	return running
}

func markSilentRestoreFixtureCandidateDead(
	t *testing.T,
	fixture *silentRestoreCheckpointFixture,
	running *models.ExecutorRunning,
) models.SilentRestoreCheckpoint {
	t.Helper()
	fixture.manager.rowLivenessFn = func(*models.ExecutorRunning) models.ProcessLiveness {
		return models.ProcessLivenessDead
	}
	dead, err := fixture.service.markSilentRestoreCandidateDead(
		fixture.ctx, fixture.store, fixture.attemptID, fixture.checkpoint, running, fixture.inputs,
	)
	require.NoError(t, err)
	require.Equal(t, models.SilentRestoreStageCandidateDead, dead.Stage)
	return dead
}

func TestSilentRestoreCandidateDeadEvidenceSurvivesCrashBeforeCleanup(t *testing.T) {
	fixture := newSilentRestoreCheckpointFixture(t)
	running := addSilentRestoreCandidateExecution(t, fixture)
	dead := markSilentRestoreFixtureCandidateDead(t, fixture, running)

	stored, err := fixture.store.GetRestoreAttempt(fixture.ctx, fixture.attemptID)
	require.NoError(t, err)
	require.Equal(t, models.SilentRestoreStageCandidateDead, stored.Checkpoint.Stage)
	require.Equal(t, running.AgentExecutionID, stored.Checkpoint.CandidateExecutionID)
	current, err := fixture.repo.GetExecutorRunningBySessionID(fixture.ctx, fixture.session.ID)
	require.NoError(t, err)
	require.Equal(t, running.AgentExecutionID, current.AgentExecutionID)

	rotated, err := fixture.service.rotateDeadSilentRestoreCandidate(
		fixture.ctx, fixture.store, fixture.attemptID, dead, fixture.inputs,
	)
	require.NoError(t, err)
	require.Equal(t, models.SilentRestoreStageCandidateAllocated, rotated.Stage)
	require.NotEqual(t, running.AgentExecutionID, rotated.CandidateExecutionID)
	_, err = fixture.repo.GetExecutorRunningBySessionID(fixture.ctx, fixture.session.ID)
	require.Error(t, err, "restart must clean the exact dead candidate row after reading durable dead evidence")
}

func TestSilentRestoreCandidateDeadEvidenceSurvivesCrashAfterCleanup(t *testing.T) {
	fixture := newSilentRestoreCheckpointFixture(t)
	running := addSilentRestoreCandidateExecution(t, fixture)
	dead := markSilentRestoreFixtureCandidateDead(t, fixture, running)
	require.NoError(t, fixture.service.cleanupProvenDeadSilentRestoreExecution(fixture.ctx, running))
	_, err := fixture.repo.GetExecutorRunningBySessionID(fixture.ctx, fixture.session.ID)
	require.Error(t, err)

	rotated, err := fixture.service.rotateDeadSilentRestoreCandidate(
		fixture.ctx, fixture.store, fixture.attemptID, dead, fixture.inputs,
	)
	require.NoError(t, err)
	require.Equal(t, models.SilentRestoreStageCandidateAllocated, rotated.Stage)
	require.NotEqual(t, running.AgentExecutionID, rotated.CandidateExecutionID)
}

func TestSilentRestoreLaunchingCandidateWithoutExecutionEvidenceRemainsBlocked(t *testing.T) {
	fixture := newSilentRestoreCheckpointFixture(t)
	starting, err := fixture.repo.GetExecutorRunningBySessionID(fixture.ctx, fixture.session.ID)
	require.NoError(t, err)
	require.NoError(t, fixture.repo.DeleteExecutorRunningIfCurrent(
		fixture.ctx, fixture.session.ID, starting.AgentExecutionID, starting.UpdatedAt,
	))
	launchChecks := 0
	fixture.manager.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launchChecks++
		return nil, nil
	}

	require.NoError(t, fixture.service.ensureSilentRestoreCandidate(
		fixture.ctx, fixture.store, fixture.attemptID, fixture.checkpoint, fixture.inputs,
	))
	stored, err := fixture.store.GetRestoreAttempt(fixture.ctx, fixture.attemptID)
	require.NoError(t, err)
	require.Equal(t, models.SilentRestoreStageCandidateLaunching, stored.Checkpoint.Stage)
	require.Equal(t, fixture.checkpoint.CandidateExecutionID, stored.Checkpoint.CandidateExecutionID)
	require.Zero(t, launchChecks, "an absent SQL row does not prove a launched process is dead")
}

func TestSilentRestartWorkerSkipsIneligibleCandidates(t *testing.T) {
	tests := []struct {
		name     string
		nativeID string
		mutate   func(*testing.T, *Service, *sqliterepo.Repository, *models.TaskSession)
	}{
		{
			name: "completed session", nativeID: "provider-session",
			mutate: func(t *testing.T, _ *Service, repo *sqliterepo.Repository, session *models.TaskSession) {
				session.State = models.TaskSessionStateCompleted
				require.NoError(t, repo.UpdateTaskSession(context.Background(), session))
			},
		},
		{
			name: "archived task", nativeID: "provider-session",
			mutate: func(t *testing.T, _ *Service, repo *sqliterepo.Repository, _ *models.TaskSession) {
				task, err := repo.GetTask(context.Background(), "t1")
				require.NoError(t, err)
				now := time.Now().UTC()
				task.ArchivedAt = &now
				require.NoError(t, repo.UpdateTask(context.Background(), task))
			},
		},
		{
			name: "automation task", nativeID: "provider-session",
			mutate: func(t *testing.T, _ *Service, repo *sqliterepo.Repository, _ *models.TaskSession) {
				task, err := repo.GetTask(context.Background(), "t1")
				require.NoError(t, err)
				task.Origin = models.TaskOriginAutomationRun
				require.NoError(t, repo.UpdateTask(context.Background(), task))
			},
		},
		{
			name: "Office task", nativeID: "provider-session",
			mutate: func(t *testing.T, _ *Service, repo *sqliterepo.Repository, _ *models.TaskSession) {
				task, err := repo.GetTask(context.Background(), "t1")
				require.NoError(t, err)
				task.ProjectID = "office-project"
				require.NoError(t, repo.UpdateTask(context.Background(), task))
			},
		},
		{
			name: "dynamic route requires user action", nativeID: "provider-session",
			mutate: func(t *testing.T, _ *Service, repo *sqliterepo.Repository, session *models.TaskSession) {
				session.RouteState = dynamicRouteStatusActionRequired
				require.NoError(t, repo.UpdateTaskSession(context.Background(), session))
			},
		},
		{name: "missing native conversation", nativeID: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, repo, manager, session, store := newSilentRestoreEligibilityFixture(t, test.nativeID, "eligibility")
			if test.mutate != nil {
				test.mutate(t, service, repo, session)
			}
			launches := 0
			manager.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launches++
				return nil, nil
			}

			service.runSilentRestoreRecoveryPass(context.Background(), store)

			require.Zero(t, launches, "startup must leave ineligible sessions untouched")
		})
	}
}

func TestSilentRestoreCapacityDenialLeavesAllocatedCheckpoint(t *testing.T) {
	fixture := newSilentRestoreCheckpointFixtureWithLaunching(t, false)
	fixture.service.sessionCeiling = newSessionCeilingController(1, nil, nil)
	decision := fixture.service.sessionCeiling.admit(fixture.ctx, admissionRequest{
		taskID: "other-task", sessionID: "other-session", origin: launchOriginAutomatic, seam: "test",
	})
	require.True(t, decision.admitted, "the control launch must occupy the only capacity slot")
	fixture.manager.rowLivenessFn = func(*models.ExecutorRunning) models.ProcessLiveness {
		return models.ProcessLivenessDead
	}
	launches := 0
	fixture.manager.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launches++
		return nil, nil
	}
	running, err := fixture.repo.GetExecutorRunningBySessionID(fixture.ctx, fixture.session.ID)
	require.NoError(t, err)

	err = fixture.service.startSilentRestoreCandidate(
		fixture.ctx, fixture.store, fixture.attemptID, fixture.checkpoint, fixture.inputs, running,
	)
	require.NoError(t, err)
	require.Zero(t, launches, "silent recovery must respect normal automatic admission limits")
	stored, err := fixture.store.GetRestoreAttempt(fixture.ctx, fixture.attemptID)
	require.NoError(t, err)
	require.Equal(t, models.SilentRestoreStageCandidateAllocated, stored.Checkpoint.Stage,
		"capacity denial must leave the candidate checkpoint available for a later pass")
}
