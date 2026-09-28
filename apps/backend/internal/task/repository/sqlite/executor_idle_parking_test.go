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
