package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// newSeam4TestService builds a cancelled, resumable session (no executor
// record required — AC-4's seam 4 resumability check waives it for
// CANCELLED/FAILED states) backed by a real sqlite repo and a mock agent
// manager that transitions the session to WAITING_FOR_INPUT on launch, the
// same fixture shape as TestCompletedSessionResumePreservesConversation.
func newSeam4TestService(t *testing.T, taskID, sessionID string) *Service {
	t.Helper()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCancelled)

	ctx := context.Background()
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "profile-1"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, Title: "T", State: v1.TaskStateInProgress}

	started := false
	agentMgr := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{},
		repo:             repo,
		sessionID:        sessionID,
		taskID:           taskID,
		onStartCalled:    &started,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	return svc
}

// TestResumeTaskSessionWithOptions_SecondAutomaticLaunchOverCeilingIsDeferred
// pins AC-4/AC-11 at the real seam: ResumeTaskSessionWithOptions itself, gated
// after the resumability/Office checks and before the executor call, must
// refuse the second automatic resume once the ceiling's single slot is held.
func TestResumeTaskSessionWithOptions_SecondAutomaticLaunchOverCeilingIsDeferred(t *testing.T) {
	ctx := context.Background()
	svcA := newSeam4TestService(t, "seam4-it-task-a", "seam4-it-session-a")
	svcA.sessionCeiling = newSessionCeilingController(1, nil, nil)

	exec1, err := svcA.ResumeTaskSessionWithOptions(ctx, "seam4-it-task-a", "seam4-it-session-a", executor.ResumeOptions{
		Origin: "automatic",
	})
	require.NoError(t, err)
	require.NotNil(t, exec1)

	// A second, independent service sharing the same controller instance
	// simulates a second resume request arriving while the first's reservation
	// is still held — ResumeTaskSessionWithOptions never releases the
	// reservation for a session it successfully resumed within this process
	// lifetime (it is consumed, matching the population's own semantics).
	svcB := newSeam4TestService(t, "seam4-it-task-b", "seam4-it-session-b")
	svcB.sessionCeiling = svcA.sessionCeiling

	exec2, err := svcB.ResumeTaskSessionWithOptions(ctx, "seam4-it-task-b", "seam4-it-session-b", executor.ResumeOptions{
		Origin: "automatic",
	})
	require.NoError(t, err)
	require.Nil(t, exec2, "a refused automatic resume must not produce a launched execution")

	record := deferredLaunchOf(t, svcB, "seam4-it-task-b")
	require.NotNil(t, record, "a ceiling refusal must persist a deferred_launch record")
	require.Equal(t, true, record[models.CeilingDeferredKey])
	require.Equal(t, string(models.CeilingLaunchResume), record[models.CeilingLaunchKindKey])
}

// TestResumeTaskSessionWithOptions_ManualLaunchOverCeilingIsAdmitted pins
// AC-14 through the real ResumeTaskSessionWithOptions entry point.
func TestResumeTaskSessionWithOptions_ManualLaunchOverCeilingIsAdmitted(t *testing.T) {
	ctx := context.Background()
	svcA := newSeam4TestService(t, "seam4-it-manual-a", "seam4-it-manual-session-a")
	svcA.sessionCeiling = newSessionCeilingController(1, nil, nil)

	_, err := svcA.ResumeTaskSessionWithOptions(ctx, "seam4-it-manual-a", "seam4-it-manual-session-a", executor.ResumeOptions{
		Origin: "automatic",
	})
	require.NoError(t, err)

	svcB := newSeam4TestService(t, "seam4-it-manual-b", "seam4-it-manual-session-b")
	svcB.sessionCeiling = svcA.sessionCeiling

	exec2, err := svcB.ResumeTaskSessionWithOptions(ctx, "seam4-it-manual-b", "seam4-it-manual-session-b", executor.ResumeOptions{
		Origin: "manual",
	})
	require.NoError(t, err)
	require.NotNil(t, exec2, "a manual resume must be admitted even over the ceiling")

	record := deferredLaunchOf(t, svcB, "seam4-it-manual-b")
	require.Nil(t, record, "a manual override must never write a ceiling_deferred record")
}

