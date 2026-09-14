package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// seam5EventData builds the watcher.AgentEventData shape
// relaunchDynamicTaskAfterFailure's real callers construct.
func seam5EventData(taskID, sessionID, executionID string) watcher.AgentEventData {
	return watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
		AgentProfileID:   "profile-1",
	}
}

// newSeam5TestService builds a RUNNING, non-Office session with a live
// executors_running row and a mock agent manager that transitions the
// session to WAITING_FOR_INPUT on launch, matching
// relaunchDynamicTaskAfterFailure's own non-Office branch
// (s.StartCreatedSession).
func newSeam5TestService(t *testing.T, taskID, sessionID, executionID string) (*Service, *sqliterepo.Repository) {
	t.Helper()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	ctx := context.Background()
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.AgentProfileID = "profile-1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)

	started := false
	agentMgr := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{},
		repo:             repo,
		sessionID:        sessionID,
		taskID:           taskID,
		onStartCalled:    &started,
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})
	return svc, repo
}

// TestRelaunchDynamicTaskAfterFailure_SecondAutomaticOverCeilingIsDeferred
// pins AC-38a/AC-42f at the real seam: relaunchDynamicTaskAfterFailure
// itself, gated at entry before the CREATED transition, must refuse the
// second automatic relaunch once the ceiling's single slot is held.
func TestRelaunchDynamicTaskAfterFailure_SecondAutomaticOverCeilingIsDeferred(t *testing.T) {
	ctx := context.Background()
	svcA, _ := newSeam5TestService(t, "seam5-it-task-a", "seam5-it-session-a", "seam5-it-exec-a")
	svcA.sessionCeiling = newSessionCeilingController(1, nil, nil)

	relaunched := svcA.relaunchDynamicTaskAfterFailure(ctx, seam5EventData("seam5-it-task-a", "seam5-it-session-a", "seam5-it-exec-a"), "profile-1", launchOriginAutomatic)
	if !relaunched {
		t.Fatal("first automatic relaunch was not admitted")
	}

	// A second, independent service sharing the same controller instance
	// simulates a second relaunch arriving while the first's reservation is
	// still held.
	svcB, repoB := newSeam5TestService(t, "seam5-it-task-b", "seam5-it-session-b", "seam5-it-exec-b")
	svcB.sessionCeiling = svcA.sessionCeiling

	relaunched = svcB.relaunchDynamicTaskAfterFailure(ctx, seam5EventData("seam5-it-task-b", "seam5-it-session-b", "seam5-it-exec-b"), "profile-1", launchOriginAutomatic)
	if relaunched {
		t.Fatal("a refused automatic relaunch must not report success")
	}

	session, err := repoB.GetTaskSession(ctx, "seam5-it-session-b")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State == models.TaskSessionStateCreated {
		t.Fatal("a refused relaunch must not have performed the CREATED transition")
	}

	record := deferredLaunchOf(t, svcB, "seam5-it-task-b")
	if record == nil {
		t.Fatal("a ceiling refusal must persist a deferred_launch record")
	}
	if record[models.CeilingDeferredKey] != true {
		t.Fatalf("ceiling_deferred = %v, want true", record[models.CeilingDeferredKey])
	}
	if record[models.CeilingLaunchKindKey] != string(models.CeilingLaunchDynamicRelaunch) {
		t.Fatalf("ceiling_launch_kind = %v, want %q", record[models.CeilingLaunchKindKey], models.CeilingLaunchDynamicRelaunch)
	}
}

