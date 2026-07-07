package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// TestReconcileSessionsOnStartupMakesRowsTrue is the restart-reconciliation
// integration test for #1597 startup reconciliation (the backlog stops
// growing, rows are made true, resumability is never lost). After a restart,
// reconciliation verifies each executors_running
// row against reality using runtime-aware liveness and:
//   - prunes a terminal row whose process is dead and that holds no resume_token
//     (the stale-row backlog trends toward zero);
//   - repairs a resumable row whose process is dead in place — status=stopped,
//     local_pid cleared, resume_token preserved — so it never keeps claiming a
//     live process, yet stays resumable;
//   - leaves a row whose process is still alive with its live handle intact;
//   - never applies the local liveness check to an SSH (remote) row.
func TestReconcileSessionsOnStartupMakesRowsTrue(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedTaskAndSession(t, repo, "taskA", "sA", models.TaskSessionStateCompleted)       // terminal, no token, dead → prune
	seedTaskAndSession(t, repo, "taskB", "sB", models.TaskSessionStateWaitingForInput) // resumable + token, dead → repair
	seedTaskAndSession(t, repo, "taskC", "sC", models.TaskSessionStateRunning)         // alive → preserve live handle
	seedTaskAndSession(t, repo, "taskD", "sD", models.TaskSessionStateWaitingForInput) // SSH remote → Unknown, untouched

	upsert := func(er *models.ExecutorRunning) {
		er.CreatedAt, er.UpdatedAt = now, now
		if err := repo.UpsertExecutorRunning(ctx, er); err != nil {
			t.Fatalf("upsert %s: %v", er.SessionID, err)
		}
	}
	upsert(&models.ExecutorRunning{ID: "sA", SessionID: "sA", TaskID: "taskA", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning, LocalPID: 111})
	upsert(&models.ExecutorRunning{ID: "sB", SessionID: "sB", TaskID: "taskB", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning, ResumeToken: "tokB", Resumable: true, LocalPID: 222})
	upsert(&models.ExecutorRunning{ID: "sC", SessionID: "sC", TaskID: "taskC", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning, LocalPID: 333})
	upsert(&models.ExecutorRunning{ID: "sD", SessionID: "sD", TaskID: "taskD", Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusRunning, ResumeToken: "tokD", PID: 444})

	agentMgr := &mockAgentManager{
		rowLivenessFn: func(r *models.ExecutorRunning) models.ProcessLiveness {
			switch r.SessionID {
			case "sA", "sB":
				return models.ProcessLivenessDead
			case "sC":
				return models.ProcessLivenessAlive
			default: // sD (SSH) — a local check must never judge a remote row
				return models.ProcessLivenessUnknown
			}
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.reconcileSessionsOnStartup(ctx)

	// sA: terminal + no resume_token + dead → pruned.
	if _, err := repo.GetExecutorRunningBySessionID(ctx, "sA"); err == nil {
		t.Error("sA: terminal dead row with no resume_token should be pruned")
	}

	// sB: resumable + token + dead → preserved AND repaired.
	b, err := repo.GetExecutorRunningBySessionID(ctx, "sB")
	if err != nil {
		t.Fatalf("sB should be preserved: %v", err)
	}
	if b.ResumeToken != "tokB" {
		t.Errorf("sB resume_token lost during repair: %q", b.ResumeToken)
	}
	if b.Status != models.ExecutorRunningStatusStopped || b.LocalPID != 0 {
		t.Errorf("sB should be repaired to stopped with cleared local_pid; got status=%q local_pid=%d", b.Status, b.LocalPID)
	}

	// sC: alive → live local handle preserved (not repaired away).
	c, err := repo.GetExecutorRunningBySessionID(ctx, "sC")
	if err != nil {
		t.Fatalf("sC should be preserved: %v", err)
	}
	if c.LocalPID != 333 {
		t.Errorf("sC live local handle must be preserved; got local_pid=%d", c.LocalPID)
	}

	// sD: SSH → local liveness Unknown; the local reconcile must not touch it.
	d, err := repo.GetExecutorRunningBySessionID(ctx, "sD")
	if err != nil {
		t.Fatalf("sD should be preserved: %v", err)
	}
	if d.PID != 444 || d.ResumeToken != "tokD" {
		t.Errorf("sD SSH row must be untouched by the local reconcile; got pid=%d token=%q", d.PID, d.ResumeToken)
	}
}

// TestReconcileSessionsOnStartup_IdleSessionDeadRowRepaired covers the office
// IDLE path: an office turn writes IDLE and tears down, so a crash/restart in
// that window leaves a row claiming status=running with a dead local_pid.
// Startup reconciliation must repair the row (stopped, local_pid cleared,
// resume_token preserved) WITHOUT flipping the session out of IDLE — the IDLE
// state is the office "between turns" shape and must be preserved (#1597).
func TestReconcileSessionsOnStartup_IdleSessionDeadRowRepaired(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedTaskAndSession(t, repo, "taskI", "sI", models.TaskSessionStateIdle)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "sI", SessionID: "sI", TaskID: "taskI", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, ResumeToken: "tokI", Resumable: true,
		LocalPID: 4343, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert sI: %v", err)
	}

	agentMgr := &mockAgentManager{
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessDead
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	svc.reconcileSessionsOnStartup(ctx)

	// Session state stays IDLE (not flipped to WAITING_FOR_INPUT).
	session, err := repo.GetTaskSession(ctx, "sI")
	if err != nil {
		t.Fatalf("GetTaskSession(sI): %v", err)
	}
	if session.State != models.TaskSessionStateIdle {
		t.Errorf("IDLE session state must be preserved; got %q", session.State)
	}
	// But the row is repaired so it no longer claims a live process.
	row, err := repo.GetExecutorRunningBySessionID(ctx, "sI")
	if err != nil {
		t.Fatalf("idle resumable row must be preserved: %v", err)
	}
	if row.Status != models.ExecutorRunningStatusStopped || row.LocalPID != 0 {
		t.Errorf("dead idle row must be repaired to stopped with cleared local_pid; got status=%q local_pid=%d", row.Status, row.LocalPID)
	}
	if row.ResumeToken != "tokI" {
		t.Errorf("resume_token must survive the idle repair; got %q", row.ResumeToken)
	}
}

// TestReconcileSessionsOnStartup_MissingSessionStopsAgentAndDeletesRow covers
// the orphan-row branch of startup reconciliation: a row whose task_session is
// gone entirely (deleted task/worktree) routes to handleMissingSessionOnStartup,
// which stops the still-registered runtime handle (forced) and, once that
// succeeds, deletes the now-meaningless executors_running row — so orphan rows
// don't survive restarts and inflate the backlog (#1597).
func TestReconcileSessionsOnStartup_MissingSessionStopsAgentAndDeletesRow(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// No backing task_session row for "sO" — GetTaskSession returns not-found.
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "sO",
		SessionID:        "sO",
		TaskID:           "taskO",
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusRunning,
		AgentExecutionID: "execO",
		LocalPID:         4242,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("upsert sO: %v", err)
	}

	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	svc.reconcileSessionsOnStartup(ctx)

	if len(agentMgr.stopAgentWithReasonArgs) != 1 || agentMgr.stopAgentWithReasonArgs[0].ExecutionID != "execO" {
		t.Fatalf("orphan row must stop its runtime handle via StopAgentWithReason; got %+v", agentMgr.stopAgentWithReasonArgs)
	}
	if !agentMgr.stopAgentWithReasonArgs[0].Force {
		t.Errorf("missing-session stop must be forced")
	}
	if _, err := repo.GetExecutorRunningBySessionID(ctx, "sO"); err == nil {
		t.Error("orphan row must be deleted once the runtime stop succeeds")
	}
}

// TestReconcileSessionsOnStartup_CreatedSessionRowPrunedUnlessResumable covers
// the never-started-session cleanup site, which routes through the resume-safety
// invariant rather than deleting unconditionally: a Created session's row with
// no resume_token is pruned (nothing to lose), while the rare Created row that
// already carries a resume_token is repaired in place — the token is the only
// handle to the agent-side conversation and must survive
// (#1597 resume-safety invariant).
func TestReconcileSessionsOnStartup_CreatedSessionRowPrunedUnlessResumable(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedTaskAndSession(t, repo, "taskE", "sE", models.TaskSessionStateCreated) // no token → prune
	seedTaskAndSession(t, repo, "taskF", "sF", models.TaskSessionStateCreated) // token → repair

	upsert := func(er *models.ExecutorRunning) {
		er.CreatedAt, er.UpdatedAt = now, now
		if err := repo.UpsertExecutorRunning(ctx, er); err != nil {
			t.Fatalf("upsert %s: %v", er.SessionID, err)
		}
	}
	upsert(&models.ExecutorRunning{ID: "sE", SessionID: "sE", TaskID: "taskE", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStarting, LocalPID: 555})
	upsert(&models.ExecutorRunning{ID: "sF", SessionID: "sF", TaskID: "taskF", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStarting, ResumeToken: "tokF", Resumable: true, LocalPID: 666})

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.reconcileSessionsOnStartup(ctx)

	if _, err := repo.GetExecutorRunningBySessionID(ctx, "sE"); err == nil {
		t.Error("sE: never-started row with no resume_token should be pruned")
	}

	f, err := repo.GetExecutorRunningBySessionID(ctx, "sF")
	if err != nil {
		t.Fatalf("sF: Created row holding a resume_token must be preserved: %v", err)
	}
	if f.ResumeToken != "tokF" {
		t.Errorf("sF resume_token lost: %q", f.ResumeToken)
	}
	if f.Status != models.ExecutorRunningStatusStopped || f.LocalPID != 0 {
		t.Errorf("sF should be repaired to stopped with cleared local_pid; got status=%q local_pid=%d", f.Status, f.LocalPID)
	}
}
