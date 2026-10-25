package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestExecutorRunningUpsertDoesNotRegressIdleSuspensionForSameExecution(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	seedExecutorRunningCleanupTask(t, repo, "task-idle-upsert")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-idle-upsert", TaskID: "task-idle-upsert", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-idle-upsert", SessionID: "session-idle-upsert", TaskID: "task-idle-upsert",
		AgentExecutionID: "execution-idle-upsert", Status: models.ExecutorRunningStatusReady,
		Resumable: true, ResumeToken: "resume-token",
	}); err != nil {
		t.Fatalf("seed running row: %v", err)
	}
	stale, err := repo.GetExecutorRunningBySessionID(ctx, "session-idle-upsert")
	if err != nil {
		t.Fatalf("read stale running row: %v", err)
	}
	for _, transition := range [][2]string{
		{models.ExecutorIdleSuspensionNone, models.ExecutorIdleSuspensionInProgress},
		{models.ExecutorIdleSuspensionInProgress, models.ExecutorIdleSuspensionAgentStopped},
	} {
		if err := repo.CompareAndSetExecutorRunningIdleSuspension(
			ctx, "session-idle-upsert", "execution-idle-upsert", time.Time{},
			transition[0], transition[1],
		); err != nil {
			t.Fatalf("advance idle suspension %q -> %q: %v", transition[0], transition[1], err)
		}
	}
	if err := repo.UpsertExecutorRunning(ctx, stale); err != nil {
		t.Fatalf("write stale lifecycle snapshot: %v", err)
	}
	got, err := repo.GetExecutorRunningBySessionID(ctx, "session-idle-upsert")
	if err != nil {
		t.Fatalf("read after stale lifecycle snapshot: %v", err)
	}
	if got.IdleSuspensionState != models.ExecutorIdleSuspensionAgentStopped {
		t.Fatalf("stale lifecycle snapshot regressed idle suspension to %q", got.IdleSuspensionState)
	}
	if err := repo.CompareAndSetExecutorRunningIdleSuspension(
		ctx, "session-idle-upsert", "execution-idle-upsert", time.Time{},
		models.ExecutorIdleSuspensionAgentStopped, models.ExecutorIdleSuspensionSuspended,
	); err != nil {
		t.Fatalf("persist suspension: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, stale); err != nil {
		t.Fatalf("write late lifecycle snapshot: %v", err)
	}
	got, err = repo.GetExecutorRunningBySessionID(ctx, "session-idle-upsert")
	if err != nil {
		t.Fatalf("read after late lifecycle snapshot: %v", err)
	}
	if got.IdleSuspensionState != models.ExecutorIdleSuspensionSuspended || got.Status != models.ExecutorRunningStatusStopped {
		t.Fatalf("late lifecycle snapshot regressed suspended row: state=%q status=%q", got.IdleSuspensionState, got.Status)
	}

	stale.AgentExecutionID = "execution-idle-upsert-successor"
	stale.IdleSuspensionState = models.ExecutorIdleSuspensionNone
	if err := repo.UpsertExecutorRunning(ctx, stale); err != nil {
		t.Fatalf("write successor lifecycle snapshot: %v", err)
	}
	got, err = repo.GetExecutorRunningBySessionID(ctx, "session-idle-upsert")
	if err != nil {
		t.Fatalf("read successor lifecycle row: %v", err)
	}
	if got.AgentExecutionID != stale.AgentExecutionID || got.IdleSuspensionState != models.ExecutorIdleSuspensionSuspended {
		t.Fatalf("successor upsert lost suspension provenance before readiness: execution=%q state=%q", got.AgentExecutionID, got.IdleSuspensionState)
	}
	if err := repo.CompareAndSetExecutorRunningIdleSuspension(
		ctx, "session-idle-upsert", stale.AgentExecutionID, time.Time{},
		models.ExecutorIdleSuspensionSuspended, models.ExecutorIdleSuspensionNone,
	); err != nil {
		t.Fatalf("clear suspension after successor readiness: %v", err)
	}
	got, err = repo.GetExecutorRunningBySessionID(ctx, "session-idle-upsert")
	if err != nil {
		t.Fatalf("read after successor readiness: %v", err)
	}
	if got.IdleSuspensionState != models.ExecutorIdleSuspensionNone {
		t.Fatalf("ready successor retained suspension provenance: %q", got.IdleSuspensionState)
	}
}

func TestClaimExecutorRunningIdleSuspensionPersistsPolicyRevision(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	workspace := &models.Workspace{
		ID: "workspace-idle-policy-revision", Name: "Idle policy revision",
		ACPIdleSuspensionEnabled: true, ACPIdleTimeoutMinutes: 120,
	}
	if err := repo.CreateWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	task := &models.Task{ID: "task-idle-policy-revision", WorkspaceID: workspace.ID, Title: "Idle policy revision"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-idle-policy-revision", TaskID: task.ID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-idle-policy-revision", SessionID: "session-idle-policy-revision", TaskID: task.ID,
		AgentExecutionID: "execution-idle-policy-revision", Status: models.ExecutorRunningStatusReady,
		Resumable: true, ResumeToken: "resume-token",
	}); err != nil {
		t.Fatal(err)
	}
	workspace, err := repo.GetWorkspace(ctx, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	running, err := repo.GetExecutorRunningBySessionID(ctx, "session-idle-policy-revision")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimExecutorRunningIdleSuspension(ctx, running.SessionID, running.AgentExecutionID,
		running.UpdatedAt, workspace.ID, workspace.UpdatedAt)
	if err != nil || !claimed {
		t.Fatalf("claim = %v, error = %v", claimed, err)
	}
	running, err = repo.GetExecutorRunningBySessionID(ctx, running.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if running.IdleSuspensionPolicyUpdatedAt.IsZero() || !running.IdleSuspensionPolicyUpdatedAt.Equal(workspace.UpdatedAt) {
		t.Fatalf("persisted policy revision = %v, want %v", running.IdleSuspensionPolicyUpdatedAt, workspace.UpdatedAt)
	}
	valid, err := repo.ValidateExecutorRunningIdleSuspensionPolicy(ctx, running.SessionID, running.AgentExecutionID,
		workspace.ID, workspace.UpdatedAt)
	if err != nil || !valid {
		t.Fatalf("current claim policy = %v, error = %v", valid, err)
	}
	workspace.ACPIdleTimeoutMinutes = 121
	if err := repo.UpdateWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	valid, err = repo.ValidateExecutorRunningIdleSuspensionPolicy(ctx, running.SessionID, running.AgentExecutionID,
		workspace.ID, workspace.UpdatedAt)
	if err != nil || valid {
		t.Fatalf("changed claim policy = %v, error = %v; want invalid", valid, err)
	}
	for _, transition := range [][2]string{
		{models.ExecutorIdleSuspensionInProgress, models.ExecutorIdleSuspensionAgentStopped},
		{models.ExecutorIdleSuspensionAgentStopped, models.ExecutorIdleSuspensionNone},
	} {
		if err := repo.CompareAndSetExecutorRunningIdleSuspension(ctx, running.SessionID, running.AgentExecutionID,
			time.Time{}, transition[0], transition[1]); err != nil {
			t.Fatal(err)
		}
		running, err = repo.GetExecutorRunningBySessionID(ctx, running.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		if transition[1] == models.ExecutorIdleSuspensionNone && !running.IdleSuspensionPolicyUpdatedAt.IsZero() {
			t.Fatalf("cleared claim retained policy revision %v", running.IdleSuspensionPolicyUpdatedAt)
		}
	}
}