// TestRelaunchDynamicTaskAfterFailure_ManualOverCeilingIsAdmitted pins AC-14
// through the real relaunchDynamicTaskAfterFailure entry point: the manual
// route-action origin is always admitted, even at the ceiling.
func TestRelaunchDynamicTaskAfterFailure_ManualOverCeilingIsAdmitted(t *testing.T) {
	ctx := context.Background()
	svcA, _ := newSeam5TestService(t, "seam5-it-manual-a", "seam5-it-manual-session-a", "seam5-it-manual-exec-a")
	svcA.sessionCeiling = newSessionCeilingController(1, nil, nil)

	if !svcA.relaunchDynamicTaskAfterFailure(ctx, seam5EventData("seam5-it-manual-a", "seam5-it-manual-session-a", "seam5-it-manual-exec-a"), "profile-1", launchOriginAutomatic) {
		t.Fatal("first automatic relaunch was not admitted")
	}

	svcB, _ := newSeam5TestService(t, "seam5-it-manual-b", "seam5-it-manual-session-b", "seam5-it-manual-exec-b")
	svcB.sessionCeiling = svcA.sessionCeiling

	if !svcB.relaunchDynamicTaskAfterFailure(ctx, seam5EventData("seam5-it-manual-b", "seam5-it-manual-session-b", "seam5-it-manual-exec-b"), "profile-1", launchOriginManual) {
		t.Fatal("a manual relaunch must be admitted even over the ceiling")
	}

	if record := deferredLaunchOf(t, svcB, "seam5-it-manual-b"); record != nil {
		t.Fatalf("a manual override must never write a ceiling_deferred record: %+v", record)
	}
}

// TestRelaunchDynamicTaskAfterFailure_ManualOverCeilingIsAuditedEvenWhenTheLaunchFails
// pins AC-14/AC-53's unconditional audit contract: the record must be written
// once the reservation is admitted onto the session being relaunched, not only
// once the subsequent StartCreatedSession call has also succeeded. A launch
// failure between admission and agent start must not erase the only evidence
// a ceiling override happened.
func TestRelaunchDynamicTaskAfterFailure_ManualOverCeilingIsAuditedEvenWhenTheLaunchFails(t *testing.T) {
	ctx := context.Background()
	svcA, _ := newSeam5TestService(t, "seam5-it-fail-a", "seam5-it-fail-session-a", "seam5-it-fail-exec-a")
	svcA.sessionCeiling = newSessionCeilingController(1, nil, nil)

	if !svcA.relaunchDynamicTaskAfterFailure(ctx, seam5EventData("seam5-it-fail-a", "seam5-it-fail-session-a", "seam5-it-fail-exec-a"), "profile-1", launchOriginAutomatic) {
		t.Fatal("first automatic relaunch was not admitted")
	}

	// Session B deliberately has no executors_running row, so the ensuing
	// StartCreatedSession call takes the full synchronous LaunchAgent path
	// (not the existing-workspace fast path) where the injected failure
	// actually propagates back to this function.
	repoB := setupTestRepo(t)
	seedTaskAndSession(t, repoB, "seam5-it-fail-b", "seam5-it-fail-session-b", models.TaskSessionStateRunning)
	sessionB, err := repoB.GetTaskSession(ctx, "seam5-it-fail-session-b")
	require.NoError(t, err)
	sessionB.AgentProfileID = "profile-1"
	require.NoError(t, repoB.UpdateTaskSession(ctx, sessionB))
	taskRepoB := newMockTaskRepo()
	seedMockTaskState(taskRepoB, "seam5-it-fail-b", v1.TaskStateInProgress)
	launchErr := errors.New("relaunch failed")
	agentMgrB := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return nil, launchErr
		},
	}
	svcB := createTestServiceWithScheduler(repoB, newMockStepGetter(), taskRepoB, agentMgrB)
	svcB.lastTurnPrompt.Store("seam5-it-fail-session-b", capturedPrompt{text: "retry the task"})
	svcB.sessionCeiling = svcA.sessionCeiling

	relaunched := svcB.relaunchDynamicTaskAfterFailure(ctx, seam5EventData("seam5-it-fail-b", "seam5-it-fail-session-b", "seam5-it-fail-exec-b"), "profile-1", launchOriginManual)
	require.False(t, relaunched, "the forced launch failure must surface as an unsuccessful relaunch")

	session, err := repoB.GetTaskSession(ctx, "seam5-it-fail-session-b")
	require.NoError(t, err)
	require.NotNil(t, session.Metadata[ceilingManualOverrideMetadataKey],
		"a manual override must be audited even when the relaunch subsequently fails")
}