// TestResumeTaskSessionWithOptions_ManualLaunchOverCeilingIsAuditedEvenWhenTheLaunchFails
// pins AC-14/AC-53's unconditional audit contract: the record must be written
// once the reservation is admitted onto the session being resumed, not only
// once the executor's subsequent LaunchAgent call has also succeeded. A launch
// failure between admission and agent start must not erase the only evidence
// a ceiling override happened.
func TestResumeTaskSessionWithOptions_ManualLaunchOverCeilingIsAuditedEvenWhenTheLaunchFails(t *testing.T) {
	ctx := context.Background()
	svcA := newSeam4TestService(t, "seam4-it-fail-a", "seam4-it-fail-session-a")
	svcA.sessionCeiling = newSessionCeilingController(1, nil, nil)

	_, err := svcA.ResumeTaskSessionWithOptions(ctx, "seam4-it-fail-a", "seam4-it-fail-session-a", executor.ResumeOptions{
		Origin: "automatic",
	})
	require.NoError(t, err)

	repoB := setupTestRepo(t)
	seedTaskAndSession(t, repoB, "seam4-it-fail-b", "seam4-it-fail-session-b", models.TaskSessionStateCancelled)
	sessionB, err := repoB.GetTaskSession(ctx, "seam4-it-fail-session-b")
	require.NoError(t, err)
	sessionB.AgentProfileID = "profile-1"
	require.NoError(t, repoB.UpdateTaskSession(ctx, sessionB))
	taskRepoB := newMockTaskRepo()
	taskRepoB.tasks["seam4-it-fail-b"] = &v1.Task{ID: "seam4-it-fail-b", Title: "T", State: v1.TaskStateInProgress}
	launchErr := errors.New("resume launch failed")
	agentMgrB := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return nil, launchErr
		},
	}
	svcB := createTestServiceWithAgent(repoB, newMockStepGetter(), taskRepoB, agentMgrB)
	svcB.executor = executor.NewExecutor(agentMgrB, repoB, testLogger(), executor.ExecutorConfig{})
	svcB.sessionCeiling = svcA.sessionCeiling

	_, err = svcB.ResumeTaskSessionWithOptions(ctx, "seam4-it-fail-b", "seam4-it-fail-session-b", executor.ResumeOptions{
		Origin: "manual",
	})
	require.Error(t, err, "the forced resume failure must still surface")

	session, err := repoB.GetTaskSession(ctx, "seam4-it-fail-session-b")
	require.NoError(t, err)
	require.NotNil(t, session.Metadata[ceilingManualOverrideMetadataKey],
		"a manual override must be audited even when the resume subsequently fails")
}

// TestResumeTaskSessionWithOptions_UnsetOriginDefaultsAutomatic pins AC-13b
// through the real entry point: launchResume is the only caller that sets
// Origin explicitly today, so every other caller's zero-value Origin must
// still be gated (and refused/deferred, not silently treated as manual) once
// the ceiling is reached.
func TestResumeTaskSessionWithOptions_UnsetOriginDefaultsAutomatic(t *testing.T) {
	ctx := context.Background()
	svcA := newSeam4TestService(t, "seam4-it-unset-a", "seam4-it-unset-session-a")
	svcA.sessionCeiling = newSessionCeilingController(1, nil, nil)

	_, err := svcA.ResumeTaskSessionWithOptions(ctx, "seam4-it-unset-a", "seam4-it-unset-session-a", executor.ResumeOptions{
		Origin: "automatic",
	})
	require.NoError(t, err)

	svcB := newSeam4TestService(t, "seam4-it-unset-b", "seam4-it-unset-session-b")
	svcB.sessionCeiling = svcA.sessionCeiling

	// No Origin set at all.
	exec2, err := svcB.ResumeTaskSessionWithOptions(ctx, "seam4-it-unset-b", "seam4-it-unset-session-b", executor.ResumeOptions{})
	require.NoError(t, err)
	require.Nil(t, exec2, "an unset origin must default to automatic and be refused over the ceiling, not silently admitted")
}
